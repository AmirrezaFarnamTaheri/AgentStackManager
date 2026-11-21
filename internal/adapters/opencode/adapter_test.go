package opencode_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/adapters/opencode"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestOpenCodeAdapterShadowVerticalSlice(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "opencode-adapter-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Step 1: Prepare synthetic OpenCode target root
	skillDir := filepath.Join(tmpDir, "skills", "test-tool")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	initialContent := []byte("---\nname: test-tool\n---\n# Test Tool\nInitial implementation.\n")
	if err := os.WriteFile(skillPath, initialContent, 0o644); err != nil {
		t.Fatalf("write initial skill: %v", err)
	}

	adapter := opencode.NewAdapter()

	// Step 2: Discover candidate revisions
	candidates, err := adapter.Discover(tmpDir)
	if err != nil {
		t.Fatalf("adapter discover: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 discovered candidate, got %d", len(candidates))
	}
	if candidates[0].RevisionID != "rev-test-tool" {
		t.Errorf("expected candidate revision ID 'rev-test-tool', got %s", candidates[0].RevisionID)
	}

	// Step 3: Render desired canonical object
	renderedBytes, fidelity, err := adapter.Render(candidates[0], "opencode")
	if err != nil {
		t.Fatalf("adapter render: %v", err)
	}
	if fidelity.Lossy {
		t.Errorf("expected non-lossy render")
	}

	// Step 4: Compute plan
	plan, err := adapter.Plan(initialContent, renderedBytes, skillPath)
	if err != nil {
		t.Fatalf("adapter plan: %v", err)
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("expected 1 plan step, got %d", len(plan.Steps))
	}
	if plan.Steps[0].OpType != "update" {
		t.Errorf("expected opType 'update', got %s", plan.Steps[0].OpType)
	}

	// Step 5: Execute shadow projection into test root
	if err := os.WriteFile(skillPath, renderedBytes, 0o644); err != nil {
		t.Fatalf("write rendered bytes: %v", err)
	}

	// Step 6: Write ownership marker (.asm-managed)
	now := time.Now().UTC()
	marker, err := resourcehub.CreateOwnershipMarker("global/Skill/test-tool", candidates[0].RevisionID, candidates[0].SourceDigest, "opencode", skillDir, plan.PlanDigest, now)
	if err != nil {
		t.Fatalf("create ownership marker: %v", err)
	}
	if err := resourcehub.WriteOwnershipMarker(skillDir, marker); err != nil {
		t.Fatalf("write ownership marker: %v", err)
	}

	// Step 7: Verify target file digest
	expectedDigest := integrity.DigestBytes(renderedBytes)
	if err := adapter.Verify(skillPath, expectedDigest); err != nil {
		t.Fatalf("adapter verify: %v", err)
	}

	// Step 8: Assert managed projection is excluded from re-import
	if !resourcehub.ShouldExcludeFromImport(skillDir) {
		t.Errorf("expected shadow projection root to be excluded from re-import")
	}
}
