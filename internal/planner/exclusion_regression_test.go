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

func TestBuildRejectsProviderOverrideToExcludedComponent(t *testing.T) {
	_, err := Build(exclusionCatalog(), model.Inventory{Items: map[string]model.InventoryItem{}}, Request{Profile: "core", Exclude: []string{"alternate"}, ProviderOverrides: map[string]string{"browser": "alternate"}})
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Fatalf("excluded provider override was accepted: %v", err)
	}
}
