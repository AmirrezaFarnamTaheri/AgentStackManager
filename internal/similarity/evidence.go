package similarity

import (
	"fmt"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
)

type RelationshipCategory string

const (
	CategoryEquivalent    RelationshipCategory = "equivalent"
	CategorySubset        RelationshipCategory = "subset"
	CategorySuperset      RelationshipCategory = "superset"
	CategoryComplementary RelationshipCategory = "complementary"
	CategoryAlternative   RelationshipCategory = "alternative"
	CategoryConflicting   RelationshipCategory = "conflicting"
	CategoryUnrelated     RelationshipCategory = "unrelated"
)

type SemanticEvidence struct {
	ScorerID        string               `json:"scorerId"`
	ScorerVersion   string               `json:"scorerVersion"`
	SourceID        string               `json:"sourceId"`
	TargetID        string               `json:"targetId"`
	Category        RelationshipCategory `json:"category"`
	Confidence      float64              `json:"confidence"`
	FeatureEvidence []string             `json:"featureEvidence"`
	Counterexamples []string             `json:"counterexamples"`
	EvaluatedAt     time.Time            `json:"evaluatedAt"`
	Invalidated     bool                 `json:"invalidated"`
	Digest          string               `json:"digest"`
}

// NewSemanticEvidence constructs an advisory evidence object.
// Acceptance rule: Semantic evidence cannot mutate canonical identity, delete a source, or authorize absorption.
func NewSemanticEvidence(scorerID, scorerVersion, sourceID, targetID string, category RelationshipCategory, confidence float64, features, counterexamples []string, evaluatedAt time.Time) (SemanticEvidence, error) {
	if strings.TrimSpace(scorerID) == "" {
		return SemanticEvidence{}, fmt.Errorf("scorer ID cannot be empty")
	}
	if strings.TrimSpace(sourceID) == "" || strings.TrimSpace(targetID) == "" {
		return SemanticEvidence{}, fmt.Errorf("source and target IDs cannot be empty")
	}
	if confidence < 0.0 || confidence > 1.0 {
		return SemanticEvidence{}, fmt.Errorf("confidence must be between 0.0 and 1.0, got %f", confidence)
	}

	ev := SemanticEvidence{
		ScorerID:        scorerID,
		ScorerVersion:   scorerVersion,
		SourceID:        sourceID,
		TargetID:        targetID,
		Category:        category,
		Confidence:      confidence,
		FeatureEvidence: features,
		Counterexamples: counterexamples,
		EvaluatedAt:     evaluatedAt.UTC(),
		Invalidated:     false,
	}

	digest, err := integrity.DigestJSON(ev)
	if err != nil {
		return SemanticEvidence{}, fmt.Errorf("digest semantic evidence: %w", err)
	}
	ev.Digest = digest
	return ev, nil
}

// InvalidateOnScorerChange marks recommendations invalid when scorer version changes.
func (ev *SemanticEvidence) InvalidateOnScorerChange(currentVersion string) {
	if ev.ScorerVersion != currentVersion {
		ev.Invalidated = true
	}
}
