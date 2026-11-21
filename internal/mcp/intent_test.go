package mcp_test

import (
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/mcp"
)

func TestSealMCPIntentSuccess(t *testing.T) {
	def := mcp.MCPServerDefinition{
		ServerID:     "mcp-github",
		DisplayName:  "GitHub Integration MCP",
		Transport:    mcp.TransportStdio,
		Command:      "npx",
		Args:         []string{"-y", "@modelcontextprotocol/server-github"},
		EnvKeys:      []string{"GITHUB_PERSONAL_ACCESS_TOKEN"},
		Capabilities: []string{"tools", "resources"},
		Health:       mcp.HealthHealthy,
		CreatedAt:    time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	}

	sealed, err := mcp.SealMCPIntent(def)
	if err != nil {
		t.Fatalf("seal mcp intent: %v", err)
	}
	if sealed.Digest == "" {
		t.Errorf("expected non-empty digest")
	}
	if !strings.HasPrefix(sealed.Digest, "sha256:") {
		t.Errorf("expected sha256 prefix in digest %s", sealed.Digest)
	}
}

func TestValidateMCPIntentSecretLeakRejection(t *testing.T) {
	// Rejection case 1: EnvKey containing assignment
	def1 := mcp.MCPServerDefinition{
		ServerID:  "mcp-leak-1",
		Transport: mcp.TransportStdio,
		EnvKeys:   []string{"GITHUB_TOKEN=secret_12345"},
	}
	if err := mcp.ValidateMCPIntent(def1); err == nil {
		t.Fatalf("expected error for envKey assignment secret leak")
	}

	// Rejection case 2: Token pattern in args
	def2 := mcp.MCPServerDefinition{
		ServerID:  "mcp-leak-2",
		Transport: mcp.TransportStdio,
		Args:      []string{"--token", "sk-proj-1234567890abcdef"},
	}
	if err := mcp.ValidateMCPIntent(def2); err == nil {
		t.Fatalf("expected error for token pattern in args secret leak")
	}
}
