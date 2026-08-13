package asmv1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/cas"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestStageResourceHubCreatesVerifiedReversibleShadow(t *testing.T) {
	hub := resourcehub.New(filepath.Join(t.TempDir(), "hub"))
	now := time.Date(2026, 8, 4, 17, 30, 0, 0, time.UTC)
	hub.Clock = func() time.Time { return now }
	first := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(first, []byte("review safely\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Import(first, resourcehub.ImportOptions{ID: "review", Kind: resourcehub.KindSkill, Tags: []string{"safety"}}); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(second, []byte("preserve user state\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Import(second, resourcehub.ImportOptions{ID: "preserve", Kind: resourcehub.KindRule}); err != nil {
		t.Fatal(err)
	}

	store := cas.New(filepath.Join(t.TempDir(), "cas"))
	receipt, err := Stage(hub, store, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCurrent(hub, store, receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Resources) != 2 || len(receipt.StagedGraph.Artifacts) != 2 {
		t.Fatalf("receipt=%#v", receipt)
	}
	for _, artifact := range receipt.StagedGraph.Artifacts {
		if !strings.HasPrefix(artifact.Content.Ref, "cas://tree/sha256/") {
			t.Fatalf("artifact content ref=%q", artifact.Content.Ref)
		}
		if _, ok := artifact.Extensions[CASExtensionKey]; !ok {
			t.Fatalf("missing CAS extension: %#v", artifact.Extensions)
		}
	}

	destination := filepath.Join(t.TempDir(), "restored")
	if err := RestoreResource(store, receipt, "review", destination); err != nil {
		t.Fatal(err)
	}
	digest, err := resourcehub.DigestPath(destination)
	if err != nil {
		t.Fatal(err)
	}
	var reviewRecord *ResourceRecord
	for index := range receipt.Resources {
		if receipt.Resources[index].ResourceID == "review" {
			reviewRecord = &receipt.Resources[index]
			break
		}
	}
	if reviewRecord == nil || digest != reviewRecord.LegacyDigest {
		t.Fatalf("restored digest=%s record=%#v", digest, reviewRecord)
	}

	repeated, err := Stage(hub, store, func() time.Time { return now.Add(2 * time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	if repeated.StagedGraph.Digest != receipt.StagedGraph.Digest {
		t.Fatalf("staged graph changed: %s %s", receipt.StagedGraph.Digest, repeated.StagedGraph.Digest)
	}
	for index := range receipt.Resources {
		if repeated.Resources[index].Object != receipt.Resources[index].Object {
			t.Fatalf("CAS object changed: %#v %#v", receipt.Resources[index], repeated.Resources[index])
		}
	}
}

func TestVerifyCurrentRejectsStaleResourceHub(t *testing.T) {
	hub := resourcehub.New(filepath.Join(t.TempDir(), "hub"))
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Import(source, resourcehub.ImportOptions{ID: "rule", Kind: resourcehub.KindRule}); err != nil {
		t.Fatal(err)
	}
	store := cas.New(filepath.Join(t.TempDir(), "cas"))
	receipt, err := Stage(hub, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Import(source, resourcehub.ImportOptions{ID: "rule", Kind: resourcehub.KindRule, Replace: true}); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCurrent(hub, store, receipt); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale receipt, got %v", err)
	}
}

func TestVerifyReceiptRejectsTampering(t *testing.T) {
	hub := resourcehub.New(filepath.Join(t.TempDir(), "hub"))
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Import(source, resourcehub.ImportOptions{ID: "rule", Kind: resourcehub.KindRule}); err != nil {
		t.Fatal(err)
	}
	store := cas.New(filepath.Join(t.TempDir(), "cas"))
	receipt, err := Stage(hub, store, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Resources[0].LegacyDigest = "sha256:" + strings.Repeat("0", 64)
	if err := VerifyReceipt(receipt); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected tamper rejection, got %v", err)
	}
}

func TestStageRejectsResourceHubChangeDuringStaging(t *testing.T) {
	now := time.Date(2026, 8, 4, 18, 0, 0, 0, time.UTC)
	hub := resourcehub.New(filepath.Join(t.TempDir(), "hub"))
	hub.Clock = func() time.Time { return now }
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("same content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Import(source, resourcehub.ImportOptions{ID: "rule", Kind: resourcehub.KindRule}); err != nil {
		t.Fatal(err)
	}
	store := cas.New(filepath.Join(t.TempDir(), "cas"))
	_, err := stage(hub, store, func() time.Time { return now }, func() error {
		now = now.Add(time.Minute)
		_, replaceErr := hub.Import(source, resourcehub.ImportOptions{ID: "rule", Kind: resourcehub.KindRule, Replace: true})
		return replaceErr
	})
	if err == nil || !strings.Contains(err.Error(), "changed during CAS staging") {
		t.Fatalf("expected in-flight source change rejection, got %v", err)
	}
}
