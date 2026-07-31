package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/agentstack/agentstack/internal/runner"
)

type RegistrationStatus string

const (
	RegistrationAdded           RegistrationStatus = "added"
	RegistrationEquivalent      RegistrationStatus = "equivalent"
	RegistrationRepaired        RegistrationStatus = "repaired"
	RegistrationForeignConflict RegistrationStatus = "foreign-conflict"
)

type RegistrationResult struct {
	Client   string             `json:"client"`
	Changed  bool               `json:"changed"`
	Repaired bool               `json:"repaired,omitempty"`
	Conflict bool               `json:"conflict"`
	Status   RegistrationStatus `json:"status"`
	Message  string             `json:"message,omitempty"`
}

type clientRegistration struct {
	Command string
	Args    []string
}

func RegisterCodex(ctx context.Context, commands runner.CommandRunner, executable, configPath string) (RegistrationResult, error) {
	result := RegistrationResult{Client: "codex"}
	if commands == nil {
		commands = runner.ExecRunner{}
	}
	expected := clientRegistration{Command: executable, Args: []string{"mcp-router", "--config", configPath}}
	existingResult := commands.Run(ctx, runner.Invocation{Command: "codex", Args: []string{"mcp", "get", "agentstack-router", "--json"}})
	if existingResult.Err == nil && existingResult.ExitCode == 0 {
		existing, err := parseClientRegistration(existingResult.Stdout)
		if err != nil {
			return result, fmt.Errorf("inspect existing Codex MCP entry: %w", err)
		}
		if registrationsEquivalent(existing, expected) {
			result.Status = RegistrationEquivalent
			result.Message = "existing Codex MCP entry is equivalent"
			return result, nil
		}
		if !registrationOwnedByAgentStack(existing) {
			result.Conflict = true
			result.Status = RegistrationForeignConflict
			result.Message = "foreign Codex MCP entry named agentstack-router was preserved"
			return result, fmt.Errorf("Codex MCP registration conflict: %s", result.Message)
		}
		return repairCodexRegistration(ctx, commands, existing, expected)
	}
	if !registrationAbsent(existingResult) {
		return result, fmt.Errorf("inspect existing Codex MCP entry: %s", resultErrorText(existingResult))
	}
	if err := addCodexRegistration(ctx, commands, expected); err != nil {
		return result, err
	}
	result.Changed = true
	result.Status = RegistrationAdded
	result.Message = "added Codex MCP entry agentstack-router"
	return result, nil
}

func repairCodexRegistration(ctx context.Context, commands runner.CommandRunner, existing, expected clientRegistration) (RegistrationResult, error) {
	result := RegistrationResult{Client: "codex"}
	removed := commands.Run(ctx, runner.Invocation{Command: "codex", Args: []string{"mcp", "remove", "agentstack-router"}})
	if removed.Err != nil || removed.ExitCode != 0 {
		return result, fmt.Errorf("remove stale AgentStack-owned Codex MCP entry: %s", resultErrorText(removed))
	}
	if err := addCodexRegistration(ctx, commands, expected); err != nil {
		restored := addCodexRegistration(ctx, commands, existing)
		if restored != nil {
			return result, fmt.Errorf("repair Codex MCP entry: %v; rollback failed: %v", err, restored)
		}
		return result, fmt.Errorf("repair Codex MCP entry: %w; previous AgentStack entry restored", err)
	}
	result.Changed = true
	result.Repaired = true
	result.Status = RegistrationRepaired
	result.Message = "repaired stale AgentStack-owned Codex MCP entry"
	return result, nil
}

func addCodexRegistration(ctx context.Context, commands runner.CommandRunner, registration clientRegistration) error {
	args := []string{"mcp", "add", "agentstack-router", "--", registration.Command}
	args = append(args, registration.Args...)
	added := commands.Run(ctx, runner.Invocation{Command: "codex", Args: args})
	if added.Err != nil || added.ExitCode != 0 {
		return fmt.Errorf("register Codex MCP entry: %s", resultErrorText(added))
	}
	return nil
}

func parseClientRegistration(payload string) (clientRegistration, error) {
	var value map[string]any
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return clientRegistration{}, fmt.Errorf("decode JSON: %w", err)
	}
	for _, candidate := range []map[string]any{value, mapValue(value["transport"]), mapValue(value["config"]), mapValue(value["stdio"])} {
		if candidate == nil {
			continue
		}
		command, _ := candidate["command"].(string)
		if command == "" {
			continue
		}
		args, err := stringSlice(candidate["args"])
		if err != nil {
			return clientRegistration{}, err
		}
		return clientRegistration{Command: command, Args: args}, nil
	}
	return clientRegistration{}, fmt.Errorf("entry has no stdio command")
}

func mapValue(value any) map[string]any {
	result, _ := value.(map[string]any)
	return result
}

func stringSlice(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("entry args are not an array")
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("entry args contain a non-string value")
		}
		result = append(result, text)
	}
	return result, nil
}

func registrationsEquivalent(left, right clientRegistration) bool {
	if normalizeExecutable(left.Command) != normalizeExecutable(right.Command) || len(left.Args) != len(right.Args) {
		return false
	}
	for index := range left.Args {
		if normalizeArgument(left.Args[index]) != normalizeArgument(right.Args[index]) {
			return false
		}
	}
	return true
}

func registrationOwnedByAgentStack(value clientRegistration) bool {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(value.Command, "\\", "/")))
	if base == "agentstack" || base == "agentstack.exe" {
		return true
	}
	for _, arg := range value.Args {
		if strings.EqualFold(arg, "mcp-router") {
			return true
		}
	}
	return false
}

func normalizeExecutable(value string) string {
	return strings.ToLower(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")))
}

func normalizeArgument(value string) string {
	if strings.Contains(value, "\\") || strings.Contains(value, "/") {
		return normalizeExecutable(value)
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func registrationAbsent(result runner.Result) bool {
	if result.ExitCode == 0 {
		return false
	}
	message := strings.ToLower(result.Stderr + "\n" + result.Stdout)
	for _, marker := range []string{"not found", "does not exist", "unknown server", "no server named", "no mcp server"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func resultErrorText(result runner.Result) string {
	parts := []string{}
	if result.Err != nil {
		parts = append(parts, result.Err.Error())
	}
	if value := strings.TrimSpace(result.Stderr); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(result.Stdout); value != "" && len(parts) == 0 {
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("exit code %d", result.ExitCode)
	}
	return strings.Join(parts, ": ")
}
