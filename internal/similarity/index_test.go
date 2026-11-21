package similarity_test

import (
	"testing"

	"github.com/agentstack/agentstack/internal/similarity"
)

func TestSimilarityIndexGrouping(t *testing.T) {
	idx := similarity.NewIndex()

	docA := "---\ntitle: A\n---\n\nCore behavior instructions."
	docB := "---\ntitle: B\n---\n\nCore behavior instructions.\n"

	idx.AddDocument("global/Skill/doc-a", "/path/a", docA, false)
	idx.AddDocument("global/Skill/doc-b", "/path/b", docB, false)

	groups := idx.Groups()
	if len(groups) != 1 {
		t.Fatalf("expected 1 similarity group, got %d", len(groups))
	}

	g := groups[0]
	if len(g.ResourceIDs) != 2 {
		t.Errorf("expected 2 resource IDs in group, got %d", len(g.ResourceIDs))
	}
	if g.VarianceKind != similarity.VarianceFrontmatterOnly {
		t.Errorf("expected VarianceFrontmatterOnly, got %v", g.VarianceKind)
	}
}
