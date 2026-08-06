package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const (
	maxMemoryEntries         = 4096
	maxMemoryValue           = 64 << 10
	maxArtifactBytes         = 100 << 20
	maxWorkspaceStateBytes   = 64 << 20
	maxWorkspaceItems        = 4096
	maxWorkspaceNameBytes    = 64 << 10
	maxWorkspacePromptBytes  = 1 << 20
	maxWorkspaceVars         = 1024
	maxWorkspaceLinks        = 4096
	maxWorkspaceVarKeyBytes  = 1024
	maxWorkspaceVarValue     = 64 << 10
	maxMemoryKeyBytes        = 16 << 10
	maxMemoryTags            = 1024
	maxArtifactEntries       = 4096
	maxArtifactMetadata      = 1024
	workspaceStoreVersion    = 1
	workspaceItemsSchema     = "agentstack.workspace.items"
	workspaceMemorySchema    = "agentstack.workspace.memory"
	workspaceArtifactsSchema = "agentstack.workspace.artifacts"
)

type collectionEnvelope[T any] struct {
	Schema  string       `json:"schema"`
	Version int          `json:"version"`
	Items   map[string]T `json:"items"`
}

type Manager struct {
	Root       string
	Clock      func() time.Time
	beforeSave func(string) error
	cleanup    func(string) error
}

