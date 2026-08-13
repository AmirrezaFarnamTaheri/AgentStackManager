package similarity

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
)

type ConsolidationRelationship string

const (
	RelationshipTemplate           ConsolidationRelationship = "Template"
	RelationshipSpecialization     ConsolidationRelationship = "Specialization"
	RelationshipAlternative        ConsolidationRelationship = "Alternative"
	RelationshipComposition        ConsolidationRelationship = "Composition"
	RelationshipDataParameterization ConsolidationRelationship = "DataParameterization"
)

type ConsolidationCandidate struct {
	ResourceID      string `json:"resourceId"`
	IsOperatorChoice bool   `json:"isOperatorChoice"`
	IsGovernedSource bool   `json:"isGovernedSource"`
	CompletenessScore int   `json:"completenessScore"`
	IsValidSchema    bool   `json:"isValidSchema"`
}

type ConsolidationDecision struct {
	ResourceID         string                    `json:"resourceId"`
	CanonicalWinnerID  string                    `json:"canonicalWinnerId"`
	PreservedMaterial  []string                  `json:"preservedMaterial"`
	RejectedMaterial   []string                  `json:"rejectedMaterial"`
	Lineage            string                    `json:"lineage"`
	TargetImpact       string                    `json:"targetImpact"`
	ReversalPath       string                    `json:"reversalPath"`
	RelationshipType   ConsolidationRelationship `json:"relationshipType"`
	ApprovedBy         string                    `json:"approvedBy"`
	ApprovedAt         time.Time                 `json:"approvedAt"`
	Digest             string                    `json:"digest"`
}

// SelectCanonicalWinner determines the canonical winner using strictly ordered rules:
// 1. Operator choice
// 2. Governed-source preference
// 3. Completeness score
// 4. Schema validity
// 5. Stable lexicographical ID tie-breaker
func SelectCanonicalWinner(candidates []ConsolidationCandidate) (ConsolidationCandidate, error) {
	if len(candidates) == 0 {
		return ConsolidationCandidate{}, fmt.Errorf("no candidates provided for winner selection")
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		c1, c2 := candidates[i], candidates[j]
		if c1.IsOperatorChoice != c2.IsOperatorChoice {
			return c1.IsOperatorChoice
		}
		if c1.IsGovernedSource != c2.IsGovernedSource {
			return c1.IsGovernedSource
		}
		if c1.CompletenessScore != c2.CompletenessScore {
			return c1.CompletenessScore > c2.CompletenessScore
		}
		if c1.IsValidSchema != c2.IsValidSchema {
			return c1.IsValidSchema
		}
		return c1.ResourceID < c2.ResourceID
	})

	return candidates[0], nil
}

// CreateConsolidationDecision creates and seals a reviewed consolidation decision with reversal path and lineage.
func CreateConsolidationDecision(
	resourceID, canonicalWinnerID, lineage, targetImpact, reversalPath string,
	relationshipType ConsolidationRelationship,
	preservedMaterial, rejectedMaterial []string,
	approvedBy string,
	now time.Time,
) (ConsolidationDecision, error) {
	if strings.TrimSpace(resourceID) == "" {
		return ConsolidationDecision{}, fmt.Errorf("resource ID cannot be empty")
	}
	if strings.TrimSpace(canonicalWinnerID) == "" {
		return ConsolidationDecision{}, fmt.Errorf("canonical winner ID cannot be empty")
	}
	if strings.TrimSpace(reversalPath) == "" {
		return ConsolidationDecision{}, fmt.Errorf("reversal path cannot be empty")
	}
	if strings.TrimSpace(approvedBy) == "" {
		return ConsolidationDecision{}, fmt.Errorf("approvedBy cannot be empty")
	}

	decision := ConsolidationDecision{
		ResourceID:        resourceID,
		CanonicalWinnerID: canonicalWinnerID,
		PreservedMaterial: preservedMaterial,
		RejectedMaterial:  rejectedMaterial,
		Lineage:           lineage,
		TargetImpact:      targetImpact,
		ReversalPath:      reversalPath,
		RelationshipType:  relationshipType,
		ApprovedBy:        approvedBy,
		ApprovedAt:        now.UTC(),
	}

	digest, err := integrity.DigestJSON(decision)
	if err != nil {
		return ConsolidationDecision{}, fmt.Errorf("digest consolidation decision: %w", err)
	}
	decision.Digest = digest
	return decision, nil
}
