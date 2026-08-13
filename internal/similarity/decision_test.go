package similarity_test

import (
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/similarity"
)

func TestCanonicalWinnerSelectionOrdering(t *testing.T) {
	candidates := []similarity.ConsolidationCandidate{
		{ResourceID: "candidate-c", IsOperatorChoice: false, IsGovernedSource: false, CompletenessScore: 80, IsValidSchema: true},
		{ResourceID: "candidate-b", IsOperatorChoice: false, IsGovernedSource: true, CompletenessScore: 90, IsValidSchema: true},
		{ResourceID: "candidate-a", IsOperatorChoice: true, IsGovernedSource: false, CompletenessScore: 50, IsValidSchema: true},
	}

	winner, err := similarity.SelectCanonicalWinner(candidates)
	if err != nil {
		t.Fatalf("select winner: %v", err)
	}
	// Rule 1: Operator choice wins over governed source and score
	if winner.ResourceID != "candidate-a" {
		t.Errorf("expected operator choice 'candidate-a', got %s", winner.ResourceID)
	}

	// Remove operator choice -> Governed source should win
	candidatesNoChoice := candidates[1:]
	winner2, err := similarity.SelectCanonicalWinner(candidatesNoChoice)
	if err != nil {
		t.Fatalf("select winner without choice: %v", err)
	}
	if winner2.ResourceID != "candidate-b" {
		t.Errorf("expected governed source 'candidate-b', got %s", winner2.ResourceID)
	}
}

func TestCreateConsolidationDecision(t *testing.T) {
	now := time.Now().UTC()
	decision, err := similarity.CreateConsolidationDecision(
		"global/Skill/legacy-tool",
		"global/Skill/canonical-tool",
		"absorbed from donor snapshot skills-manager",
		"none; alias preserved",
		"restore /path/to/legacy-tool",
		similarity.RelationshipSpecialization,
		[]string{"examples", "options"},
		[]string{"deprecated_flag"},
		"operator-admin",
		now,
	)
	if err != nil {
		t.Fatalf("create consolidation decision: %v", err)
	}
	if decision.Digest == "" {
		t.Errorf("expected non-empty decision digest")
	}
	if decision.CanonicalWinnerID != "global/Skill/canonical-tool" {
		t.Errorf("expected canonical winner ID 'global/Skill/canonical-tool', got %s", decision.CanonicalWinnerID)
	}
}
