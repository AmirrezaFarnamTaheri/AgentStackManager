package diagnostics

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/state"
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

func TestCreateRedactsEventMessagesAtExportBoundary(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "diagnostics.zip")
	secret := "diagnostics-bearer-secret"
	input := Input{Destination: destination, Version: "test", Events: []state.Event{{Type: "child.error", Message: "Authorization: Bearer " + secret, Fields: map[string]any{"token": "json-secret"}}}}
	if err := Create(input); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if file.Name != "events.json" {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, leaked := range []string{secret, "json-secret"} {
			if bytes.Contains(data, []byte(leaked)) {
				t.Fatalf("diagnostics events contain secret %q: %s", leaked, data)
			}
		}
		return
	}
	t.Fatal("events.json missing")
}
