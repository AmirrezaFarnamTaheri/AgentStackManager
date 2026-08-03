package pathenv

import (
	"strings"
	"testing"
)

func TestMergeWindowsPreservesOrderAndRemovesEquivalentSegments(t *testing.T) {
	got := MergeWindows(`C:\Windows; C:\Tools\;`, `c:\tools;C:\Users\Me\bin;;`)
	want := `C:\Windows;C:\Tools\;C:\Users\Me\bin`
	if got != want {
		t.Fatalf("MergeWindows() = %q, want %q", got, want)
	}
}

func TestAppendWindowsIsIdempotent(t *testing.T) {
	current := `C:\Windows;C:\Tools`
	got, changed := AppendWindows(current, `c:\tools\`)
	if changed || got != current {
		t.Fatalf("equivalent segment appended: %q changed=%v", got, changed)
	}
	got, changed = AppendWindows(current, `C:\AgentStack\bin`)
	if !changed || got != current+`;C:\AgentStack\bin` {
		t.Fatalf("new segment not appended: %q changed=%v", got, changed)
	}
}

func TestMergeWindowsNormalizesQuotesSeparatorsAndDotSegments(t *testing.T) {
	got := MergeWindows(`"C:\Tools\bin\";C:\SDK\tools\..\bin`, `c:/tools/bin;C:/SDK/bin`)
	want := `"C:\Tools\bin\";C:\SDK\tools\..\bin`
	if got != want {
		t.Fatalf("MergeWindows() = %q, want %q", got, want)
	}
}

func TestAppendWindowsPreservesUnrelatedDuplicateSegments(t *testing.T) {
	current := `C:\Tools;C:\Tools;C:\Windows;`
	got, changed := AppendWindows(current, `C:\AgentStack\bin`)
	if !changed {
		t.Fatal("new AgentStack segment was not appended")
	}
	want := `C:\Tools;C:\Tools;C:\Windows;C:\AgentStack\bin`
	if got != want {
		t.Fatalf("unrelated PATH entries changed: got %q want %q", got, want)
	}
}

func TestAppendWindowsPreservesEmptySegments(t *testing.T) {
	current := `C:\Tools;;;`
	got, changed := AppendWindows(current, `C:\AgentStack\bin`)
	if !changed || got != current+`C:\AgentStack\bin` {
		t.Fatalf("persistent PATH was normalized: got %q changed=%v", got, changed)
	}
}

func TestAppendWindowsKeepsDriveRootDistinctFromDriveRelativePath(t *testing.T) {
	current := `C:`
	got, changed := AppendWindows(current, `C:\`)
	if !changed || got != `C:;C:\` {
		t.Fatalf("drive root was conflated with drive-relative path: got %q changed=%v", got, changed)
	}
}

func TestAppendWindowsKeepsUNCPathDistinctFromRootedLocalPath(t *testing.T) {
	current := `\server\share`
	got, changed := AppendWindows(current, `\\server\share`)
	if !changed || got != current+`;\\server\share` {
		t.Fatalf("UNC path was conflated with rooted local path: got %q changed=%v", got, changed)
	}
}

func TestMergeWindowsRecognizesEquivalentUNCSeparators(t *testing.T) {
	got := MergeWindows(`\\server\share\tools`, `//SERVER/share/tools/`)
	if got != `\\server\share\tools` {
		t.Fatalf("equivalent UNC path was not deduplicated: %q", got)
	}
}

func TestWindowsStringTransportRoundTripPreservesUnicode(t *testing.T) {
	value := `C:\Tools;C:\کاربر\ابزار;C:\工具\bin;C:\emoji-😀`
	encoded := EncodeWindowsString(value)
	decoded, err := DecodeWindowsString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != value {
		t.Fatalf("transport changed Windows PATH: got %q want %q", decoded, value)
	}
}

func TestDecodeWindowsStringRejectsMalformedPayload(t *testing.T) {
	for _, value := range []string{"%%%", "YQ=="} {
		if _, err := DecodeWindowsString(value); err == nil {
			t.Fatalf("malformed UTF-16 transport accepted: %q", value)
		}
	}
}

func TestWindowsStringTransportBudgetCoversMaximumEnvironmentValue(t *testing.T) {
	value := strings.Repeat("x", 32767)
	if encoded := EncodeWindowsString(value); len(encoded) > MaxWindowsStringTransportBytes {
		t.Fatalf("encoded maximum environment value uses %d bytes, budget is %d", len(encoded), MaxWindowsStringTransportBytes)
	}
}
