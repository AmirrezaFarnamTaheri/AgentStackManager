package planner

import (
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func exclusionCatalog() model.Catalog {
	return model.Catalog{Version: 1, Components: []model.Component{
		{ID: "preferred", Capability: "browser", Preferred: true},
		{ID: "alternate", Capability: "browser"},
	}, Profiles: []model.Profile{{ID: "core", Components: []string{"preferred"}}}}
}

func TestBuildRejectsUnknownExcludedComponent(t *testing.T) {
	_, err := Build(exclusionCatalog(), model.Inventory{Items: map[string]model.InventoryItem{}}, Request{Profile: "core", Exclude: []string{"typo"}})
	if err == nil || !strings.Contains(err.Error(), "unknown excluded") {
		t.Fatalf("unknown exclusion was accepted: %v", err)
	}
}

func TestBuildRejectsIncludeExcludeConflict(t *testing.T) {
	_, err := Build(exclusionCatalog(), model.Inventory{Items: map[string]model.InventoryItem{}}, Request{Profile: "core", Include: []string{"alternate"}, Exclude: []string{"alternate"}})
	if err == nil || !strings.Contains(err.Error(), "both included and excluded") {
		t.Fatalf("include/exclude conflict was accepted: %v", err)
	}
}

func TestBuildProviderOverrideWinsOverProfileExclusion(t *testing.T) {
	plan, err := Build(exclusionCatalog(), model.Inventory{Items: map[string]model.InventoryItem{}}, Request{Profile: "core", Exclude: []string{"alternate"}, ProviderOverrides: map[string]string{"browser": "alternate"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Providers["browser"] != "alternate" {
		t.Fatalf("providers = %#v", plan.Providers)
	}
}

func TestBuildProviderOverrideSelectsProviderAndItsDependencies(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "preferred", Capability: "browser", Preferred: true},
		{ID: "alternate", Capability: "browser", DependsOn: []string{"node"}},
		{ID: "node"},
	}, Profiles: []model.Profile{{ID: "core", Components: []string{"preferred"}}}}
	plan, err := Build(catalog, model.Inventory{Items: map[string]model.InventoryItem{}}, Request{
		Profile:           "core",
		Exclude:           []string{"preferred", "alternate", "node"},
		ProviderOverrides: map[string]string{"browser": "alternate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	selected := map[string]bool{}
	for _, action := range plan.Actions {
		selected[action.ComponentID] = true
	}
	if !selected["alternate"] || !selected["node"] || selected["preferred"] {
		t.Fatalf("selected actions = %#v", selected)
	}
	if plan.Providers["browser"] != "alternate" {
		t.Fatalf("providers = %#v", plan.Providers)
	}
}
