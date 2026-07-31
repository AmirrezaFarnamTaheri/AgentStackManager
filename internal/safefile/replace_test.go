package safefile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplaceCreatesMissingDestination(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "destination.json")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Replace(source, destination); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, destination, "new")
}

func TestReplacePreservesNewContentWhenDestinationExists(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "destination.json")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Replace(source, destination); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, destination, "new")
	matches, err := filepath.Glob(filepath.Join(dir, ".agentstack-backup-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("replacement backup residue remains: %#v", matches)
	}
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("expected %q, got %q", expected, data)
	}
}

func TestReplacePreservesDestinationPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertion is not meaningful on Windows")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "destination.json")
	if err := os.WriteFile(source, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Replace(source, destination); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("destination permissions changed to %o", got)
	}
}

func TestReplacePreservesSpecialPOSIXModeBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertion is not meaningful on Windows")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "destination")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o700); err != nil {
		t.Fatal(err)
	}
	wanted := os.FileMode(0o700) | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if err := os.Chmod(destination, wanted); err != nil {
		t.Skipf("filesystem does not support special mode bits: %v", err)
	}
	before, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if before.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 {
		t.Skip("filesystem cleared special mode bits")
	}
	if err := Replace(source, destination); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	mask := os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	if got, want := after.Mode()&mask, before.Mode()&mask; got != want {
		t.Fatalf("special metadata changed: got=%v want=%v", got, want)
	}
}
