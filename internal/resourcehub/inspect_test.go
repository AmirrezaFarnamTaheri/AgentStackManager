package resourcehub

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectClassifiesInstalledDriftDuplicatesAndCollisions(t *testing.T) {
	manager := New(t.TempDir())
	manager.Clock = func() time.Time { return time.Date(2026, 8, 6, 1, 0, 0, 0, time.UTC) }
	first := filepath.Join(t.TempDir(), "review")
	second := filepath.Join(t.TempDir(), "review-other")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(first, "SKILL.md"), []byte("same"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "SKILL.md"), []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]string{"namespace": "local", "version": "1.0.0", "container": "quality-stack"}
	a, err := manager.Import(first, ImportOptions{ID: "review-a", Kind: KindSkill, Name: "Review", Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Import(first, ImportOptions{ID: "review-b", Kind: KindSkill, Name: "Review", Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Import(second, ImportOptions{ID: "review-c", Kind: KindSkill, Name: "Review", Metadata: metadata}); err != nil {
		t.Fatal(err)
	}
	targetRoot := t.TempDir()
	if err := manager.RegisterTarget(Target{ID: "codex-main", Agent: AgentCodex, Root: targetRoot, Mode: ModeCopy, Enabled: true, Scope: "global"}); err != nil {
		t.Fatal(err)
	}
	plan, err := manager.PlanSync("codex-main", []string{a.ID}, PlanOptions{TTL: time.Hour, AllowRisk: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ApplySync(plan.ID, plan.Digest, true); err != nil {
		t.Fatal(err)
	}
	destination, err := targetDestination(Target{ID: "codex-main", Agent: AgentCodex, Root: targetRoot, Mode: ModeCopy, Enabled: true}, a)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "SKILL.md"), []byte("drifted"), 0o600); err != nil {
		t.Fatal(err)
	}

	inspection, err := manager.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Counts.Managed != 3 || inspection.Counts.Contained != 3 || inspection.Counts.Installed != 1 || inspection.Counts.Drifted != 1 {
		t.Fatalf("counts=%#v", inspection.Counts)
	}
	classes := map[DuplicateClass]bool{}
	for _, group := range inspection.Duplicates {
		classes[group.Class] = true
	}
	if !classes[DuplicateExact] || !classes[DuplicateCollision] {
		t.Fatalf("duplicates=%#v", inspection.Duplicates)
	}
}
