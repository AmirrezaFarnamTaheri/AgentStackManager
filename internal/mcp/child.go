package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/processctl"
	"github.com/agentstack/agentstack/internal/redact"
)

var SupportedProtocolVersions = []string{"2025-06-18", "2025-03-26"}

var errRequestNotStarted = errors.New("child request did not acquire the session")

const (
	defaultChildTimeout = 45 * time.Second
	defaultMessageLimit = 4 << 20
	defaultStderrLimit  = 256 << 10
)

type DoctorItem struct {
	Status          string        `json:"status"`
	Message         string        `json:"message,omitempty"`
	ProtocolVersion string        `json:"protocolVersion,omitempty"`
	ToolCount       int           `json:"toolCount,omitempty"`
	Duration        time.Duration `json:"duration,omitempty"`
}

type ChildClient interface {
	ListTools(context.Context, ServerConfig) (json.RawMessage, error)
	CallTool(context.Context, ServerConfig, string, json.RawMessage) (json.RawMessage, error)
	Doctor(context.Context, ServerConfig) DoctorItem
}

// ChildEvent is a privacy-minimized lifecycle signal for one child MCP
// process. ServerKey is a digest of the effective server configuration; raw
// arguments and environment values are deliberately excluded.
type ChildEvent struct {
	Type      string        `json:"type"`
	ServerKey string        `json:"serverKey"`
	Command   string        `json:"command"`
	Status    string        `json:"status"`
	Method    string        `json:"method,omitempty"`
	Duration  time.Duration `json:"duration"`
	Message   string        `json:"message,omitempty"`
}

type ChildObserver func(ChildEvent)

type StdIOChildClient struct {
	Timeout         time.Duration
	MaxMessageBytes int
	MaxStderrBytes  int
	Observer        ChildObserver
}

func (c StdIOChildClient) ListTools(ctx context.Context, server ServerConfig) (json.RawMessage, error) {
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	session, err := c.start(ctx, server)
	if err != nil {
		return nil, err
	}
	defer session.closeAfterOperation(ctx)
	return session.request(ctx, "tools/list", map[string]any{})
}

func (c StdIOChildClient) CallTool(ctx context.Context, server ServerConfig, name string, arguments json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	var decoded any = map[string]any{}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &decoded); err != nil {
			return nil, fmt.Errorf("decode child tool arguments: %w", err)
		}
	}
	session, err := c.start(ctx, server)
	if err != nil {
		return nil, err
	}
	defer session.closeAfterOperation(ctx)
	return session.request(ctx, "tools/call", map[string]any{"name": name, "arguments": decoded})
}

func (c StdIOChildClient) Doctor(ctx context.Context, server ServerConfig) DoctorItem {
	ctx, cancel := c.operationContext(ctx)
	defer cancel()
	started := time.Now()
	if server.Command == "" {
		return DoctorItem{Status: "error", Message: "command is empty"}
	}
	if _, err := exec.LookPath(server.Command); err != nil {
		return DoctorItem{Status: "missing", Message: err.Error()}
	}
	session, err := c.start(ctx, server)
	if err != nil {
		return DoctorItem{Status: "error", Message: redactChildError(err.Error()), Duration: time.Since(started)}
	}
	defer session.closeAfterOperation(ctx)
	result, err := session.request(ctx, "tools/list", map[string]any{})
	if err != nil {
		return DoctorItem{Status: "error", Message: redactChildError(err.Error()), ProtocolVersion: session.protocolVersion, Duration: time.Since(started)}
	}
	var payload struct {
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		return DoctorItem{Status: "error", Message: "tools/list returned invalid JSON: " + err.Error(), ProtocolVersion: session.protocolVersion, Duration: time.Since(started)}
	}
	return DoctorItem{Status: "ok", ProtocolVersion: session.protocolVersion, ToolCount: len(payload.Tools), Duration: time.Since(started)}
}

func (c StdIOChildClient) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultChildTimeout
	}
	return context.WithTimeout(parent, timeout)
}

