// Package mcplink discovers and safely links the AgentStack MCP router into
// supported client configurations through reviewed, digest-bound plans.
package mcplink

import (
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
)

type ClientKind string

const (
	ClientCodex    ClientKind = "codex"
	ClientClaude   ClientKind = "claude"
	ClientCursor   ClientKind = "cursor"
	ClientAgy      ClientKind = "agy"
	ClientOpenCode ClientKind = "opencode"
)

type Mode string

const (
	ModeLink   Mode = "link"
	ModeUnlink Mode = "unlink"
)

type Action string

const (
	ActionCreate   Action = "create"
	ActionUpdate   Action = "update"
	ActionRemove   Action = "remove"
	ActionNoop     Action = "noop"
	ActionConflict Action = "conflict"
)

type Operation struct {
	AdapterID        string            `json:"adapterId"`
	AdapterVersion   string            `json:"adapterVersion"`
	CapabilityDigest string            `json:"capabilityDigest"`
	LossReportDigest string            `json:"lossReportDigest"`
	Fidelity         adapters.Fidelity `json:"fidelity"`
	Losses           []adapters.Loss   `json:"losses,omitempty"`
	Client           ClientKind        `json:"client"`
	Path             string            `json:"path,omitempty"`
	Action           Action            `json:"action"`
	BeforeDigest     string            `json:"beforeDigest"`
	AfterDigest      string            `json:"afterDigest,omitempty"`
	Reason           string            `json:"reason"`
}

type Plan struct {
	ID                  string                   `json:"id"`
	Digest              string                   `json:"digest"`
	Mode                Mode                     `json:"mode"`
	GeneratedAt         time.Time                `json:"generatedAt"`
	ExpiresAt           time.Time                `json:"expiresAt"`
	Executable          string                   `json:"executable"`
	RouterConfig        string                   `json:"routerConfig"`
	AdapterContract     string                   `json:"adapterContract"`
	CapabilitySnapshots []adapters.CapabilitySet `json:"capabilitySnapshots"`
	LossReports         []adapters.LossReport    `json:"lossReports"`
	Operations          []Operation              `json:"operations"`
}

type Report struct {
	PlanID      string                `json:"planId"`
	StartedAt   time.Time             `json:"startedAt"`
	FinishedAt  time.Time             `json:"finishedAt"`
	Applied     []Operation           `json:"applied"`
	Skipped     []Operation           `json:"skipped,omitempty"`
	Backups     []string              `json:"backups,omitempty"`
	RolledBack  bool                  `json:"rolledBack,omitempty"`
	LossReports []adapters.LossReport `json:"lossReports"`
}
