package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/processctl"
)

func TestTimeoutRegressionHangingHelper(t *testing.T) {
	if os.Getenv("GO_WANT_TIMEOUT_REGRESSION_HELPER") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var request map[string]any
		if err := decoder.Decode(&request); err != nil {
			os.Exit(0)
		}
		id, hasID := request["id"]
		if !hasID {
			continue
		}
		switch request["method"] {
		case "initialize":
			if marker := os.Getenv("GO_HANGING_INIT_PID_FILE"); marker != "" {
				_ = os.WriteFile(marker, []byte(fmt.Sprintf("%d", os.Getpid())), 0o600)
				for {
					time.Sleep(time.Hour)
				}
			}
			if marker := os.Getenv("GO_DELAYED_INIT_MARKER"); marker != "" {
				_ = os.WriteFile(marker, []byte("ready"), 0o600)
				time.Sleep(600 * time.Millisecond)
			}
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "timeout-regression", "version": "1"},
			}})
		case "tools/list":
			if os.Getenv("GO_DELAYED_INIT_MARKER") != "" {
				_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []any{}}})
				continue
			}
			for {
				time.Sleep(time.Hour)
			}
		}
	}
}

func TestSlowPersistentStartupDoesNotBlockDifferentServer(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "initialize.started")
	slow := ServerConfig{Command: os.Args[0], Args: []string{"-test.run=TestTimeoutRegressionHangingHelper", "--"}, Env: map[string]string{
		"GO_WANT_TIMEOUT_REGRESSION_HELPER": "1", "GO_DELAYED_INIT_MARKER": marker,
	}, Persistent: true}
	fast := ServerConfig{Command: os.Args[0], Args: []string{"-test.run=TestDelayedToolsListHelper", "--"}, Env: map[string]string{
		"GO_WANT_DELAYED_TOOLS_HELPER": "1",
	}, Persistent: true}
	client := &PooledChildClient{Base: StdIOChildClient{Timeout: 2 * time.Second}, IdleTTL: time.Minute}
	defer client.Close()
	slowDone := make(chan error, 1)
	go func() {
		_, err := client.ListTools(context.Background(), slow)
		slowDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("slow helper did not begin initialization")
		}
		time.Sleep(10 * time.Millisecond)
	}
	started := time.Now()
	if _, err := client.ListTools(context.Background(), fast); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("unrelated server startup was serialized behind slow initialization: %s", elapsed)
	}
	if err := <-slowDone; err != nil {
		t.Fatal(err)
	}
}

func TestConfiguredTimeoutBoundsWholeToolsListOperation(t *testing.T) {
	client := StdIOChildClient{Timeout: 60 * time.Millisecond}
	server := ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestTimeoutRegressionHangingHelper", "--"},
		Env:     map[string]string{"GO_WANT_TIMEOUT_REGRESSION_HELPER": "1"},
	}
	caller, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := client.ListTools(caller, server)
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected configured deadline, got %v", err)
	}
	if elapsed > 350*time.Millisecond {
		t.Fatalf("configured timeout did not bound the full operation: %s", elapsed)
	}
}

func TestChildErrorRedactionRemovesBearerJSONAndJWTSecrets(t *testing.T) {
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhdWRpdCJ9.signature123"
	input := `Authorization: Bearer audit-secret {"token":"audit-json-secret"} ` + jwt
	output := redactChildError(input)
	for _, secret := range []string{"audit-secret", "audit-json-secret", jwt} {
		if strings.Contains(output, secret) {
			t.Fatalf("secret remained in redacted child error: %q", output)
		}
	}
}

