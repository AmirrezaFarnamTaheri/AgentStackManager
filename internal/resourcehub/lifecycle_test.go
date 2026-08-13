package resourcehub_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestLifecycleRecordCreationAndSealing(t *testing.T) {
	now := time.Now().UTC()
	art := artifactgraph.Artifact{
		APIVersion: artifactgraph.APIVersion,
		ID:         "global/Skill/test-skill",
		Kind:       artifactgraph.KindSkill,
		Metadata: artifactgraph.Metadata{
			Namespace: "global",
			Name:      "test-skill",
		},
		Content: artifactgraph.ContentReference{
			Ref:       "cas:1111",
			Digest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			MediaType: "text/markdown",
		},
		Source: artifactgraph.SourceReference{
			Type: "file",
			URI:  "file:///skills/test.md",
		},
		Security: artifactgraph.SecurityClassification{
			ExecutionClass: artifactgraph.ExecutionDeclarative,
		},
		Provenance: artifactgraph.Provenance{
			Origin:     "test",
			ImportedBy: "tester",
			ImportedAt: now,
			UpdatedAt:  now,
		},
	}
	sealedArt, err := artifactgraph.Seal(art)
	if err != nil {
		t.Fatalf("seal artifact: %v", err)
	}

	rec, err := resourcehub.NewLifecycleRecord("lc-001", sealedArt.ID, sealedArt.Digest, "file:///skills/test.md", "sha256:1111111111111111111111111111111111111111111111111111111111111111", now)
	if err != nil {
		t.Fatalf("new lifecycle record: %v", err)
	}

	if rec.State != resourcehub.StateObserved {
		t.Errorf("expected initial state %q, got %q", resourcehub.StateObserved, rec.State)
	}

	if err := resourcehub.VerifyLifecycleRecord(rec); err != nil {
		t.Errorf("verify lifecycle record failed: %v", err)
	}
}

func TestLifecycleIllegalTransitions(t *testing.T) {
	now := time.Now().UTC()
	rec, err := resourcehub.NewLifecycleRecord("lc-002", "art-002", "digest-2222", "file:///rule.md", "digest-source", now)
	if err != nil {
		t.Fatalf("new lifecycle record: %v", err)
	}

	// Direct leap from observed to canonical must be rejected (never silently promote an observation)
	err = resourcehub.TransitionTo(&rec, resourcehub.StateCanonical, "operator", "silent promotion test", "ev-1", now)
	if err == nil {
		t.Fatalf("expected error for silent promotion from observed to canonical, got nil")
	}

	// Direct leap from observed to candidate must also be rejected
	err = resourcehub.TransitionTo(&rec, resourcehub.StateCandidate, "operator", "silent promotion test", "ev-1", now)
	if err == nil {
		t.Fatalf("expected error for leap from observed to candidate, got nil")
	}

	// Legal step-by-step path: observed -> parsed -> classified -> candidate -> canonical -> projected -> verified
	steps := []resourcehub.LifecycleState{
		resourcehub.StateParsed,
		resourcehub.StateClassified,
		resourcehub.StateCandidate,
		resourcehub.StateCanonical,
		resourcehub.StateProjected,
		resourcehub.StateVerified,
	}

	for _, step := range steps {
		if err := resourcehub.TransitionTo(&rec, step, "operator", "valid step", "ev-step", now); err != nil {
			t.Fatalf("failed valid transition to %q: %v", step, err)
		}
	}

	if rec.State != resourcehub.StateVerified {
		t.Errorf("expected final state %q, got %q", resourcehub.StateVerified, rec.State)
	}
}

