package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRevision = "2356a0290239f3a7551a6db9dd7bb76f563fa96d"

func writeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SOURCE_REVISION"), []byte("unreleased-base:"+testRevision+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provenance := `{"schemaVersion":1,"status":"unreleased-test","baseRevision":"` + testRevision + `","candidateRevision":null}`
	if err := os.WriteFile(filepath.Join(root, "SOURCE_PROVENANCE.json"), []byte(provenance), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRunRequiresRoot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "--root is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunWriteVerifyAndRequire(t *testing.T) {
	root := writeFixture(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", root, "--manifest-mode", "write"}, &stdout, &stderr); code != 0 {
		t.Fatalf("write code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "wrote SOURCE_MANIFEST.sha256") {
		t.Fatalf("write stdout = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--root", root, "--manifest-mode", "verify"}, &stdout, &stderr); code != 0 {
		t.Fatalf("verify code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "verified 3 source files") {
		t.Fatalf("verify stdout = %q", stdout.String())
	}

	out := filepath.Join(t.TempDir(), "source.zip")
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"--root", root, "--out", out, "--prefix", "source", "--manifest-mode", "require"}, &stdout, &stderr); code != 0 {
		t.Fatalf("require code = %d, stderr = %q", code, stderr.String())
	}
	reader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if len(reader.File) != 4 {
		t.Fatalf("archive members = %d, want 4", len(reader.File))
	}
}

func TestRunRejectsInvalidMode(t *testing.T) {
	root := writeFixture(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", root, "--manifest-mode", "invalid"}, &stdout, &stderr); code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "invalid --manifest-mode") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	root := writeFixture(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--root", root, "unexpected"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unexpected positional argument") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
