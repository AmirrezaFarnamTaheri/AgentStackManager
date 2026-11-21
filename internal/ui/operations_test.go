package ui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/app"
)

func waitForTerminalOperation(t *testing.T, store *operationStore, id string) operationStatus {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		operation, ok := store.get(id)
		if !ok {
			t.Fatal("operation disappeared")
		}
		if operation.Status != "running" {
			return operation
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation did not finish: %+v", operation)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestOperationStoreConvertsWorkerPanicToFailedStatus(t *testing.T) {
	store := newOperationStore()
	receipt, err := store.start(context.Background(), "panic-test", "/operations/", func(context.Context, ProgressReporter) (any, error) {
		panic("Authorization: Bearer panic-secret")
	})
	if err != nil {
		t.Fatal(err)
	}
	operation := waitForTerminalOperation(t, store, receipt.OperationID)
	if operation.Status != "failed" || operation.Failure == nil {
		t.Fatalf("panic did not become a failed operation: %+v", operation)
	}
	if operation.Error == "" || strings.Contains(operation.Error, "panic-secret") || strings.Contains(operation.Failure.Message, "panic-secret") {
		t.Fatalf("panic was not safely redacted: %+v", operation)
	}
}

func TestOperationStorePublishesCancelledStatus(t *testing.T) {
	store := newOperationStore()
	result := applyOperationResult{Outcome: ApplyOutcome{Phase: "finished", Outcome: "cancelled", Requested: 1, Skipped: 1, Retryable: true}}
	receipt, err := store.start(context.Background(), "apply", "/operations/", func(context.Context, ProgressReporter) (any, error) {
		return result, context.Canceled
	})
	if err != nil {
		t.Fatal(err)
	}
	operation := waitForTerminalOperation(t, store, receipt.OperationID)
	if operation.Status != "cancelled" || operation.Failure == nil || operation.Failure.Code != "operation_cancelled" {
		t.Fatalf("cancelled operation = %+v", operation)
	}
}

func TestOperationStorePublishesProgressWhileRunning(t *testing.T) {
	store := newOperationStore()
	reported := make(chan struct{})
	release := make(chan struct{})
	receipt, err := store.start(context.Background(), "apply", "/operations/", func(_ context.Context, report ProgressReporter) (any, error) {
		report(OperationProgress{
			Phase:        "installing",
			Completed:    1,
			Total:        3,
			CurrentID:    "node",
			CurrentLabel: "Node.js",
			Items:        []OperationProgressItem{{ID: "git", Label: "Git", Action: "install", Status: "succeeded"}},
		})
		close(reported)
		<-release
		return map[string]any{"ok": true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-reported
	operation, ok := store.get(receipt.OperationID)
	if !ok || operation.Progress == nil {
		t.Fatalf("progress missing: %+v", operation)
	}
	if operation.Status != "running" || operation.Progress.Phase != "installing" || operation.Progress.Completed != 1 || operation.Progress.Total != 3 || operation.Progress.CurrentID != "node" {
		t.Fatalf("unexpected progress: %+v", operation)
	}
	operation.Progress.Items[0].Status = "tampered"
	fresh, _ := store.get(receipt.OperationID)
	if fresh.Progress.Items[0].Status != "succeeded" {
		t.Fatalf("get returned shared progress slice: %+v", fresh.Progress)
	}
	close(release)
	terminal := waitForTerminalOperation(t, store, receipt.OperationID)
	if terminal.Status != "succeeded" {
		t.Fatalf("terminal operation = %+v", terminal)
	}
}

func TestOperationStoreProgressIsRaceSafe(t *testing.T) {
	store := newOperationStore()
	release := make(chan struct{})
	receipt, err := store.start(context.Background(), "apply", "/operations/", func(_ context.Context, report ProgressReporter) (any, error) {
		for i := 0; i < 100; i++ {
			report(OperationProgress{Phase: "installing", Completed: i, Total: 100, Items: []OperationProgressItem{{ID: "tool", Status: "running"}}})
		}
		<-release
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = store.get(receipt.OperationID)
			}
		}()
	}
	wg.Wait()
	close(release)
	_ = waitForTerminalOperation(t, store, receipt.OperationID)
}

func TestOperationProgressFromApplyPreservesOutcomeCounts(t *testing.T) {
	progress := operationProgressFromApply(app.ApplyProgress{
		Phase: "complete", Completed: 3, Processed: 3, Succeeded: 1, Failed: 1, Skipped: 1, Total: 3,
		Items: []app.ApplyProgressItem{{ID: "ok", Status: "succeeded"}, {ID: "bad", Status: "failed"}, {ID: "later", Status: "skipped"}},
	})
	if progress.Processed != 3 || progress.Succeeded != 1 || progress.Failed != 1 || progress.Skipped != 1 || progress.Completed != 3 {
		t.Fatalf("progress = %#v", progress)
	}
}
