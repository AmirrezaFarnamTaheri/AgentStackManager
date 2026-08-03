package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/agentstack/agentstack/internal/supervisor"
)

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
	stderr := newCappedThreadSafeBuffer(stderrLimit)
	piped, err := (supervisor.Runtime{}).StartPiped(ctx, supervisor.Spec{
		Command: server.Command,
		Args:    append([]string(nil), server.Args...),
		Env:     server.Env,
		Stderr:  stderr,
		Limits: supervisor.Limits{
			MemoryBytes:     server.Limits.MemoryBytes,
			CPUPercent:      server.Limits.CPUPercent,
			ActiveProcesses: server.Limits.ActiveProcesses,
		},
	})
	if err != nil {
		emit("error", err.Error())
		return nil, fmt.Errorf("start child %s: %w", server.Command, err)
	}
	session := &childSession{
		process:      piped.Process,
		stdin:        piped.Stdin,
		stdout:       piped.Stdout,
		reader:       bufio.NewReaderSize(piped.Stdout, 64*1024),
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
