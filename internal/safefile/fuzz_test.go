package safefile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func FuzzReplacePreservesNewContent(f *testing.F) {
	f.Add([]byte("old"), []byte("new"))
	f.Fuzz(func(t *testing.T, oldData, newData []byte) {
		if len(oldData) > 4096 || len(newData) > 4096 {
			return
		}
		dir := t.TempDir()
		dst := filepath.Join(dir, "value")
		src := filepath.Join(dir, "source")
		if err := os.WriteFile(dst, oldData, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, newData, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := Replace(src, dst); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, newData) {
			t.Fatalf("got %q want %q", got, newData)
		}
	})
}
