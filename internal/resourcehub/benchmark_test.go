package resourcehub

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkAuditResource(b *testing.B) {
	manager := New(b.TempDir())
	source := b.TempDir()
	for index := 0; index < 100; index++ {
		path := filepath.Join(source, fmt.Sprintf("file-%03d.md", index))
		if err := os.WriteFile(path, []byte("# Safe skill\nRun bounded commands and preserve user changes.\n"), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := manager.Import(source, ImportOptions{ID: "benchmark", Kind: KindSkill}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := manager.Audit("benchmark"); err != nil {
			b.Fatal(err)
		}
	}
}
