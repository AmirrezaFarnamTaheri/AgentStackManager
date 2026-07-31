package state

import (
	"strings"
	"testing"
)

func FuzzSafeName(f *testing.F) {
	f.Add("mutation")
	f.Add("../../Windows/System32")
	f.Fuzz(func(t *testing.T, value string) {
		got := safeName(value)
		if strings.Contains(got, "/") || strings.Contains(got, "\\") || strings.Contains(got, "..") {
			t.Fatalf("unsafe name %q", got)
		}
	})
}
