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
