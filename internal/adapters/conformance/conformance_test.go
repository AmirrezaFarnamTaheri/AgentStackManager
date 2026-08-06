package conformance

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/adapters/builtin"
)

func TestEmbeddedCorpusCoversEveryBuiltinAdapter(t *testing.T) {
	corpus, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCorpus(corpus); err != nil {
		t.Fatal(err)
	}
	registry := builtin.MustRegistry()
	if len(corpus.Targets) != len(registry.IDs()) {
		t.Fatalf("corpus targets=%d registry targets=%d", len(corpus.Targets), len(registry.IDs()))
	}
	for index, id := range registry.IDs() {
		if corpus.Targets[index].Target != id {
			t.Fatalf("corpus target %q does not match registry target %q", corpus.Targets[index].Target, id)
		}
	}
}

func TestEmbeddedCorpusPassesAllBuiltinAdapters(t *testing.T) {
	root := t.TempDir()
	report, err := RunEmbedded(context.Background(), builtin.MustRegistry(), RunOptions{Environment: builtin.RuntimeEnvironment(root, root, root, filepath.Join(root, "agy-mcp.json"))})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(report); err != nil {
		t.Fatal(err)
	}
	if !report.Passed() {
		for _, target := range report.Targets {
			for _, item := range target.Cases {
				if item.Status == StatusFailed {
					t.Logf("%s: %s", item.ID, item.Reason)
				}
			}
		}
		t.Fatalf("conformance report failed: %#v", report.Summary)
	}
	if report.Summary.Targets != 7 || report.Summary.Total != 64 || report.Summary.Passed != 64 || report.Summary.Failed != 0 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
}

func TestTargetFilterDeduplicatesAliases(t *testing.T) {
	root := t.TempDir()
	report, err := RunEmbedded(context.Background(), builtin.MustRegistry(), RunOptions{
		Environment: builtin.RuntimeEnvironment(root, root, root, filepath.Join(root, "agy-mcp.json")),
		Targets:     []string{"agy", "gemini", "antigravity"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Targets) != 1 || report.Targets[0].Target != builtin.TargetAgy || !report.Passed() {
		t.Fatalf("report=%#v", report)
	}
}

func TestCorpusRejectsTraversalAndDifferentialDrift(t *testing.T) {
	corpus, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	mutated := corpus
	mutated.Targets = append([]TargetFixture(nil), corpus.Targets...)
	for i := range mutated.Targets {
		if mutated.Targets[i].Target != builtin.TargetCodex {
			continue
		}
		mutated.Targets[i].Artifacts = append([]ArtifactFixture(nil), mutated.Targets[i].Artifacts...)
		mutated.Targets[i].Artifacts[0].RelativeDestination = "../{{name}}"
	}
	if _, err := SealCorpus(mutated); err == nil || !strings.Contains(err.Error(), "invalid projection") {
		t.Fatalf("traversal accepted: %v", err)
	}

	mutated = corpus
	mutated.Targets = append([]TargetFixture(nil), corpus.Targets...)
	for i := range mutated.Targets {
		if mutated.Targets[i].Target != builtin.TargetCodex {
			continue
		}
		mutated.Targets[i].Artifacts = append([]ArtifactFixture(nil), mutated.Targets[i].Artifacts...)
		for j := range mutated.Targets[i].Artifacts {
			if mutated.Targets[i].Artifacts[j].Kind == "Rule" {
				mutated.Targets[i].Artifacts[j].Directory = ".wrong/rules"
			}
		}
	}
	mutated, err = SealCorpus(mutated)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	report, err := Run(context.Background(), builtin.MustRegistry(), mutated, RunOptions{Environment: builtin.RuntimeEnvironment(root, root, root, filepath.Join(root, "agy-mcp.json")), Targets: []string{builtin.TargetCodex}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed() || report.Summary.Failed == 0 {
		t.Fatalf("differential drift passed: %#v", report)
	}
}

func TestReportTamperingIsRejected(t *testing.T) {
	root := t.TempDir()
	report, err := RunEmbedded(context.Background(), builtin.MustRegistry(), RunOptions{Environment: builtin.RuntimeEnvironment(root, root, root, filepath.Join(root, "agy-mcp.json")), Targets: []string{builtin.TargetCodex}})
	if err != nil {
		t.Fatal(err)
	}
	report.Targets[0].Cases[0].EvidenceDigest = "sha256:" + strings.Repeat("0", 64)
	if err := VerifyReport(report); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("tampered report accepted: %v", err)
	}
}
