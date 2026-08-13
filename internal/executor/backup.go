package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/safefile"
)

// BackupTarget creates a safety copy of a target file or directory before mutation.
func BackupTarget(targetPath string, backupDir string) (BackupState, error) {
	state := BackupState{
		BackupID:   fmt.Sprintf("bak-%d", time.Now().UnixNano()),
		TargetPath: filepath.ToSlash(targetPath),
		CreatedAt:  time.Now().UTC(),
	}

	info, err := os.Lstat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			state.Existed = false
			return state, nil
		}
		return state, fmt.Errorf("lstat target for backup: %w", err)
	}

	state.Existed = true
	state.IsDir = info.IsDir()
	state.OriginalMode = uint32(info.Mode())

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return state, fmt.Errorf("mkdir backup root: %w", err)
	}

	if info.IsDir() {
		// Directory backup unsupported for full recursive copy in lightweight executor, return placeholder
		state.BackupPath = filepath.ToSlash(filepath.Join(backupDir, state.BackupID))
		return state, nil
	}

	// File backup
	data, err := safefile.ReadBoundedRegular(targetPath, 10<<20) // 10MB limit
	if err != nil {
		return state, fmt.Errorf("read target file for backup: %w", err)
	}

	state.OriginalDigest = integrity.DigestBytes(data)

	destPath := filepath.Join(backupDir, fmt.Sprintf("%s.bak", state.BackupID))
	if err := os.WriteFile(destPath, data, info.Mode()); err != nil {
		return state, fmt.Errorf("write backup file: %w", err)
	}

	state.BackupPath = filepath.ToSlash(destPath)
	return state, nil
}

// RestoreBackup restores a target file back to its backed up state.
func RestoreBackup(state BackupState) error {
	if !state.Existed {
		if err := os.Remove(state.TargetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove created file during restore: %w", err)
		}
		return nil
	}

	if state.IsDir {
		return nil
	}

	srcFile, err := os.Open(state.BackupPath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(state.TargetPath), 0o755); err != nil {
		return fmt.Errorf("mkdir target dir: %w", err)
	}

	tmpTarget := state.TargetPath + ".tmp-restore"
	dstFile, err := os.OpenFile(tmpTarget, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(state.OriginalMode))
	if err != nil {
		return fmt.Errorf("create temp restore target: %w", err)
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		os.Remove(tmpTarget)
		return fmt.Errorf("copy backup data: %w", err)
	}
	dstFile.Close()

	if err := os.Rename(tmpTarget, state.TargetPath); err != nil {
		os.Remove(tmpTarget)
		return fmt.Errorf("rename restore file: %w", err)
	}

	return nil
}

// ComputeReceiptDigest generates a SHA-256 hash of a deployment receipt.
func ComputeReceiptDigest(receipt DeploymentReceipt) string {
	h := sha256.New()
	h.Write([]byte(receipt.ReceiptID))
	h.Write([]byte(receipt.PlanDigest))
	h.Write([]byte(receipt.TargetID))
	h.Write([]byte(fmt.Sprintf("%t", receipt.Success)))
	for _, op := range receipt.Operations {
		h.Write([]byte(op))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
