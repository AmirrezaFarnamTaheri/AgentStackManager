package mcplink

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/runner"
)

func TestAgyClientPathFailsClosedWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	} else {
		t.Setenv("HOME", "")
	}
	manager := New(t.TempDir(), Options{ProjectRoot: t.TempDir(), Executable: "agentstack", RouterConfig: "router.json"})
	if _, err := manager.clientPath(ClientAgy); err == nil || !strings.Contains(err.Error(), "user home") {
		t.Fatalf("expected missing home error, got %v", err)
	}
}

type fakeCommands struct {
	calls  []runner.Invocation
	result runner.Result
}

func (f *fakeCommands) Run(_ context.Context, invocation runner.Invocation) runner.Result {
	f.calls = append(f.calls, invocation)
	if len(invocation.Args) >= 2 && invocation.Args[0] == "mcp" && invocation.Args[1] == "get" {
		return f.result
	}
	return runner.Result{ExitCode: 0}
}

func TestPlanAndApplyLinksFileClientsPreservingForeignServers(t *testing.T) {
	project := t.TempDir()
	cursorPath := filepath.Join(project, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cursorPath, []byte(`{"mcpServers":{"foreign":{"command":"foreign"}},"other":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "C:/AgentStack/agentstack.exe", RouterConfig: "C:/AgentStack/router.json"})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientCursor, ClientClaude}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 2 {
		t.Fatalf("unexpected operations: %#v", plan.Operations)
	}
	report, err := manager.Apply(context.Background(), plan.ID, plan.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Applied) != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	data, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "foreign") || !strings.Contains(text, "agentstack-router") {
		t.Fatalf("unexpected cursor config: %s", text)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); err != nil {
		t.Fatal(err)
	}
}

func TestPlanReportsForeignNamedConflict(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"agentstack-router":{"command":"other","args":["serve"]}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientClaude}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Operations[0].Action != ActionConflict {
		t.Fatalf("expected conflict: %#v", plan.Operations[0])
	}
	if _, err := manager.Apply(context.Background(), plan.ID, plan.Digest, true); err == nil {
		t.Fatal("expected conflict failure")
	}
}

func TestCodexRegistrationUsesReviewedPlan(t *testing.T) {
	commands := &fakeCommands{result: runner.Result{ExitCode: 1, Stderr: "not found"}}
	manager := New(t.TempDir(), Options{ProjectRoot: t.TempDir(), Executable: "agentstack", RouterConfig: "router.json", Commands: commands})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientCodex}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Apply(context.Background(), plan.ID, "sha256:wrong", true); err == nil {
		t.Fatal("expected digest mismatch")
	}
	if _, err := manager.Apply(context.Background(), plan.ID, plan.Digest, true); err != nil {
		t.Fatal(err)
	}
	if len(commands.calls) < 2 {
		t.Fatalf("expected inspect and add calls: %#v", commands.calls)
	}
}

func TestUnlinkRemovesOnlyAgentStackEntry(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := `{"mcpServers":{"foreign":{"command":"foreign"},"agentstack-router":{"command":"agentstack","args":["mcp-router","--config","router.json"]}}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	plan, err := manager.Plan(ModeUnlink, []ClientKind{ClientCursor}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Apply(context.Background(), plan.ID, plan.Digest, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "foreign") || strings.Contains(text, "agentstack-router") {
		t.Fatalf("unexpected unlink: %s", text)
	}
}

func TestApplyRollsBackEarlierClientWhenLaterClientFails(t *testing.T) {
	project := t.TempDir()
	cursorPath := filepath.Join(project, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(cursorPath), 0o700); err != nil {
		t.Fatal(err)
	}
	originalCursor := []byte(`{"mcpServers":{"foreign":{"command":"foreign"}}}`)
	if err := os.WriteFile(cursorPath, originalCursor, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientClaude, ClientCursor}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	manager.beforeApply = func(Operation) error {
		calls++
		if calls == 2 {
			return fmt.Errorf("simulated second client failure")
		}
		return nil
	}
	report, err := manager.Apply(context.Background(), plan.ID, plan.Digest, true)
	if err == nil {
		t.Fatal("expected transactional client-link failure")
	}
	if !report.RolledBack {
		t.Fatalf("rollback was not reported: %#v", report)
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf("earlier client config remained after rollback: %v", err)
	}
	currentCursor, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentCursor) != string(originalCursor) {
		t.Fatalf("untouched client config changed: %s", currentCursor)
	}
}

func TestPlanRejectsOversizedClientConfiguration(t *testing.T) {
	root := t.TempDir()
	manager := New(t.TempDir(), Options{ProjectRoot: root, Executable: "agentstack", RouterConfig: "router.json", Commands: &fakeCommands{}})
	path := filepath.Join(root, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxMCPClientConfigBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Plan(ModeLink, []ClientKind{ClientCursor}, time.Minute); err == nil {
		t.Fatal("oversized MCP client configuration was accepted")
	}
}

func TestPlansAndBackupsDoNotPersistUnrelatedClientSecrets(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".cursor", "mcp.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	secret := "unrelated-client-secret-value"
	config := fmt.Sprintf(`{"mcpServers":{"foreign":{"command":"foreign","env":{"ACCESS_TOKEN":%q}}},"privateSetting":%q}`, secret, secret)
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	stateRoot := t.TempDir()
	manager := New(stateRoot, Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientCursor}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	planData, err := os.ReadFile(filepath.Join(stateRoot, "plans", plan.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(planData), secret) {
		t.Fatalf("reviewed plan duplicated unrelated client secret: %s", planData)
	}
	report, err := manager.Apply(context.Background(), plan.ID, plan.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Backups) != 1 {
		t.Fatalf("expected one minimal backup record: %#v", report)
	}
	backupData, err := os.ReadFile(report.Backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(backupData), secret) {
		t.Fatalf("backup duplicated unrelated client secret: %s", backupData)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), secret) || !strings.Contains(string(updated), "agentstack-router") {
		t.Fatalf("apply did not preserve foreign configuration while adding ASM: %s", updated)
	}
}

func TestPlanRejectsDuplicateClientConfigurationKeys(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".mcp.json")
	config := `{"mcpServers":{},"mcpServers":{"foreign":{"command":"foreign"}}}`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	if _, err := manager.Plan(ModeLink, []ClientKind{ClientClaude}, time.Hour); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate-key MCP configuration was accepted: %v", err)
	}
}

func TestPlanPreservesSecretBearingNamedEntryAsForeignConflict(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".mcp.json")
	config := `{"mcpServers":{"agentstack-router":{"command":"agentstack","args":["mcp-router","--config","router.json"],"env":{"TOKEN":"must-not-copy"}}}}`
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientClaude}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Operations[0].Action != ActionConflict {
		t.Fatalf("secret-bearing named entry should be preserved as foreign: %#v", plan.Operations[0])
	}
}

