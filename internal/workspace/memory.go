package workspace

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (m Manager) Remember(entry MemoryEntry) (MemoryEntry, error) {
	if err := validateLayer(entry.Layer); err != nil {
		return MemoryEntry{}, err
	}
	entry.Key = strings.TrimSpace(entry.Key)
	if entry.Key == "" {
		return MemoryEntry{}, fmt.Errorf("memory key is required")
	}
	if len(entry.Value) > maxMemoryValue {
		return MemoryEntry{}, fmt.Errorf("memory value exceeds %d bytes", maxMemoryValue)
	}
	if layerNeedsScope(entry.Layer) && strings.TrimSpace(entry.Scope) == "" {
		return MemoryEntry{}, fmt.Errorf("memory layer %q requires a scope", entry.Layer)
	}
	entries, err := m.loadMemory()
	if err != nil {
		return MemoryEntry{}, err
	}
	id := memoryID(entry.Layer, entry.Scope, entry.Key)
	now := m.now()
	previous, exists := entries[id]
	entry.ID = id
	entry.CreatedAt = now
	if exists {
		entry.CreatedAt = previous.CreatedAt
	}
	entry.UpdatedAt = now
	entry.Tags = uniqueStrings(entry.Tags)
	entry.Digest = contentDigest(entry.Value)
	entries[id] = entry
	if len(entries) > maxMemoryEntries {
		pruneExpired(entries, now)
		if len(entries) > maxMemoryEntries {
			return MemoryEntry{}, fmt.Errorf("memory entry limit reached")
		}
	}
	if err := m.saveMemory(entries); err != nil {
		return MemoryEntry{}, err
	}
	return entry, nil
}

func (m Manager) Recall(workspaceID, key, sessionID string) (MemoryEntry, error) {
	entries, err := m.loadMemory()
	if err != nil {
		return MemoryEntry{}, err
	}
	now := m.now()
	candidates := []struct {
		layer MemoryLayer
		scope string
	}{{LayerSession, sessionID}, {LayerWorkspace, workspaceID}}
	if workspaceID != "" {
		if item, getErr := m.Get(workspaceID); getErr == nil {
			candidates = append(candidates, struct {
				layer MemoryLayer
				scope string
			}{LayerProject, item.Root})
		}
	}
	candidates = append(candidates, struct {
		layer MemoryLayer
		scope string
	}{LayerUser, ""})
	for _, candidate := range candidates {
		if candidate.layer == LayerSession && candidate.scope == "" {
			continue
		}
		entry, ok := entries[memoryID(candidate.layer, candidate.scope, key)]
		if !ok || expired(entry, now) {
			continue
		}
		if entry.Digest != contentDigest(entry.Value) {
			return MemoryEntry{}, fmt.Errorf("memory digest mismatch for %q", entry.ID)
		}
		return entry, nil
	}
	return MemoryEntry{}, fmt.Errorf("memory key %q was not found", key)
}

func (m Manager) SearchMemory(query, workspaceID, sessionID string) ([]MemoryEntry, error) {
	entries, err := m.loadMemory()
	if err != nil {
		return nil, err
	}
	tokens := tokenize(query)
	now := m.now()
	type scored struct {
		entry MemoryEntry
		score int
	}
	var matches []scored
	allowed := map[string]struct{}{"user\x00": {}}
	if workspaceID != "" {
		allowed[string(LayerWorkspace)+"\x00"+workspaceID] = struct{}{}
		if item, getErr := m.Get(workspaceID); getErr == nil {
			allowed[string(LayerProject)+"\x00"+item.Root] = struct{}{}
		}
	}
	if sessionID != "" {
		allowed[string(LayerSession)+"\x00"+sessionID] = struct{}{}
	}
	for _, entry := range entries {
		if expired(entry, now) {
			continue
		}
		if _, ok := allowed[string(entry.Layer)+"\x00"+entry.Scope]; !ok {
			continue
		}
		haystack := strings.ToLower(entry.Key + " " + entry.Value + " " + strings.Join(entry.Tags, " "))
		score := 0
		for _, token := range tokens {
			if strings.Contains(haystack, token) {
				score++
			}
		}
		if len(tokens) == 0 || score > 0 {
			matches = append(matches, scored{entry, score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].entry.UpdatedAt.After(matches[j].entry.UpdatedAt)
		}
		return matches[i].score > matches[j].score
	})
	result := make([]MemoryEntry, len(matches))
	for i, item := range matches {
		result[i] = item.entry
	}
	return result, nil
}

func (m Manager) Forget(layer MemoryLayer, scope, key string) error {
	if err := validateLayer(layer); err != nil {
		return err
	}
	entries, err := m.loadMemory()
	if err != nil {
		return err
	}
	delete(entries, memoryID(layer, scope, key))
	return m.saveMemory(entries)
}
func validateLayer(layer MemoryLayer) error {
	switch layer {
	case LayerUser, LayerProject, LayerWorkspace, LayerSession:
		return nil
	default:
		return fmt.Errorf("unsupported memory layer %q", layer)
	}
}
func layerNeedsScope(layer MemoryLayer) bool { return layer != LayerUser }
func memoryID(layer MemoryLayer, scope, key string) string {
	return string(layer) + "\x00" + strings.TrimSpace(scope) + "\x00" + strings.TrimSpace(key)
}
func expired(entry MemoryEntry, now time.Time) bool {
	return !entry.ExpiresAt.IsZero() && !now.Before(entry.ExpiresAt)
}
func pruneExpired(entries map[string]MemoryEntry, now time.Time) {
	for id, entry := range entries {
		if expired(entry, now) {
			delete(entries, id)
		}
	}
}
func tokenize(value string) []string {
	fields := strings.Fields(strings.ToLower(value))
	return uniqueStrings(fields)
}
