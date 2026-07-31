package ui

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOperationStoreConvertsWorkerPanicToFailedStatus(t *testing.T) {
	store := newOperationStore()
	receipt, err := store.start(context.Background(), "panic-test", "/operations/", func(context.Context) (any, error) {
		panic("Authorization: Bearer panic-secret")
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		operation, ok := store.get(receipt.OperationID)
		if !ok {
			t.Fatal("operation disappeared")
		}
		if operation.Status == "failed" {
			if operation.Error == "" || strings.Contains(operation.Error, "panic-secret") {
				t.Fatalf("panic was not safely redacted: %+v", operation)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("panic did not become a terminal failed operation: %+v", operation)
		}
		time.Sleep(time.Millisecond)
	}
}
