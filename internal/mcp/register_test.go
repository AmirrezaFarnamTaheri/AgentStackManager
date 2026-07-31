package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/runner"
)

type registerRunner struct {
	calls   []runner.Invocation
	results []runner.Result
}

func (r *registerRunner) Run(_ context.Context, inv runner.Invocation) runner.Result {
	r.calls = append(r.calls, inv)
	if len(r.results) == 0 {
		return runner.Result{}
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result
}

func TestRegisterCodexKeepsEquivalentEntry(t *testing.T) {
	commands := &registerRunner{results: []runner.Result{{ExitCode: 0, Stdout: `{"name":"agentstack-router","command":"C:\\agentstack.exe","args":["mcp-router","--config","C:\\router.json"]}`}}}
	result, err := RegisterCodex(context.Background(), commands, `C:\agentstack.exe`, `C:\router.json`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed || result.Conflict || result.Status != RegistrationEquivalent {
		t.Fatalf("unexpected result %#v", result)
	}
	if len(commands.calls) != 1 {
		t.Fatalf("expected only get call, got %d", len(commands.calls))
	}
}

func TestRegisterCodexRejectsForeignConflict(t *testing.T) {
	commands := &registerRunner{results: []runner.Result{{ExitCode: 0, Stdout: `{"name":"agentstack-router","command":"foreign.exe","args":["serve"]}`}}}
	result, err := RegisterCodex(context.Background(), commands, `C:\agentstack.exe`, `C:\router.json`)
	if err == nil || !result.Conflict || result.Status != RegistrationForeignConflict {
		t.Fatalf("expected explicit foreign conflict, result=%#v err=%v", result, err)
	}
	if len(commands.calls) != 1 {
		t.Fatalf("foreign entry must be preserved, calls=%#v", commands.calls)
	}
}

func TestRegisterCodexLookupFailureIsNotTreatedAsAbsent(t *testing.T) {
	commands := &registerRunner{results: []runner.Result{{ExitCode: 2, Err: os.ErrPermission, Stderr: "permission denied"}}}
	_, err := RegisterCodex(context.Background(), commands, `C:\agentstack.exe`, `C:\router.json`)
	if err == nil || !strings.Contains(err.Error(), "inspect") {
		t.Fatalf("lookup failure should stop registration: %v", err)
	}
	if len(commands.calls) != 1 {
		t.Fatalf("lookup failure must not trigger add: %#v", commands.calls)
	}
}

func TestRegisterCodexAddsOnlyOnExplicitAbsence(t *testing.T) {
	commands := &registerRunner{results: []runner.Result{{ExitCode: 1, Stderr: "MCP server 'agentstack-router' not found"}, {ExitCode: 0}}}
	result, err := RegisterCodex(context.Background(), commands, `C:\agentstack.exe`, `C:\router.json`)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.Status != RegistrationAdded {
		t.Fatalf("expected add %#v", result)
	}
	if len(commands.calls) != 2 {
		t.Fatalf("expected get and add, got %d", len(commands.calls))
	}
}

func TestRegisterCodexRepairsOwnedStaleEntryWithRollbackData(t *testing.T) {
	commands := &registerRunner{results: []runner.Result{
		{ExitCode: 0, Stdout: `{"name":"agentstack-router","command":"C:\\Old\\agentstack.exe","args":["mcp-router","--config","C:\\old.json"]}`},
		{ExitCode: 0},
		{ExitCode: 0},
	}}
	result, err := RegisterCodex(context.Background(), commands, `C:\New\agentstack.exe`, `C:\new.json`)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !result.Repaired || result.Status != RegistrationRepaired {
		t.Fatalf("expected owned repair %#v", result)
	}
	if len(commands.calls) != 3 || commands.calls[1].Args[1] != "remove" || commands.calls[2].Args[1] != "add" {
		t.Fatalf("unexpected repair sequence %#v", commands.calls)
	}
}

func TestMergeAgyKeepsEquivalentAndRejectsForeignConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp_config.json")
	exe := `C:\AgentStack\agentstack.exe`
	args := []string{"mcp-router", "--config", `C:\AgentStack\router.json`}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"agentstack-router":{"command":"C:\\AgentStack\\agentstack.exe","args":["mcp-router","--config","C:\\AgentStack\\router.json"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := MergeAgyConfig(path, exe, args, filepath.Join(dir, "backups"))
	if err != nil || result.Changed || result.Conflict || result.Status != RegistrationEquivalent {
		t.Fatalf("equivalent result=%#v err=%v", result, err)
	}

	if err := os.WriteFile(path, []byte(`{"mcpServers":{"agentstack-router":{"command":"foreign.exe","args":["serve"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err = MergeAgyConfig(path, exe, args, filepath.Join(dir, "backups"))
	if err == nil || !result.Conflict || result.Status != RegistrationForeignConflict {
		t.Fatalf("foreign result=%#v err=%v", result, err)
	}
}

func TestMergeAgyRepairsOwnedStaleEntryWithBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp_config.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"agentstack-router":{"command":"C:\\Old\\agentstack.exe","args":["mcp-router","--config","C:\\old.json"]}},"untouched":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := MergeAgyConfig(path, `C:\New\agentstack.exe`, []string{"mcp-router", "--config", `C:\new.json`}, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || !result.Repaired || result.BackupPath == "" || result.Status != RegistrationRepaired {
		t.Fatalf("unexpected repair %#v", result)
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"untouched": true`) || !strings.Contains(string(data), `C:\\New\\agentstack.exe`) {
		t.Fatalf("unrelated config not preserved or entry not repaired: %s", data)
	}
}
