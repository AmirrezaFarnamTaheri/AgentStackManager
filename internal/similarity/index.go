package similarity

import (
	"slices"
	"sort"
	"sync"
)

type VarianceKind string

const (
	VarianceExact                 VarianceKind = "Exact"
	VarianceFrontmatterOnly       VarianceKind = "FrontmatterOnly"
	VarianceWhitespaceOnly        VarianceKind = "WhitespaceOnly"
	VarianceStructuralDivergence VarianceKind = "StructuralDivergence"
)

type SimilarityGroup struct {
	NormalizedDigest string       `json:"normalizedDigest"`
	ResourceIDs      []string     `json:"resourceIds"`
	Locations        []string     `json:"locations"`
	VarianceKind     VarianceKind `json:"varianceKind"`
}

type Index struct {
	mu     sync.RWMutex
	groups map[string]*SimilarityGroup
}

func NewIndex() *Index {
	return &Index{
		groups: make(map[string]*SimilarityGroup),
	}
}

// AddDocument indexes a document by computing its normalized body digest and grouping advisory similarity candidates.
func (idx *Index) AddDocument(resourceID, location, rawContent string, exactMatch bool) VarianceKind {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	normDigest := ComputeNormalizedDigest(rawContent)
	group, exists := idx.groups[normDigest]
	if !exists {
		group = &SimilarityGroup{
			NormalizedDigest: normDigest,
			ResourceIDs:      []string{resourceID},
			Locations:        []string{location},
			VarianceKind:     VarianceExact,
		}
		idx.groups[normDigest] = group
		return VarianceExact
	}

	// Group exists -> add resource and location if new
	if !slices.Contains(group.ResourceIDs, resourceID) {
		group.ResourceIDs = append(group.ResourceIDs, resourceID)
		sort.Strings(group.ResourceIDs)
	}
	if !slices.Contains(group.Locations, location) {
		group.Locations = append(group.Locations, location)
		sort.Strings(group.Locations)
	}

	if exactMatch {
		group.VarianceKind = VarianceExact
		return VarianceExact
	}

	group.VarianceKind = VarianceFrontmatterOnly
	return VarianceFrontmatterOnly
}

// Groups returns all similarity groups containing 2 or more resources.
func (idx *Index) Groups() []SimilarityGroup {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var result []SimilarityGroup
	for _, g := range idx.groups {
		if len(g.ResourceIDs) > 1 {
			result = append(result, *g)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].NormalizedDigest < result[j].NormalizedDigest
	})
	return result
}
