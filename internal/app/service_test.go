package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/inventory"
	"github.com/agentstack/agentstack/internal/mcp"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/runner"
	"github.com/agentstack/agentstack/internal/skills"
	"github.com/agentstack/agentstack/internal/state"
)

type appLocator map[string]string

func (f appLocator) LookPath(name string) (string, error) {
	if path, ok := f[name]; ok {
		return path, nil
	}
	return "", errors.New("missing")
}

type appProbe struct{}

func (appProbe) Run(context.Context, string, ...string) inventory.CommandResult {
	return inventory.CommandResult{ExitCode: 1}
}

type appRunner struct{ calls []runner.Invocation }

func (r *appRunner) Run(_ context.Context, inv runner.Invocation) runner.Result {
	r.calls = append(r.calls, inv)
	return runner.Result{ExitCode: 0, Stdout: "ok"}
}

type appSkills struct{}

func (appSkills) Install(context.Context, model.Component) runner.Result {
	return runner.Result{ExitCode: 0}
}

func minimalService(t *testing.T, c model.Catalog, commands *appRunner) *Service {
	t.Helper()
	root := t.TempDir()
	paths := Paths{
		DataRoot:     root,
		RouterConfig: filepath.Join(root, "router", "router.json"),
		BackupRoot:   filepath.Join(root, "backups"),
		AgyConfig:    filepath.Join(root, "agy", "mcp_config.json"),
		Executable:   filepath.Join(root, "agentstack.exe"),
	}
	return &Service{
		Catalog:   c,
		Scanner:   inventory.Scanner{Locator: appLocator{}, Probe: appProbe{}},
		Store:     state.NewStore(root),
		Installer: runner.Engine{Commands: commands, Skills: appSkills{}, Catalog: c},
		Commands:  commands,
		Paths:     paths,
		LookPath:  appLocator{},
	}
}

func TestApplyPlannedRequiresExplicitConfirmation(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Name: "Git", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}, Profiles: []model.Profile{{ID: "essential", Components: []string{"git"}}}}
	commands := &appRunner{}
	service := minimalService(t, c, commands)
	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ApplyPlanned(context.Background(), plan.ID, plan.Digest, false)
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if len(commands.calls) != 0 {
		t.Fatal("command executed without confirmation")
	}
}

func TestApplyPlannedConsumesReviewedPlanBeforeExternalMutation(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Name: "Git", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}, Profiles: []model.Profile{{ID: "essential", Components: []string{"git"}}}}
	service := minimalService(t, c, &appRunner{})
	service.Installer.Commands = failingAppRunner{}
	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyPlanned(context.Background(), plan.ID, plan.Digest, true); err == nil {
		t.Fatal("expected simulated installation failure")
	}
	if _, err := service.Store.LoadPlan(plan.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reviewed plan remained replayable after mutation began: %v", err)
	}
}

func TestApplyPlannedInstallsMissingComponentAndPersistsOwnership(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Name: "Git", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}, Profiles: []model.Profile{{ID: "essential", Components: []string{"git"}}}}
	commands := &appRunner{}
	service := minimalService(t, c, commands)
	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.ApplyPlanned(context.Background(), plan.ID, plan.Digest, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Transaction.Status != model.TransactionSucceeded {
		t.Fatalf("unexpected transaction %#v", report.Transaction)
	}
	ownership, err := service.Store.LoadOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ownership.ManagedComponents["git"]; !ok {
		t.Fatal("successful install not recorded as AgentStack-owned")
	}
	if len(commands.calls) != 1 || commands.calls[0].Command != "winget" {
		t.Fatalf("unexpected calls %#v", commands.calls)
	}
}