func TestPersistentClientTimeoutIncludesWaitingForBusySession(t *testing.T) {
	base := StdIOChildClient{Timeout: 500 * time.Millisecond}
	server := ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestTimeoutRegressionHangingHelper", "--"},
		Env:     map[string]string{"GO_WANT_TIMEOUT_REGRESSION_HELPER": "1"},
	}
	startCtx, startCancel := context.WithTimeout(context.Background(), time.Second)
	defer startCancel()
	session, err := base.start(startCtx, server)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	firstDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 220*time.Millisecond)
		defer cancel()
		_, err := session.request(ctx, "tools/list", map[string]any{})
		firstDone <- err
	}()
	time.Sleep(25 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = session.request(ctx, "tools/list", map[string]any{})
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected timeout while waiting for busy session, got %v", err)
	}
	if elapsed > 180*time.Millisecond {
		t.Fatalf("busy session wait escaped configured timeout: %s", elapsed)
	}
	<-firstDone
}

func TestPersistentWaiterTimeoutDoesNotCloseActiveSession(t *testing.T) {
	server := ServerConfig{
		Command:    os.Args[0],
		Args:       []string{"-test.run=TestDelayedToolsListHelper", "--"},
		Env:        map[string]string{"GO_WANT_DELAYED_TOOLS_HELPER": "1"},
		Persistent: true,
	}
	client := &PooledChildClient{Base: StdIOChildClient{Timeout: 500 * time.Millisecond}, IdleTTL: time.Minute}
	defer client.Close()

	firstDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		defer cancel()
		_, err := client.ListTools(ctx, server)
		firstDone <- err
	}()
	time.Sleep(25 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := client.ListTools(ctx, server); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued request error = %v; want deadline exceeded", err)
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("active request was interrupted by queued timeout: %v", err)
	}

	client.mu.Lock()
	worker := client.workers[serverKey(server)]
	client.mu.Unlock()
	if worker == nil {
		t.Fatal("healthy pooled session was evicted")
	}
}

func TestDelayedToolsListHelper(t *testing.T) {
	if os.Getenv("GO_WANT_DELAYED_TOOLS_HELPER") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var request map[string]any
		if err := decoder.Decode(&request); err != nil {
			os.Exit(0)
		}
		id, hasID := request["id"]
		if !hasID {
			continue
		}
		switch request["method"] {
		case "initialize":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "delayed-tools", "version": "1"},
			}})
		case "tools/list":
			time.Sleep(120 * time.Millisecond)
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []any{}}})
		}
	}
}

func TestRemovingFailedPooledWorkerDoesNotDeleteNewerReplacement(t *testing.T) {
	oldWorker := &pooledWorker{}
	newWorker := &pooledWorker{}
	pool := &PooledChildClient{workers: map[string]*pooledWorker{"server": newWorker}}
	pool.removeWorkerIfCurrent("server", oldWorker)
	if pool.workers["server"] != newWorker {
		t.Fatal("stale failed caller deleted a newer pooled worker")
	}
	pool.removeWorkerIfCurrent("server", newWorker)
	if _, exists := pool.workers["server"]; exists {
		t.Fatal("current failed worker was not removed")
	}
}

func TestStaleIdleTimerCannotExpireReusedPooledWorker(t *testing.T) {
	worker := &pooledWorker{generation: 2}
	pool := &PooledChildClient{workers: map[string]*pooledWorker{"server": worker}}

	pool.expire("server", worker, 1)
	if pool.workers["server"] != worker {
		t.Fatal("stale idle timer removed a worker that had been reused")
	}
}

func TestPooledClientCloseCancelsWorkerStillInitializing(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	server := ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestTimeoutRegressionHangingHelper", "--"},
		Env: map[string]string{
			"GO_WANT_TIMEOUT_REGRESSION_HELPER": "1",
			"GO_HANGING_INIT_PID_FILE":          pidFile,
		},
		Persistent: true,
	}
	client := &PooledChildClient{Base: StdIOChildClient{Timeout: 10 * time.Second}}
	requestDone := make(chan error, 1)
	go func() {
		_, err := client.ListTools(context.Background(), server)
		requestDone <- err
	}()

	var pid int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, err = strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 || !processctl.IsAlive(pid) {
		t.Fatalf("initializing child did not become live: pid=%d", pid)
	}

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for processctl.IsAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processctl.IsAlive(pid) {
		t.Fatalf("Close() left initializing child process %d alive", pid)
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("request remained blocked after Close()")
	}
}
