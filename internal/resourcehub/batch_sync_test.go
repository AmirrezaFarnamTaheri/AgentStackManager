package resourcehub

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestBatchSyncAppliesIndependentTargetsConcurrently(t *testing.T) {
	manager, resource := batchSyncFixture(t, false)
	var mu sync.Mutex
	active, peak := 0, 0
	manager.beforeSyncOperation = func(SyncOperation) error {
		mu.Lock()
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return nil
	}
	plan, err := manager.PlanBatchSync([]string{"codex-a", "codex-b"}, []string{resource.ID}, PlanOptions{TTL: time.Hour, AllowRisk: true}, 2)
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.ApplyBatchSync(context.Background(), plan.ID, plan.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Succeeded != 2 || report.Failed != 0 || peak < 2 {
		t.Fatalf("report=%#v peak=%d", report, peak)
	}
}

func TestBatchSyncSerializesTargetsWithSharedRoot(t *testing.T) {
	manager, resource := batchSyncFixture(t, true)
	var mu sync.Mutex
	active, peak := 0, 0
	manager.beforeSyncOperation = func(SyncOperation) error {
		mu.Lock()
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return nil
	}
	plan, err := manager.PlanBatchSync([]string{"codex-a", "codex-b"}, []string{resource.ID}, PlanOptions{TTL: time.Hour, AllowRisk: true}, 2)
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.ApplyBatchSync(context.Background(), plan.ID, plan.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Succeeded != 2 || peak != 1 {
		t.Fatalf("report=%#v peak=%d", report, peak)
	}
}

func batchSyncFixture(t *testing.T, sharedRoot bool) (Manager, Resource) {
	t.Helper()
	manager := New(t.TempDir())
	source := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("parallel"), 0o600); err != nil {
		t.Fatal(err)
	}
	resource, err := manager.Import(source, ImportOptions{ID: "parallel", Kind: KindSkill, Name: "Parallel"})
	if err != nil {
		t.Fatal(err)
	}
	rootA, rootB := t.TempDir(), t.TempDir()
	if sharedRoot {
		rootB = rootA
	}
	if err := manager.RegisterTarget(Target{ID: "codex-a", Agent: AgentCodex, Root: rootA, Mode: ModeCopy, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	agentB := AgentCodex
	if sharedRoot {
		agentB = AgentClaude
	}
	if err := manager.RegisterTarget(Target{ID: "codex-b", Agent: agentB, Root: rootB, Mode: ModeCopy, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return manager, resource
}
