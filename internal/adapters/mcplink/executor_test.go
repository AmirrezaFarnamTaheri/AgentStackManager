package mcplink_test

import (
	"fmt"
	"testing"

	"github.com/agentstack/agentstack/internal/adapters/mcplink"
)

type mockInvoker struct {
	shouldFail bool
}

func (m *mockInvoker) InvokeTool(serverID string, toolName string, payload map[string]any) (map[string]any, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock tool invocation failed for %s/%s", serverID, toolName)
	}
	return map[string]any{
		"status":  "ok",
		"updated": true,
	}, nil
}

func TestMCPExecutorSuccess(t *testing.T) {
	inv := &mockInvoker{shouldFail: false}
	exec := mcplink.NewMCPExecutor(inv)

	mutation := mcplink.MCPMutation{
		MutationID: "mut-1",
		Target: mcplink.MCPTargetDescriptor{
			TargetID: "server-a",
			ServerID: "mcp-server-1",
			ToolName: "update_config",
		},
		Payload: map[string]any{
			"key": "value",
		},
		PreconditionDigest: "sha256:123456",
		TimeoutSeconds:     10,
	}

	result, err := exec.Execute(mutation)
	if err != nil {
		t.Fatalf("mcp execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected result success=true")
	}
	if result.ResultPayload["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result.ResultPayload["status"])
	}
	if result.OutputDigest == "" {
		t.Errorf("expected non-empty output digest")
	}
}

func TestMCPExecutorFailureSafeState(t *testing.T) {
	inv := &mockInvoker{shouldFail: true}
	exec := mcplink.NewMCPExecutor(inv)

	mutation := mcplink.MCPMutation{
		MutationID: "mut-fail",
		Target: mcplink.MCPTargetDescriptor{
			TargetID: "server-b",
			ServerID: "mcp-server-2",
			ToolName: "invalid_tool",
		},
		Payload: map[string]any{
			"key": "bad",
		},
	}

	result, err := exec.Execute(mutation)
	if err == nil {
		t.Fatalf("expected error from failed invoker, got nil")
	}
	if result.Success {
		t.Fatalf("expected result success=false")
	}
	if result.Error == "" {
		t.Errorf("expected non-empty result error message")
	}
}
