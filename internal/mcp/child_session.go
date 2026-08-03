package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/supervisor"
)

type childSession struct {
	process         *supervisor.Process
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
