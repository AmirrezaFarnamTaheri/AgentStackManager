package sqlite

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type databasePathState struct {
	path    string
	info    os.FileInfo
	created bool
}

type backupDestination struct {
	destination   string
	temporary     string
	temporaryInfo os.FileInfo
}

func prepareDatabasePath(path string, create bool) (databasePathState, error) {
	if strings.TrimSpace(path) == "" {
		return databasePathState{}, fmt.Errorf("SQLite metadata path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return databasePathState{}, err
	}
	if err := ensureSecureDirectory(filepath.Dir(absolute), create); err != nil {
		return databasePathState{}, err
	}
	info, err := os.Lstat(absolute)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return databasePathState{}, fmt.Errorf("SQLite metadata requires a regular non-symlink file: %s", absolute)
		}
		if info.Size() > maxDatabaseBytes {
			return databasePathState{}, fmt.Errorf("SQLite metadata exceeds %d bytes: %s", maxDatabaseBytes, absolute)
		}
		return databasePathState{path: absolute, info: info}, nil
	}
	if !os.IsNotExist(err) {
		return databasePathState{}, err
	}
	if !create {
		return databasePathState{}, err
	}
	file, err := os.OpenFile(absolute, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return databasePathState{}, fmt.Errorf("create SQLite metadata: %w", err)
	}
	closeErr := file.Close()
	if closeErr != nil {
		_ = os.Remove(absolute)
		return databasePathState{}, closeErr
	}
	info, err = os.Lstat(absolute)
	if err != nil {
		return databasePathState{}, err
	}
	return databasePathState{path: absolute, info: info, created: true}, nil
}

func confirmDatabaseIdentity(state databasePathState) error {
	current, err := os.Stat(state.path)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(state.info, current) {
		return fmt.Errorf("SQLite metadata file changed during open: %s", state.path)
	}
	if current.Size() > maxDatabaseBytes {
		return fmt.Errorf("SQLite metadata exceeds %d bytes: %s", maxDatabaseBytes, state.path)
	}
	return nil
}

func removeCreatedDatabase(state databasePathState) error {
	current, err := os.Lstat(state.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if current.Mode().IsRegular() && os.SameFile(state.info, current) {
		_ = os.Remove(state.path + "-wal")
		_ = os.Remove(state.path + "-shm")
		return os.Remove(state.path)
	}
	return nil
}

func secureDatabaseFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		info, err := os.Lstat(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("SQLite state file is not a regular non-symlink file: %s", candidate)
		}
		if err := os.Chmod(candidate, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func ensureSecureDirectory(path string, create bool) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	rest := strings.TrimPrefix(absolute, volume)
	parts := strings.FieldsFunc(rest, func(r rune) bool { return r == '/' || r == '\\' })
	current := volume
	if filepath.IsAbs(absolute) {
		current += string(os.PathSeparator)
	}
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && create {
			if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
				return fmt.Errorf("create SQLite metadata directory %s: %w", current, err)
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("SQLite metadata path contains a non-directory or symlink: %s", current)
		}
	}
	return nil
}

func prepareBackupDestination(path string) (backupDestination, error) {
	if strings.TrimSpace(path) == "" {
		return backupDestination{}, fmt.Errorf("SQLite backup destination is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return backupDestination{}, err
	}
	if err := ensureSecureDirectory(filepath.Dir(absolute), true); err != nil {
		return backupDestination{}, err
	}
	if _, err := os.Lstat(absolute); err == nil {
		return backupDestination{}, fmt.Errorf("SQLite backup destination already exists: %s", absolute)
	} else if !os.IsNotExist(err) {
		return backupDestination{}, err
	}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return backupDestination{}, err
	}
	temporary := filepath.Join(filepath.Dir(absolute), "."+filepath.Base(absolute)+".incomplete-"+hex.EncodeToString(random))
	file, err := os.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return backupDestination{}, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(temporary)
		return backupDestination{}, err
	}
	info, err := os.Lstat(temporary)
	if err != nil {
		_ = os.Remove(temporary)
		return backupDestination{}, err
	}
	return backupDestination{destination: absolute, temporary: temporary, temporaryInfo: info}, nil
}

func publishNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		if _, statErr := os.Lstat(destination); statErr == nil {
			return fmt.Errorf("SQLite backup destination already exists: %s", destination)
		}
		return fmt.Errorf("publish SQLite backup: %w", err)
	}
	// source was created and verified with mode 0600; a hard link preserves the
	// same inode and permissions. Avoid any fallible post-publication mutation.
	return nil
}
