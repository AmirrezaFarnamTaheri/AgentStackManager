package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (m Manager) AddArtifact(workspaceID, source string, options ArtifactOptions) (Artifact, error) {
	item, err := m.Get(workspaceID)
	if err != nil {
		return Artifact{}, err
	}
	if item.Type != TypeWorkspace {
		return Artifact{}, fmt.Errorf("item %q is not a workspace", workspaceID)
	}
	if !validID(options.ID) {
		return Artifact{}, fmt.Errorf("artifact id is empty or invalid")
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return Artifact{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return Artifact{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return Artifact{}, fmt.Errorf("artifact source must be a regular file")
	}
	if info.Size() > maxArtifactBytes {
		return Artifact{}, fmt.Errorf("artifact exceeds %d bytes", maxArtifactBytes)
	}
	artifacts, err := m.loadArtifacts()
	if err != nil {
		return Artifact{}, err
	}
	previous, exists := artifacts[options.ID]
	if exists && !options.Replace {
		return Artifact{}, fmt.Errorf("artifact %q already exists", options.ID)
	}
	dir := filepath.Join(m.Root, "artifacts", workspaceID, options.ID)
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return Artifact{}, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dir), ".artifact-*")
	if err != nil {
		return Artifact{}, err
	}
	defer os.RemoveAll(stage)
	target := filepath.Join(stage, filepath.Base(absolute))
	digest, size, err := copyArtifactFile(absolute, target, info.Mode().Perm())
	if err != nil {
		return Artifact{}, err
	}

	backup := ""
	if exists {
		oldDir := filepath.Dir(previous.Path)
		backup = nextSiblingPath(oldDir + ".rollback-" + m.now().Format("20060102T150405.000000000Z"))
		if err := os.Rename(oldDir, backup); err != nil {
			return Artifact{}, err
		}
	}
	installed := false
	rollback := func() error {
		var rollbackErr error
		if installed {
			if err := os.RemoveAll(dir); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		if backup != "" {
			if err := os.Rename(backup, filepath.Dir(previous.Path)); err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
			}
		}
		return rollbackErr
	}
	if err := os.Rename(stage, dir); err != nil {
		if backup != "" {
			return Artifact{}, errors.Join(err, os.Rename(backup, filepath.Dir(previous.Path)))
		}
		return Artifact{}, err
	}
	installed = true
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = filepath.Base(absolute)
	}
	artifact := Artifact{ID: options.ID, WorkspaceID: workspaceID, Name: name, Path: filepath.Join(dir, filepath.Base(absolute)), FileName: filepath.Base(absolute), MediaType: strings.TrimSpace(options.MediaType), SHA256: digest, Size: size, CreatedAt: m.now(), Metadata: cloneMap(options.Metadata)}
	artifacts[artifact.ID] = artifact
	if err := m.saveArtifacts(artifacts); err != nil {
		return Artifact{}, errors.Join(err, rollback())
	}
	if backup != "" {
		m.cleanupCommitted(backup)
	}
	return artifact, nil
}
func (m Manager) VerifyArtifact(id string) (bool, error) {
	artifacts, err := m.loadArtifacts()
	if err != nil {
		return false, err
	}
	artifact, ok := artifacts[id]
	if !ok {
		return false, fmt.Errorf("unknown artifact %q", id)
	}
	digest, size, err := hashArtifact(artifact.Path)
	if err != nil {
		return false, err
	}
	return digest == artifact.SHA256 && size == artifact.Size, nil
}
func (m Manager) ListArtifacts(workspaceID string) ([]Artifact, error) {
	artifacts, err := m.loadArtifacts()
	if err != nil {
		return nil, err
	}
	var result []Artifact
	for _, artifact := range artifacts {
		if workspaceID == "" || artifact.WorkspaceID == workspaceID {
			result = append(result, artifact)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}
func (m Manager) RemoveArtifact(id string) error {
	artifacts, err := m.loadArtifacts()
	if err != nil {
		return err
	}
	artifact, ok := artifacts[id]
	if !ok {
		return nil
	}
	dir := filepath.Dir(artifact.Path)
	quarantine := nextSiblingPath(dir + ".removed-" + m.now().Format("20060102T150405.000000000Z"))
	if err := os.Rename(dir, quarantine); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	moved := true
	if _, err := os.Lstat(quarantine); errors.Is(err, os.ErrNotExist) {
		moved = false
	}
	delete(artifacts, id)
	if err := m.saveArtifacts(artifacts); err != nil {
		if moved {
			return errors.Join(err, os.Rename(quarantine, dir))
		}
		return err
	}
	if moved {
		m.cleanupCommitted(quarantine)
	}
	return nil
}
func copyArtifactFile(source, destination string, mode os.FileMode) (string, int64, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return "", 0, err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", 0, err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(output, h), io.LimitReader(input, maxArtifactBytes+1))
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return "", 0, copyErr
	}
	if size > maxArtifactBytes {
		return "", 0, fmt.Errorf("artifact exceeds %d bytes", maxArtifactBytes)
	}
	if syncErr != nil {
		return "", 0, syncErr
	}
	if closeErr != nil {
		return "", 0, closeErr
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), size, nil
}
func hashArtifact(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	h := sha256.New()
	size, err := io.Copy(h, file)
	if err != nil {
		return "", 0, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), size, nil
}

func nextSiblingPath(base string) string {
	for suffix := 0; ; suffix++ {
		candidate := base
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}
