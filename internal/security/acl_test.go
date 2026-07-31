//go:build !windows

package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePrivateDirRemovesGroupAndOtherPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := EnsurePrivateDir(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
}
