package changeset_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/adapters/conformance"
	"github.com/agentstack/agentstack/internal/adapters/mcplink"
	"github.com/agentstack/agentstack/internal/changeset"
	"github.com/agentstack/agentstack/internal/executor"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

type mockInvoker struct {
	shouldFail bool
}

func (m *mockInvoker) InvokeTool(serverID string, toolName string, payload map[string]any) (map[string]any, error) {
	if m.shouldFail {
		return nil, fmt.Errorf("mock tool invocation failed")
	}
	return map[string]any{"status": "ok"}, nil
}

func TestComposeAndExecuteChangeSetSuccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "changeset-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "skills", "my-skill", "SKILL.md")
	content := []byte("# Skill Content\n")
	digest := integrity.DigestBytes(content)

	fileOps := []executor.FileOperation{
		{
			OperationID: "op-cs-1",
			TargetID:    "opencode",
			TargetPath:  targetFile,
			OpType:      "create",
			BaseDescriptor: executor.FileDescriptor{
				Path:   targetFile,
				Exists: false,
			},
			DesiredDescriptor: executor.FileDescriptor{
				Path:   targetFile,
				Digest: digest,
				Size:   int64(len(content)),
				Exists: true,
			},
			DesiredContent: content,
			OwnershipMarker: resourcehub.OwnershipManifest{
				ResourceID: "global/Skill/my-skill",
				ArtifactID: "op-cs-1",
			},
			BackupRequired: true,
		},
	}

	mcpMutations := []mcplink.MCPMutation{
		{
			MutationID: "mut-cs-1",
			Target: mcplink.MCPTargetDescriptor{
				TargetID: "opencode",
				ServerID: "mcp-server-1",
				ToolName: "sync_config",
			},
			Payload: map[string]any{"key": "val"},
		},
	}

	cs, err := changeset.ComposeChangeSet(
		"opencode",
		fileOps,
		mcpMutations,
		make(map[string]conformance.FidelityRecord),
		nil,
		"3-retries",
		"backup-rollback",
		time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("compose changeset: %v", err)
	}
	if cs.ChangeSetDigest == "" {
		t.Fatalf("expected non-empty changeset digest")
	}

	depExec := executor.NewDeploymentExecutor(filepath.Join(tmpDir, "backups"))
	mcpExec := mcplink.NewMCPExecutor(&mockInvoker{shouldFail: false})

	result, err := changeset.ExecuteChangeSet(cs, depExec, mcpExec)
	if err != nil {
		t.Fatalf("execute changeset: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected result success=true")
	}
	if result.PartialSuccess {
		t.Fatalf("expected result partialSuccess=false")
	}

	// Verify file was written
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %s", string(data))
	}
}

func TestExecuteChangeSetPartialSuccessReporting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "changeset-partial-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "skills", "my-skill", "SKILL.md")
	content := []byte("# Skill Content\n")
	digest := integrity.DigestBytes(content)

	fileOps := []executor.FileOperation{
		{
			OperationID: "op-cs-1",
			TargetID:    "opencode",
			TargetPath:  targetFile,
			OpType:      "create",
			BaseDescriptor: executor.FileDescriptor{
				Path:   targetFile,
				Exists: false,
			},
			DesiredDescriptor: executor.FileDescriptor{
				Path:   targetFile,
				Digest: digest,
				Size:   int64(len(content)),
				Exists: true,
			},
			DesiredContent: content,
			BackupRequired: true,
		},
	}

	// MCP mutation that fails
	mcpMutations := []mcplink.MCPMutation{
		{
			MutationID: "mut-fail-1",
			Target: mcplink.MCPTargetDescriptor{
				TargetID: "opencode",
				ServerID: "mcp-server-1",
				ToolName: "failing_tool",
			},
		},
	}

	cs, err := changeset.ComposeChangeSet(
		"opencode",
		fileOps,
		mcpMutations,
		nil,
		nil,
		"no-retry",
		"backup-restore",
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("compose changeset: %v", err)
	}

	depExec := executor.NewDeploymentExecutor(filepath.Join(tmpDir, "backups"))
	mcpExec := mcplink.NewMCPExecutor(&mockInvoker{shouldFail: true})

	result, _ := changeset.ExecuteChangeSet(cs, depExec, mcpExec)
	if result.Success {
		t.Fatalf("expected success=false for partial failure")
	}
	if !result.PartialSuccess {
		t.Fatalf("expected partialSuccess=true when filesystem succeeds but MCP tool fails")
	}
}
