package releasepack

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPackIsDeterministicAcrossMetadataChanges(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "a.txt")
	os.WriteFile(path, []byte("same"), 0o600)
	one := filepath.Join(t.TempDir(), "one.zip")
	two := filepath.Join(t.TempDir(), "two.zip")
	if err := Pack(root, one, "bundle"); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(path, time.Now(), time.Now().Add(time.Hour))
	os.Chmod(path, 0o644)
	if err := Pack(root, two, "bundle"); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(one)
	b, _ := os.ReadFile(two)
	if !bytes.Equal(a, b) {
		t.Fatal("archives differ")
	}
}

func TestPackRejectsInvalidPrefix(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	os.MkdirAll(root, 0o700)
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o600)
	out := filepath.Join(t.TempDir(), "out.zip")

	if err := Pack(root, out, "../traversal"); err == nil {
		t.Fatal("expected error for traversal prefix")
	}
}

func TestPackExcludesDestinationInsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	os.MkdirAll(root, 0o700)
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o600)
	destInside := filepath.Join(root, "out.zip")

	if err := Pack(root, destInside, "bundle"); err != nil {
		t.Fatalf("unexpected error packing inside root: %v", err)
	}
}

func TestPackRejectsPlatformIndependentAbsolutePrefixes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, prefix := range []string{"//absolute", `C:/absolute`, `C:\\absolute`} {
		t.Run(prefix, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.zip")
			if err := Pack(root, out, prefix); err == nil {
				t.Fatalf("Pack() accepted unsafe prefix %q", prefix)
			}
		})
	}
}

func TestPackRejectsBackslashArchiveMember(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	name := "nested\\file.txt"
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.zip")
	if err := packRelativeFiles(root, out, "bundle", []string{name}); err == nil {
		t.Fatalf("packRelativeFiles() accepted unsafe member %q", name)
	}
}
