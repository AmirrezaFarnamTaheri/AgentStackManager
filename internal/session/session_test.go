package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestSessionExecHelperProcess(t *testing.T) {
	if os.Getenv("AGENTSTACK_SESSION_HELPER") != "1" {
		return
	}
	if path := os.Getenv("AGENTSTACK_SESSION_OUTPUT"); path != "" {
		_ = os.WriteFile(path, []byte(os.Getenv("AGENTSTACK_SESSION_VALUE")), 0o600)
	}
}

func TestExecStarterRunsManagedProcessWithEnvironment(t *testing.T) {
	output := filepath.Join(t.TempDir(), "session.txt")
	err := (ExecStarter{}).Run(context.Background(), StartRequest{
		Command: os.Args[0], Args: []string{"-test.run=^TestSessionExecHelperProcess$"},
		Env: map[string]string{"AGENTSTACK_SESSION_HELPER": "1", "AGENTSTACK_SESSION_OUTPUT": output, "AGENTSTACK_SESSION_VALUE": "ok"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "ok" {
		t.Fatalf("session process did not receive environment: data=%q err=%v", data, err)
	}
}
