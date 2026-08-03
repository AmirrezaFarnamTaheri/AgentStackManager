// Package contextengine builds deterministic project fingerprints, scores
// agent context quality, and applies reviewed cross-agent context refreshes.
package contextengine

import (
	"time"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

const (
	managedStart = "<!-- agentstack-context:start"
	managedEnd   = "<!-- agentstack-context:end -->"
)

type Snapshot struct {
	Root         string            `json:"root"`
	GeneratedAt  time.Time         `json:"generatedAt"`
	Fingerprint  string            `json:"fingerprint"`
	Module       string            `json:"module,omitempty"`
	Languages    map[string]int    `json:"languages"`
	Frameworks   []string          `json:"frameworks,omitempty"`
	Manifests    []string          `json:"manifests,omitempty"`
	TopLevel     []string          `json:"topLevel,omitempty"`
	Commands     map[string]string `json:"commands,omitempty"`
	AgentConfigs []string          `json:"agentConfigs,omitempty"`
	MCPConfigs   []string          `json:"mcpConfigs,omitempty"`
	SourceFiles  int               `json:"sourceFiles"`
	SourceBytes  int64             `json:"sourceBytes"`
	Truncated    bool              `json:"truncated,omitempty"`
}

type Check struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	Name       string `json:"name"`
	Earned     int    `json:"earned"`
	Maximum    int    `json:"maximum"`
	Passed     bool   `json:"passed"`
	Detail     string `json:"detail"`
	Suggestion string `json:"suggestion,omitempty"`
}

type ScoreResult struct {
	Score       int                 `json:"score"`
	Grade       string              `json:"grade"`
	GeneratedAt time.Time           `json:"generatedAt"`
	Targets     []resourcehub.Agent `json:"targets"`
	Checks      []Check             `json:"checks"`
	Snapshot    Snapshot            `json:"snapshot"`
}

type RefreshAction string

const (
	RefreshCreate RefreshAction = "create"
	RefreshUpdate RefreshAction = "update"
	RefreshNoop   RefreshAction = "noop"
)

type RefreshOperation struct {
	Agent        resourcehub.Agent `json:"agent"`
	Path         string            `json:"path"`
	Action       RefreshAction     `json:"action"`
	BeforeDigest string            `json:"beforeDigest"`
	AfterDigest  string            `json:"afterDigest"`
	Content      string            `json:"content"`
}

type RefreshPlan struct {
	ID                 string             `json:"id"`
	Digest             string             `json:"digest"`
	Root               string             `json:"root"`
	ProjectFingerprint string             `json:"projectFingerprint"`
	GeneratedAt        time.Time          `json:"generatedAt"`
	ExpiresAt          time.Time          `json:"expiresAt"`
	Operations         []RefreshOperation `json:"operations"`
}

type RefreshReport struct {
	PlanID     string             `json:"planId"`
	StartedAt  time.Time          `json:"startedAt"`
	FinishedAt time.Time          `json:"finishedAt"`
	Applied    []RefreshOperation `json:"applied"`
	Skipped    []RefreshOperation `json:"skipped,omitempty"`
	Backups    []string           `json:"backups,omitempty"`
	RolledBack bool               `json:"rolledBack,omitempty"`
}