func TestMCPInitWritesManagedConfigAndDoesNotRequireClientRegistration(t *testing.T) {
	c := model.Catalog{
		Version:    1,
		Components: []model.Component{{ID: "memory", Name: "Memory", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallRouter}, Router: &model.RouterServerSpec{Command: "npx", Args: []string{"memory"}}}},
		Profiles:   []model.Profile{{ID: "essential", Components: []string{"memory"}}},
	}
	commands := &appRunner{}
	service := minimalService(t, c, commands)
	report, err := service.MCPInit(context.Background(), MCPInitOptions{Request: planner.Request{Profile: "essential"}, RegisterClients: false, Warm: false, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.RouterConfigPath != service.Paths.RouterConfig {
		t.Fatalf("unexpected path %s", report.RouterConfigPath)
	}
	if !report.RouterChanged {
		t.Fatal("first initialization should create the router config")
	}
	if _, err := os.Stat(service.Paths.RouterConfig); err != nil {
		t.Fatal(err)
	}
	ownership, _ := service.Store.LoadOwnership()
	if _, ok := ownership.ManagedComponents["memory"]; !ok {
		t.Fatal("configured router component not recorded")
	}
}

func TestMCPInitRemovesTransientPlanAfterConfiguration(t *testing.T) {
	c := model.Catalog{
		Version:    1,
		Components: []model.Component{{ID: "memory", Name: "Memory", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallRouter}, Router: &model.RouterServerSpec{Command: "npx", Args: []string{"memory"}}}},
		Profiles:   []model.Profile{{ID: "essential", Components: []string{"memory"}}},
	}
	service := minimalService(t, c, &appRunner{})
	if _, err := service.MCPInit(context.Background(), MCPInitOptions{Request: planner.Request{Profile: "essential"}, Confirm: true}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(service.Paths.DataRoot, "plans"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("transient MCP plan was retained: %#v", entries)
	}
}

func TestMCPInitDoesNotRewriteOrBackupEquivalentConfig(t *testing.T) {
	c := model.Catalog{
		Version:    1,
		Components: []model.Component{{ID: "memory", Name: "Memory", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallRouter}, Router: &model.RouterServerSpec{Command: "npx", Args: []string{"memory"}}}},
		Profiles:   []model.Profile{{ID: "essential", Components: []string{"memory"}}},
	}
	service := minimalService(t, c, &appRunner{})
	first, err := service.MCPInit(context.Background(), MCPInitOptions{Request: planner.Request{Profile: "essential"}, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(service.Paths.RouterConfig)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.MCPInit(context.Background(), MCPInitOptions{Request: planner.Request{Profile: "essential"}, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	infoAfter, err := os.Stat(service.Paths.RouterConfig)
	if err != nil {
		t.Fatal(err)
	}
	if !first.RouterChanged || second.RouterChanged {
		t.Fatalf("unexpected change reports: first=%v second=%v", first.RouterChanged, second.RouterChanged)
	}
	if second.BackupPath != "" {
		t.Fatalf("equivalent config should not be backed up: %s", second.BackupPath)
	}
	if !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		t.Fatal("equivalent router config was rewritten")
	}
}

func TestMCPInitRequiresExplicitConfirmation(t *testing.T) {
	c := model.Catalog{
		Version:    1,
		Components: []model.Component{{ID: "memory", Name: "Memory", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallRouter}, Router: &model.RouterServerSpec{Command: "npx", Args: []string{"memory"}}}},
		Profiles:   []model.Profile{{ID: "essential", Components: []string{"memory"}}},
	}
	service := minimalService(t, c, &appRunner{})
	_, err := service.MCPInit(context.Background(), MCPInitOptions{Request: planner.Request{Profile: "essential"}})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("expected confirmation error, got %v", err)
	}
	if _, statErr := os.Stat(service.Paths.RouterConfig); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("router config was mutated without confirmation: %v", statErr)
	}
}

func TestInventoryMarksMissingOwnedSkillPathsRepairable(t *testing.T) {
	c := model.Catalog{
		Version: 1,
		Components: []model.Component{{
			ID: "superpowers", Name: "Superpowers", Tier: model.TierEssential,
			Install: model.InstallSpec{Kind: model.InstallSkillPack, Repository: "https://example.invalid/skills.git"},
		}},
		Profiles: []model.Profile{{ID: "essential", Components: []string{"superpowers"}}},
	}
	service := minimalService(t, c, &appRunner{})
	missing := filepath.Join(service.Paths.DataRoot, "missing-skill")
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"superpowers": {ID: "superpowers", Source: "agentstack", Paths: []string{missing}},
	}}); err != nil {
		t.Fatal(err)
	}
	current, err := service.Inventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := current.Items["superpowers"]
	if !item.Broken || item.Installed {
		t.Fatalf("missing AgentStack-owned skill path should be repairable, got %#v", item)
	}
}

