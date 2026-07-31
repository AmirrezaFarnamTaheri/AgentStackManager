package planner

import (
	"fmt"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func BenchmarkBuildLargeCatalog(b *testing.B) {
	const size = 200
	catalog := model.Catalog{Version: 1}
	profile := model.Profile{ID: "large"}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	for index := 0; index < size; index++ {
		id := fmt.Sprintf("component-%03d", index)
		component := model.Component{ID: id}
		if index > 0 {
			component.DependsOn = []string{fmt.Sprintf("component-%03d", index-1)}
		}
		catalog.Components = append(catalog.Components, component)
		profile.Components = append(profile.Components, id)
		inventory.Items[id] = model.InventoryItem{ComponentID: id}
	}
	catalog.Profiles = []model.Profile{profile}
	request := Request{Profile: profile.ID}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := Build(catalog, inventory, request); err != nil {
			b.Fatal(err)
		}
	}
}