func New(root string) Manager {
	return Manager{Root: root, Clock: func() time.Time { return time.Now().UTC() }}
}
func (m Manager) now() time.Time {
	if m.Clock == nil {
		return time.Now().UTC()
	}
	return m.Clock().UTC()
}
func (m Manager) ensure() error {
	for _, rel := range []string{"artifacts", "backups"} {
		if err := os.MkdirAll(filepath.Join(m.Root, rel), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (m Manager) Create(item Item) (Item, error) {
	if !validID(item.ID) {
		return Item{}, fmt.Errorf("workspace id is empty or invalid")
	}
	if strings.TrimSpace(item.Name) == "" {
		return Item{}, fmt.Errorf("workspace name is required")
	}
	if item.Type != TypeWorkspace && item.Type != TypeFolder {
		return Item{}, fmt.Errorf("unsupported workspace type %q", item.Type)
	}
	if item.ParentID != "" && !validID(item.ParentID) {
		return Item{}, fmt.Errorf("parent id is invalid")
	}
	items, err := m.loadItems()
	if err != nil {
		return Item{}, err
	}
	if _, exists := items[item.ID]; exists {
		return Item{}, fmt.Errorf("workspace %q already exists", item.ID)
	}
	if item.ParentID != "" {
		parent, ok := items[item.ParentID]
		if !ok || parent.Type != TypeFolder {
			return Item{}, fmt.Errorf("parent folder %q does not exist", item.ParentID)
		}
	}
	if item.Type == TypeWorkspace {
		root, err := filepath.Abs(item.Root)
		if err != nil {
			return Item{}, err
		}
		info, err := os.Stat(root)
		if err != nil {
			return Item{}, err
		}
		if !info.IsDir() {
			return Item{}, fmt.Errorf("workspace root is not a directory")
		}
		item.Root = root
	} else {
		item.Root = ""
		item.Prompt = ""
		item.Vars = nil
	}
	now := m.now()
	item.CreatedAt = now
	item.UpdatedAt = now
	item.Vars = cloneMap(item.Vars)
	item.ResourceIDs = uniqueStrings(item.ResourceIDs)
	item.RoutineIDs = uniqueStrings(item.RoutineIDs)
	items[item.ID] = item
	if err := m.saveItems(items); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (m Manager) Update(item Item) (Item, error) {
	items, err := m.loadItems()
	if err != nil {
		return Item{}, err
	}
	previous, ok := items[item.ID]
	if !ok {
		return Item{}, fmt.Errorf("unknown workspace %q", item.ID)
	}
	if item.Type == "" {
		item.Type = previous.Type
	}
	if item.Type != previous.Type {
		return Item{}, fmt.Errorf("workspace type cannot change")
	}
	if strings.TrimSpace(item.Name) == "" {
		item.Name = previous.Name
	}
	if item.ParentID != "" {
		parent, ok := items[item.ParentID]
		if !ok || parent.Type != TypeFolder {
			return Item{}, fmt.Errorf("parent folder %q does not exist", item.ParentID)
		}
		if createsCycle(items, item.ID, item.ParentID) {
			return Item{}, fmt.Errorf("workspace parent would create a cycle")
		}
	}
	if item.Type == TypeWorkspace {
		if item.Root == "" {
			item.Root = previous.Root
		}
		root, err := filepath.Abs(item.Root)
		if err != nil {
			return Item{}, err
		}
		info, err := os.Stat(root)
		if err != nil {
			return Item{}, err
		}
		if !info.IsDir() {
			return Item{}, fmt.Errorf("workspace root is not a directory")
		}
		item.Root = root
	} else {
		item.Root = ""
		item.Prompt = ""
		item.Vars = nil
	}
	item.CreatedAt = previous.CreatedAt
	item.UpdatedAt = m.now()
	item.Vars = cloneMap(item.Vars)
	item.ResourceIDs = uniqueStrings(item.ResourceIDs)
	item.RoutineIDs = uniqueStrings(item.RoutineIDs)
	items[item.ID] = item
	if err := m.saveItems(items); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (m Manager) Get(id string) (Item, error) {
	items, err := m.loadItems()
	if err != nil {
		return Item{}, err
	}
	item, ok := items[id]
	if !ok {
		return Item{}, fmt.Errorf("unknown workspace %q", id)
	}
	return item, nil
}
func (m Manager) List() ([]Item, error) {
	items, err := m.loadItems()
	if err != nil {
		return nil, err
	}
	result := make([]Item, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ParentID == result[j].ParentID {
			return result[i].Name < result[j].Name
		}
		return result[i].ParentID < result[j].ParentID
	})
	return result, nil
}

func (m Manager) Delete(id string) error {
	items, err := m.loadItems()
	if err != nil {
		return err
	}
	if _, ok := items[id]; !ok {
		return nil
	}
	memories, err := m.loadMemory()
	if err != nil {
		return err
	}
	artifacts, err := m.loadArtifacts()
	if err != nil {
		return err
	}

	deleteSet := map[string]struct{}{id: {}}
	for changed := true; changed; {
		changed = false
		for child, item := range items {
			if _, parentDeleted := deleteSet[item.ParentID]; parentDeleted {
				if _, already := deleteSet[child]; !already {
					deleteSet[child] = struct{}{}
					changed = true
				}
			}
		}
	}
	for target := range deleteSet {
		delete(items, target)
	}
	for key, entry := range memories {
		if entry.Layer == LayerWorkspace {
			if _, deleted := deleteSet[entry.Scope]; deleted {
				delete(memories, key)
			}
		}
	}
	for key, artifact := range artifacts {
		if _, deleted := deleteSet[artifact.WorkspaceID]; deleted {
			delete(artifacts, key)
		}
	}

	itemData, err := encodeCollection(workspaceItemsSchema, items)
	if err != nil {
		return err
	}
	memoryData, err := encodeCollection(workspaceMemorySchema, memories)
	if err != nil {
		return err
	}
	artifactData, err := encodeCollection(workspaceArtifactsSchema, artifacts)
	if err != nil {
		return err
	}

	type movedDir struct{ from, to string }
	var moved []movedDir
	rollbackDirs := func() error {
		var rollbackErr error
		for index := len(moved) - 1; index >= 0; index-- {
			entry := moved[index]
			if err := os.Rename(entry.to, entry.from); err != nil && !errors.Is(err, os.ErrNotExist) {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		return rollbackErr
	}
	for target := range deleteSet {
		from := filepath.Join(m.Root, "artifacts", target)
		if _, err := os.Lstat(from); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return errors.Join(err, rollbackDirs())
		}
		to := nextSiblingPath(filepath.Join(m.Root, "backups", fmt.Sprintf("workspace-delete-%s-%s", m.now().Format("20060102T150405.000000000Z"), target)))
		if err := os.Rename(from, to); err != nil {
			return errors.Join(err, rollbackDirs())
		}
		moved = append(moved, movedDir{from: from, to: to})
	}

	err = m.commitCollections(
		collectionUpdate{path: filepath.Join(m.Root, "workspaces.json"), data: itemData},
		collectionUpdate{path: filepath.Join(m.Root, "memory.json"), data: memoryData},
		collectionUpdate{path: filepath.Join(m.Root, "artifacts.json"), data: artifactData},
	)
	if err != nil {
		return errors.Join(err, rollbackDirs())
	}
	for _, entry := range moved {
		m.cleanupCommitted(entry.to)
	}
	return nil
}

func (m Manager) cleanupCommitted(path string) {
	if m.cleanup != nil {
		_ = m.cleanup(path)
		return
	}
	_ = os.RemoveAll(path)
}

func createsCycle(items map[string]Item, id, parent string) bool {
	seen := map[string]struct{}{id: {}}
	current := parent
	for current != "" {
		if _, ok := seen[current]; ok {
			return true
		}
		seen[current] = struct{}{}
		next, ok := items[current]
		if !ok {
			return false
		}
		current = next.ParentID
	}
	return false
}

func (m Manager) loadItems() (map[string]Item, error) {
	if err := m.recoverPendingTransaction(false); err != nil {
		return nil, err
	}
	items, err := loadCollection[Item](filepath.Join(m.Root, "workspaces.json"), workspaceItemsSchema)
	if err != nil {
		return nil, err
	}
	if err := validateLoadedItems(items); err != nil {
		return nil, err
	}
	return items, nil
}
func (m Manager) saveItems(value map[string]Item) error {
	data, err := encodeCollection(workspaceItemsSchema, value)
	if err != nil {
		return err
	}
	return m.commitCollections(collectionUpdate{path: filepath.Join(m.Root, "workspaces.json"), data: data})
}
func (m Manager) loadMemory() (map[string]MemoryEntry, error) {
	if err := m.recoverPendingTransaction(false); err != nil {
		return nil, err
	}
	entries, err := loadCollection[MemoryEntry](filepath.Join(m.Root, "memory.json"), workspaceMemorySchema)
	if err != nil {
		return nil, err
	}
	if err := validateLoadedMemory(entries); err != nil {
		return nil, err
	}
	return entries, nil
}
func (m Manager) saveMemory(value map[string]MemoryEntry) error {
	data, err := encodeCollection(workspaceMemorySchema, value)
	if err != nil {
		return err
	}
	return m.commitCollections(collectionUpdate{path: filepath.Join(m.Root, "memory.json"), data: data})
}
func (m Manager) loadArtifacts() (map[string]Artifact, error) {
	if err := m.recoverPendingTransaction(false); err != nil {
		return nil, err
	}
	artifacts, err := loadCollection[Artifact](filepath.Join(m.Root, "artifacts.json"), workspaceArtifactsSchema)
	if err != nil {
		return nil, err
	}
	if err := m.validateLoadedArtifacts(artifacts); err != nil {
		return nil, err
	}
	return artifacts, nil
}
func (m Manager) saveArtifacts(value map[string]Artifact) error {
	data, err := encodeCollection(workspaceArtifactsSchema, value)
	if err != nil {
		return err
	}
	return m.commitCollections(collectionUpdate{path: filepath.Join(m.Root, "artifacts.json"), data: data})
}

func loadCollection[T any](path, schema string) (map[string]T, error) {
	data, err := safefile.ReadBoundedRegular(path, maxWorkspaceStateBytes)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]T{}, nil
	}
	if err != nil {
		return nil, err
	}
	var header struct {
		Schema json.RawMessage `json:"schema"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	if len(header.Schema) != 0 {
		var storedSchema string
		if json.Unmarshal(header.Schema, &storedSchema) == nil && strings.HasPrefix(storedSchema, "agentstack.") {
			if storedSchema != schema {
				return nil, fmt.Errorf("unexpected persistence schema %q; expected %q", storedSchema, schema)
			}
			var envelope collectionEnvelope[T]
			if err := strictjson.Decode(data, &envelope); err != nil {
				return nil, err
			}
			if envelope.Version != workspaceStoreVersion {
				return nil, fmt.Errorf("unsupported %s schema version %d", schema, envelope.Version)
			}
			if envelope.Items == nil {
				envelope.Items = map[string]T{}
			}
			return envelope.Items, nil
		}
	}
	var legacy map[string]T
	if err := strictjson.Decode(data, &legacy); err != nil {
		return nil, err
	}
	if legacy == nil {
		legacy = map[string]T{}
	}
	return legacy, nil
}

func validateLoadedItems(items map[string]Item) error {
	if len(items) > maxWorkspaceItems {
		return fmt.Errorf("workspace item limit exceeded")
	}
	for key, item := range items {
		if key != item.ID || !validID(item.ID) {
			return fmt.Errorf("persisted workspace key %q does not match a valid id", key)
		}
		if strings.TrimSpace(item.Name) == "" || len(item.Name) > maxWorkspaceNameBytes {
			return fmt.Errorf("persisted workspace %q has an invalid name", key)
		}
		if item.Type != TypeWorkspace && item.Type != TypeFolder {
			return fmt.Errorf("persisted workspace %q has unsupported type %q", key, item.Type)
		}
		if item.ParentID != "" && !validID(item.ParentID) {
			return fmt.Errorf("persisted workspace %q has invalid parent id", key)
		}
		if item.Type == TypeWorkspace {
			if item.Root == "" || !filepath.IsAbs(item.Root) || strings.ContainsRune(item.Root, '\x00') {
				return fmt.Errorf("persisted workspace %q root must be absolute", key)
			}
			if len(item.Prompt) > maxWorkspacePromptBytes || len(item.Vars) > maxWorkspaceVars || len(item.ResourceIDs) > maxWorkspaceLinks || len(item.RoutineIDs) > maxWorkspaceLinks {
				return fmt.Errorf("persisted workspace %q exceeds configured bounds", key)
			}
			for varKey, varValue := range item.Vars {
				if strings.TrimSpace(varKey) == "" || len(varKey) > maxWorkspaceVarKeyBytes || strings.ContainsRune(varKey, '\x00') || len(varValue) > maxWorkspaceVarValue || strings.ContainsRune(varValue, '\x00') {
					return fmt.Errorf("persisted workspace %q has invalid prompt variables", key)
				}
			}
		} else if item.Root != "" || item.Prompt != "" || len(item.Vars) != 0 {
			return fmt.Errorf("persisted folder %q contains workspace-only fields", key)
		}
		for _, id := range append(append([]string(nil), item.ResourceIDs...), item.RoutineIDs...) {
			if !validID(id) {
				return fmt.Errorf("persisted workspace %q references invalid id %q", key, id)
			}
		}
	}
	for key, item := range items {
		if item.ParentID == "" {
			continue
		}
		parent, ok := items[item.ParentID]
		if !ok || parent.Type != TypeFolder {
			return fmt.Errorf("persisted workspace %q references missing folder %q", key, item.ParentID)
		}
		if createsCycle(items, key, item.ParentID) {
			return fmt.Errorf("persisted workspace %q participates in a parent cycle", key)
		}
	}
	return nil
}

func validateLoadedMemory(entries map[string]MemoryEntry) error {
	if len(entries) > maxMemoryEntries {
		return fmt.Errorf("memory entry limit exceeded")
	}
	for id, entry := range entries {
		if id != entry.ID || id != memoryID(entry.Layer, entry.Scope, entry.Key) {
			return fmt.Errorf("persisted memory entry %q has inconsistent identity", id)
		}
		if err := validateLayer(entry.Layer); err != nil {
			return fmt.Errorf("persisted memory entry %q: %w", id, err)
		}
		if strings.TrimSpace(entry.Key) == "" || len(entry.Key) > maxMemoryKeyBytes || len(entry.Value) > maxMemoryValue || len(entry.Tags) > maxMemoryTags {
			return fmt.Errorf("persisted memory entry %q exceeds configured bounds", id)
		}
		if layerNeedsScope(entry.Layer) && strings.TrimSpace(entry.Scope) == "" {
			return fmt.Errorf("persisted memory entry %q lacks required scope", id)
		}
		if entry.Digest != contentDigest(entry.Value) {
			return fmt.Errorf("persisted memory entry %q digest mismatch", id)
		}
	}
	return nil
}

func (m Manager) validateLoadedArtifacts(artifacts map[string]Artifact) error {
	if len(artifacts) > maxArtifactEntries {
		return fmt.Errorf("artifact entry limit exceeded")
	}
	artifactRoot, err := filepath.Abs(filepath.Join(m.Root, "artifacts"))
	if err != nil {
		return err
	}
	for id, artifact := range artifacts {
		if id != artifact.ID || !validID(artifact.ID) || !validID(artifact.WorkspaceID) {
			return fmt.Errorf("persisted artifact %q has inconsistent identity", id)
		}
		if artifact.Size < 0 || artifact.Size > maxArtifactBytes || !validContentDigest(artifact.SHA256) || len(artifact.Metadata) > maxArtifactMetadata {
			return fmt.Errorf("persisted artifact %q has invalid metadata", id)
		}
		if artifact.FileName == "" || filepath.Base(artifact.FileName) != artifact.FileName {
			return fmt.Errorf("persisted artifact %q has invalid file name", id)
		}
		absolutePath, err := filepath.Abs(artifact.Path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(artifactRoot, absolutePath)
		if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("persisted artifact %q path escapes artifact storage", id)
		}
	}
	return nil
}

func validContentDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	for _, r := range strings.TrimPrefix(value, "sha256:") {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return value != "." && value != ".."
}
func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
func uniqueStrings(input []string) []string {
	set := map[string]struct{}{}
	for _, value := range input {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
func contentDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
