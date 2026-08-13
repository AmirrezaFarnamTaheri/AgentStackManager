package executor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/executor"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestDeploymentExecutorSuccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	backupRoot := filepath.Join(tmpDir, "backups")
	targetFile := filepath.Join(tmpDir, "skills", "my-skill", "SKILL.md")

	exec := executor.NewDeploymentExecutor(backupRoot)

	// Op 1: Create file
	desiredContent := []byte("# My Skill\nContent\n")
	desiredDigest := integrity.DigestBytes(desiredContent)

	op1 := executor.FileOperation{
		OperationID: "op-1",
		TargetID:    "opencode",
		TargetPath:  targetFile,
		OpType:      "create",
		BaseDescriptor: executor.FileDescriptor{
			Path:   targetFile,
			Exists: false,
		},
		DesiredDescriptor: executor.FileDescriptor{
			Path:   targetFile,
			Digest: desiredDigest,
			Size:   int64(len(desiredContent)),
			Exists: true,
		},
		DesiredContent: desiredContent,
		OwnershipMarker: resourcehub.OwnershipManifest{
			ResourceID: "global/Skill/my-skill",
			ArtifactID: "rev-1",
		},
		BackupRequired: true,
	}

	receipt, err := exec.Execute("opencode", "plan-digest-1", []executor.FileOperation{op1})
	if err != nil {
		t.Fatalf("executor execute failed: %v", err)
	}
	if !receipt.Success {
		t.Fatalf("expected receipt success")
	}
	if len(receipt.Operations) != 1 {
		t.Errorf("expected 1 operation, got %d", len(receipt.Operations))
	}

	// Verify file content
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read created target file: %v", err)
	}
	if string(data) != string(desiredContent) {
		t.Errorf("content mismatch: got %s", string(data))
	}

	// Verify ownership marker (.asm-managed)
	markerPath := filepath.Join(filepath.Dir(targetFile), ".asm-managed")
	if _, err := os.Stat(markerPath); err != nil {
		t.Errorf("expected ownership marker at %s: %v", markerPath, err)
	}
}

func TestDeploymentExecutorPreflightDriftRejectionAndRollback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-drift-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	backupRoot := filepath.Join(tmpDir, "backups")
	targetFile := filepath.Join(tmpDir, "skills", "my-skill", "SKILL.md")

	if err := os.MkdirAll(filepath.Dir(targetFile), 0o755); err != nil {
		t.Fatalf("mkdirall: %v", err)
	}

	initialContent := []byte("# Initial\n")
	if err := os.WriteFile(targetFile, initialContent, 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	// Simulate drift: file content on disk differs from base descriptor digest
	exec := executor.NewDeploymentExecutor(backupRoot)

	op1 := executor.FileOperation{
		OperationID: "op-drift",
		TargetID:    "opencode",
		TargetPath:  targetFile,
		OpType:      "update",
		BaseDescriptor: executor.FileDescriptor{
			Path:   targetFile,
			Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", // stale/wrong base digest
			Exists: true,
		},
		DesiredDescriptor: executor.FileDescriptor{
			Path:   targetFile,
			Digest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			Exists: true,
		},
		DesiredContent: []byte("# New Content\n"),
		BackupRequired: true,
	}

	receipt, err := exec.Execute("opencode", "plan-drift", []executor.FileOperation{op1})
	if err == nil {
		t.Fatalf("expected preflight drift rejection error, got nil")
	}
	if receipt.Success {
		t.Fatalf("expected receipt success=false")
	}

	// Verify original file content was preserved on disk
	data, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read target file after drift rejection: %v", err)
	}
	if string(data) != string(initialContent) {
		t.Errorf("file was mutated despite drift rejection! got %s", string(data))
	}
}

func TestBackupAndRestore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-backup-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "test.txt")
	backupDir := filepath.Join(tmpDir, "backups")

	originalContent := []byte("hello world")
	if err := os.WriteFile(targetFile, originalContent, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	bak, err := executor.BackupTarget(targetFile, backupDir)
	if err != nil {
		t.Fatalf("backup target: %v", err)
	}
	if !bak.Existed {
		t.Fatalf("expected bak.Existed=true")
	}

	// Mutate original file
	if err := os.WriteFile(targetFile, []byte("mutated!"), 0o644); err != nil {
		t.Fatalf("mutate file: %v", err)
	}

	// Restore from backup
	if err := executor.RestoreBackup(bak); err != nil {
		t.Fatalf("restore backup: %v", err)
	}

	// Verify restored content
	restored, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(restored) != string(originalContent) {
		t.Errorf("expected restored content %s, got %s", string(originalContent), string(restored))
	}
}
