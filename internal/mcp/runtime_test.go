package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/processctl"
)

func TestManagedChildRuntimeSelectsPersistentLifecycle(t *testing.T) {
	countFile := t.TempDir() + string(os.PathSeparator) + "starts.txt"
	runtime := NewManagedChildRuntime(ChildRuntimeOptions{Timeout: 5 * time.Second, IdleTTL: time.Minute})
	defer runtime.Close()
	server := ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPHelperProcess", "--"},
		Env: map[string]string{
			"GO_WANT_MCP_HELPER":    "1",
			"MCP_HELPER_COUNT_FILE": countFile,
		},
		Persistent: true,
	}
	for index := 0; index < 3; index++ {
		if _, err := runtime.ListTools(context.Background(), server); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "start\n" {
		t.Fatalf("persistent runtime starts = %q", data)
	}
}

func TestManagedChildRuntimeKeepsOneShotServersOneShot(t *testing.T) {
	countFile := t.TempDir() + string(os.PathSeparator) + "starts.txt"
	runtime := NewManagedChildRuntime(ChildRuntimeOptions{Timeout: 5 * time.Second})
	defer runtime.Close()
	server := ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPHelperProcess", "--"},
		Env: map[string]string{
			"GO_WANT_MCP_HELPER":    "1",
			"MCP_HELPER_COUNT_FILE": countFile,
		},
	}
	for index := 0; index < 2; index++ {
		if _, err := runtime.ListTools(context.Background(), server); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if starts := strings.Count(string(data), "start\n"); starts != 2 {
		t.Fatalf("one-shot runtime starts = %d, want 2", starts)
	}
}

func TestManagedChildRuntimeForwardsCallsAndDoctor(t *testing.T) {
	runtime := NewManagedChildRuntime(ChildRuntimeOptions{Timeout: 5 * time.Second})
	defer runtime.Close()
	server := ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestMCPHelperProcess", "--"},
		Env:     map[string]string{"GO_WANT_MCP_HELPER": "1"},
	}
	called, err := runtime.CallTool(context.Background(), server, "helper_tool", json.RawMessage(`{"value":9}`))
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !strings.Contains(string(called), "value=9") {
		t.Fatalf("CallTool() result = %s", called)
	}
	doctor := runtime.Doctor(context.Background(), server)
	if doctor.Status != "ok" || doctor.ToolCount != 1 {
		t.Fatalf("Doctor() = %+v", doctor)
	}
}

func TestManagedChildRuntimeNilCloseIsSafe(t *testing.T) {
	var runtime *ManagedChildRuntime
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestManagedChildRuntimeCloseCancelsOneShotInitialization(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	runtime := NewManagedChildRuntime(ChildRuntimeOptions{Timeout: 10 * time.Second})
	server := ServerConfig{
		Command: os.Args[0],
		Args:    []string{"-test.run=TestTimeoutRegressionHangingHelper", "--"},
		Env: map[string]string{
			"GO_WANT_TIMEOUT_REGRESSION_HELPER": "1",
			"GO_HANGING_INIT_PID_FILE":          pidFile,
		},
	}
	requestDone := make(chan error, 1)
	go func() {
		_, err := runtime.ListTools(context.Background(), server)
		requestDone <- err
	}()

	pid := waitForChildPID(t, pidFile)
	if !processctl.IsAlive(pid) {
		t.Fatalf("one-shot child did not become live: pid=%d", pid)
	}

	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for processctl.IsAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if processctl.IsAlive(pid) {
		t.Fatalf("Close() left one-shot child process %d alive", pid)
	}
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("one-shot request remained blocked after Close()")
	}
}

func TestManagedChildRuntimeRejectsOperationsAfterClose(t *testing.T) {
	runtime := NewManagedChildRuntime(ChildRuntimeOptions{})
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	server := ServerConfig{Command: "unused"}
	if _, err := runtime.ListTools(context.Background(), server); !errors.Is(err, ErrChildRuntimeClosed) {
		t.Fatalf("ListTools() error = %v, want ErrChildRuntimeClosed", err)
	}
	if _, err := runtime.CallTool(context.Background(), server, "unused", nil); !errors.Is(err, ErrChildRuntimeClosed) {
		t.Fatalf("CallTool() error = %v, want ErrChildRuntimeClosed", err)
	}
	doctor := runtime.Doctor(context.Background(), server)
	if doctor.Status != "error" || !strings.Contains(doctor.Message, ErrChildRuntimeClosed.Error()) {
		t.Fatalf("Doctor() = %+v", doctor)
	}
}

func waitForChildPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			value := strings.TrimSpace(string(data))
			if value == "" {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			pid, err := strconv.Atoi(value)
			if err != nil {
				t.Fatal(err)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for child pid file %s", pidFile)
	return 0
}
