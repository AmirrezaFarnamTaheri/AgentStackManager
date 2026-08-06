package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/workspace"
)

type EnvironmentOverview struct {
	GeneratedAt       time.Time         `json:"generatedAt"`
	HealthScore       int               `json:"healthScore"`
	IssueCount        int               `json:"issueCount"`
	ConnectedCount    int               `json:"connectedCount"`
	RecommendedAction string            `json:"recommendedAction,omitempty"`
	Environments      []Environment     `json:"environments"`
	Connections       []ConnectionState `json:"connections"`
}

type Environment struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	Kind              string                `json:"kind"`
	State             string                `json:"state"`
	Version           string                `json:"version,omitempty"`
	HealthScore       int                   `json:"healthScore"`
	IssueCount        int                   `json:"issueCount"`
	RecommendedAction string                `json:"recommendedAction,omitempty"`
	Resources         []EnvironmentResource `json:"resources,omitempty"`
	Message           string                `json:"message,omitempty"`
}

type EnvironmentResource struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	State      string   `json:"state"`
	Version    string   `json:"version,omitempty"`
	Message    string   `json:"message,omitempty"`
	SharedWith []string `json:"sharedWith,omitempty"`
}

type ConnectionState struct {
	ID            string `json:"id"`
	Agent         string `json:"agent"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	State         string `json:"state"`
	Mode          string `json:"mode,omitempty"`
	Scope         string `json:"scope,omitempty"`
	Label         string `json:"label,omitempty"`
	ResourceCount int    `json:"resourceCount"`
	Message       string `json:"message,omitempty"`
}

type knownEnvironment struct {
	ID           string
	Name         string
	Kind         string
	Agent        resourcehub.Agent
	SupportLevel string
	Writable     bool
	Executables  []string
	Markers      []string
}

var knownAgentEnvironments = []knownEnvironment{
	{ID: "codex", Name: "Codex", Kind: "ai-app", Agent: resourcehub.AgentCodex, SupportLevel: "verified", Writable: true, Executables: []string{"codex"}, Markers: []string{".codex"}},
	{ID: "claude", Name: "Claude Code", Kind: "ai-app", Agent: resourcehub.AgentClaude, SupportLevel: "verified", Writable: true, Executables: []string{"claude"}, Markers: []string{".claude"}},
	{ID: "agy", Name: "Gemini CLI", Kind: "ai-app", Agent: resourcehub.AgentAGY, SupportLevel: "verified", Writable: true, Executables: []string{"gemini"}, Markers: []string{".gemini"}},
	{ID: "opencode", Name: "OpenCode", Kind: "ai-app", Agent: resourcehub.AgentOpenCode, SupportLevel: "verified", Writable: true, Executables: []string{"opencode"}, Markers: []string{".opencode", ".config/opencode"}},
	{ID: "cursor", Name: "Cursor", Kind: "ide", Agent: resourcehub.AgentCursor, SupportLevel: "verified", Writable: true, Executables: []string{"cursor"}, Markers: []string{".cursor"}},
	{ID: "github-copilot", Name: "GitHub Copilot", Kind: "ide", Agent: resourcehub.AgentCopilot, SupportLevel: "verified", Writable: true, Executables: []string{"copilot"}, Markers: []string{".github", ".copilot"}},
	{ID: "vscode", Name: "Visual Studio Code", Kind: "ide", Agent: resourcehub.AgentVSCode, SupportLevel: "known", Executables: []string{"code"}, Markers: []string{".vscode"}},
	{ID: "jetbrains", Name: "JetBrains IDEs", Kind: "ide", Agent: resourcehub.AgentJetBrains, SupportLevel: "known", Executables: []string{"idea", "pycharm", "webstorm"}, Markers: []string{".idea"}},
	{ID: "windsurf", Name: "Windsurf", Kind: "ide", Agent: resourcehub.AgentWindsurf, SupportLevel: "known", Executables: []string{"windsurf"}, Markers: []string{".windsurf"}},
	{ID: "zed", Name: "Zed", Kind: "ide", Agent: resourcehub.AgentZed, SupportLevel: "known", Executables: []string{"zed"}, Markers: []string{".config/zed"}},
	{ID: "kiro", Name: "Kiro", Kind: "ide", Agent: resourcehub.AgentKiro, SupportLevel: "known", Executables: []string{"kiro"}, Markers: []string{".kiro"}},
	{ID: "trae", Name: "Trae", Kind: "ide", Agent: resourcehub.AgentTrae, SupportLevel: "known", Executables: []string{"trae"}, Markers: []string{".trae"}},
	{ID: "cline", Name: "Cline", Kind: "ide-extension", Agent: resourcehub.AgentCline, SupportLevel: "known", Markers: []string{".cline"}},
	{ID: "roo-code", Name: "Roo Code", Kind: "ide-extension", Agent: resourcehub.AgentRooCode, SupportLevel: "known", Markers: []string{".roo"}},
	{ID: "continue", Name: "Continue", Kind: "ide-extension", Agent: resourcehub.AgentContinue, SupportLevel: "known", Markers: []string{".continue"}},
	{ID: "aider", Name: "Aider", Kind: "cli", Agent: resourcehub.AgentAider, SupportLevel: "known", Executables: []string{"aider"}, Markers: []string{".aider"}},
	{ID: "goose", Name: "Goose", Kind: "cli", Agent: resourcehub.AgentGoose, SupportLevel: "known", Executables: []string{"goose"}, Markers: []string{".config/goose"}},
	{ID: "claude-desktop", Name: "Claude Desktop", Kind: "desktop-app", Agent: resourcehub.AgentClaudeDesktop, SupportLevel: "known", Markers: []string{".config/Claude", "AppData/Roaming/Claude"}},
}

func buildEnvironmentOverview(platform string, catalog model.Catalog, inventory model.Inventory, resources []resourcehub.Resource, targets []resourcehub.Target, workspaces []workspace.Item) EnvironmentOverview {
	overview := EnvironmentOverview{GeneratedAt: inventory.GeneratedAt}
	if overview.GeneratedAt.IsZero() {
		overview.GeneratedAt = time.Now().UTC()
	}
	targetsByAgent := map[resourcehub.Agent][]resourcehub.Target{}
	for _, target := range targets {
		targetsByAgent[target.Agent] = append(targetsByAgent[target.Agent], target)
	}
	for agent := range targetsByAgent {
		sort.Slice(targetsByAgent[agent], func(i, j int) bool { return targetsByAgent[agent][i].ID < targetsByAgent[agent][j].ID })
	}

	for _, known := range knownAgentEnvironments {
		agentTargets := targetsByAgent[known.Agent]
		state := "not-connected"
		message := "Known target; no writable adapter is verified yet."
		if known.Writable {
			message = "Verified adapter; not connected yet."
		}
		for _, target := range agentTargets {
			connectionState := "paused"
			connectionMessage := "Connection exists but is disabled."
			if target.Enabled {
				connectionState = "connected"
				connectionMessage = "Connected to AgentStack-managed resources."
				state = "connected"
				message = fmt.Sprintf("%d connected target%s.", enabledTargetCount(agentTargets), pluralEnvironmentSuffix(enabledTargetCount(agentTargets)))
			} else if state != "connected" {
				state = "paused"
				message = "All registered connections are paused."
			}
			environmentResources := resourcesForAgent(resources, known.Agent, connectionState)
			label := strings.TrimSpace(target.Label)
			if label == "" {
				label = target.ID
			}
			overview.Connections = append(overview.Connections, ConnectionState{
				ID: target.ID, Agent: string(known.Agent), Name: known.Name, Kind: known.Kind, State: connectionState,
				Mode: string(target.Mode), Scope: target.Scope, Label: label, ResourceCount: len(environmentResources), Message: connectionMessage,
			})
		}
		environmentResources := resourcesForAgent(resources, known.Agent, state)
		overview.Environments = append(overview.Environments, Environment{
			ID: known.ID, Name: known.Name, Kind: known.Kind, State: state,
			Resources: environmentResources, Message: message,
		})
	}

	cli := Environment{ID: "command-line", Name: "Command line tools", Kind: "cli", State: "available", Message: "Runtimes and developer tools available to local agents."}
	mcp := Environment{ID: "mcp-servers", Name: "MCP servers", Kind: "mcp", State: "available", Message: "Local tool servers that can be routed to supported environments."}
	for _, component := range catalog.Components {
		item := inventory.Items[component.ID]
		resource := EnvironmentResource{
			ID: component.ID, Name: component.Name, Type: component.Category,
			State: componentEnvironmentState(platform, component, item), Version: item.Version,
			Message: componentEnvironmentMessage(component, item),
		}
		if component.Router != nil || component.Category == "mcp" || component.Install.Kind == model.InstallRouter {
			mcp.Resources = append(mcp.Resources, resource)
			mcp.State = strongestEnvironmentState(mcp.State, resource.State)
		} else {
			cli.Resources = append(cli.Resources, resource)
			cli.State = strongestEnvironmentState(cli.State, resource.State)
		}
	}
	sortEnvironmentResources(cli.Resources)
	sortEnvironmentResources(mcp.Resources)
	overview.Environments = append(overview.Environments, cli, mcp)

	for _, item := range workspaces {
		if item.Type != workspace.TypeWorkspace {
			continue
		}
		environmentResources := make([]EnvironmentResource, 0, len(item.ResourceIDs))
		for _, id := range item.ResourceIDs {
			environmentResources = append(environmentResources, EnvironmentResource{ID: id, Name: id, Type: "workspace-resource", State: "available"})
		}
		overview.Environments = append(overview.Environments, Environment{
			ID: "workspace:" + item.ID, Name: item.Name, Kind: "workspace", State: "available",
			Resources: environmentResources, Message: "Tracked local workspace.",
		})
	}
	finalizeEnvironmentHealth(&overview)
	return overview
}

func enabledTargetCount(targets []resourcehub.Target) int {
	count := 0
	for _, target := range targets {
		if target.Enabled {
			count++
		}
	}
	return count
}

func pluralEnvironmentSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func finalizeEnvironmentHealth(overview *EnvironmentOverview) {
	if overview == nil || len(overview.Environments) == 0 {
		return
	}
	total := 0
	for index := range overview.Environments {
		environment := &overview.Environments[index]
		environment.IssueCount = 0
		for _, resource := range environment.Resources {
			if resource.State == "needs-attention" {
				environment.IssueCount++
			}
		}
		base := map[string]int{"connected": 100, "installed": 95, "shared": 95, "available": 85, "not-connected": 60, "paused": 45, "needs-attention": 50, "not-supported": 75}[environment.State]
		if base == 0 {
			base = 70
		}
		if environment.State == "not-connected" || environment.State == "paused" {
			environment.IssueCount++
		}
		environment.HealthScore = base - environment.IssueCount*12
		if environment.HealthScore < 0 {
			environment.HealthScore = 0
		}
		switch {
		case environment.IssueCount > 0 && environment.State == "needs-attention":
			environment.RecommendedAction = fmt.Sprintf("Review %d item%s that need attention.", environment.IssueCount, pluralSuffix(environment.IssueCount))
		case environment.State == "not-connected":
			environment.RecommendedAction = "Connect this environment to share reviewed AgentStack resources."
		case environment.State == "paused":
			environment.RecommendedAction = "Reconnect this environment when you are ready to resume sharing."
		case environment.State == "connected":
			environment.RecommendedAction = "No action required."
		default:
			environment.RecommendedAction = "Review optional additions when needed."
		}
		if environment.State == "connected" {
			overview.ConnectedCount++
		}
		overview.IssueCount += environment.IssueCount
		total += environment.HealthScore
	}
	overview.HealthScore = total / len(overview.Environments)
	if overview.IssueCount > 0 {
		overview.RecommendedAction = fmt.Sprintf("Review %d environment issue%s, starting with failed tools and paused connections.", overview.IssueCount, pluralSuffix(overview.IssueCount))
	} else {
		overview.RecommendedAction = "No urgent environment repairs are required."
	}
}

func resourcesForAgent(resources []resourcehub.Resource, agent resourcehub.Agent, connectionState string) []EnvironmentResource {
	result := []EnvironmentResource{}
	for _, resource := range resources {
		if !resource.Enabled || !containsAgent(resource.Targets, agent) {
			continue
		}
		state := "available"
		if connectionState == "connected" {
			state = "shared"
		}
		result = append(result, EnvironmentResource{ID: resource.ID, Name: resource.Name, Type: string(resource.Kind), State: state})
	}
	sortEnvironmentResources(result)
	return result
}

func containsAgent(values []resourcehub.Agent, expected resourcehub.Agent) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func componentEnvironmentState(platform string, component model.Component, item model.InventoryItem) string {
	if len(component.Platforms) > 0 {
		supported := false
		for _, value := range component.Platforms {
			if strings.EqualFold(value, platform) {
				supported = true
				break
			}
		}
		if !supported {
			return "not-supported"
		}
	}
	if item.Broken || item.Incompatible {
		return "needs-attention"
	}
	if item.Installed {
		return "installed"
	}
	return "available"
}

func componentEnvironmentMessage(component model.Component, item model.InventoryItem) string {
	if item.Broken || item.Incompatible {
		if strings.TrimSpace(item.HealthMessage) != "" {
			return item.HealthMessage
		}
		return "Detected but needs attention."
	}
	if item.Installed {
		return "Detected locally."
	}
	if component.CredentialRequired {
		return "Available after explicit credential setup."
	}
	return "Available to add through reviewed changes."
}

func strongestEnvironmentState(current, candidate string) string {
	priority := map[string]int{"not-supported": 0, "not-connected": 1, "available": 1, "installed": 2, "connected": 2, "paused": 2, "needs-attention": 3}
	if priority[candidate] > priority[current] {
		return candidate
	}
	return current
}

func sortEnvironmentResources(values []EnvironmentResource) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].State != values[j].State {
			return values[i].State < values[j].State
		}
		return strings.ToLower(values[i].Name) < strings.ToLower(values[j].Name)
	})
}
