package operator

import (
	"fmt"
	"sort"
	"time"

	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/similarity"
)

type DecisionQueueType string

const (
	QueueExactDuplicates  DecisionQueueType = "exact_duplicates"
	QueueAliases          DecisionQueueType = "aliases"
	QueueConflicts        DecisionQueueType = "conflicts"
	QueueUniquePreserved  DecisionQueueType = "unique_preserved"
	QueueIncompatibility  DecisionQueueType = "incompatibility"
	QueueQuarantine       DecisionQueueType = "quarantine"
	QueueRetirement       DecisionQueueType = "retirement"
)

type CorpusQueueItem struct {
	ResourceID   string            `json:"resourceId"`
	CanonicalID  string            `json:"canonicalId"`
	QueueType    DecisionQueueType `json:"queueType"`
	Reason       string            `json:"reason"`
	Locations    []string          `json:"locations,omitempty"`
	FidelityLoss bool              `json:"fidelityLoss,omitempty"`
	CreatedAt    time.Time         `json:"createdAt"`
}

type CorpusStateSummary struct {
	TotalProcessed   int                     `json:"totalProcessed"`
	QueueCounts      map[DecisionQueueType]int `json:"queueCounts"`
	QueuedItems      []CorpusQueueItem       `json:"queuedItems"`
	TargetHealthMap  map[string]string       `json:"targetHealthMap"`
	LastEvaluatedAt  time.Time               `json:"lastEvaluatedAt"`
}

// CategorizeCorpusQueues partitions observed resources and similarity decisions into standard operator queues.
func CategorizeCorpusQueues(
	matches []resourcehub.IdentityMatch,
	decisions []similarity.ConsolidationDecision,
	targetHealth map[string]string,
	now time.Time,
) CorpusStateSummary {
	summary := CorpusStateSummary{
		QueueCounts:     make(map[DecisionQueueType]int),
		TargetHealthMap: targetHealth,
		LastEvaluatedAt: now.UTC(),
	}

	// Initialize queue counts
	allQueues := []DecisionQueueType{
		QueueExactDuplicates, QueueAliases, QueueConflicts,
		QueueUniquePreserved, QueueIncompatibility, QueueQuarantine, QueueRetirement,
	}
	for _, q := range allQueues {
		summary.QueueCounts[q] = 0
	}

	for _, m := range matches {
		summary.TotalProcessed++
		item := CorpusQueueItem{
			ResourceID:  m.PayloadDigest,
			CanonicalID: m.CanonicalID,
			Locations:   m.Locations,
			CreatedAt:   now.UTC(),
		}

		switch m.MatchKind {
		case resourcehub.MatchExactDigest:
			if len(m.Locations) > 1 {
				item.QueueType = QueueExactDuplicates
				item.Reason = fmt.Sprintf("exact payload match across %d locations", len(m.Locations))
			} else {
				item.QueueType = QueueUniquePreserved
				item.Reason = "unique canonical resource"
			}
		case resourcehub.MatchCaseFoldCollision:
			item.QueueType = QueueAliases
			item.Reason = "case-insensitive alias collision"
		case resourcehub.MatchDivergentSameName:
			item.QueueType = QueueConflicts
			item.Reason = "divergent payload content sharing identical resource name"
		default:
			item.QueueType = QueueQuarantine
			item.Reason = "unrecognized match kind"
		}

		summary.QueueCounts[item.QueueType]++
		summary.QueuedItems = append(summary.QueuedItems, item)
	}

	// Process consolidation decisions
	for _, dec := range decisions {
		if dec.ReversalPath != "" {
			item := CorpusQueueItem{
				ResourceID:  dec.ResourceID,
				CanonicalID: dec.CanonicalWinnerID,
				QueueType:   QueueRetirement,
				Reason:      fmt.Sprintf("retired via consolidation rule: %s", dec.Lineage),
				CreatedAt:   dec.ApprovedAt,
			}
			summary.QueueCounts[QueueRetirement]++
			summary.QueuedItems = append(summary.QueuedItems, item)
		}
	}

	sort.Slice(summary.QueuedItems, func(i, j int) bool {
		return summary.QueuedItems[i].CanonicalID < summary.QueuedItems[j].CanonicalID
	})

	return summary
}
