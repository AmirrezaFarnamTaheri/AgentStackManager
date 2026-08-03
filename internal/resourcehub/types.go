// Package resourcehub manages one reviewed source of truth for agent skills,
// agents, rules, commands, prompts, and MCP server definitions, then syncs
// them to supported coding-agent targets through digest-bound plans.
package resourcehub

import "time"

type Kind string

const (
	KindSkill     Kind = "skill"
	KindAgent     Kind = "agent"
	KindRule      Kind = "rule"
	KindCommand   Kind = "command"
	KindPrompt    Kind = "prompt"
	KindMCPServer Kind = "mcp-server"
	KindContext   Kind = "context"
)

type Agent string

const (
	AgentCodex    Agent = "codex"
	AgentClaude   Agent = "claude"
	AgentCursor   Agent = "cursor"
	AgentOpenCode Agent = "opencode"
	AgentCopilot  Agent = "github-copilot"
	AgentGeneric  Agent = "generic"
)

type SyncMode string

const (
	ModeAuto SyncMode = "auto"
	ModeCopy SyncMode = "copy"
	ModeLink SyncMode = "link"
)

type Resource struct {
	ID          string            `json:"id"`
	Kind        Kind              `json:"kind"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Digest      string            `json:"digest"`
	Entry       string            `json:"entry"`
	Source      string            `json:"source,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Targets     []Agent           `json:"targets,omitempty"`
	Scope       string            `json:"scope,omitempty"`
	Enabled     bool              `json:"enabled"`
	ImportedAt  time.Time         `json:"importedAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type Target struct {
	ID      string   `json:"id"`
	Agent   Agent    `json:"agent"`
	Root    string   `json:"root"`
	Mode    SyncMode `json:"mode"`
	Enabled bool     `json:"enabled"`
}

type Registry struct {
	Version   int                 `json:"version"`
	Resources map[string]Resource `json:"resources"`
	Targets   map[string]Target   `json:"targets"`
	UpdatedAt time.Time           `json:"updatedAt"`
}

type ImportOptions struct {
	ID            string
	Kind          Kind
	Name          string
	Description   string
	Tags          []string
	Targets       []Agent
	Scope         string
	Metadata      map[string]string
	Replace       bool
	trackedSource string
}

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

type Finding struct {
	Severity Severity `json:"severity"`
	Category string   `json:"category"`
	RuleID   string   `json:"ruleId"`
	File     string   `json:"file"`
	Line     int      `json:"line"`
	Message  string   `json:"message"`
	Snippet  string   `json:"snippet,omitempty"`
}

type AuditResult struct {
	ResourceID   string    `json:"resourceId"`
	ScannedAt    time.Time `json:"scannedAt"`
	Findings     []Finding `json:"findings"`
	RiskScore    int       `json:"riskScore"`
	RiskLabel    string    `json:"riskLabel"`
	Blocked      bool      `json:"blocked"`
	FilesScanned int       `json:"filesScanned"`
	BytesScanned int64     `json:"bytesScanned"`
	FilesSkipped int       `json:"filesSkipped"`
}

type SyncAction string

const (
	ActionCreate   SyncAction = "create"
	ActionUpdate   SyncAction = "update"
	ActionRemove   SyncAction = "remove"
	ActionNoop     SyncAction = "noop"
	ActionConflict SyncAction = "conflict"
)

type SyncOperation struct {
	ResourceID    string     `json:"resourceId,omitempty"`
	Kind          Kind       `json:"kind,omitempty"`
	Action        SyncAction `json:"action"`
	Source        string     `json:"source,omitempty"`
	Destination   string     `json:"destination"`
	DesiredDigest string     `json:"desiredDigest,omitempty"`
	CurrentDigest string     `json:"currentDigest,omitempty"`
	Reason        string     `json:"reason"`
}

type SyncPlan struct {
	ID             string          `json:"id"`
	Digest         string          `json:"digest"`
	TargetID       string          `json:"targetId"`
	GeneratedAt    time.Time       `json:"generatedAt"`
	ExpiresAt      time.Time       `json:"expiresAt"`
	RegistryDigest string          `json:"registryDigest"`
	AllowRisk      bool            `json:"allowRisk"`
	Prune          bool            `json:"prune"`
	Operations     []SyncOperation `json:"operations"`
}

type PlanOptions struct {
	TTL       time.Duration
	AllowRisk bool
	Prune     bool
}

type SyncReport struct {
	PlanID     string          `json:"planId"`
	TargetID   string          `json:"targetId"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt time.Time       `json:"finishedAt"`
	Applied    []SyncOperation `json:"applied"`
	Skipped    []SyncOperation `json:"skipped,omitempty"`
	Backups    []string        `json:"backups,omitempty"`
}

type managedEntry struct {
	ResourceID  string    `json:"resourceId"`
	Destination string    `json:"destination"`
	Digest      string    `json:"digest"`
	ManagedAt   time.Time `json:"managedAt"`
}

type managedState struct {
	Version   int                     `json:"version"`
	TargetID  string                  `json:"targetId"`
	Entries   map[string]managedEntry `json:"entries"`
	UpdatedAt time.Time               `json:"updatedAt"`
}

type RefreshAction string

const (
	RefreshUpdate   RefreshAction = "update"
	RefreshNoop     RefreshAction = "noop"
	RefreshConflict RefreshAction = "conflict"
)

type RefreshOperation struct {
	ResourceID   string        `json:"resourceId"`
	Action       RefreshAction `json:"action"`
	Source       string        `json:"source,omitempty"`
	BeforeDigest string        `json:"beforeDigest,omitempty"`
	SourceDigest string        `json:"sourceDigest,omitempty"`
	Reason       string        `json:"reason"`
}

type RefreshPlan struct {
	ID             string             `json:"id"`
	Digest         string             `json:"digest"`
	RegistryDigest string             `json:"registryDigest"`
	GeneratedAt    time.Time          `json:"generatedAt"`
	ExpiresAt      time.Time          `json:"expiresAt"`
	Operations     []RefreshOperation `json:"operations"`
}

type RefreshReport struct {
	PlanID     string             `json:"planId"`
	StartedAt  time.Time          `json:"startedAt"`
	FinishedAt time.Time          `json:"finishedAt"`
	Applied    []RefreshOperation `json:"applied"`
	Skipped    []RefreshOperation `json:"skipped,omitempty"`
	Backups    []string           `json:"backups,omitempty"`
}

type BackupInfo struct {
	ID         string    `json:"id"`
	ResourceID string    `json:"resourceId"`
	Digest     string    `json:"digest"`
	CreatedAt  time.Time `json:"createdAt"`
	Path       string    `json:"-"`
}
