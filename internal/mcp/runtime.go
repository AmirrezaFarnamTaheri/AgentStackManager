package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// ChildRuntimeOptions configures the single lifecycle boundary used by router,
// doctor, and CLI callers. Persistent and one-shot server behavior remains a
// property of each ServerConfig rather than a caller-side implementation choice.
type ChildRuntimeOptions struct {
	Timeout         time.Duration
	IdleTTL         time.Duration
	MaxMessageBytes int
	MaxStderrBytes  int
	Observer        ChildObserver
}

// ManagedChildRuntime hides one-shot process creation, persistent worker reuse,
// idle expiry, protocol negotiation, bounded framing, and shutdown behind one
// ChildClient implementation.
type ManagedChildRuntime struct {
	mu        sync.Mutex
	pool      *PooledChildClient
	root      context.Context
	cancel    context.CancelFunc
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func NewManagedChildRuntime(options ChildRuntimeOptions) *ManagedChildRuntime {
	root, cancel := context.WithCancel(context.Background())
	return &ManagedChildRuntime{
		root:   root,
		cancel: cancel,
		pool: &PooledChildClient{
			Base: StdIOChildClient{
				Timeout:         options.Timeout,
				MaxMessageBytes: options.MaxMessageBytes,
				MaxStderrBytes:  options.MaxStderrBytes,
				Observer:        options.Observer,
			},
			IdleTTL: options.IdleTTL,
		},
	}
}

func (r *ManagedChildRuntime) ListTools(ctx context.Context, server ServerConfig) (json.RawMessage, error) {
	operationCtx, cancel, client, err := r.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	return client.ListTools(operationCtx, server)
}

func (r *ManagedChildRuntime) CallTool(ctx context.Context, server ServerConfig, name string, arguments json.RawMessage) (json.RawMessage, error) {
	operationCtx, cancel, client, err := r.operationContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	return client.CallTool(operationCtx, server, name, arguments)
}

func (r *ManagedChildRuntime) Doctor(ctx context.Context, server ServerConfig) DoctorItem {
	operationCtx, cancel, client, err := r.operationContext(ctx)
	if err != nil {
		return DoctorItem{Status: "error", Message: err.Error()}
	}
	defer cancel()
	return client.Doctor(operationCtx, server)
}

func (r *ManagedChildRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		cancel := r.cancel
		client := r.pool
		r.mu.Unlock()

		if cancel != nil {
			cancel()
		}
		if client != nil {
			r.closeErr = client.Close()
		}
	})
	return r.closeErr
}

func (r *ManagedChildRuntime) operationContext(parent context.Context) (context.Context, context.CancelFunc, *PooledChildClient, error) {
	if r == nil {
		return nil, nil, nil, ErrChildRuntimeClosed
	}
	if parent == nil {
		parent = context.Background()
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, nil, nil, ErrChildRuntimeClosed
	}
	if r.root == nil {
		r.root, r.cancel = context.WithCancel(context.Background())
	}
	if r.pool == nil {
		r.pool = &PooledChildClient{}
	}
	root := r.root
	client := r.pool
	r.mu.Unlock()

	operationCtx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(root, cancel)
	return operationCtx, func() {
		stop()
		cancel()
	}, client, nil
}
