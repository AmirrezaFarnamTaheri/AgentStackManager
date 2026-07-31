package diagnostics

import (
	"archive/zip"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func TestCreateSanitizesExecutablePaths(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "diagnostics.zip")
	inventory := model.Inventory{Items: map[string]model.InventoryItem{"git": {ComponentID: "git", ExecutablePath: `C:\\Users\\alice\\bin\\git.exe`}}}
	if err := Create(Input{Destination: destination, Version: "test", CatalogDigest: "abc", Inventory: inventory}); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name == "inventory.json" {
			r, _ := file.Open()
			defer r.Close()
			var got model.Inventory
			if err := json.NewDecoder(r).Decode(&got); err != nil {
				t.Fatal(err)
			}
			path := got.Items["git"].ExecutablePath
			if path == inventory.Items["git"].ExecutablePath || path == "" {
				t.Fatalf("path not sanitized: %q", path)
			}
			return
		}
	}
	t.Fatal("inventory missing")
}
