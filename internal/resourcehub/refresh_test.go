package resourcehub

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewedResourceRefreshDetectsAndAppliesSourceChanges(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(source, []byte("version one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(root)
	if _, err := manager.Import(source, ImportOptions{ID: "review", Kind: KindSkill}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("version two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := manager.PlanRefresh(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Action != RefreshUpdate {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	report, err := manager.ApplyRefresh(plan.ID, plan.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Applied) != 1 || len(report.Backups) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if _, err := os.Stat(report.Backups[0]); err != nil {
		t.Fatalf("refresh recovery backup was not retained: %v", err)
	}
	items, err := manager.ListResources()
	if err != nil || len(items) != 1 || items[0].Digest != plan.Operations[0].SourceDigest {
		t.Fatalf("items=%#v err=%v", items, err)
	}
}

func TestResourceRefreshRejectsSourceChangedAfterReview(t *testing.T) {
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir())
	if _, err := manager.Import(source, ImportOptions{ID: "rule", Kind: KindRule}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := manager.PlanRefresh(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("three\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ApplyRefresh(plan.ID, plan.Digest, true); err == nil {
		t.Fatal("expected source drift rejection")
	}
}

func TestRefreshRollsBackAllCanonicalResourcesOnFailure(t *testing.T) {
	manager := New(t.TempDir())
	originalDigests := map[string]string{}
	for _, id := range []string{"a", "b"} {
		source := filepath.Join(t.TempDir(), id+".md")
		if err := os.WriteFile(source, []byte("original-"+id), 0o600); err != nil {
			t.Fatal(err)
		}
		resource, err := manager.Import(source, ImportOptions{ID: id, Kind: KindRule})
		if err != nil {
			t.Fatal(err)
		}
		originalDigests[id] = resource.Digest
		if err := os.WriteFile(source, []byte("updated-"+id), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := manager.PlanRefresh([]string{"a", "b"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	manager.beforeRefreshOperation = func(RefreshOperation) error {
		calls++
		if calls == 2 {
			return fmt.Errorf("simulated second refresh failure")
		}
		return nil
	}
	if _, err := manager.ApplyRefresh(plan.ID, plan.Digest, true); err == nil {
		t.Fatal("expected transactional refresh failure")
	}
	manager.beforeRefreshOperation = nil
	registry, err := manager.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for id, original := range originalDigests {
		resource := registry.Resources[id]
		if resource.Digest != original {
			t.Fatalf("registry advanced for %s after rollback: %s", id, resource.Digest)
		}
		canonical, err := treeDigest(manager.resourceSource(resource))
		if err != nil || canonical != original {
			t.Fatalf("canonical resource %s was not restored: digest=%s err=%v", id, canonical, err)
		}
	}
}
