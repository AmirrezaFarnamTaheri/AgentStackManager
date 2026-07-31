package safefile

import (
	"os"
	"path/filepath"
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
