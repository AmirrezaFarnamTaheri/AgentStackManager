package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type PooledChildClient struct {
	Base      StdIOChildClient
	IdleTTL   time.Duration
	mu        sync.Mutex
	workers   map[string]*pooledWorker
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

type pooledWorker struct {
	session     *childSession
	ready       chan struct{}
	startErr    error
	lastUsed    time.Time
	timer       *time.Timer
	startCancel context.CancelFunc
	generation  uint64
	active      int
}

func (p *PooledChildClient) ListTools(ctx context.Context, server ServerConfig) (json.RawMessage, error) {
	if !server.Persistent {
		return p.Base.ListTools(ctx, server)
	}
	ctx, cancel := p.Base.operationContext(ctx)
	defer cancel()
	return p.withSession(ctx, server, func(session *childSession) (json.RawMessage, error) {
		return session.request(ctx, "tools/list", map[string]any{})
	})
}

func (p *PooledChildClient) CallTool(ctx context.Context, server ServerConfig, name string, arguments json.RawMessage) (json.RawMessage, error) {
	if !server.Persistent {
		return p.Base.CallTool(ctx, server, name, arguments)
	}
	ctx, cancel := p.Base.operationContext(ctx)
	defer cancel()
	var decoded any = map[string]any{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &decoded); err != nil {
			return nil, err
		}
	}
	return p.withSession(ctx, server, func(session *childSession) (json.RawMessage, error) {
		return session.request(ctx, "tools/call", map[string]any{"name": name, "arguments": decoded})
	})
}

func (p *PooledChildClient) Doctor(ctx context.Context, server ServerConfig) DoctorItem {
	if !server.Persistent {
		return p.Base.Doctor(ctx, server)
	}
	started := time.Now()
	result, err := p.ListTools(ctx, server)
	if err != nil {
		return DoctorItem{Status: "error", Message: redactChildError(err.Error()), Duration: time.Since(started)}
	}
	var payload struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return DoctorItem{Status: "error", Message: err.Error(), Duration: time.Since(started)}
	}
	return DoctorItem{Status: "ok", ToolCount: len(payload.Tools), Duration: time.Since(started)}
}

func (p *PooledChildClient) withSession(ctx context.Context, server ServerConfig, operation func(*childSession) (json.RawMessage, error)) (json.RawMessage, error) {
	key := serverKey(server)
	worker, err := p.acquireWorker(ctx, key, server)
	if err != nil {
		return nil, err
	}

	result, err := operation(worker.session)
	p.mu.Lock()
	if worker.active > 0 {
		worker.active--
	}
	noActiveCallers := worker.active == 0
	current := p.workers[key] == worker
	sessionInvalid := err != nil && !errors.Is(err, errRequestNotStarted)
	if sessionInvalid && current {
		delete(p.workers, key)
	}
	if sessionInvalid {
		p.mu.Unlock()
		if noActiveCallers {
			_ = worker.session.Close()
		}
		return nil, err
	}
	if !current {
		p.mu.Unlock()
		if noActiveCallers {
			_ = worker.session.Close()
		}
		return result, err
	}
	worker.lastUsed = time.Now()
	if worker.active == 0 {
		ttl := p.IdleTTL
		if server.IdleTTLSeconds > 0 {
			ttl = time.Duration(server.IdleTTLSeconds) * time.Second
		}
		if ttl <= 0 {
			ttl = 2 * time.Minute
		}
		generation := worker.generation
		worker.timer = time.AfterFunc(ttl, func() { p.expire(key, worker, generation) })
	}
	p.mu.Unlock()
	return result, err
}

func (p *PooledChildClient) acquireWorker(ctx context.Context, key string, server ServerConfig) (*pooledWorker, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrChildRuntimeClosed
	}
	if p.workers == nil {
		p.workers = map[string]*pooledWorker{}
	}
	worker := p.workers[key]
	if worker == nil {
		startCtx, startCancel := context.WithCancel(ctx)
		worker = &pooledWorker{ready: make(chan struct{}), startCancel: startCancel}
		p.workers[key] = worker
		p.mu.Unlock()

		session, err := p.Base.start(startCtx, server)
		startCancel()
		p.mu.Lock()
		worker.session = session
		worker.startCancel = nil
		current := p.workers[key] == worker
		closed := p.closed
		if closed {
			err = ErrChildRuntimeClosed
		}
		worker.startErr = err
		close(worker.ready)
		if err != nil && current {
			delete(p.workers, key)
		}
		p.mu.Unlock()
		if !current || closed {
			if session != nil {
				_ = session.Close()
			}
			return nil, ErrChildRuntimeClosed
		}
		if err != nil {
			return nil, err
		}
	} else if worker.ready != nil {
		ready := worker.ready
		p.mu.Unlock()
		select {
		case <-ready:
			if worker.startErr != nil {
				return nil, worker.startErr
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		p.mu.Unlock()
	}

	p.mu.Lock()
	if p.closed || p.workers[key] != worker || worker.session == nil {
		p.mu.Unlock()
		return nil, ErrChildRuntimeClosed
	}
	if worker.timer != nil {
		worker.timer.Stop()
		worker.timer = nil
	}
	worker.ready = nil
	worker.generation++
	worker.active++
	worker.lastUsed = time.Now()
	p.mu.Unlock()
	return worker, nil
}

func (p *PooledChildClient) removeWorkerIfCurrent(key string, expected *pooledWorker) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.workers[key] == expected {
		delete(p.workers, key)
	}
}

func (p *PooledChildClient) expire(key string, expected *pooledWorker, generation uint64) {
	p.mu.Lock()
	worker := p.workers[key]
	if worker != expected || worker.generation != generation || worker.active != 0 {
		p.mu.Unlock()
		return
	}
	delete(p.workers, key)
	p.mu.Unlock()
	_ = worker.session.Close()
}

func (p *PooledChildClient) Close() error {
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		workers := p.workers
		p.workers = map[string]*pooledWorker{}
		for _, worker := range workers {
			if worker.timer != nil {
				worker.timer.Stop()
			}
			if worker.startCancel != nil {
				worker.startCancel()
			}
		}
		p.mu.Unlock()

		var errorsList []string
		for _, worker := range workers {
			if worker.ready != nil {
				<-worker.ready
			}
			if worker.session != nil {
				if err := worker.session.Close(); err != nil {
					errorsList = append(errorsList, err.Error())
				}
			}
		}
		if len(errorsList) > 0 {
			sort.Strings(errorsList)
			p.closeErr = errors.New(strings.Join(errorsList, "; "))
		}
	})
	return p.closeErr
}

func serverKey(server ServerConfig) string {
	data, _ := json.Marshal(server)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
