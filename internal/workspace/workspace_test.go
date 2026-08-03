package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWorkspacePromptMemoryAndArtifacts(t *testing.T) {
	manager := New(t.TempDir())
	ws, err := manager.Create(Item{ID: "asm", Name: "ASM", Type: TypeWorkspace, Root: t.TempDir(), Prompt: "Project ${workspace.name} at ${workspace.root}; goal=${goal}"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remember(MemoryEntry{Layer: LayerWorkspace, Scope: ws.ID, Key: "goal", Value: "ship safely", Tags: []string{"release"}}); err != nil {
		t.Fatal(err)
	}
	memory, err := manager.Recall(ws.ID, "goal", "")
	if err != nil {
		t.Fatal(err)
	}
	if memory.Value != "ship safely" {
		t.Fatalf("unexpected memory: %#v", memory)
	}
	prompt, err := manager.RenderPrompt(ws.ID, map[string]string{"goal": memory.Value}, time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Project ASM") || !strings.Contains(prompt, "ship safely") {
		t.Fatalf("unexpected prompt: %s", prompt)
	}

	file := filepath.Join(t.TempDir(), "report.md")
	if err := os.WriteFile(file, []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact, err := manager.AddArtifact(ws.ID, file, ArtifactOptions{ID: "report", Name: "Report", MediaType: "text/markdown"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := manager.VerifyArtifact(artifact.ID); err != nil || !ok {
		t.Fatalf("verify artifact: ok=%t err=%v", ok, err)
	}
}

func TestMemoryPrecedenceSearchExpiryAndForget(t *testing.T) {
	now := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	manager := New(t.TempDir())
	manager.Clock = func() time.Time { return now }
	if _, err := manager.Remember(MemoryEntry{Layer: LayerUser, Key: "editor", Value: "vim"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remember(MemoryEntry{Layer: LayerWorkspace, Scope: "w", Key: "editor", Value: "vscode"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remember(MemoryEntry{Layer: LayerSession, Scope: "s", Key: "editor", Value: "zed", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	entry, err := manager.Recall("w", "editor", "s")
	if err != nil || entry.Value != "zed" {
		t.Fatalf("unexpected precedence: %#v %v", entry, err)
	}
	results, err := manager.SearchMemory("release", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("unexpected search results: %#v", results)
	}
	now = now.Add(2 * time.Minute)
	entry, err = manager.Recall("w", "editor", "s")
	if err != nil || entry.Value != "vscode" {
		t.Fatalf("expired session should fall back: %#v %v", entry, err)
	}
	if err := manager.Forget(LayerWorkspace, "w", "editor"); err != nil {
		t.Fatal(err)
	}
	entry, err = manager.Recall("w", "editor", "")
	if err != nil || entry.Value != "vim" {
		t.Fatalf("workspace forget should fall back to user: %#v %v", entry, err)
	}
}

func TestScopedMemoryRequiresExplicitScope(t *testing.T) {
	manager := New(t.TempDir())
	for _, layer := range []MemoryLayer{LayerProject, LayerWorkspace, LayerSession} {
		if _, err := manager.Remember(MemoryEntry{Layer: layer, Key: "decision", Value: "keep"}); err == nil || !strings.Contains(err.Error(), "requires a scope") {
			t.Fatalf("layer %s accepted implicit scope: %v", layer, err)
		}
	}
	if _, err := manager.Remember(MemoryEntry{Layer: LayerUser, Key: "decision", Value: "keep"}); err != nil {
		t.Fatalf("user memory should not require scope: %v", err)
	}
}

func TestRecursiveFolderDeleteRemovesChildren(t *testing.T) {
	manager := New(t.TempDir())
	if _, err := manager.Create(Item{ID: "folder", Name: "Folder", Type: TypeFolder}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(Item{ID: "child", Name: "Child", Type: TypeWorkspace, ParentID: "folder", Root: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Delete("folder"); err != nil {
		t.Fatal(err)
	}
	items, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected recursive delete: %#v", items)
	}
}

func TestWorkspaceAssociationsPersistAndCyclesAreRejected(t *testing.T) {
	manager := New(t.TempDir())
	folder, err := manager.Create(Item{ID: "folder", Name: "Folder", Type: TypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	workspaceItem, err := manager.Create(Item{ID: "project", Name: "Project", Type: TypeWorkspace, ParentID: folder.ID, Root: t.TempDir(), ResourceIDs: []string{"skill-a", "skill-a", "rule-b"}, RoutineIDs: []string{"daily"}})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := manager.Get(workspaceItem.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ResourceIDs) != 2 || len(loaded.RoutineIDs) != 1 {
		t.Fatalf("associations were not normalized and persisted: %#v", loaded)
	}
	folder.ParentID = workspaceItem.ID
	if _, err := manager.Update(folder); err == nil {
		t.Fatal("expected folder cycle rejection")
	}
}

func TestRenderPromptRejectsUnknownVariable(t *testing.T) {
	manager := New(t.TempDir())
	if _, err := manager.Create(Item{ID: "w", Name: "W", Type: TypeWorkspace, Root: t.TempDir(), Prompt: "${missing}"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RenderPrompt("w", nil, time.Now()); err == nil {
		t.Fatal("expected unresolved variable error")
	}
}

func TestLegacyWorkspaceStoresMigrateToVersionedEnvelopes(t *testing.T) {
	root := t.TempDir()
	workspaceRoot := t.TempDir()
	legacyWorkspace := `{"legacy":{"id":"legacy","name":"Legacy","type":"workspace","root":` + strconv.Quote(workspaceRoot) + `,"createdAt":"2026-08-03T00:00:00Z","updatedAt":"2026-08-03T00:00:00Z"}}`
	legacyMemory := `{"user\u0000\u0000editor":{"id":"user\u0000\u0000editor","layer":"user","key":"editor","value":"vim","digest":"sha256:0f2ed9e33d29ff4f3b0f664ca1e1dc3df1f8b9b315b2af284c6e0e3dc52be290","createdAt":"2026-08-03T00:00:00Z","updatedAt":"2026-08-03T00:00:00Z"}}`
	legacyArtifacts := `{}`
	for name, contents := range map[string]string{
		"workspaces.json": legacyWorkspace,
		"memory.json":     legacyMemory,
		"artifacts.json":  legacyArtifacts,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	manager := New(root)
	if _, err := manager.Get("legacy"); err != nil {
		t.Fatalf("legacy workspace should load: %v", err)
	}
	if _, err := manager.Recall("", "editor", ""); err != nil {
		t.Fatalf("legacy memory should load: %v", err)
	}
	if _, err := manager.Create(Item{ID: "new", Name: "New", Type: TypeWorkspace, Root: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remember(MemoryEntry{Layer: LayerUser, Key: "shell", Value: "pwsh"}); err != nil {
		t.Fatal(err)
	}
	artifactSource := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(artifactSource, []byte("artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddArtifact("new", artifactSource, ArtifactOptions{ID: "artifact"}); err != nil {
		t.Fatal(err)
	}

	for name, schema := range map[string]string{
		"workspaces.json": workspaceItemsSchema,
		"memory.json":     workspaceMemorySchema,
		"artifacts.json":  workspaceArtifactsSchema,
	} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"schema": "`+schema+`"`) || !strings.Contains(string(data), `"version": 1`) {
			t.Fatalf("%s was not migrated to a versioned envelope: %s", name, data)
		}
	}
}

func TestWorkspaceStoreRejectsUnsupportedSchemaVersion(t *testing.T) {
	root := t.TempDir()
	data := `{"schema":"agentstack.workspace.items","version":99,"items":{}}`
	if err := os.WriteFile(filepath.Join(root, "workspaces.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).List(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported schema version error, got %v", err)
	}
}

func TestArtifactReplacementRollsBackWhenRegistryCommitFails(t *testing.T) {
	manager := New(t.TempDir())
	workspaceRoot := t.TempDir()
	if _, err := manager.Create(Item{ID: "w", Name: "W", Type: TypeWorkspace, Root: workspaceRoot}); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(t.TempDir(), "first.txt")
	second := filepath.Join(t.TempDir(), "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := manager.AddArtifact("w", first, ArtifactOptions{ID: "artifact"})
	if err != nil {
		t.Fatal(err)
	}
	manager.beforeSave = func(path string) error {
		if filepath.Base(path) == "artifacts.json" {
			return fmt.Errorf("simulated artifact registry failure")
		}
		return nil
	}
	if _, err := manager.AddArtifact("w", second, ArtifactOptions{ID: "artifact", Replace: true}); err == nil {
		t.Fatal("expected replacement commit failure")
	}
	manager.beforeSave = nil
	stored, err := manager.loadArtifacts()
	if err != nil {
		t.Fatal(err)
	}
	if stored["artifact"].SHA256 != original.SHA256 {
		t.Fatalf("original registry entry was not restored: %#v", stored["artifact"])
	}
	data, err := os.ReadFile(original.Path)
	if err != nil || string(data) != "first" {
		t.Fatalf("original artifact content was not restored: data=%q err=%v", data, err)
	}
}

func TestArtifactRemovalRollsBackWhenRegistryCommitFails(t *testing.T) {
	manager := New(t.TempDir())
	if _, err := manager.Create(Item{ID: "w", Name: "W", Type: TypeWorkspace, Root: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(source, []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact, err := manager.AddArtifact("w", source, ArtifactOptions{ID: "artifact"})
	if err != nil {
		t.Fatal(err)
	}
	manager.beforeSave = func(path string) error { return fmt.Errorf("simulated commit failure") }
	if err := manager.RemoveArtifact("artifact"); err == nil {
		t.Fatal("expected removal commit failure")
	}
	manager.beforeSave = nil
	if ok, err := manager.VerifyArtifact("artifact"); err != nil || !ok {
		t.Fatalf("artifact was not restored after failed removal: ok=%t err=%v", ok, err)
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		t.Fatalf("artifact file was not restored: %v", err)
	}
}

func TestWorkspaceDeleteRollsBackAllStoresAndArtifacts(t *testing.T) {
	manager := New(t.TempDir())
	if _, err := manager.Create(Item{ID: "folder", Name: "Folder", Type: TypeFolder}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Create(Item{ID: "child", Name: "Child", Type: TypeWorkspace, ParentID: "folder", Root: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Remember(MemoryEntry{Layer: LayerWorkspace, Scope: "child", Key: "decision", Value: "keep"}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "artifact.txt")
	if err := os.WriteFile(source, []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact, err := manager.AddArtifact("child", source, ArtifactOptions{ID: "artifact"})
	if err != nil {
		t.Fatal(err)
	}
	manager.beforeSave = func(path string) error {
		if filepath.Base(path) == "memory.json" {
			return fmt.Errorf("simulated second-store failure")
		}
		return nil
	}
	if err := manager.Delete("folder"); err == nil {
		t.Fatal("expected transactional delete failure")
	}
	manager.beforeSave = nil
	if _, err := manager.Get("folder"); err != nil {
		t.Fatalf("folder state was not restored: %v", err)
	}
	if _, err := manager.Get("child"); err != nil {
		t.Fatalf("child state was not restored: %v", err)
	}
	if entry, err := manager.Recall("child", "decision", ""); err != nil || entry.Value != "keep" {
		t.Fatalf("workspace memory was not restored: %#v err=%v", entry, err)
	}
	if ok, err := manager.VerifyArtifact("artifact"); err != nil || !ok {
		t.Fatalf("artifact registry/content was not restored: ok=%t err=%v", ok, err)
	}
	if _, err := os.Stat(artifact.Path); err != nil {
		t.Fatalf("artifact directory was not restored: %v", err)
	}
}

func TestWorkspaceReadersFenceAndRecoverStaleTransactions(t *testing.T) {
	manager := New(t.TempDir())
	root := t.TempDir()
	created, err := manager.Create(Item{ID: "recoverable", Name: "Recoverable", Type: TypeWorkspace, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(manager.Root, "workspaces.json")
	emptyData, err := encodeCollection(workspaceItemsSchema, map[string]Item{})
	if err != nil {
		t.Fatal(err)
	}
	journal, _, err := manager.beginTransaction([]collectionUpdate{{path: path, data: emptyData}})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeBytesAtomic(path, emptyData); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.List(); !errors.Is(err, ErrWorkspaceTransactionInProgress) {
		t.Fatalf("active transaction should fence readers, got %v", err)
	}
	pointer := transactionPointer{
		Schema: workspaceTransactionSchema, Version: workspaceTransactionVersion,
		ID: journal.ID, CreatedAt: time.Now().UTC().Add(-workspaceTransactionStaleAfter - time.Minute),
	}
	if err := writeJSONAtomic(manager.transactionPointerPath(), pointer); err != nil {
		t.Fatal(err)
	}
	items, err := manager.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("stale transaction did not restore the prior snapshot: %#v", items)
	}
	if _, err := os.Stat(manager.transactionPointerPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovered transaction pointer remains: %v", err)
	}
}

func TestCommittedArtifactMutationIsNotReportedFailedWhenCleanupIsDeferred(t *testing.T) {
	manager := New(t.TempDir())
	if _, err := manager.Create(Item{ID: "w", Name: "W", Type: TypeWorkspace, Root: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(t.TempDir(), "first.txt")
	second := filepath.Join(t.TempDir(), "second.txt")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddArtifact("w", first, ArtifactOptions{ID: "artifact"}); err != nil {
		t.Fatal(err)
	}
	manager.cleanup = func(string) error { return fmt.Errorf("simulated deferred cleanup") }
	replaced, err := manager.AddArtifact("w", second, ArtifactOptions{ID: "artifact", Replace: true})
	if err != nil {
		t.Fatalf("committed replacement was reported as failed because cleanup was deferred: %v", err)
	}
	data, err := os.ReadFile(replaced.Path)
	if err != nil || string(data) != "second" {
		t.Fatalf("replacement did not commit: data=%q err=%v", data, err)
	}
	if err := manager.RemoveArtifact("artifact"); err != nil {
		t.Fatalf("committed removal was reported as failed because cleanup was deferred: %v", err)
	}
	if _, err := manager.VerifyArtifact("artifact"); err == nil {
		t.Fatal("removed artifact remained authoritative")
	}
}

func TestWorkspacePersistenceRejectsRelativeRoot(t *testing.T) {
	root := t.TempDir()
	data := `{"schema":"agentstack.workspace.items","version":1,"items":{"bad":{"id":"bad","name":"Bad","type":"workspace","root":"relative","createdAt":"2026-08-03T00:00:00Z","updatedAt":"2026-08-03T00:00:00Z"}}}`
	if err := os.WriteFile(filepath.Join(root, "workspaces.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).List(); err == nil || !strings.Contains(err.Error(), "root must be absolute") {
		t.Fatalf("expected relative persisted root rejection, got %v", err)
	}
}
