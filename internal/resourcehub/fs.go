package resourcehub

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/safefile"
)

const (
	maxResourceFiles      = 10_000
	maxResourceFileBytes  = 16 << 20
	maxResourceTotalBytes = 64 << 20
)

var ignoredNames = map[string]struct{}{
	".git": {}, "node_modules": {}, "dist": {}, "build": {}, "target": {},
	"__pycache__": {}, ".pytest_cache": {}, ".mypy_cache": {}, ".ruff_cache": {},
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return value != "." && value != ".."
}

func validateKind(kind Kind) error {
	switch kind {
	case KindSkill, KindAgent, KindRule, KindCommand, KindPrompt, KindMCPServer, KindContext:
		return nil
	default:
		return fmt.Errorf("unsupported resource kind %q", kind)
	}
}

func validateAgent(agent Agent) error {
	switch agent {
	case AgentCodex, AgentClaude, AgentCursor, AgentOpenCode, AgentCopilot, AgentGeneric:
		return nil
	default:
		return fmt.Errorf("unsupported agent target %q", agent)
	}
}

func validateMode(mode SyncMode) error {
	switch mode {
	case "", ModeAuto, ModeCopy, ModeLink:
		return nil
	default:
		return fmt.Errorf("unsupported sync mode %q", mode)
	}
}

func treeDigest(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(root)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, "L\x00"+target)
		return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
	}
	if !info.IsDir() {
		if info.Size() > maxResourceFileBytes {
			return "", fmt.Errorf("resource file exceeds %d bytes: %s", maxResourceFileBytes, root)
		}
		_, _ = io.WriteString(h, "F\x00"+filepath.Base(root)+"\x00")
		if err := hashFileInto(h, root); err != nil {
			return "", err
		}
		return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
	}
	var paths []string
	var totalBytes int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if _, ignored := ignoredNames[entry.Name()]; ignored && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("resource contains symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if len(paths) >= maxResourceFiles {
			return fmt.Errorf("resource contains more than %d files", maxResourceFiles)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxResourceFileBytes {
			return fmt.Errorf("resource file exceeds %d bytes: %s", maxResourceFileBytes, path)
		}
		totalBytes += info.Size()
		if totalBytes > maxResourceTotalBytes {
			return fmt.Errorf("resource exceeds %d total bytes", maxResourceTotalBytes)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	for _, rel := range paths {
		clean := filepath.Clean(filepath.FromSlash(rel))
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
			return "", fmt.Errorf("unsafe relative path %q", rel)
		}
		_, _ = io.WriteString(h, "F\x00"+rel+"\x00")
		if err := hashFileInto(h, filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func hashFileInto(writer io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	return err
}

func copyResource(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink source: %s", source)
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".agentstack-sync-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, filepath.Base(destination))
	if info.IsDir() {
		if err := copyTree(source, staged); err != nil {
			return err
		}
	} else {
		if err := copyFile(source, staged, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return replacePath(staged, destination)
}

func copyTree(source, destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	files := 0
	var totalBytes int64
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		if _, ignored := ignoredNames[entry.Name()]; ignored && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("resource contains symlink: %s", path)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		files++
		if files > maxResourceFiles {
			return fmt.Errorf("resource contains more than %d files", maxResourceFiles)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxResourceFileBytes {
			return fmt.Errorf("resource file exceeds %d bytes: %s", maxResourceFileBytes, path)
		}
		totalBytes += info.Size()
		if totalBytes > maxResourceTotalBytes {
			return fmt.Errorf("resource exceeds %d total bytes", maxResourceTotalBytes)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, io.LimitReader(input, maxResourceFileBytes+1))
	if copyErr == nil && written > maxResourceFileBytes {
		copyErr = fmt.Errorf("resource file exceeds %d bytes: %s", maxResourceFileBytes, source)
	}
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func replacePath(source, destination string) error {
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return os.Rename(source, destination)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return safefile.Replace(source, destination)
	}
	backup := destination + ".agentstack-backup"
	for i := 0; ; i++ {
		candidate := backup
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", backup, i)
		}
		if _, statErr := os.Lstat(candidate); errors.Is(statErr, os.ErrNotExist) {
			backup = candidate
			break
		}
	}
	if err := os.Rename(destination, backup); err != nil {
		return fmt.Errorf("stage existing target: %w", err)
	}
	if err := os.Rename(source, destination); err != nil {
		_ = os.Rename(backup, destination)
		return fmt.Errorf("install synced target: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("sync succeeded but cleanup failed: %w", err)
	}
	return nil
}

func linkResource(source, destination string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("directory links require privileges on Windows")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	stage := destination + ".agentstack-link"
	_ = os.Remove(stage)
	if err := os.Symlink(source, stage); err != nil {
		return err
	}
	return replacePath(stage, destination)
}

func removeManagedPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}
