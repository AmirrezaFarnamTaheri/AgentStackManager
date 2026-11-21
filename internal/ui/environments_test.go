package ui

import (
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/workspace"
)

func TestBuildEnvironmentOverviewGroupsToolsTargetsAndWorkspaces(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "node", Name: "Node.js", Category: "foundation", Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallWinget}},
		{ID: "git", Name: "Git", Category: "foundation", Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallWinget}},
		{ID: "memory-mcp", Name: "Memory MCP", Category: "mcp", Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallRouter}, Router: &model.RouterServerSpec{Command: "npx"}},
		{ID: "linux-only", Name: "Linux Only", Category: "foundation", Platforms: []string{"linux"}, Install: model.InstallSpec{Kind: model.InstallManual}},
	}}
	inventory := model.Inventory{GeneratedAt: time.Now().UTC(), Items: map[string]model.InventoryItem{
		"node": {ComponentID: "node", Installed: true, Version: "24.0.0"},
		"git":  {ComponentID: "git", Installed: true, Broken: true, HealthMessage: "repair required"},
	}}
	resources := []resourcehub.Resource{{ID: "rules", Name: "Shared rules", Kind: resourcehub.KindRule, Enabled: true, Targets: []resourcehub.Agent{resourcehub.AgentCodex}}}
	targets := []resourcehub.Target{{ID: "codex-user", Agent: resourcehub.AgentCodex, Enabled: true, Mode: resourcehub.ModeCopy}}
	workspaces := []workspace.Item{{ID: "workspace-1", Name: "AgentStack", Type: workspace.TypeWorkspace, ResourceIDs: []string{"rules"}}}

	overview := buildEnvironmentOverview("windows", catalog, inventory, resources, targets, workspaces)
	byID := map[string]Environment{}
	for _, environment := range overview.Environments {
		byID[environment.ID] = environment
	}
	if byID["codex"].State != "connected" || len(byID["codex"].Resources) != 1 {
		t.Fatalf("codex environment = %#v", byID["codex"])
	}
	if byID["command-line"].State != "needs-attention" {
		t.Fatalf("command line state = %#v", byID["command-line"])
	}
	if byID["mcp-servers"].Kind != "mcp" || len(byID["mcp-servers"].Resources) != 1 {
		t.Fatalf("mcp environment = %#v", byID["mcp-servers"])
	}
	if byID["workspace:workspace-1"].Kind != "workspace" {
		t.Fatalf("workspace environment = %#v", byID["workspace:workspace-1"])
	}
	states := map[string]string{}
	for _, resource := range byID["command-line"].Resources {
		states[resource.ID] = resource.State
	}
	if states["node"] != "installed" || states["git"] != "needs-attention" || states["linux-only"] != "not-supported" {
		t.Fatalf("command line resources = %#v", states)
	}
	if len(overview.Connections) != 1 || overview.Connections[0].State != "connected" || overview.Connections[0].ResourceCount != 1 {
		t.Fatalf("connections = %#v", overview.Connections)
	}
}

func TestBuildEnvironmentOverviewDistinguishesSupportedFromConnected(t *testing.T) {
	overview := buildEnvironmentOverview("windows", model.Catalog{}, model.Inventory{Items: map[string]model.InventoryItem{}}, nil, nil, nil)
	byID := map[string]Environment{}
	for _, environment := range overview.Environments {
		byID[environment.ID] = environment
	}
	if byID["codex"].State != "not-connected" || byID["codex"].Message != "Verified adapter; not connected yet." {
		t.Fatalf("codex = %#v", byID["codex"])
	}
}

func TestBuildEnvironmentOverviewCalculatesDeterministicHealth(t *testing.T) {
	catalog := model.Catalog{Components: []model.Component{
		{ID: "broken", Name: "Broken", Category: "cli"},
		{ID: "ready", Name: "Ready", Category: "cli"},
	}}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{
		"broken": {ComponentID: "broken", Installed: true, Broken: true, HealthMessage: "probe failed"},
		"ready":  {ComponentID: "ready", Installed: true},
	}}
	overview := buildEnvironmentOverview("windows", catalog, inventory, nil, []resourcehub.Target{{ID: "codex", Agent: resourcehub.AgentCodex, Root: t.TempDir(), Mode: resourcehub.ModeCopy, Enabled: true}}, nil)
	if overview.HealthScore <= 0 || overview.HealthScore > 100 || overview.IssueCount == 0 {
		t.Fatalf("overview health=%#v", overview)
	}
	byID := map[string]Environment{}
	for _, environment := range overview.Environments {
		byID[environment.ID] = environment
	}
	if byID["codex"].HealthScore != 100 || byID["command-line"].IssueCount != 1 || byID["command-line"].RecommendedAction == "" {
		t.Fatalf("environment health=%#v", byID)
	}
}

func TestBuildEnvironmentOverviewPreservesMultipleConnectionsPerAgent(t *testing.T) {
	targets := []resourcehub.Target{
		{ID: "codex-personal", Agent: resourcehub.AgentCodex, Enabled: true, Mode: resourcehub.ModeCopy, Scope: "global", Label: "Personal"},
		{ID: "codex-project", Agent: resourcehub.AgentCodex, Enabled: false, Mode: resourcehub.ModeCopy, Scope: "project", Label: "Project"},
	}
	overview := buildEnvironmentOverview("windows", model.Catalog{}, model.Inventory{Items: map[string]model.InventoryItem{}}, nil, targets, nil)
	var codexConnections []ConnectionState
	for _, connection := range overview.Connections {
		if connection.Agent == string(resourcehub.AgentCodex) {
			codexConnections = append(codexConnections, connection)
		}
	}
	if len(codexConnections) != 2 {
		t.Fatalf("connections=%#v", overview.Connections)
	}
	if codexConnections[0].ID == codexConnections[1].ID {
		t.Fatalf("connections collapsed=%#v", codexConnections)
	}
}
