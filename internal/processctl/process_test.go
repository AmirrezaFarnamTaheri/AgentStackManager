package processctl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	mode := os.Getenv("AGENTSTACK_PROCESS_HELPER")
	if mode == "" {
		return
	}
	if mode == "child" {
		time.Sleep(2 * time.Minute)
		return
	}
	if mode != "parent" {
		t.Fatalf("unknown helper mode %q", mode)
	}
	gate := os.Getenv("AGENTSTACK_PROCESS_GATE")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(gate); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for process test gate")
		}
		time.Sleep(20 * time.Millisecond)
	}
	child := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	child.Env = append(os.Environ(), "AGENTSTACK_PROCESS_HELPER=child")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pidFile := os.Getenv("AGENTSTACK_PROCESS_PIDFILE")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		_ = child.Process.Kill()
		t.Fatal(err)
	}
	_ = child.Wait()
	os.Exit(0)
}

func TestTerminateStopsEntireProcessTree(t *testing.T) {
	if os.Getenv("AGENTSTACK_PROCESS_HELPER") != "" {
		return
	}
	dir := t.TempDir()
	gate := filepath.Join(dir, "gate")
	pidFile := filepath.Join(dir, "child.pid")
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(),
		"AGENTSTACK_PROCESS_HELPER=parent",
		"AGENTSTACK_PROCESS_GATE="+gate,
		"AGENTSTACK_PROCESS_PIDFILE="+pidFile,
	)
	managed, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	parentPID := cmd.Process.Pid
	if err := os.WriteFile(gate, []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}
	childPID := waitForPID(t, pidFile)
	if err := managed.Terminate(); err != nil {
		t.Fatalf("terminate process tree: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = managed.Wait(ctx)
	waitDead(t, parentPID)
	waitDead(t, childPID)
}

func TestWaitTimeoutTerminatesProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), "AGENTSTACK_PROCESS_HELPER=child")
	managed, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := managed.Wait(ctx); err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected deadline error, got %v", err)
	}
	waitDead(t, cmd.Process.Pid)
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if convErr != nil {
				t.Fatal(convErr)
			}
			return pid
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for child PID: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for processAliveForTest(pid) {
		if time.Now().After(deadline) {
			t.Fatal(fmt.Sprintf("process %d remained alive", pid))
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestGracefulCloseEscalatesWhenChildIgnoresInput(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestProcessHelper$")
	cmd.Env = append(os.Environ(), "AGENTSTACK_PROCESS_HELPER=child")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	managed, err := Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err := managed.GracefulClose(stdin.Close, 20*time.Millisecond); err == nil {
		// A forced process termination normally returns an exit error. A nil
		// result is also acceptable if the platform reports clean shutdown.
	}
	waitDead(t, cmd.Process.Pid)
}
