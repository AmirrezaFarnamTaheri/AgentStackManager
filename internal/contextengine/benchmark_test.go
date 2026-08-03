package contextengine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkScanProject(b *testing.B) {
	project := b.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.com/project\n\ngo 1.23\n"), 0o600); err != nil {
		b.Fatal(err)
	}
	for index := 0; index < 250; index++ {
		dir := filepath.Join(project, "internal", fmt.Sprintf("pkg%03d", index))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "file.go"), []byte("package pkg\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	manager := New(b.TempDir())
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := manager.Scan(project); err != nil {
			b.Fatal(err)
		}
	}
}