func TestPlanBindsCapabilitySnapshotsAndLossReports(t *testing.T) {
	project := t.TempDir()
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientClaude, ClientCursor}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdapterContract != adapters.ContractVersion || len(plan.CapabilitySnapshots) != 2 || len(plan.LossReports) != 2 {
		t.Fatalf("adapter evidence missing: %#v", plan)
	}
	for _, capability := range plan.CapabilitySnapshots {
		if err := adapters.VerifyCapabilitySet(capability); err != nil {
			t.Fatal(err)
		}
	}
	for _, report := range plan.LossReports {
		if err := adapters.VerifyLossReport(report); err != nil {
			t.Fatal(err)
		}
		if report.Fidelity != adapters.FidelityFull || report.HasLosses() {
			t.Fatalf("MCP registration should be full fidelity: %#v", report)
		}
	}
	if err := manager.verifyAdapterPlan(plan); err != nil {
		t.Fatal(err)
	}
}

func TestApplyRejectsTamperedAdapterLossBinding(t *testing.T) {
	project := t.TempDir()
	manager := New(t.TempDir(), Options{ProjectRoot: project, Executable: "agentstack", RouterConfig: "router.json"})
	plan, err := manager.Plan(ModeLink, []ClientKind{ClientClaude}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	plan.Operations[0].Losses = []adapters.Loss{{ArtifactID: "local/MCPServer/agentstack-router", Field: "/registration", Kind: adapters.LossTransformation, Code: "tampered", Reason: "not reviewed"}}
	plan.Operations[0].Fidelity = adapters.FidelityPartial
	plan.Digest, err = planDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(manager.Root, "plans", plan.ID+".json"), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Apply(context.Background(), plan.ID, plan.Digest, true); err == nil || !strings.Contains(err.Error(), "loss report mismatch") {
		t.Fatalf("tampered adapter loss binding was accepted: %v", err)
	}
}
