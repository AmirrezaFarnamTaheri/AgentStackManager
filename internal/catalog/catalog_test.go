package catalog

import (
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func TestValidateRejectsDuplicateComponentIDs(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git"}, {ID: "git"}}}
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "duplicate component id") {
		t.Fatalf("expected duplicate component error, got %v", err)
	}
}

func TestValidateRejectsUnknownProfileComponent(t *testing.T) {
	c := model.Catalog{
		Version:    1,
		Components: []model.Component{{ID: "git"}},
		Profiles:   []model.Profile{{ID: "essential", Components: []string{"missing"}}},
	}
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "unknown component") {
		t.Fatalf("expected unknown component error, got %v", err)
	}
}

func TestLoadDefaultCatalogIsValid(t *testing.T) {
	c, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if len(c.Components) < 30 {
		t.Fatalf("expected rich catalog, got %d components", len(c.Components))
	}
	if _, ok := c.ComponentByID("playwright-mcp"); !ok {
		t.Fatal("playwright-mcp missing")
	}
	if _, ok := c.ProfileByID("essential"); !ok {
		t.Fatal("essential profile missing")
	}
}

func TestDecodeRejectsTrailingJSONContent(t *testing.T) {
	_, err := decode([]byte(`{"version":1,"components":[],"profiles":[]} {}`))
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("expected trailing content error, got %v", err)
	}
}

func TestValidateRejectsUnknownDependency(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "tool", DependsOn: []string{"missing"}}}}
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "unknown dependency") {
		t.Fatalf("expected unknown dependency error, got %v", err)
	}
}

func TestValidateRejectsDependencyCycle(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "a", DependsOn: []string{"b"}},
		{ID: "b", DependsOn: []string{"a"}},
	}}
	err := Validate(c)
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected dependency cycle error, got %v", err)
	}
}

func TestDefaultAutomaticPackagesDoNotFloatOnLatest(t *testing.T) {
	c, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range c.Components {
		values := []string{component.Install.Package}
		if component.Router != nil {
			values = append(values, component.Router.Args...)
			values = append(values, warmArgs(component)...)
		}
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), "@latest") {
				t.Fatalf("component %s contains floating version value %q", component.ID, value)
			}
		}
	}
}

func warmArgs(component model.Component) []string {
	if component.Router == nil || component.Router.Warm == nil {
		return nil
	}
	return component.Router.Warm.Args
}

func TestValidateRejectsUnlockedAutomaticPackages(t *testing.T) {
	tests := []model.Component{
		{ID: "winget", Tier: model.TierEssential, Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Vendor.Tool"}},
		{ID: "uv", Tier: model.TierEssential, Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallUVTool, Package: "ruff"}},
		{ID: "npm", Tier: model.TierEssential, Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "tool@latest"}},
		{ID: "skills", Tier: model.TierEssential, Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallSkillPack, Repository: "https://example.test/repo.git"}},
	}
	for _, component := range tests {
		if err := Validate(model.Catalog{Version: 3, Components: []model.Component{component}}); err == nil {
			t.Fatalf("component %s should fail supply-chain validation", component.ID)
		}
	}
}

func TestDefaultCatalogHasMinimalCoreAndLockedAutomaticAcquisitions(t *testing.T) {
	c, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	core, ok := c.ProfileByID("core")
	if !ok || len(core.Components) == 0 || len(core.Components) > 8 {
		t.Fatalf("core profile should remain minimal: %#v", core)
	}
	for _, component := range c.Components {
		if component.Tier == "" {
			continue
		}
		if err := validateSupplyChainLock(component); err != nil {
			t.Fatalf("component %s is not locked: %v", component.ID, err)
		}
		if component.CredentialRequired && (!strings.HasPrefix(component.Install.DocumentationURL, "https://") || component.Install.LoginHint == "") {
			t.Fatalf("credential component %s lacks guided setup metadata", component.ID)
		}
	}
}

func TestDefaultRouterComponentsDeclareHardResourceLimits(t *testing.T) {
	c, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range c.Components {
		if component.Install.Kind != model.InstallRouter || component.Router == nil {
			continue
		}
		limits := component.Router.Limits
		if limits.MemoryBytes == 0 || limits.CPUPercent == 0 || limits.ActiveProcesses == 0 {
			t.Fatalf("router component %s lacks a complete hard resource ceiling: %#v", component.ID, limits)
		}
	}
}

func TestValidateRejectsMultiplePreferredProvidersForCapability(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "first", Capability: "search", Preferred: true},
		{ID: "second", Capability: "search", Preferred: true},
	}}
	err := Validate(catalog)
	if err == nil || !strings.Contains(err.Error(), "multiple preferred providers") {
		t.Fatalf("expected preferred-provider conflict, got %v", err)
	}
}

func TestValidateAllowsPreferredComponentsWithoutCapability(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "first", Preferred: true},
		{ID: "second", Preferred: true},
	}}
	if err := Validate(catalog); err != nil {
		t.Fatalf("capability-free preferred components should not collide: %v", err)
	}
}

func TestDefaultCatalogUsesCurrentVerifiedWinGetPins(t *testing.T) {
	catalog, err := LoadDefault()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]model.InstallSpec{
		"yq":    {Kind: model.InstallWinget, WingetID: "MikeFarah.yq", Source: "winget", Version: "4.53.2", Publisher: "Mike Farah"},
		"trivy": {Kind: model.InstallWinget, WingetID: "AquaSecurity.Trivy", Source: "winget", Version: "0.70.0", Publisher: "Aqua Security"},
		"scc":   {Kind: model.InstallWinget, WingetID: "BenBoyter.scc", Source: "winget", Version: "3.7.0", Publisher: "Ben Boyter"},
	}
	for id, expected := range want {
		component, ok := catalog.ComponentByID(id)
		if !ok {
			t.Fatalf("component %s missing", id)
		}
		if component.Install.WingetID != expected.WingetID || component.Install.Source != expected.Source || component.Install.Version != expected.Version || component.Install.Publisher != expected.Publisher {
			t.Fatalf("component %s install pin = %#v, want %#v", id, component.Install, expected)
		}
	}
}
