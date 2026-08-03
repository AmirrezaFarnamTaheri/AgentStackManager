package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var SupportedProtocolVersions = []string{"2025-06-18", "2025-03-26"}

var errRequestNotStarted = errors.New("child request did not acquire the session")
var ErrChildRuntimeClosed = errors.New("child runtime is closed")

const (
	defaultChildTimeout = 45 * time.Second
	defaultMessageLimit = 4 << 20
	defaultStderrLimit  = 256 << 10
)

type DoctorItem struct {
	Status          string        `json:"status"`
	Message         string        `json:"message,omitempty"`
	ProtocolVersion string        `json:"protocolVersion,omitempty"`
	ToolCount       int           `json:"toolCount,omitempty"`
	Duration        time.Duration `json:"duration,omitempty"`
}

type ChildClient interface {
	ListTools(context.Context, ServerConfig) (json.RawMessage, error)
	CallTool(context.Context, ServerConfig, string, json.RawMessage) (json.RawMessage, error)
	Doctor(context.Context, ServerConfig) DoctorItem
}

// ChildEvent is a privacy-minimized lifecycle signal for one child MCP
// process. ServerKey is a digest of the effective server configuration; raw
// arguments and environment values are deliberately excluded.
type ChildEvent struct {
	Type      string        `json:"type"`
	ServerKey string        `json:"serverKey"`
	Command   string        `json:"command"`
	Status    string        `json:"status"`
	Method    string        `json:"method,omitempty"`
	Duration  time.Duration `json:"duration"`
	Message   string        `json:"message,omitempty"`
}

type ChildObserver func(ChildEvent)
