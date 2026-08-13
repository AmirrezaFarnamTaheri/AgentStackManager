package resourcehub_test

import (
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestIdentityIndexExactDuplicates(t *testing.T) {
	idx := resourcehub.NewIdentityIndex()

	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	match1 := idx.IndexPayload("global/Skill/tool-a", "tool-a", "/path/a/SKILL.md", digest)
	match2 := idx.IndexPayload("global/Skill/tool-a", "tool-a", "/path/b/SKILL.md", digest)

	if match1 != resourcehub.MatchExactDigest || match2 != resourcehub.MatchExactDigest {
		t.Fatalf("expected MatchExactDigest, got %v and %v", match1, match2)
	}

	dups := idx.ExactDuplicates()
	if len(dups) != 1 {
		t.Fatalf("expected 1 exact duplicate group, got %d", len(dups))
	}
	if len(dups[0].Locations) != 2 {
		t.Errorf("expected 2 locations for duplicate payload, got %d", len(dups[0].Locations))
	}
}

func TestIdentityIndexDivergentSameName(t *testing.T) {
	idx := resourcehub.NewIdentityIndex()

	digest1 := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	digest2 := "sha256:2222222222222222222222222222222222222222222222222222222222222222"

	idx.IndexPayload("global/Skill/helper", "helper", "/path/1/SKILL.md", digest1)
	match2 := idx.IndexPayload("global/Skill/helper-v2", "helper", "/path/2/SKILL.md", digest2)

	if match2 != resourcehub.MatchDivergentSameName {
		t.Errorf("expected MatchDivergentSameName, got %v", match2)
	}

	divergent := idx.DivergentSameNameResources()
	if len(divergent["helper"]) != 2 {
		t.Errorf("expected 2 digests for divergent name 'helper', got %d", len(divergent["helper"]))
	}
}

func TestRecordAliasDecision(t *testing.T) {
	idx := resourcehub.NewIdentityIndex()

	decision := resourcehub.AliasDecision{
		ResourceID:                 "global/Skill/legacy-tool",
		CanonicalID:                "global/Skill/standard-tool",
		AliasName:                  "legacy-tool",
		Lineage:                    "absorbed from donor snapshot skills-manager",
		ReversibleRetirementIntent: true,
		ApprovedBy:                 "operator-admin",
		ApprovedAt:                 time.Now().UTC(),
	}

	recorded, err := idx.RecordAliasDecision(decision)
	if err != nil {
		t.Fatalf("record alias decision: %v", err)
	}
	if recorded.Digest == "" {
		t.Errorf("expected non-empty sealed decision digest")
	}

	retrieved, ok := idx.GetAliasDecision("global/Skill/legacy-tool")
	if !ok {
		t.Fatalf("expected decision to be retrievable")
	}
	if retrieved.CanonicalID != "global/Skill/standard-tool" {
		t.Errorf("expected canonical ID 'global/Skill/standard-tool', got %s", retrieved.CanonicalID)
	}
	if !retrieved.ReversibleRetirementIntent {
		t.Errorf("expected reversible retirement intent to be true")
	}
}
