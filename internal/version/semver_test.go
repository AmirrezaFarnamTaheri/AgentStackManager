package version

import "testing"

func TestExtractUsesConfiguredCaptureAndDefaultSemver(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pattern string
		want    string
	}{
		{name: "configured", input: "tool release=2.4.1", pattern: `release=([0-9.]+)`, want: "2.4.1"},
		{name: "default", input: "tool v1.7.3-beta.1", want: "1.7.3-beta.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Extract(test.input, test.pattern)
			if err != nil || got != test.want {
				t.Fatalf("Extract()=%q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestExtractRejectsInvalidPatternAndMissingVersion(t *testing.T) {
	if _, err := Extract("1.2.3", "("); err == nil {
		t.Fatal("invalid extraction pattern was accepted")
	}
	if _, err := Extract("no version here", ""); err == nil {
		t.Fatal("missing semantic version was accepted")
	}
}

func TestParseCompareAndCompatible(t *testing.T) {
	parsed, err := Parse("v2.5.1+build")
	if err != nil || parsed.Major != 2 || parsed.Minor != 5 || parsed.Patch != 1 {
		t.Fatalf("unexpected parsed version: %#v err=%v", parsed, err)
	}
	if comparison, err := Compare("2.5.1", "2.6.0"); err != nil || comparison != -1 {
		t.Fatalf("unexpected comparison=%d err=%v", comparison, err)
	}
	if compatible, err := Compatible("2.5.1", "2.0.0", "3.0.0"); err != nil || !compatible {
		t.Fatalf("expected compatible version: compatible=%v err=%v", compatible, err)
	}
	if compatible, err := Compatible("4.0.0", "2.0.0", "3.0.0"); err != nil || compatible {
		t.Fatalf("expected upper-bound rejection: compatible=%v err=%v", compatible, err)
	}
	if _, err := Parse("invalid"); err == nil {
		t.Fatal("invalid version was accepted")
	}
}
