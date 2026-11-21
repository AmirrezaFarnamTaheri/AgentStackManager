package ui

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/app"
)

const maxRetainedOperations = 64

type OperationProgressItem struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type OperationProgress struct {
	Phase string `json:"phase"`
	// Completed is retained for compatibility and is identical to Processed.
	Completed    int                     `json:"completed"`
	Processed    int                     `json:"processed"`
	Succeeded    int                     `json:"succeeded"`
	Failed       int                     `json:"failed"`
	Skipped      int                     `json:"skipped"`
	Total        int                     `json:"total"`
	CurrentID    string                  `json:"currentId,omitempty"`
	CurrentLabel string                  `json:"currentLabel,omitempty"`
	Items        []OperationProgressItem `json:"items,omitempty"`
}

type ProgressReporter func(OperationProgress)

type operationFailureProvider interface {
	ClientFailure(error) ClientFailure
}

type operationStatus struct {
	OperationID string             `json:"operationId"`
	Kind        string             `json:"kind"`
	Status      string             `json:"status"`
	StartedAt   time.Time          `json:"startedAt"`
	FinishedAt  time.Time          `json:"finishedAt,omitempty"`
	Progress    *OperationProgress `json:"progress,omitempty"`
	Result      any                `json:"result,omitempty"`
	Failure     *ClientFailure     `json:"failure,omitempty"`
	// Error remains as a compatibility field for older browser clients. It is
	// always the path-free ClientFailure message, never the internal error.
	Error string `json:"error,omitempty"`
}

type operationReceipt struct {
	OperationID string `json:"operationId"`
	Status      string `json:"status"`
	StatusURL   string `json:"statusUrl"`
}

type operationStore struct {
	mu         sync.RWMutex
	operations map[string]*operationStatus
}

func newOperationStore() *operationStore {
	return &operationStore{operations: map[string]*operationStatus{}}
}

func (s *operationStore) start(ctx context.Context, kind, statusURLPrefix string, work func(context.Context, ProgressReporter) (any, error)) (operationReceipt, error) {
	if work == nil {
		return operationReceipt{}, fmt.Errorf("operation work is nil")
	}
	id, err := randomToken(rand.Reader, 16)
	if err != nil {
		return operationReceipt{}, fmt.Errorf("create operation id: %w", err)
	}
	operation := &operationStatus{OperationID: id, Kind: kind, Status: "running", StartedAt: time.Now().UTC()}
	s.mu.Lock()
	s.operations[id] = operation
	s.compactLocked()
	s.mu.Unlock()

	report := func(progress OperationProgress) {
		copyProgress := cloneOperationProgress(progress)
		s.mu.Lock()
		if current := s.operations[id]; current != nil && current.Status == "running" {
			current.Progress = &copyProgress
		}
		s.mu.Unlock()
	}

	go func() {
		result, runErr := runOperationWork(ctx, func(ctx context.Context) (any, error) { return work(ctx, report) })
		s.mu.Lock()
		defer s.mu.Unlock()
		current := s.operations[id]
		if current == nil {
			return
		}
		current.FinishedAt = time.Now().UTC()
		current.Result = result
		if runErr != nil {
			failure := clientFailureFor(runErr)
			if provider, ok := result.(operationFailureProvider); ok {
				failure = provider.ClientFailure(runErr)
			}
			if errors.Is(runErr, context.Canceled) {
				current.Status = "cancelled"
			} else {
				current.Status = "failed"
			}
			current.Failure = &failure
			current.Error = failure.Message
			return
		}
		current.Status = "succeeded"
	}()
	return operationReceipt{OperationID: id, Status: "running", StatusURL: statusURLPrefix + id}, nil
}

func cloneOperationProgress(progress OperationProgress) OperationProgress {
	copyProgress := progress
	copyProgress.Items = append([]OperationProgressItem(nil), progress.Items...)
	return copyProgress
}

func runOperationWork(ctx context.Context, work func(context.Context) (any, error)) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("operation panicked: %v", recovered)
		}
	}()
	return work(ctx)
}

func (s *operationStore) get(id string) (operationStatus, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	operation, ok := s.operations[id]
	if !ok {
		return operationStatus{}, false
	}
	copyOperation := *operation
	if operation.Progress != nil {
		progress := cloneOperationProgress(*operation.Progress)
		copyOperation.Progress = &progress
	}
	if operation.Failure != nil {
		failure := *operation.Failure
		copyOperation.Failure = &failure
	}
	return copyOperation, true
}

func (s *operationStore) compactLocked() {
	if len(s.operations) <= maxRetainedOperations {
		return
	}
	completed := make([]*operationStatus, 0, len(s.operations))
	for _, operation := range s.operations {
		if operation.Status != "running" {
			completed = append(completed, operation)
		}
	}
	sort.Slice(completed, func(i, j int) bool { return completed[i].FinishedAt.Before(completed[j].FinishedAt) })
	for len(s.operations) > maxRetainedOperations && len(completed) > 0 {
		delete(s.operations, completed[0].OperationID)
		completed = completed[1:]
	}
}

func operationProgressFromApply(progress app.ApplyProgress) OperationProgress {
	items := make([]OperationProgressItem, len(progress.Items))
	for index, item := range progress.Items {
		items[index] = OperationProgressItem{
			ID: item.ID, Label: item.Label, Action: item.Action, Status: item.Status, Message: item.Message,
		}
	}
	return OperationProgress{
		Phase: progress.Phase, Completed: progress.Completed, Processed: progress.Processed, Succeeded: progress.Succeeded, Failed: progress.Failed, Skipped: progress.Skipped, Total: progress.Total,
		CurrentID: progress.CurrentID, CurrentLabel: progress.CurrentLabel, Items: items,
	}
}