func (c StdIOChildClient) start(ctx context.Context, server ServerConfig) (*childSession, error) {
	startedAt := time.Now()
	key := serverKey(server)
	commandName := filepath.Base(server.Command)
	emit := func(status, message string) {
		if c.Observer != nil {
			c.Observer(ChildEvent{Type: "child.launch", ServerKey: key, Command: commandName, Status: status, Duration: time.Since(startedAt), Message: redactChildError(message)})
		}
	}
	messageLimit := c.MaxMessageBytes
	if messageLimit <= 0 {
		messageLimit = defaultMessageLimit
	}
	stderrLimit := c.MaxStderrBytes
	if stderrLimit <= 0 {
		stderrLimit = defaultStderrLimit
	}
	cmd := exec.Command(server.Command, server.Args...)
	cmd.Env = os.Environ()
	for key, value := range server.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		emit("error", err.Error())
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		emit("error", err.Error())
		return nil, err
	}
	stderr := newCappedThreadSafeBuffer(stderrLimit)
	cmd.Stderr = stderr
	type startResult struct {
		process *processctl.Process
		err     error
	}
	started := make(chan startResult, 1)
	go func() {
		process, startErr := processctl.StartWithLimits(cmd, processctl.Limits{MemoryBytes: server.Limits.MemoryBytes, CPUPercent: server.Limits.CPUPercent, ActiveProcesses: server.Limits.ActiveProcesses})
		started <- startResult{process: process, err: startErr}
	}()
	var process *processctl.Process
	select {
	case result := <-started:
		process, err = result.process, result.err
	case <-ctx.Done():
		go func() {
			result := <-started
			_ = stdin.Close()
			_ = stdout.Close()
			if result.process != nil {
				_ = result.process.Terminate()
			}
		}()
		emit("error", ctx.Err().Error())
		return nil, ctx.Err()
	}
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		emit("error", err.Error())
		return nil, fmt.Errorf("start child %s: %w", server.Command, err)
	}
	session := &childSession{
		process:      process,
		stdin:        stdin,
		stdout:       stdout,
		reader:       bufio.NewReaderSize(stdout, 64*1024),
		stderr:       stderr,
		messageLimit: messageLimit,
		nextID:       1,
		serverKey:    key,
		command:      commandName,
		startedAt:    startedAt,
		observer:     c.Observer,
		requestGate:  make(chan struct{}, 1),
	}
	session.requestGate <- struct{}{}
	if err := session.initialize(ctx); err != nil {
		session.closeAfterOperation(ctx)
		emit("error", err.Error())
		return nil, err
	}
	emit("ok", "")
	return session, nil
}

type childSession struct {
	process         *processctl.Process
	stdin           io.WriteCloser
	stdout          io.ReadCloser
	reader          *bufio.Reader
	stderr          *cappedThreadSafeBuffer
	messageLimit    int
	protocolVersion string
	nextID          int
	stateMu         sync.Mutex
	requestGate     chan struct{}
	closed          bool
	serverKey       string
	command         string
	startedAt       time.Time
	observer        ChildObserver
}

func (s *childSession) initialize(ctx context.Context) error {
	result, err := s.requestRaw(ctx, "initialize", map[string]any{
		"protocolVersion": SupportedProtocolVersions[0],
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "agentstack-router", "version": "0.2.0"},
	})
	if err != nil {
		return childError(err, s.stderr.String(), s.stderr.Truncated())
	}
	var initialized struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(result, &initialized); err != nil {
		return fmt.Errorf("decode child initialize result: %w", err)
	}
	if !supportedProtocol(initialized.ProtocolVersion) {
		return fmt.Errorf("child negotiated unsupported MCP protocol %q", initialized.ProtocolVersion)
	}
	s.protocolVersion = initialized.ProtocolVersion
	return s.notify(ctx, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
}

func (s *childSession) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	started := time.Now()
	result, err := s.requestRaw(ctx, method, params)
	if s.observer != nil {
		status := "ok"
		message := ""
		if err != nil {
			status = "error"
			message = redactChildError(err.Error())
		}
		s.observer(ChildEvent{Type: "child.request", ServerKey: s.serverKey, Command: s.command, Method: method, Status: status, Duration: time.Since(started), Message: message})
	}
	if err != nil {
		return nil, childError(err, s.stderr.String(), s.stderr.Truncated())
	}
	return result, nil
}

func (s *childSession) requestRaw(ctx context.Context, method string, params any) (json.RawMessage, error) {
	select {
	case <-s.requestGate:
		defer func() { s.requestGate <- struct{}{} }()
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %w", errRequestNotStarted, ctx.Err())
	}
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return nil, errors.New("child session is closed")
	}
	s.stateMu.Unlock()
	id := s.nextID
	s.nextID++
	request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	if err := writeJSONLine(s.stdin, request, s.messageLimit); err != nil {
		return nil, err
	}
	type responseResult struct {
		result json.RawMessage
		err    error
	}
	responseCh := make(chan responseResult, 1)
	go func() {
		result, err := readResultLine(s.reader, id, s.messageLimit)
		responseCh <- responseResult{result: result, err: err}
	}()
	select {
	case response := <-responseCh:
		return response.result, response.err
	case <-ctx.Done():
		_ = s.process.Terminate()
		return nil, ctx.Err()
	}
}

func (s *childSession) notify(ctx context.Context, value any) error {
	select {
	case <-s.requestGate:
		defer func() { s.requestGate <- struct{}{} }()
	case <-ctx.Done():
		return ctx.Err()
	}
	return writeJSONLine(s.stdin, value, s.messageLimit)
}

func (s *childSession) Close() error {
	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return nil
	}
	s.closed = true
	s.stateMu.Unlock()
	err := s.process.GracefulClose(s.stdin.Close, 2*time.Second)
	_ = s.stdout.Close()
	if s.observer != nil {
		status := "ok"
		message := ""
		if err != nil {
			status = "error"
			message = redactChildError(err.Error())
		}
		s.observer(ChildEvent{Type: "child.stop", ServerKey: s.serverKey, Command: s.command, Status: status, Duration: time.Since(s.startedAt), Message: message})
	}
	return err
}

