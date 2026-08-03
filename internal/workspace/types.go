// Package workspace provides persistent multi-workspace organization, layered
// local memory, deterministic prompt variables, and content-addressed artifacts.
package workspace

import "time"

type ItemType string

const (
	TypeWorkspace ItemType = "workspace"
	TypeFolder    ItemType = "folder"
)

type Item struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Type        ItemType          `json:"type"`
	ParentID    string            `json:"parentId,omitempty"`
	Root        string            `json:"root,omitempty"`
	Prompt      string            `json:"prompt,omitempty"`
	Vars        map[string]string `json:"vars,omitempty"`
	ResourceIDs []string          `json:"resourceIds,omitempty"`
	RoutineIDs  []string          `json:"routineIds,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

type MemoryLayer string

const (
	LayerUser      MemoryLayer = "user"
	LayerProject   MemoryLayer = "project"
	LayerWorkspace MemoryLayer = "workspace"
	LayerSession   MemoryLayer = "session"
)

type MemoryEntry struct {
	ID        string      `json:"id"`
	Layer     MemoryLayer `json:"layer"`
	Scope     string      `json:"scope,omitempty"`
	Key       string      `json:"key"`
	Value     string      `json:"value"`
	Tags      []string    `json:"tags,omitempty"`
	Source    string      `json:"source,omitempty"`
	Digest    string      `json:"digest"`
	CreatedAt time.Time   `json:"createdAt"`
	UpdatedAt time.Time   `json:"updatedAt"`
	ExpiresAt time.Time   `json:"expiresAt,omitempty"`
}

type Artifact struct {
	ID          string            `json:"id"`
	WorkspaceID string            `json:"workspaceId"`
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	FileName    string            `json:"fileName"`
	MediaType   string            `json:"mediaType,omitempty"`
	SHA256      string            `json:"sha256"`
	Size        int64             `json:"size"`
	CreatedAt   time.Time         `json:"createdAt"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type ArtifactOptions struct {
	ID        string
	Name      string
	MediaType string
	Metadata  map[string]string
	Replace   bool
}
