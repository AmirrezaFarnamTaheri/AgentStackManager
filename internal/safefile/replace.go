package safefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Replace atomically installs source at destination when the operating system
// permits it. On platforms where rename cannot replace an existing file, the
// old destination is staged in the same directory and restored if installing
// the new file fails.
func Replace(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if _, err := os.Stat(destination); errors.Is(err, os.ErrNotExist) {
		return os.Rename(source, destination)
	} else if err != nil {
		return err
	}

	backup, err := reserveBackupPath(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("reserve replacement backup: %w", err)
	}
	if err := os.Rename(destination, backup); err != nil {
		return fmt.Errorf("stage existing destination: %w", err)
	}
	if err := os.Rename(source, destination); err != nil {
		rollbackErr := os.Rename(backup, destination)
		if rollbackErr != nil {
			return fmt.Errorf("install replacement: %w; rollback failed: %v; previous file remains at %s", err, rollbackErr, backup)
		}
		return fmt.Errorf("install replacement: %w; previous destination restored", err)
	}
	if err := os.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replacement succeeded but cleanup of staged destination failed at %s: %w", backup, err)
	}
	return nil
}

func reserveBackupPath(dir string) (string, error) {
	file, err := os.CreateTemp(dir, ".agentstack-backup-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}
