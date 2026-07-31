package ui

import (
	"context"
	"crypto/rand"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/redact"
)

const maxRetainedOperations = 64

type operationStatus struct {
	OperationID string    `json:"operationId"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"startedAt"`
	FinishedAt  time.Time `json:"finishedAt,omitempty"`
	Result      any       `json:"result,omitempty"`
	Error       string    `json:"error,omitempty"`
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

func (s *operationStore) start(ctx context.Context, kind, statusURLPrefix string, work func(context.Context) (any, error)) (operationReceipt, error) {
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
	go func() {
		result, runErr := runOperationWork(ctx, work)
		s.mu.Lock()
		defer s.mu.Unlock()
		current := s.operations[id]
		if current == nil {
			return
		}
		current.FinishedAt = time.Now().UTC()
		current.Result = result
		if runErr != nil {
			current.Status = "failed"
			current.Error = redact.Text(runErr.Error())
			return
		}
		current.Status = "succeeded"
	}()
	return operationReceipt{OperationID: id, Status: "running", StatusURL: statusURLPrefix + id}, nil
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
	return *operation, true
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
