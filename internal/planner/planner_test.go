package planner

import (
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func testCatalog() model.Catalog {
	return model.Catalog{
		Version: 1,
		Components: []model.Component{
			{ID: "git", Name: "Git", Tier: model.TierEssential, DetectCommands: []string{"git"}, Install: model.InstallSpec{Kind: model.InstallWinget, Package: "Git.Git"}},
			{ID: "playwright-mcp", Name: "Playwright", Tier: model.TierEssential, Capability: "browser", Preferred: true, Install: model.InstallSpec{Kind: model.InstallRouter}},
			{ID: "puppeteer-mcp", Name: "Puppeteer", Tier: model.TierOptionalLocal, Capability: "browser", Install: model.InstallSpec{Kind: model.InstallRouter}},
			{ID: "github-mcp", Name: "GitHub MCP", Tier: model.TierCredential, CredentialRequired: true, Install: model.InstallSpec{Kind: model.InstallRouter}},
		},
		Profiles: []model.Profile{{ID: "essential", Components: []string{"git", "playwright-mcp"}}},
	}
}

func TestPlanKeepsExistingTool(t *testing.T) {
	inv := model.Inventory{Items: map[string]model.InventoryItem{"git": {ComponentID: "git", Installed: true, DetectedCommand: "git"}}}
	result, err := Build(testCatalog(), inv, Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	action := result.ActionFor("git")
	if action.Kind != model.ActionKeep {
		t.Fatalf("expected keep, got %s", action.Kind)
	}
}

func TestPlanPreservesDominatedInstalledProviderInactive(t *testing.T) {
	inv := model.Inventory{Items: map[string]model.InventoryItem{"puppeteer-mcp": {ComponentID: "puppeteer-mcp", Installed: true}}}
	result, err := Build(testCatalog(), inv, Request{Profile: "essential", Include: []string{"puppeteer-mcp"}})
	if err != nil {
		t.Fatal(err)
	}
	action := result.ActionFor("puppeteer-mcp")
	if action.Kind != model.ActionPreserveInactive {
		t.Fatalf("expected preserve-inactive, got %s", action.Kind)
	}
}

func TestPlanRequiresExplicitCredentialConsent(t *testing.T) {
	result, err := Build(testCatalog(), model.Inventory{Items: map[string]model.InventoryItem{}}, Request{Profile: "essential", Include: []string{"github-mcp"}})
	if err != nil {
		t.Fatal(err)
	}
	action := result.ActionFor("github-mcp")
	if action.Kind != model.ActionConsentRequired {
		t.Fatalf("expected consent-required, got %s", action.Kind)
	}
}

func TestProviderOverrideCanActivateNonDefaultProvider(t *testing.T) {
	inv := model.Inventory{Items: map[string]model.InventoryItem{"puppeteer-mcp": {ComponentID: "puppeteer-mcp", Installed: true}}}
	result, err := Build(testCatalog(), inv, Request{
		Profile:           "essential",
		Include:           []string{"puppeteer-mcp"},
		ProviderOverrides: map[string]string{"browser": "puppeteer-mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.ActionFor("puppeteer-mcp").Kind; got != model.ActionKeep {
		t.Fatalf("expected override provider to be kept active, got %s", got)
	}
	if got := result.ActionFor("playwright-mcp").Kind; got != model.ActionPreserveInactive && got != model.ActionSkipDominated {
		t.Fatalf("expected default provider inactive/skipped, got %s", got)
	}
}

func TestBuildExpandsAndOrdersDependencies(t *testing.T) {
	c := model.Catalog{
		Version: 1,
		Components: []model.Component{
			{ID: "ast-grep", Name: "ast-grep", Tier: model.TierEssential, DependsOn: []string{"node"}, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "@ast-grep/cli"}},
			{ID: "node", Name: "Node.js", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "OpenJS.NodeJS.LTS"}},
		},
		Profiles: []model.Profile{{ID: "custom"}},
	}
	plan, err := Build(c, model.Inventory{Items: map[string]model.InventoryItem{}}, Request{Profile: "custom", Include: []string{"ast-grep"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("expected dependency plus requested component, got %#v", plan.Actions)
	}
	if plan.Actions[0].ComponentID != "node" || plan.Actions[1].ComponentID != "ast-grep" {
		t.Fatalf("dependencies must precede dependents, got %#v", plan.Actions)
	}
}

func TestBuildRejectsExcludedDependency(t *testing.T) {
	c := model.Catalog{
		Version: 1,
		Components: []model.Component{
			{ID: "ast-grep", Name: "ast-grep", Tier: model.TierEssential, DependsOn: []string{"node"}},
			{ID: "node", Name: "Node.js", Tier: model.TierEssential},
		},
		Profiles: []model.Profile{{ID: "custom"}},
	}
	_, err := Build(c, model.Inventory{Items: map[string]model.InventoryItem{}}, Request{Profile: "custom", Include: []string{"ast-grep"}, Exclude: []string{"node"}})
	if err == nil || !strings.Contains(err.Error(), "depends on excluded") {
		t.Fatalf("expected excluded dependency error, got %v", err)
	}
}

func TestPlanRepairsBrokenManagedComponent(t *testing.T) {
	c := model.Catalog{
		Version:    1,
		Components: []model.Component{{ID: "gitleaks", Name: "Gitleaks", Tier: model.TierEssential, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Gitleaks.Gitleaks"}}},
		Profiles:   []model.Profile{{ID: "essential", Components: []string{"gitleaks"}}},
	}
	inv := model.Inventory{Items: map[string]model.InventoryItem{"gitleaks": {ComponentID: "gitleaks", Managed: true, Broken: true}}}
	plan, err := Build(c, inv, Request{Profile: "essential"})
	if err != nil {
		t.Fatal(err)
	}
	if action := plan.ActionFor("gitleaks"); action.Kind != model.ActionRepair {
		t.Fatalf("expected repair action, got %#v", action)
	}
}

func TestExcludingPreferredProviderActivatesOnlySelectedAlternative(t *testing.T) {
	c := testCatalog()
	c.Profiles = []model.Profile{{ID: "full", Components: []string{"playwright-mcp", "puppeteer-mcp"}}}
	plan, err := Build(c, model.Inventory{Items: map[string]model.InventoryItem{}}, Request{
		Profile: "full",
		Exclude: []string{"playwright-mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Providers["browser"]; got != "puppeteer-mcp" {
		t.Fatalf("expected selected alternative to become active, got %q", got)
	}
	if got := plan.ActionFor("puppeteer-mcp").Kind; got != model.ActionConfigure {
		t.Fatalf("expected alternative provider configured, got %s", got)
	}
}

func TestManualCredentialIntegrationSurfacesLoginHint(t *testing.T) {
	c := model.Catalog{
		Version: 1,
		Components: []model.Component{{
			ID: "figma", Name: "Figma", Tier: model.TierCredential, CredentialRequired: true,
			Install: model.InstallSpec{Kind: model.InstallManual, LoginHint: "Connect with Figma OAuth."},
		}},
		Profiles: []model.Profile{{ID: "custom"}},
	}
	plan, err := Build(c, model.Inventory{Items: map[string]model.InventoryItem{}}, Request{
		Profile: "custom", Include: []string{"figma"}, AllowCredentialed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	action := plan.ActionFor("figma")
	if action.Kind != model.ActionSkip || !strings.Contains(action.Reason, "Connect with Figma OAuth") {
		t.Fatalf("expected actionable login hint, got %#v", action)
	}
}