type reportingSkills struct{ path string }

func (r reportingSkills) Install(context.Context, model.Component) runner.Result {
	payload, _ := json.Marshal(skills.Report{Added: []string{r.path}})
	return runner.Result{ExitCode: 0, Stdout: string(payload)}
}

func TestApplyRecordsOnlySkillPathsAgentStackAdded(t *testing.T) {
	c := model.Catalog{
		Version: 1,
		Components: []model.Component{{
			ID: "superpowers", Name: "Superpowers", Tier: model.TierEssential,
			Install: model.InstallSpec{Kind: model.InstallSkillPack, Repository: "https://example.invalid/skills.git"},
		}},
		Profiles: []model.Profile{{ID: "essential", Components: []string{"superpowers"}}},
	}
	service := minimalService(t, c, &appRunner{})
	added := filepath.Join(service.Paths.DataRoot, "owned-skill")
	service.Installer.Skills = reportingSkills{path: added}
	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyPlanned(context.Background(), plan.ID, plan.Digest, true); err != nil {
		t.Fatal(err)
	}
	ownership, err := service.Store.LoadOwnership()
	if err != nil {
		t.Fatal(err)
	}
	paths := ownership.ManagedComponents["superpowers"].Paths
	if len(paths) != 1 || paths[0] != added {
		t.Fatalf("expected added skill path ownership, got %#v", paths)
	}
}

type failingAppRunner struct{}

func (failingAppRunner) Run(_ context.Context, inv runner.Invocation) runner.Result {
	return runner.Result{ExitCode: 1, Err: errors.New("simulated install failure"), Stderr: inv.Command + " failed"}
}

func TestApplyDoesNotConfigureRouterAfterInstallFailure(t *testing.T) {
	c := model.Catalog{
		Version: 1,
		Components: []model.Component{
			{ID: "node", Name: "Node", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "OpenJS.NodeJS.LTS"}},
			{
				ID: "memory", Name: "Memory", Tier: model.TierEssential,
				Install:   model.InstallSpec{Kind: model.InstallRouter},
				Router:    &model.RouterServerSpec{Command: "npx", Args: []string{"memory"}},
				DependsOn: []string{"node"},
			},
		},
		Profiles: []model.Profile{{ID: "essential", Components: []string{"memory"}}},
	}
	service := minimalService(t, c, &appRunner{})
	failure := failingAppRunner{}
	service.Installer.Commands = failure
	service.Commands = failure

	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	report, err := service.ApplyPlanned(context.Background(), plan.ID, plan.Digest, true)
	if err == nil {
		t.Fatal("expected failed installation")
	}
	if report.Transaction.Status != model.TransactionFailed {
		t.Fatalf("expected failed transaction, got %s", report.Transaction.Status)
	}
	if report.Router != nil {
		t.Fatal("failed install phase must not configure the managed router")
	}
	if _, statErr := os.Stat(service.Paths.RouterConfig); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("router config should remain absent after failed install, stat error=%v", statErr)
	}
}

func TestApplyPlannedRejectsDigestMismatch(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Name: "Git", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}, Profiles: []model.Profile{{ID: "essential", Components: []string{"git"}}}}
	commands := &appRunner{}
	service := minimalService(t, c, commands)
	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyPlanned(context.Background(), plan.ID, "sha256:tampered", true); !errors.Is(err, ErrPlanMismatch) {
		t.Fatalf("expected plan mismatch, got %v", err)
	}
	if len(commands.calls) != 0 {
		t.Fatal("tampered plan executed commands")
	}
}

