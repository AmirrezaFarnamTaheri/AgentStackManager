package contextengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestScanScoreAndRefreshPreservesUserContent(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/demo\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "cmd", "demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "cmd", "demo", "main.go"), []byte("package main\nfunc main(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("# Human rules\nKeep this.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	manager := New(t.TempDir())
	snapshot, err := manager.Scan(project)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Languages["go"] != 1 || snapshot.Commands["test"] != "go test ./..." {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}

	before, err := manager.Score(project, []resourcehub.Agent{resourcehub.AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	if before.Score >= 100 {
		t.Fatalf("expected incomplete config score, got %#v", before)
	}

	plan, err := manager.PlanRefresh(project, []resourcehub.Agent{resourcehub.AgentCodex, resourcehub.AgentClaude}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 2 {
		t.Fatalf("unexpected operations: %#v", plan.Operations)
	}
	if _, err := manager.ApplyRefresh(plan.ID, plan.Digest, true); err != nil {
		t.Fatal(err)
	}

	agents, err := os.ReadFile(filepath.Join(project, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(agents)
	if !strings.Contains(text, "# Human rules") || !strings.Contains(text, managedStart) || !strings.Contains(text, "go test ./...") {
		t.Fatalf("unexpected AGENTS.md:\n%s", text)
	}
	claude, err := os.ReadFile(filepath.Join(project, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claude), managedStart) {
		t.Fatalf("missing managed block: %s", claude)
	}

	after, err := manager.Score(project, []resourcehub.Agent{resourcehub.AgentCodex, resourcehub.AgentClaude})
	if err != nil {
		t.Fatal(err)
	}
	if after.Score <= before.Score {
		t.Fatalf("expected score improvement: before=%d after=%d", before.Score, after.Score)
	}
}

func TestRefreshRejectsChangedProjectAndUnreviewedDigest(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"test":"vitest run","build":"vite build"},"dependencies":{"react":"19.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir())
	plan, err := manager.PlanRefresh(project, []resourcehub.Agent{resourcehub.AgentCopilot}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ApplyRefresh(plan.ID, "sha256:wrong", true); err == nil {
		t.Fatal("expected digest mismatch")
	}
	if err := os.WriteFile(filepath.Join(project, "new.ts"), []byte("export const x = 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ApplyRefresh(plan.ID, plan.Digest, true); err == nil || !strings.Contains(err.Error(), "changed after plan review") {
		t.Fatalf("expected stale project error, got %v", err)
	}
}

func TestScoreDetectsBrokenGroundingReference(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("Use `src/missing.go` and run `go test ./...`."), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir())
	result, err := manager.Score(project, []resourcehub.Agent{resourcehub.AgentCodex})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, check := range result.Checks {
		if check.ID == "grounding.paths" && !check.Passed {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing grounding failure: %#v", result.Checks)
	}
}

func TestContextTargetPathsAreTargetNativeAndDeduplicated(t *testing.T) {
	root := t.TempDir()
	expected := map[resourcehub.Agent]string{
		resourcehub.AgentCodex:    filepath.Join(root, "AGENTS.md"),
		resourcehub.AgentOpenCode: filepath.Join(root, "AGENTS.md"),
		resourcehub.AgentClaude:   filepath.Join(root, "CLAUDE.md"),
		resourcehub.AgentCursor:   filepath.Join(root, ".cursor", "rules", "agentstack-context.mdc"),
		resourcehub.AgentCopilot:  filepath.Join(root, ".github", "copilot-instructions.md"),
	}
	for agent, want := range expected {
		if got := contextPath(root, agent); got != want {
			t.Fatalf("agent %s target path=%s want=%s", agent, got, want)
		}
	}
	manager := New(t.TempDir())
	plan, err := manager.PlanRefresh(root, []resourcehub.Agent{resourcehub.AgentCodex, resourcehub.AgentOpenCode}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Path != filepath.Join(root, "AGENTS.md") {
		t.Fatalf("shared target path was not deduplicated: %#v", plan.Operations)
	}
}

func TestContextRefreshRollsBackEarlierFilesWhenLaterWriteFails(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.com/project\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir())
	plan, err := manager.PlanRefresh(project, []resourcehub.Agent{resourcehub.AgentCodex, resourcehub.AgentClaude}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	manager.beforeApply = func(RefreshOperation) error {
		calls++
		if calls == 2 {
			return fmt.Errorf("simulated second context write failure")
		}
		return nil
	}
	report, err := manager.ApplyRefresh(plan.ID, plan.Digest, true)
	if err == nil {
		t.Fatal("expected transactional context refresh failure")
	}
	if !report.RolledBack {
		t.Fatalf("rollback was not reported: %#v", report)
	}
	for _, operation := range plan.Operations {
		if operation.Action == RefreshNoop {
			continue
		}
		if _, err := os.Stat(operation.Path); !os.IsNotExist(err) {
			t.Fatalf("partial context file remained after rollback: %s err=%v", operation.Path, err)
		}
	}
}
