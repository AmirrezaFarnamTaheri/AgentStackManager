package resourcehub

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
)

type MatchKind string

const (
	MatchExactDigest         MatchKind = "ExactDigestMatch"
	MatchCaseFoldCollision   MatchKind = "CaseInsensitiveAlias"
	MatchDivergentSameName   MatchKind = "DivergentSameName"
	MatchProjectionShadow    MatchKind = "ProjectionShadow"
)

type IdentityMatch struct {
	PayloadDigest string    `json:"payloadDigest"`
	CanonicalID   string    `json:"canonicalId"`
	Names         []string  `json:"names"`
	Locations     []string  `json:"locations"`
	MatchKind     MatchKind `json:"matchKind"`
}

type AliasDecision struct {
	ResourceID                 string    `json:"resourceId"`
	CanonicalID                string    `json:"canonicalId"`
	AliasName                  string    `json:"aliasName"`
	Lineage                    string    `json:"lineage"`
	ReversibleRetirementIntent bool      `json:"reversibleRetirementIntent"`
	ApprovedBy                 string    `json:"approvedBy"`
	ApprovedAt                 time.Time `json:"approvedAt"`
	Digest                     string    `json:"digest"`
}

type IdentityIndex struct {
	mu          sync.RWMutex
	byDigest    map[string]*IdentityMatch
	byName      map[string][]string // lowercased name -> digests
	decisions   map[string]AliasDecision // resourceId -> decision
}

func NewIdentityIndex() *IdentityIndex {
	return &IdentityIndex{
		byDigest:  make(map[string]*IdentityMatch),
		byName:    make(map[string][]string),
		decisions: make(map[string]AliasDecision),
	}
}

// IndexPayload indexes a physical resource payload and groups exact content duplicates independent of folder name.
func (idx *IdentityIndex) IndexPayload(canonicalID, name, location, payloadDigest string) MatchKind {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	normName := strings.ToLower(strings.TrimSpace(name))

	// Check if this name already maps to different content digests
	existingDigests := idx.byName[normName]
	isDivergent := false
	for _, d := range existingDigests {
		if d != payloadDigest {
			isDivergent = true
			break
		}
	}

	match, exists := idx.byDigest[payloadDigest]
	if !exists {
		kind := MatchExactDigest
		if isDivergent {
			kind = MatchDivergentSameName
		}
		match = &IdentityMatch{
			PayloadDigest: payloadDigest,
			CanonicalID:   canonicalID,
			Names:         []string{name},
			Locations:     []string{location},
			MatchKind:     kind,
		}
		idx.byDigest[payloadDigest] = match
		if !containsString(idx.byName[normName], payloadDigest) {
			idx.byName[normName] = append(idx.byName[normName], payloadDigest)
		}
		return kind
	}

	// Payload digest exists -> check if location or name is new
	if !containsString(match.Locations, location) {
		match.Locations = append(match.Locations, location)
		sort.Strings(match.Locations)
	}
	if !containsString(match.Names, name) {
		match.Names = append(match.Names, name)
		sort.Strings(match.Names)
	}

	if isDivergent {
		match.MatchKind = MatchDivergentSameName
		return MatchDivergentSameName
	}

	return MatchExactDigest
}

// RecordAliasDecision records an operator-reviewed alias decision with absorption lineage and reversible retirement intent.
func (idx *IdentityIndex) RecordAliasDecision(decision AliasDecision) (AliasDecision, error) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if strings.TrimSpace(decision.ResourceID) == "" {
		return AliasDecision{}, fmt.Errorf("resource ID cannot be empty")
	}
	if strings.TrimSpace(decision.CanonicalID) == "" {
		return AliasDecision{}, fmt.Errorf("canonical ID cannot be empty")
	}
	if strings.TrimSpace(decision.AliasName) == "" {
		return AliasDecision{}, fmt.Errorf("alias name cannot be empty")
	}
	if decision.ApprovedAt.IsZero() {
		decision.ApprovedAt = time.Now().UTC()
	}

	decision.Digest = ""
	d, err := integrity.DigestJSON(decision)
	if err != nil {
		return AliasDecision{}, fmt.Errorf("digest alias decision: %w", err)
	}
	decision.Digest = d

	idx.decisions[decision.ResourceID] = decision
	return decision, nil
}

// GetAliasDecision retrieves an alias decision by resource ID.
func (idx *IdentityIndex) GetAliasDecision(resourceID string) (AliasDecision, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	d, ok := idx.decisions[resourceID]
	return d, ok
}

// ExactDuplicates returns all identity matches that have more than one observed location.
func (idx *IdentityIndex) ExactDuplicates() []IdentityMatch {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var dups []IdentityMatch
	for _, m := range idx.byDigest {
		if len(m.Locations) > 1 {
			dups = append(dups, *m)
		}
	}
	sort.Slice(dups, func(i, j int) bool {
		return dups[i].CanonicalID < dups[j].CanonicalID
	})
	return dups
}

// DivergentSameNameResources returns all name collisions where different content shares the same name.
func (idx *IdentityIndex) DivergentSameNameResources() map[string][]string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make(map[string][]string)
	for name, digests := range idx.byName {
		if len(digests) > 1 {
			result[name] = digests
		}
	}
	return result
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
