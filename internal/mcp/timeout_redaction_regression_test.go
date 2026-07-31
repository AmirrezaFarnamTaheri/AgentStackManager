package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
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
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "timeout-regression", "version": "1"},
			}})
		case "tools/list":
			for {
				time.Sleep(time.Hour)
			}
		}
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
