package operator_test

import (
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/operator"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/similarity"
)

func TestCategorizeCorpusQueues(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	matches := []resourcehub.IdentityMatch{
		{
			PayloadDigest: "sha256:1111",
			CanonicalID:   "skill-1",
			Locations:     []string{"path/a", "path/b"},
			MatchKind:     resourcehub.MatchExactDigest,
		},
		{
			PayloadDigest: "sha256:2222",
			CanonicalID:   "skill-2",
			Locations:     []string{"path/c"},
			MatchKind:     resourcehub.MatchExactDigest,
		},
		{
			PayloadDigest: "sha256:3333",
			CanonicalID:   "skill-3",
			Locations:     []string{"path/d"},
			MatchKind:     resourcehub.MatchCaseFoldCollision,
		},
	}

	decisions := []similarity.ConsolidationDecision{
		{
			ResourceID:        "skill-dup",
			CanonicalWinnerID: "skill-1",
			Lineage:           "rule-1",
			ReversalPath:      "restore-1",
			ApprovedAt:        now,
		},
	}

	targetHealth := map[string]string{
		"opencode": "healthy",
	}

	summary := operator.CategorizeCorpusQueues(matches, decisions, targetHealth, now)

	if summary.TotalProcessed != 3 {
		t.Errorf("expected total processed 3, got %d", summary.TotalProcessed)
	}
	if summary.QueueCounts[operator.QueueExactDuplicates] != 1 {
		t.Errorf("expected 1 exact duplicate, got %d", summary.QueueCounts[operator.QueueExactDuplicates])
	}
	if summary.QueueCounts[operator.QueueUniquePreserved] != 1 {
		t.Errorf("expected 1 unique preserved, got %d", summary.QueueCounts[operator.QueueUniquePreserved])
	}
	if summary.QueueCounts[operator.QueueAliases] != 1 {
		t.Errorf("expected 1 alias, got %d", summary.QueueCounts[operator.QueueAliases])
	}
	if summary.QueueCounts[operator.QueueRetirement] != 1 {
		t.Errorf("expected 1 retirement, got %d", summary.QueueCounts[operator.QueueRetirement])
	}
}

func TestBuildWavePlanAndExecuteDrill(t *testing.T) {
	plan, err := operator.BuildWavePlan(operator.Wave1OpenCode)
	if err != nil {
		t.Fatalf("build wave plan: %v", err)
	}
	if len(plan.TargetIDs) != 1 || plan.TargetIDs[0] != "opencode" {
		t.Errorf("unexpected target IDs in wave 1: %v", plan.TargetIDs)
	}
	if len(plan.RequiredSteps) != 7 {
		t.Errorf("expected 7 required migration steps, got %d", len(plan.RequiredSteps))
	}

	res, err := operator.ExecuteWaveDrill(plan)
	if err != nil {
		t.Fatalf("execute wave drill: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected wave drill success=true")
	}
	if len(res.StepReports) != 7 {
		t.Errorf("expected 7 step reports, got %d", len(res.StepReports))
	}
}
