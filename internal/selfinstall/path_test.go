package selfinstall

import "testing"

func TestAppendPathSegmentIsIdempotentAndPreservesExisting(t *testing.T) {
	current := `C:\Windows;C:\Tools;C:\Users\Me\bin`
	target := `C:\Users\Me\AppData\Local\Programs\AgentStack\bin`
	first, changed := AppendPathSegment(current, target)
	if !changed {
		t.Fatal("expected append")
	}
	expected := current + ";" + target
	if first != expected {
		t.Fatalf("unexpected path %q", first)
	}
	second, changed := AppendPathSegment(first, target+`\`)
	if changed || second != first {
		t.Fatalf("duplicate path was appended: %q", second)
	}
}

func TestAppendPathSegmentHandlesEmptyAndCaseInsensitiveWindowsPaths(t *testing.T) {
	target := `C:\AgentStack\bin`
	got, changed := AppendPathSegment("", target)
	if !changed || got != target {
		t.Fatalf("unexpected empty result %q", got)
	}
	got, changed = AppendPathSegment(`c:\agentstack\BIN`, target)
	if changed || got != `c:\agentstack\BIN` {
		t.Fatalf("case-insensitive duplicate not detected: %q", got)
	}
}
