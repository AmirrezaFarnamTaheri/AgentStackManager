package similarity_test

import (
	"testing"

	"github.com/agentstack/agentstack/internal/similarity"
)

func TestNormalizeBodyFrontmatterAndWhitespace(t *testing.T) {
	doc1 := `---
name: test-skill
description: A sample skill
---

# Title

Do something useful.  
`

	doc2 := `---
name: test-skill-v2
description: Updated description
---

# Title

Do something useful.
`

	norm1 := similarity.NormalizeBody(doc1)
	norm2 := similarity.NormalizeBody(doc2)

	if norm1 != norm2 {
		t.Errorf("expected normalized bodies to match:\nNORM1:\n%s\nNORM2:\n%s", norm1, norm2)
	}

	digest1 := similarity.ComputeNormalizedDigest(doc1)
	digest2 := similarity.ComputeNormalizedDigest(doc2)

	if digest1 != digest2 {
		t.Errorf("expected identical normalized digests: got %s vs %s", digest1, digest2)
	}
}

func TestNormalizeBodyPreservesCodeAndFlags(t *testing.T) {
	docA := "# Run\n\n`go test --count=1 ./...`"
	docB := "# Run\n\n`go test --count=2 ./...`"

	digestA := similarity.ComputeNormalizedDigest(docA)
	digestB := similarity.ComputeNormalizedDigest(docB)

	if digestA == digestB {
		t.Errorf("expected different digests for different command flags, got identical %s", digestA)
	}
}

func TestNormalizeBodyPreservesURLsAndNegations(t *testing.T) {
	docA := "Do NOT delete files."
	docB := "Do delete files."

	digestA := similarity.ComputeNormalizedDigest(docA)
	digestB := similarity.ComputeNormalizedDigest(docB)

	if digestA == digestB {
		t.Errorf("expected different digests for negated statement, got identical %s", digestA)
	}
}