func TestApplyPlannedRejectsInventoryChange(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Name: "Git", Tier: model.TierEssential, DetectCommands: []string{"git"}, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}, Profiles: []model.Profile{{ID: "essential", Components: []string{"git"}}}}
	commands := &appRunner{}
	service := minimalService(t, c, commands)
	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	service.Scanner.Locator = appLocator{"git": filepath.Join(service.Paths.DataRoot, "git.exe")}
	if _, err := service.ApplyPlanned(context.Background(), plan.ID, plan.Digest, true); !errors.Is(err, ErrPlanStale) {
		t.Fatalf("expected stale plan, got %v", err)
	}
	if len(commands.calls) != 0 {
		t.Fatal("stale plan executed commands")
	}
}

func TestApplyPlannedRejectsExpiredPlan(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Name: "Git", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}, Profiles: []model.Profile{{ID: "essential", Components: []string{"git"}}}}
	service := minimalService(t, c, &appRunner{})
	service.PlanTTL = time.Millisecond
	plan, err := service.Plan(context.Background(), planner.Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := service.ApplyPlanned(context.Background(), plan.ID, plan.Digest, true); !errors.Is(err, ErrPlanStale) {
		t.Fatalf("expected expired plan rejection, got %v", err)
	}
}

func TestServiceMCPHelperProcess(t *testing.T) {
	mode := os.Getenv("AGENTSTACK_APP_MCP_HELPER")
	if mode == "" {
		return
	}
	if countFile := os.Getenv("AGENTSTACK_APP_MCP_COUNT"); countFile != "" {
		file, _ := os.OpenFile(countFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if file != nil {
			_, _ = file.WriteString("start\n")
			_ = file.Close()
		}
	}
	if mode == "broken" {
		os.Exit(42)
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
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "test", "version": "1"}}})
		case "tools/list":
			_ = encoder.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{"tools": []any{map[string]any{"name": "ok", "inputSchema": map[string]any{"type": "object"}}}}})
		}
	}
}

