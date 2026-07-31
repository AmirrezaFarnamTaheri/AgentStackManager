package planner

import (
	"github.com/agentstack/agentstack/internal/catalog"
	"github.com/agentstack/agentstack/internal/model"
	"testing"
)

func FuzzPlannerPreservation(f *testing.F) {
	f.Add("core", "", "", false, false)
	f.Add("custom", "playwright-mcp", "", false, false)
	f.Fuzz(func(t *testing.T, profile, include, exclude string, credentials, upgrades bool) {
		c, err := catalog.LoadDefault()
		if err != nil {
			t.Fatal(err)
		}
		request := Request{Profile: profile, Include: splitCSV(include), Exclude: splitCSV(exclude), AllowCredentialed: credentials, AllowUpgrades: upgrades}
		plan, err := Build(c, model.Inventory{Items: map[string]model.InventoryItem{}}, request)
		if err != nil {
			return
		}
		seen := map[string]bool{}
		for _, action := range plan.Actions {
			if seen[action.ComponentID] {
				t.Fatalf("duplicate action %q", action.ComponentID)
			}
			seen[action.ComponentID] = true
			if action.Kind == model.ActionInstall && !action.Selected {
				t.Fatalf("unselected install: %#v", action)
			}
		}
	})
}
func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	out := []string{}
	start := 0
	for i, r := range value {
		if r == ',' {
			out = append(out, value[start:i])
			start = i + 1
		}
	}
	return append(out, value[start:])
}
