package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/catalog"
	"github.com/agentstack/agentstack/internal/model"
)

func TestBuildRouterConfigIncludesOnlyActiveRouterActions(t *testing.T) {
	c, err := catalog.LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	plan := model.Plan{Profile: "essential", Actions: []model.PlanAction{
		{ComponentID: "memory-mcp", Kind: model.ActionConfigure},
		{ComponentID: "playwright-mcp", Kind: model.ActionConfigure},
		{ComponentID: "puppeteer-mcp", Kind: model.ActionPreserveInactive},
		{ComponentID: "github-mcp", Kind: model.ActionConsentRequired},
	}}
	config, err := BuildRouterConfig(c, plan, `C:\Users\Test\AppData\Local\AgentStack`)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Servers) != 2 {
		t.Fatalf("expected two servers, got %d", len(config.Servers))
	}
	memory := config.Servers["memory-mcp"]
	if got := memory.Env["MEMORY_FILE_PATH"]; got != `C:\Users\Test\AppData\Local\AgentStack/memory/knowledge-graph.jsonl` && got != `C:\Users\Test\AppData\Local\AgentStack\memory\knowledge-graph.jsonl` {
		t.Fatalf("data path not expanded: %q", got)
	}
	if _, exists := config.Servers["puppeteer-mcp"]; exists {
		t.Fatal("inactive duplicate included")
	}
}

func TestValidateRouterConfigRejectsIdleTTLOverflow(t *testing.T) {
	config := RouterConfig{Version: 1, Servers: map[string]ServerConfig{
		"overflow": {Command: "server", IdleTTLSeconds: int(^uint(0) >> 1)},
	}}
	if err := validateRouterConfig(config); err == nil || !strings.Contains(err.Error(), "idle TTL") {
		t.Fatalf("expected oversized idle TTL rejection, got %v", err)
	}
}

func TestMCPIdleTTLUsesFallbackForUnvalidatedValues(t *testing.T) {
	fallback := 2 * time.Minute
	if got := mcpIdleTTL(int(^uint(0)>>1), fallback); got != fallback {
		t.Fatalf("overflowing idle TTL should use fallback, got %s", got)
	}
	if got := mcpIdleTTL(60, fallback); got != time.Minute {
		t.Fatalf("valid idle TTL should be converted, got %s", got)
	}
}

func TestWriteRouterConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.json")
	input := RouterConfig{Version: 1, Profile: "essential", Servers: map[string]ServerConfig{"memory": {Command: "npx", Args: []string{"server"}}}}
	if err := WriteRouterConfig(path, input); err != nil {
		t.Fatal(err)
	}
	output, err := LoadRouterConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if output.Servers["memory"].Command != "npx" {
		t.Fatalf("unexpected config %#v", output)
	}
}

func TestLoadRouterConfigRejectsTrailingJSONContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"servers":{}} trailing`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRouterConfig(path); err == nil {
		t.Fatal("router config with trailing content was accepted")
	}
}

func TestLoadRouterConfigRejectsInvalidServerCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"servers":{"bad":{"command":""}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRouterConfig(path); err == nil || !strings.Contains(err.Error(), "command is invalid") {
		t.Fatalf("expected invalid server command rejection, got %v", err)
	}
}

func TestMergeAgyConfigRejectsDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp_config.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{},"mcpServers":{"agentstack-router":{"command":"foreign"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeAgyConfig(path, "agentstack", []string{"mcp-router"}, t.TempDir()); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate-key rejection, got %v", err)
	}
}

func TestRouterConfigEquivalentIgnoresUpdatedTimestamp(t *testing.T) {
	left := RouterConfig{Version: 1, Profile: "essential", UpdatedAt: time.Unix(1, 0), Servers: map[string]ServerConfig{"memory": {Command: "npx", Args: []string{"server"}}}}
	right := RouterConfig{Version: 1, Profile: "essential", UpdatedAt: time.Unix(2, 0), Servers: map[string]ServerConfig{"memory": {Command: "npx", Args: []string{"server"}}}}
	if !RouterConfigEquivalent(left, right) {
		t.Fatal("timestamps should not make equivalent router profiles different")
	}
	right.Servers["memory"] = ServerConfig{Command: "npx", Args: []string{"different"}}
	if RouterConfigEquivalent(left, right) {
		t.Fatal("server changes must make router profiles different")
	}
}

func TestMergeAgyConfigPreservesUnknownKeysAndExistingServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp_config.json")
	original := `{"theme":"dark","mcpServers":{"existing":{"command":"keep"}}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := MergeAgyConfig(path, `C:\AgentStack\agentstack.exe`, []string{"mcp-router", "--config", `C:\config.json`}, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Changed || result.BackupPath == "" {
		t.Fatalf("expected change and backup: %#v", result)
	}
	data, _ := os.ReadFile(path)
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["theme"] != "dark" {
		t.Fatal("unknown key lost")
	}
	servers := parsed["mcpServers"].(map[string]any)
	if _, ok := servers["existing"]; !ok {
		t.Fatal("existing server lost")
	}
	if _, ok := servers["agentstack-router"]; !ok {
		t.Fatal("router missing")
	}
}

func TestMergeAgyConfigConflictDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp_config.json")
	original := `{"mcpServers":{"agentstack-router":{"command":"custom"}}}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := MergeAgyConfig(path, "agentstack", []string{"mcp-router"}, filepath.Join(dir, "backups"))
	if err == nil {
		t.Fatal("foreign registration conflict should be explicit")
	}
	if result.Changed || !result.Conflict {
		t.Fatalf("expected non-mutating conflict: %#v", result)
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatal("conflicting config was mutated")
	}
}

func TestBuildRouterConfigCopiesResourceLimits(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{
		ID: "limited", Name: "Limited", Install: model.InstallSpec{Kind: model.InstallRouter},
		Router: &model.RouterServerSpec{Command: "server", Limits: model.ProcessLimits{MemoryBytes: 2 << 30, CPUPercent: 80, ActiveProcesses: 32}},
	}}}
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "limited", Kind: model.ActionConfigure}}}
	config, err := BuildRouterConfig(c, plan, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got := config.Servers["limited"].Limits
	if got.MemoryBytes != 2<<30 || got.CPUPercent != 80 || got.ActiveProcesses != 32 {
		t.Fatalf("resource limits were not preserved: %#v", got)
	}
}
