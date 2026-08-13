package similarity_test

import (
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/similarity"
)

func TestSemanticEvidenceCreationAndInvalidation(t *testing.T) {
	now := time.Now().UTC()
	ev, err := similarity.NewSemanticEvidence(
		"asm.semantic.v1",
		"1.0.0",
		"global/Skill/tool-a",
		"global/Skill/tool-b",
		similarity.CategoryEquivalent,
		0.95,
		[]string{"identical docstring", "same parameter schema"},
		nil,
		now,
	)

	if err != nil {
		t.Fatalf("create semantic evidence: %v", err)
	}
	if ev.Digest == "" {
		t.Errorf("expected non-empty digest for sealed evidence")
	}

	if ev.Invalidated {
		t.Errorf("expected new evidence to be valid")
	}

	ev.InvalidateOnScorerChange("1.1.0")
	if !ev.Invalidated {
		t.Errorf("expected evidence to be invalidated after scorer version bump")
	}
}

func TestSemanticEvidenceConfidenceBounds(t *testing.T) {
	now := time.Now().UTC()
	_, err := similarity.NewSemanticEvidence(
		"asm.semantic.v1",
		"1.0.0",
		"global/Skill/tool-a",
		"global/Skill/tool-b",
		similarity.CategoryEquivalent,
		1.5, // Invalid > 1.0
		nil,
		nil,
		now,
	)
	if err == nil {
		t.Errorf("expected error for confidence > 1.0, got nil")
	}
}
