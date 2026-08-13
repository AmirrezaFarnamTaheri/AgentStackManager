package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/safefile"
)

type DeploymentExecutor struct {
	mu         sync.Mutex
	targetLocks map[string]*sync.Mutex
	backupRoot  string
}

func NewDeploymentExecutor(backupRoot string) *DeploymentExecutor {
	return &DeploymentExecutor{
		targetLocks: make(map[string]*sync.Mutex),
		backupRoot:  backupRoot,
	}
}

func (e *DeploymentExecutor) getTargetLock(targetID string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()

	lock, exists := e.targetLocks[targetID]
	if !exists {
		lock = &sync.Mutex{}
		e.targetLocks[targetID] = lock
	}
	return lock
}

// Execute applies a batch of file operations under per-target locking, revalidation, backup, and atomic replace.
func (e *DeploymentExecutor) Execute(targetID string, planDigest string, ops []FileOperation) (DeploymentReceipt, error) {
	targetLock := e.getTargetLock(targetID)
	targetLock.Lock()
	defer targetLock.Unlock()

	receipt := DeploymentReceipt{
		ReceiptID:  fmt.Sprintf("rcpt-%d", time.Now().UnixNano()),
		PlanDigest: planDigest,
		TargetID:   targetID,
		AppliedAt:  time.Now().UTC(),
		Success:    false,
	}

	var appliedBackups []BackupState

	for _, op := range ops {
		// Step 1: Preflight revalidate base digest immediately before write
		if err := e.revalidateBase(op); err != nil {
			receipt.ErrorReason = fmt.Sprintf("preflight revalidation failed for %s: %v", op.TargetPath, err)
			e.rollback(appliedBackups)
			receipt.ReceiptDigest = ComputeReceiptDigest(receipt)
			return receipt, fmt.Errorf("preflight revalidation failed: %w", err)
		}

		// Step 2: Backup target file
		bak, err := BackupTarget(op.TargetPath, filepath.Join(e.backupRoot, receipt.ReceiptID))
		if err != nil {
			receipt.ErrorReason = fmt.Sprintf("backup failed for %s: %v", op.TargetPath, err)
			e.rollback(appliedBackups)
			receipt.ReceiptDigest = ComputeReceiptDigest(receipt)
			return receipt, fmt.Errorf("backup failed: %w", err)
		}
		appliedBackups = append(appliedBackups, bak)

		// Step 3: Apply mutation (atomic write or delete)
		if err := e.applyOperation(op); err != nil {
			receipt.ErrorReason = fmt.Sprintf("apply operation %s failed for %s: %v", op.OpType, op.TargetPath, err)
			e.rollback(appliedBackups)
			receipt.ReceiptDigest = ComputeReceiptDigest(receipt)
			return receipt, fmt.Errorf("apply operation failed: %w", err)
		}

		// Step 4: Write ownership marker if creating/updating
		if op.OpType != "delete" {
			targetDir := filepath.Dir(op.TargetPath)
			now := time.Now().UTC()
			marker, err := resourcehub.CreateOwnershipMarker(op.OwnershipMarker.ResourceID, op.OwnershipMarker.ArtifactID, op.DesiredDescriptor.Digest, targetID, targetDir, planDigest, now)
			if err == nil {
				_ = resourcehub.WriteOwnershipMarker(targetDir, marker)
			}
		}

		receipt.Operations = append(receipt.Operations, fmt.Sprintf("%s:%s:%s", op.OpType, op.TargetPath, op.DesiredDescriptor.Digest))
	}

	receipt.BackupStates = appliedBackups
	receipt.Success = true
	receipt.ReceiptDigest = ComputeReceiptDigest(receipt)
	return receipt, nil
}

func (e *DeploymentExecutor) revalidateBase(op FileOperation) error {
	info, err := os.Stat(op.TargetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if op.BaseDescriptor.Exists {
				return fmt.Errorf("target file disappeared, expected digest %s", op.BaseDescriptor.Digest)
			}
			return nil
		}
		return fmt.Errorf("stat target file: %w", err)
	}

	if !op.BaseDescriptor.Exists {
		return fmt.Errorf("target file already exists unexpectedly")
	}

	data, err := safefile.ReadBoundedRegular(op.TargetPath, 10<<20)
	if err != nil {
		return fmt.Errorf("read target file for preflight: %w", err)
	}

	actualDigest := integrity.DigestBytes(data)
	if !strings.EqualFold(actualDigest, op.BaseDescriptor.Digest) {
		return fmt.Errorf("preflight drift detected: base digest %s vs actual digest %s", op.BaseDescriptor.Digest, actualDigest)
	}

	_ = info
	return nil
}

func (e *DeploymentExecutor) applyOperation(op FileOperation) error {
	if op.OpType == "delete" {
		if err := os.Remove(op.TargetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete target file: %w", err)
		}
		return nil
	}

	targetDir := filepath.Dir(op.TargetPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("mkdir target parent dir: %w", err)
	}

	tmpFile := op.TargetPath + ".tmp-asm"
	if err := os.WriteFile(tmpFile, op.DesiredContent, 0o644); err != nil {
		return fmt.Errorf("write temp target file: %w", err)
	}

	if err := os.Rename(tmpFile, op.TargetPath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("atomic rename target file: %w", err)
	}

	return nil
}

func (e *DeploymentExecutor) rollback(backups []BackupState) {
	for i := len(backups) - 1; i >= 0; i-- {
		_ = RestoreBackup(backups[i])
	}
}
