package state

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func TestAppendEventRedactsSecretsAndRotatesBoundedLog(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AppendEvent(Event{Type: "test", Fields: map[string]any{"token": "secret", "count": 2}}); err != nil {
		t.Fatal(err)
	}
	events, err := store.RecentEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Fields["token"] != "[REDACTED]" {
		t.Fatalf("events=%#v", events)
	}
}

func TestExportDataExcludesEphemeralLocksAndPlans(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.ensure(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "state", "inventory.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "locks", "mutation.lock"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "export.zip")
	if err := store.ExportData(destination); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	names := map[string]bool{}
	for _, f := range archive.File {
		names[f.Name] = true
	}
	if !names["state/inventory.json"] || !names["EXPORT-MANIFEST.json"] || names["locks/mutation.lock"] {
		t.Fatalf("names=%#v", names)
	}
}

func TestClearDataScopesPreserveOwnershipUnlessAll(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SaveOwnership(Ownership{ManagedComponents: map[string]ManagedComponent{"x": {ID: "x"}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveInventory(model.Inventory{Items: map[string]model.InventoryItem{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClearData(ClearOperational); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadOwnership(); err != nil {
		t.Fatalf("ownership should remain: %v", err)
	}
	if _, err := store.ClearData(ClearAll); err != nil {
		t.Fatal(err)
	}
	ownership, err := store.LoadOwnership()
	if err != nil || len(ownership.ManagedComponents) != 0 {
		t.Fatalf("ownership=%#v err=%v", ownership, err)
	}
}

func TestAppendEventRedactsNestedSecrets(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.AppendEvent(Event{Type: "test", Fields: map[string]any{
		"nested": map[string]any{"authorization": "Bearer abc", "note": "token=not-a-key-name ghp_example"},
		"list":   []any{map[string]any{"password": "secret"}},
	}}); err != nil {
		t.Fatal(err)
	}
	events, err := store.RecentEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	nested := events[0].Fields["nested"].(map[string]any)
	if nested["authorization"] != "[REDACTED]" || nested["note"] != "[REDACTED]" {
		t.Fatalf("nested secrets were not redacted: %#v", events[0].Fields)
	}
}