func (s *childSession) closeAfterOperation(ctx context.Context) {
	if ctx.Err() == nil {
		_ = s.Close()
		return
	}

	s.stateMu.Lock()
	if s.closed {
		s.stateMu.Unlock()
		return
	}
	s.closed = true
	s.stateMu.Unlock()
	_ = s.stdin.Close()
	_ = s.stdout.Close()
	_ = s.process.Terminate()
	if s.observer != nil {
		s.observer(ChildEvent{Type: "child.stop", ServerKey: s.serverKey, Command: s.command, Status: "timeout", Duration: time.Since(s.startedAt), Message: ctx.Err().Error()})
	}
}

type PooledChildClient struct {
	Base    StdIOChildClient
	IdleTTL time.Duration
	mu      sync.Mutex
	workers map[string]*pooledWorker
}

type pooledWorker struct {
	session    *childSession
	ready      chan struct{}
	startErr   error
	lastUsed   time.Time
	timer      *time.Timer
	generation uint64
	active     int
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
	current := p.workers[key] == worker
	sessionInvalid := err != nil && !errors.Is(err, errRequestNotStarted)
	if sessionInvalid && current {
		delete(p.workers, key)
	}
	if sessionInvalid {
		p.mu.Unlock()
		_ = worker.session.Close()
		return nil, err
	}
	if !current {
		p.mu.Unlock()
		_ = worker.session.Close()
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
	if p.workers == nil {
		p.workers = map[string]*pooledWorker{}
	}
	worker := p.workers[key]
	if worker == nil {
		worker = &pooledWorker{ready: make(chan struct{})}
		p.workers[key] = worker
		p.mu.Unlock()

		session, err := p.Base.start(ctx, server)
		p.mu.Lock()
		worker.session = session
		worker.startErr = err
		close(worker.ready)
		if err != nil && p.workers[key] == worker {
			delete(p.workers, key)
		}
		p.mu.Unlock()
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
	p.mu.Lock()
	workers := p.workers
	p.workers = map[string]*pooledWorker{}
	p.mu.Unlock()
	var errorsList []string
	for _, worker := range workers {
		if worker.timer != nil {
			worker.timer.Stop()
		}
		if worker.session != nil {
			if err := worker.session.Close(); err != nil {
				errorsList = append(errorsList, err.Error())
			}
		}
	}
	if len(errorsList) > 0 {
		sort.Strings(errorsList)
		return errors.New(strings.Join(errorsList, "; "))
	}
	return nil
}

func serverKey(server ServerConfig) string {
	data, _ := json.Marshal(server)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type childResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

func writeJSONLine(writer io.Writer, value any, limit int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("MCP message exceeds %d-byte limit", limit)
	}
	data = append(data, '\n')
	_, err = writer.Write(data)
	return err
}

func readResultLine(reader *bufio.Reader, expected int, limit int) (json.RawMessage, error) {
	for {
		line, err := readLimitedLine(reader, limit)
		if err != nil {
			return nil, err
		}
		var response childResponse
		if err := json.Unmarshal(line, &response); err != nil {
			return nil, fmt.Errorf("decode child JSON-RPC response: %w", err)
		}
		var id int
		if len(response.ID) == 0 || json.Unmarshal(response.ID, &id) != nil || id != expected {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("child JSON-RPC error %d: %s", response.Error.Code, response.Error.Message)
		}
		if len(response.Result) == 0 {
			return nil, fmt.Errorf("child response has no result")
		}
		return response.Result, nil
	}
}

func readLimitedLine(reader *bufio.Reader, limit int) ([]byte, error) {
	var result []byte
	for {
		fragment, prefix, err := reader.ReadLine()
		if err != nil {
			return nil, err
		}
		if len(result)+len(fragment) > limit {
			return nil, fmt.Errorf("MCP message exceeds %d-byte limit", limit)
		}
		result = append(result, fragment...)
		if !prefix {
			return result, nil
		}
	}
}

func supportedProtocol(version string) bool {
	for _, candidate := range SupportedProtocolVersions {
		if version == candidate {
			return true
		}
	}
	return false
}

func childError(err error, stderr string, truncated bool) error {
	stderr = redactChildError(stderr)
	if stderr == "" {
		return err
	}
	if truncated {
		stderr += " (truncated)"
	}
	return fmt.Errorf("%w; child stderr: %s", err, stderr)
}

func redactChildError(value string) string {
	value = redact.Text(strings.TrimSpace(value))
	if len(value) > defaultStderrLimit {
		value = value[:defaultStderrLimit] + "…"
	}
	return value
}

type cappedThreadSafeBuffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	limit     int
	truncated bool
}

func newCappedThreadSafeBuffer(limit int) *cappedThreadSafeBuffer {
	return &cappedThreadSafeBuffer{limit: limit}
}
func (b *cappedThreadSafeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	remaining := b.limit - b.data.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.data.Write(p[:remaining])
			b.truncated = true
		} else {
			_, _ = b.data.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return original, nil
}
func (b *cappedThreadSafeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}
func (b *cappedThreadSafeBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
