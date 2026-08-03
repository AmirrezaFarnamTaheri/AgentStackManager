package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRuntimeHelperProcess(t *testing.T) {
	if os.Getenv("AGENTSTACK_SUPERVISOR_HELPER") == "" {
		return
	}
	switch os.Getenv("AGENTSTACK_SUPERVISOR_HELPER") {
	case "output":
		fmt.Fprint(os.Stdout, strings.Repeat("o", 256))
		fmt.Fprint(os.Stderr, strings.Repeat("e", 256))
	case "sleep":
		time.Sleep(2 * time.Second)
	case "exit":
		os.Exit(7)
	case "pipe":
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			os.Exit(8)
		}
		fmt.Fprint(os.Stdout, "echo:"+line)
	}
	os.Exit(0)
}

func helperSpec(mode string) Spec {
	return Spec{
		Command: os.Args[0],
		Args:    []string{"-test.run=^TestRuntimeHelperProcess$"},
		Env:     map[string]string{"AGENTSTACK_SUPERVISOR_HELPER": mode},
	}
}

func TestRuntimeRunBoundsOutputAndPreservesExitCode(t *testing.T) {
	result := (Runtime{}).Run(context.Background(), helperSpec("output"), RunOptions{MaxOutputBytes: 64})
	if result.Err != nil {
		t.Fatalf("Run() error = %v", result.Err)
	}
	if len(result.Stdout) != 64 || len(result.Stderr) != 64 {
		t.Fatalf("captured lengths = %d/%d, want 64/64", len(result.Stdout), len(result.Stderr))
	}
	if !result.Truncated {
		t.Fatal("Run() did not report truncation")
	}

	failed := (Runtime{}).Run(context.Background(), helperSpec("exit"), RunOptions{})
	if failed.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", failed.ExitCode)
	}
	if failed.Err == nil {
		t.Fatal("Run() unexpectedly succeeded")
	}
}

func TestRuntimeRunAppliesTimeout(t *testing.T) {
	started := time.Now()
	result := (Runtime{}).Run(context.Background(), helperSpec("sleep"), RunOptions{Timeout: 30 * time.Millisecond})
	if result.Err == nil {
		t.Fatal("Run() unexpectedly succeeded")
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", result.Err)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(started))
	}
}

func TestRuntimeStartPipedRoundTrip(t *testing.T) {
	piped, err := (Runtime{}).StartPiped(context.Background(), helperSpec("pipe"))
	if err != nil {
		t.Fatalf("StartPiped() error = %v", err)
	}
	defer piped.Close()
	if _, err := io.WriteString(piped.Stdin, "hello\n"); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := piped.Stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	got, err := io.ReadAll(piped.Stdout)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if string(got) != "echo:hello\n" {
		t.Fatalf("stdout = %q", got)
	}
	if err := piped.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestRuntimeStartRejectsEmptyCommand(t *testing.T) {
	_, err := (Runtime{}).Start(context.Background(), Spec{})
	if err == nil {
		t.Fatal("Start() unexpectedly succeeded")
	}
}
