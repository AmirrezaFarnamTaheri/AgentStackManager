package selfinstall

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFromCopiesExplicitConsoleBinary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "local"))
	source := filepath.Join(root, "release", "agentstack-windows-amd64.exe")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	want := []byte("console-subsystem-binary")
	if err := os.WriteFile(source, want, 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := InstallFrom(source)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(report.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) || report.Source != source || !report.Copied {
		t.Fatalf("unexpected explicit install report=%#v content=%q", report, got)
	}
}

func TestFindReleaseConsoleSiblingUsesArchitectureName(t *testing.T) {
	dir := t.TempDir()
	setup := filepath.Join(dir, "AgentStack-Setup.exe")
	console := filepath.Join(dir, "agentstack-windows-amd64.exe")
	if err := os.WriteFile(setup, []byte("setup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(console, []byte("console"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindReleaseConsoleSibling(setup, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != console {
		t.Fatalf("expected %q, got %q", console, got)
	}
}

func TestVerifyFileSHA256AcceptsExpectedAndRejectsTamper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentstack.exe")
	if err := os.WriteFile(path, []byte("trusted"), 0o755); err != nil {
		t.Fatal(err)
	}
	const trusted = "a9a089195c68d2adeee23beaa2c3a93b1d4cdf09046e7a9e520b3b166dff3e6a"
	if err := VerifyFileSHA256(path, trusted); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFileSHA256(path, trusted); err == nil {
		t.Fatal("expected tampered console binary to be rejected")
	}
}
