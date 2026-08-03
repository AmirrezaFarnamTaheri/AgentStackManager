package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type fakeStarter struct {
	calls []StartRequest
	err   error
}

func (f *fakeStarter) Run(_ context.Context, request StartRequest) error {
	f.calls = append(f.calls, request)
	return f.err
}

func TestLaunchInitializesBeforeStartingAndForwardsArguments(t *testing.T) {
	order := []string{}
	starter := &fakeStarter{}
	err := Launch(context.Background(), "codex", []string{"--model", "gpt"}, func(context.Context) error {
		order = append(order, "init")
		return nil
	}, starter)
	if err != nil {
		t.Fatal(err)
	}
	order = append(order, "run")
	if !reflect.DeepEqual(order, []string{"init", "run"}) {
		t.Fatalf("unexpected order %#v", order)
	}
	if len(starter.calls) != 1 || starter.calls[0].Command != "codex" || !reflect.DeepEqual(starter.calls[0].Args, []string{"--model", "gpt"}) {
		t.Fatalf("unexpected launch %#v", starter.calls)
	}
}

func TestLaunchStopsWhenInitializationFails(t *testing.T) {
	starter := &fakeStarter{}
	expected := errors.New("warm failed")
	err := Launch(context.Background(), "agy", nil, func(context.Context) error { return expected }, starter)
	if !errors.Is(err, expected) {
		t.Fatalf("unexpected error %v", err)
	}
	if len(starter.calls) != 0 {
		t.Fatal("process started despite init failure")
	}
}

func TestLaunchRejectsEmptyCommand(t *testing.T) {
	if err := Launch(context.Background(), "", nil, nil, &fakeStarter{}); err == nil {
		t.Fatal("empty session command was accepted")
	}
}

func TestLaunchPropagatesStarterFailure(t *testing.T) {
	expected := errors.New("start failed")
	err := Launch(context.Background(), "codex", nil, nil, &fakeStarter{err: expected})
	if !errors.Is(err, expected) {
		t.Fatalf("starter error was not propagated: %v", err)
	}
}

func TestExecStarterRunsManagedProcessWithEnvironment(t *testing.T) {
	output := filepath.Join(t.TempDir(), "session.txt")
	command := "sh"
	args := []string{"-c", `printf %s "$AGENTSTACK_SESSION_VALUE" > "$AGENTSTACK_SESSION_OUTPUT"`}
	if runtime.GOOS == "windows" {
		command = "powershell.exe"
		args = []string{"-NoProfile", "-NonInteractive", "-Command", `Set-Content -LiteralPath $env:AGENTSTACK_SESSION_OUTPUT -Value $env:AGENTSTACK_SESSION_VALUE -NoNewline`}
	}
	err := (ExecStarter{}).Run(context.Background(), StartRequest{
		Command: command,
		Args:    args,
		Env: map[string]string{
			"AGENTSTACK_SESSION_OUTPUT": output,
			"AGENTSTACK_SESSION_VALUE":  "ok",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "ok" {
		t.Fatalf("session process did not receive environment: data=%q err=%v", data, err)
	}
}

func TestExecStarterReportsStartFailure(t *testing.T) {
	err := (ExecStarter{}).Run(context.Background(), StartRequest{Command: "agentstack-command-that-does-not-exist"})
	if err == nil || !strings.Contains(err.Error(), "start agentstack-command-that-does-not-exist") {
		t.Fatalf("Run() error = %v, want start failure", err)
	}
}

func TestExecStarterReportsProcessFailure(t *testing.T) {
	command := "sh"
	args := []string{"-c", "exit 7"}
	if runtime.GOOS == "windows" {
		command = "powershell.exe"
		args = []string{"-NoProfile", "-NonInteractive", "-Command", "exit 7"}
	}
	err := (ExecStarter{}).Run(context.Background(), StartRequest{Command: command, Args: args})
	if err == nil || !strings.Contains(err.Error(), "run "+command) {
		t.Fatalf("Run() error = %v, want process failure", err)
	}
}
