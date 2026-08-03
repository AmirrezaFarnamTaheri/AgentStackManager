package workspace

import (
	"fmt"
	"testing"
	"time"
)

func BenchmarkSearchMemory(b *testing.B) {
	manager := New(b.TempDir())
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	entries := make(map[string]MemoryEntry, 1000)
	for index := 0; index < 1000; index++ {
		key := fmt.Sprintf("key-%04d", index)
		entry := MemoryEntry{ID: memoryID(LayerUser, "", key), Layer: LayerUser, Key: key, Value: fmt.Sprintf("project release context %d", index), Digest: contentDigest(fmt.Sprintf("project release context %d", index)), CreatedAt: now, UpdatedAt: now}
		entries[entry.ID] = entry
	}
	if err := manager.saveMemory(entries); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := manager.SearchMemory("release context 999", "", ""); err != nil {
			b.Fatal(err)
		}
	}
}