func TestMCPDoctorProbesChildAndCachesRecentResult(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	service.DoctorTTL = time.Minute
	countFile := filepath.Join(t.TempDir(), "starts.txt")
	config := mcp.RouterConfig{Version: 1, Servers: map[string]mcp.ServerConfig{
		"healthy": {Command: os.Args[0], Args: []string{"-test.run=^TestServiceMCPHelperProcess$"}, Env: map[string]string{"AGENTSTACK_APP_MCP_HELPER": "healthy", "AGENTSTACK_APP_MCP_COUNT": countFile}},
	}}
	if err := mcp.WriteRouterConfig(service.Paths.RouterConfig, config); err != nil {
		t.Fatal(err)
	}
	first, err := service.MCPDoctor(context.Background())
	if err != nil || !first.Healthy || first.Cached || first.HealthyCount != 1 {
		t.Fatalf("unexpected first doctor report: %+v err=%v", first, err)
	}
	second, err := service.MCPDoctor(context.Background())
	if err != nil || !second.Cached {
		t.Fatalf("expected cached doctor report: %+v err=%v", second, err)
	}
	data, err := os.ReadFile(countFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "start\n" {
		t.Fatalf("doctor started child more than once: %q", data)
	}
}

func TestMCPDoctorReportsInitializationFailure(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	config := mcp.RouterConfig{Version: 1, Servers: map[string]mcp.ServerConfig{
		"broken": {Command: os.Args[0], Args: []string{"-test.run=^TestServiceMCPHelperProcess$"}, Env: map[string]string{"AGENTSTACK_APP_MCP_HELPER": "broken"}},
	}}
	if err := mcp.WriteRouterConfig(service.Paths.RouterConfig, config); err != nil {
		t.Fatal(err)
	}
	report, err := service.MCPDoctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy || report.UnhealthyCount != 1 || report.Servers["broken"].Status == "ok" {
		t.Fatalf("broken child was reported healthy: %+v", report)
	}
}

func TestRestoreBackupValidatesRouterConfiguration(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	original := mcp.RouterConfig{Version: 1, Servers: map[string]mcp.ServerConfig{
		"one": {Command: os.Args[0], Args: []string{"-test.run=^TestServiceMCPHelperProcess$"}, Env: map[string]string{"AGENTSTACK_APP_MCP_HELPER": "healthy"}},
	}}
	if err := mcp.WriteRouterConfig(service.Paths.RouterConfig, original); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Store.BackupFile(service.Paths.RouterConfig, "router"); err != nil {
		t.Fatal(err)
	}
	backups, err := service.Store.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %+v err=%v", backups, err)
	}
	if err := os.WriteFile(service.Paths.RouterConfig, []byte(`{"broken":`), 0o600); err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewRestore(backups[0].ID, "")
	if err != nil || !preview.DigestVerified || !preview.StructuralValidated {
		t.Fatalf("restore preview failed: %+v err=%v", preview, err)
	}
	report, err := service.RestoreBackup(context.Background(), backups[0].ID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Validated || !report.LiveValidated || report.Doctor == nil || report.Backup.ID != backups[0].ID {
		t.Fatalf("restore was not validated: %+v", report)
	}
	loaded, err := mcp.LoadRouterConfig(service.Paths.RouterConfig)
	if err != nil || len(loaded.Servers) != 1 {
		t.Fatalf("restored router invalid: %+v err=%v", loaded, err)
	}
}

func TestRestoreBackupRollsBackWhenLiveRouterValidationFails(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	broken := mcp.RouterConfig{Version: 1, Servers: map[string]mcp.ServerConfig{
		"broken": {Command: os.Args[0], Args: []string{"-test.run=^TestServiceMCPHelperProcess$"}, Env: map[string]string{"AGENTSTACK_APP_MCP_HELPER": "broken"}},
	}}
	if err := mcp.WriteRouterConfig(service.Paths.RouterConfig, broken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Store.BackupFile(service.Paths.RouterConfig, "router"); err != nil {
		t.Fatal(err)
	}
	backups, err := service.Store.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %+v err=%v", backups, err)
	}
	healthy := mcp.RouterConfig{Version: 1, Servers: map[string]mcp.ServerConfig{
		"healthy": {Command: os.Args[0], Args: []string{"-test.run=^TestServiceMCPHelperProcess$"}, Env: map[string]string{"AGENTSTACK_APP_MCP_HELPER": "healthy"}},
	}}
	if err := mcp.WriteRouterConfig(service.Paths.RouterConfig, healthy); err != nil {
		t.Fatal(err)
	}
	report, err := service.RestoreBackup(context.Background(), backups[0].ID, "", true)
	if err == nil {
		t.Fatal("expected live validation failure")
	}
	if !report.RolledBack {
		t.Fatalf("failed restore did not roll back: %+v", report)
	}
	loaded, loadErr := mcp.LoadRouterConfig(service.Paths.RouterConfig)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if _, ok := loaded.Servers["healthy"]; !ok || len(loaded.Servers) != 1 {
		t.Fatalf("pre-restore router was not recovered: %+v", loaded)
	}
}

func TestInventoryReconcilesOwnershipHealth(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", DetectCommands: []string{"git"}, Install: model.InstallSpec{Kind: model.InstallWinget}}}}
	service := minimalService(t, catalog, &appRunner{})
	service.Scanner = inventory.Scanner{Locator: appLocator{"git": filepath.Join(t.TempDir(), "git")}, Probe: appProbe{}}
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{"git": {ID: "git", Source: "agentstack", Healthy: false}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Inventory(context.Background()); err != nil {
		t.Fatal(err)
	}
	ownership, err := service.Store.LoadOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if !ownership.ManagedComponents["git"].Healthy || ownership.ManagedComponents["git"].LastVerified.IsZero() {
		t.Fatalf("ownership health not reconciled: %+v", ownership.ManagedComponents["git"])
	}
}

func TestGuidedIntegrationsExposeOfficialNextStepWithoutSecretStorage(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{{ID: "provider", Name: "Provider", CredentialRequired: true, GuidedSetup: true, Install: model.InstallSpec{Kind: model.InstallManual, LoginHint: "provider login", DocumentationURL: "https://provider.example/setup"}}}}
	service := minimalService(t, catalog, &appRunner{})
	items, err := service.GuidedIntegrations(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].NextStep != "provider login" || items[0].DocumentationURL != "https://provider.example/setup" || items[0].AgentStoresSecret {
		t.Fatalf("unexpected guided integration: %+v", items)
	}
}
