package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
)

type TransportType string

const (
	TransportStdio TransportType = "stdio"
	TransportSSE   TransportType = "sse"
	TransportHTTP  TransportType = "http"
)

type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthDegraded  HealthStatus = "degraded"
	HealthUnreachable HealthStatus = "unreachable"
)

type MCPServerDefinition struct {
	ServerID             string        `json:"serverId"`
	DisplayName          string        `json:"displayName"`
	Transport            TransportType `json:"transport"`
	Command              string        `json:"command,omitempty"`
	Args                 []string      `json:"args,omitempty"`
	EnvKeys              []string      `json:"envKeys,omitempty"` // Secret-free list of env var names required
	Capabilities         []string      `json:"capabilities,omitempty"`
	Compatibility        string        `json:"compatibility,omitempty"`
	Profile              string        `json:"profile,omitempty"`
	Assignment           string        `json:"assignment,omitempty"`
	ObservedRegistration string        `json:"observedRegistration,omitempty"`
	Health               HealthStatus  `json:"health"`
	CreatedAt            time.Time     `json:"createdAt"`
	Digest               string        `json:"digest"`
}

// ValidateMCPIntent checks for canonical validity and ensures no raw secret values exist in env keys or args.
func ValidateMCPIntent(def MCPServerDefinition) error {
	if strings.TrimSpace(def.ServerID) == "" {
		return fmt.Errorf("server ID cannot be empty")
	}
	if def.Transport == "" {
		return fmt.Errorf("transport type cannot be empty")
	}

	// Secret leak checks: ensure env keys do not contain values (e.g. '=' operator) or raw tokens
	for _, envKey := range def.EnvKeys {
		if strings.Contains(envKey, "=") {
			return fmt.Errorf("envKey %q contains an assignment or secret value; only key names allowed", envKey)
		}
		if len(envKey) > 128 {
			return fmt.Errorf("envKey %q exceeds maximum key length; potential raw token leak", envKey)
		}
	}

	for _, arg := range def.Args {
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "ghp_") || strings.HasPrefix(lower, "bearer ") {
			return fmt.Errorf("arg contains potential raw API key or token pattern")
		}
	}

	return nil
}

// SealMCPIntent normalizes and seals an MCP server intent definition with SHA-256 digest.
func SealMCPIntent(def MCPServerDefinition) (MCPServerDefinition, error) {
	if err := ValidateMCPIntent(def); err != nil {
		return MCPServerDefinition{}, err
	}

	// Sort slices for deterministic hashing
	sort.Strings(def.Args)
	sort.Strings(def.EnvKeys)
	sort.Strings(def.Capabilities)

	if def.Health == "" {
		def.Health = HealthUnknown
	}

	def.Digest = ""
	d, err := integrity.DigestJSON(def)
	if err != nil {
		return MCPServerDefinition{}, fmt.Errorf("digest MCP intent: %w", err)
	}
	def.Digest = d
	return def, nil
}

// ComputeMCPDigest generates a SHA-256 digest of an MCP server intent.
func ComputeMCPDigest(def MCPServerDefinition) string {
	sealed, err := SealMCPIntent(def)
	if err != nil {
		h := sha256.Sum256([]byte(def.ServerID + string(def.Transport)))
		return "sha256:" + hex.EncodeToString(h[:])
	}
	return sealed.Digest
}
