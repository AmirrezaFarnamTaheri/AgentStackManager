package ui

import (
	"testing"
	"time"
)

func BenchmarkOperationStoreGet(b *testing.B) {
	store := newOperationStore()
	store.operations["operation"] = &operationStatus{OperationID: "operation", Status: "succeeded", StartedAt: time.Unix(0, 0), FinishedAt: time.Unix(1, 0), Result: map[string]any{"ok": true}}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if _, ok := store.get("operation"); !ok {
			b.Fatal("operation missing")
		}
	}
}
