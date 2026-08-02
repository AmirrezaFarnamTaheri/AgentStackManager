package integrity

import (
	"strings"
	"testing"
)

func TestDigestJSONIsDeterministicAcrossMapOrder(t *testing.T) {
	left, err := DigestJSON(map[string]any{"a": 1, "b": "two"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := DigestJSON(map[string]any{"b": "two", "a": 1})
	if err != nil {
		t.Fatal(err)
	}
	if left != right || !strings.HasPrefix(left, "sha256:") || len(left) != len("sha256:")+64 {
		t.Fatalf("unexpected deterministic digests: %q %q", left, right)
	}
}

func TestDigestJSONReportsUnsupportedValues(t *testing.T) {
	if _, err := DigestJSON(make(chan int)); err == nil || !strings.Contains(err.Error(), "marshal digest input") {
		t.Fatalf("unsupported value error=%v", err)
	}
}