func TestLifecycleVersionValidation(t *testing.T) {
	invalidJSON := []byte(`{
		"apiVersion": "resourcehub.asm.dev/v2",
		"id": "lc-003",
		"artifactId": "art-003",
		"artifactDigest": "digest-3333",
		"state": "observed",
		"sourceUri": "file:///test",
		"sourceDigest": "digest-src",
		"createdAt": "2026-08-13T00:00:00Z",
		"updatedAt": "2026-08-13T00:00:00Z",
		"digest": "fake"
	}`)

	_, err := resourcehub.UnmarshalLifecycleRecordJSON(invalidJSON)
	if err == nil {
		t.Fatalf("expected error for unsupported API version v2, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported lifecycle record API version") {
		t.Errorf("expected unsupported API version error, got %v", err)
	}
}

func TestCandidateRevisionAfterSourceEdit(t *testing.T) {
	now := time.Now().UTC()
	rec, err := resourcehub.NewLifecycleRecord("lc-004", "art-004", "digest-4444", "file:///code.rs", "digest-v1", now)
	if err != nil {
		t.Fatalf("new lifecycle record: %v", err)
	}

	if err := resourcehub.AddCandidateRevision(&rec, "digest-v2", "developer", "source modified", now.Add(time.Hour)); err != nil {
		t.Fatalf("add candidate revision: %v", err)
	}

	if len(rec.CandidateRevisions) != 1 {
		t.Fatalf("expected 1 candidate revision, got %d", len(rec.CandidateRevisions))
	}

	if rec.CandidateRevisions[0].SourceDigest != "digest-v2" {
		t.Errorf("expected source digest digest-v2, got %q", rec.CandidateRevisions[0].SourceDigest)
	}

	if rec.SourceDigest != "digest-v2" {
		t.Errorf("expected record source digest digest-v2, got %q", rec.SourceDigest)
	}

	// Same digest revision should be rejected
	if err := resourcehub.AddCandidateRevision(&rec, "digest-v2", "developer", "no change", now); err == nil {
		t.Errorf("expected error when adding identical candidate revision digest")
	}
}

func TestAliasCollision(t *testing.T) {
	now := time.Now().UTC()
	rec, err := resourcehub.NewLifecycleRecord("lc-005", "art-005", "digest-5555", "file:///rule.md", "digest-src", now)
	if err != nil {
		t.Fatalf("new lifecycle record: %v", err)
	}

	if err := resourcehub.AddAlias(&rec, "global-rule-5"); err != nil {
		t.Fatalf("add alias: %v", err)
	}

	// Adding same alias again must fail with collision error
	err = resourcehub.AddAlias(&rec, "global-rule-5")
	if err == nil {
		t.Fatalf("expected alias collision error, got nil")
	}
	if !strings.Contains(err.Error(), "alias collision") {
		t.Errorf("expected alias collision message, got %v", err)
	}
}

func TestRetirementAndReversal(t *testing.T) {
	now := time.Now().UTC()
	rec, err := resourcehub.NewLifecycleRecord("lc-006", "art-006", "digest-6666", "file:///old-rule.md", "digest-src", now)
	if err != nil {
		t.Fatalf("new lifecycle record: %v", err)
	}

	// Move to parsed then classified
	if err := resourcehub.TransitionTo(&rec, resourcehub.StateParsed, "op", "parse", "ev", now); err != nil {
		t.Fatalf("transition parsed: %v", err)
	}
	if err := resourcehub.TransitionTo(&rec, resourcehub.StateClassified, "op", "classify", "ev", now); err != nil {
		t.Fatalf("transition classified: %v", err)
	}

	// Retire record
	if err := resourcehub.Retire(&rec, "security-auditor", "deprecated rule", "ev-retire", now); err != nil {
		t.Fatalf("retire record: %v", err)
	}

	if rec.State != resourcehub.StateRetired {
		t.Errorf("expected state %q, got %q", resourcehub.StateRetired, rec.State)
	}

	// Reverse retirement
	if err := resourcehub.ReverseRetirement(&rec, "lead-architect", "reinstated rule", "ev-reinstate", now.Add(time.Hour)); err != nil {
		t.Fatalf("reverse retirement: %v", err)
	}

	if rec.State != resourcehub.StateClassified {
		t.Errorf("expected restored state %q, got %q", resourcehub.StateClassified, rec.State)
	}

	if rec.Retirement == nil || rec.Retirement.Reversal == nil {
		t.Fatalf("expected non-nil retirement reversal info")
	}
	if rec.Retirement.Reversal.ReversedBy != "lead-architect" {
		t.Errorf("expected ReversedBy lead-architect, got %q", rec.Retirement.Reversal.ReversedBy)
	}
}

func TestManagerLifecyclePersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-lifecycle-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := resourcehub.New(tmpDir)
	now := time.Now().UTC()

	rec, err := resourcehub.NewLifecycleRecord("lc-101", "art-101", "digest-101", "file:///skill.md", "digest-src", now)
	if err != nil {
		t.Fatalf("new lifecycle record: %v", err)
	}

	if err := mgr.SaveLifecycleRecord(rec); err != nil {
		t.Fatalf("save lifecycle record: %v", err)
	}

	loaded, err := mgr.LoadLifecycleRecord("lc-101")
	if err != nil {
		t.Fatalf("load lifecycle record: %v", err)
	}

	if loaded.Digest != rec.Digest {
		t.Errorf("expected digest %q, got %q", rec.Digest, loaded.Digest)
	}

	records, err := mgr.ListLifecycleRecords()
	if err != nil {
		t.Fatalf("list lifecycle records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record in list, got %d", len(records))
	}
}
