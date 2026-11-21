// Package cas provides ASM's local immutable content-addressed object store.
// It stores raw file blobs and deterministic flat tree manifests without
// changing the authority of the Resource Hub or any target configuration.
package cas

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const APIVersion = "fabric.asm.dev/cas/v1alpha1"

const (
	maxBlobBytes     = 16 << 20
	maxTreeBytes     = 64 << 20
	maxTreeFiles     = 10_000
	maxManifestBytes = 8 << 20
)

type Kind string

const (
	KindBlob Kind = "blob"
	KindTree Kind = "tree"
)

type EntryKind string

const (
	EntryDirectory EntryKind = "directory"
	EntryFile      EntryKind = "file"
)

// Ref is an immutable object identity. Size is the raw blob size for blobs and
// the aggregate file payload size for trees.
type Ref struct {
	Kind   Kind   `json:"kind"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

func (r Ref) URI() string {
	return "cas://" + string(r.Kind) + "/sha256/" + strings.TrimPrefix(r.Digest, "sha256:")
}

type TreeEntry struct {
	Path string    `json:"path"`
	Kind EntryKind `json:"kind"`
	Mode uint32    `json:"mode"`
	Size int64     `json:"size,omitempty"`
	Blob Ref       `json:"blob,omitempty"`
}

type TreeManifest struct {
	APIVersion string      `json:"apiVersion"`
	Entries    []TreeEntry `json:"entries"`
}

type Store struct {
	Root string
}

// ValidateRef checks the structural bounds of an immutable object reference
// without reading the store. Call Verify when object presence and content
// integrity must also be established.
func ValidateRef(ref Ref) error {
	return validateRef(ref)
}

func New(root string) Store {
	if strings.TrimSpace(root) == "" {
		return Store{}
	}
	return Store{Root: filepath.Clean(root)}
}

func (s Store) PutBlob(data []byte) (Ref, error) {
	if int64(len(data)) > maxBlobBytes {
		return Ref{}, fmt.Errorf("CAS blob exceeds %d bytes", maxBlobBytes)
	}
	ref := Ref{Kind: KindBlob, Digest: digestBytes(data), Size: int64(len(data))}
	if err := s.writeObject(ref, bytes.NewReader(data)); err != nil {
		return Ref{}, err
	}
	return ref, nil
}

// PutTree stores a regular directory tree. Symlinks, devices, sockets and
// other special files are rejected. Empty directories and POSIX permission
// bits are preserved in the deterministic manifest.
func (s Store) PutTree(root string) (Ref, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return Ref{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Ref{}, fmt.Errorf("CAS tree source must be a non-symlink directory: %s", root)
	}

	entries := make([]TreeEntry, 0)
	var total int64
	files := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("CAS tree contains symlink: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := validateRelativePath(rel); err != nil {
			return err
		}
		mode := uint32(info.Mode().Perm())
		if entry.IsDir() {
			entries = append(entries, TreeEntry{Path: rel, Kind: EntryDirectory, Mode: mode})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("CAS tree contains unsupported special file: %s", path)
		}
		files++
		if files > maxTreeFiles {
			return fmt.Errorf("CAS tree contains more than %d files", maxTreeFiles)
		}
		if info.Size() > maxBlobBytes {
			return fmt.Errorf("CAS tree file exceeds %d bytes: %s", maxBlobBytes, path)
		}
		total += info.Size()
		if total > maxTreeBytes {
			return fmt.Errorf("CAS tree exceeds %d total bytes", maxTreeBytes)
		}
		data, err := safefile.ReadBoundedRegular(path, maxBlobBytes)
		if err != nil {
			return err
		}
		blob, err := s.PutBlob(data)
		if err != nil {
			return fmt.Errorf("store CAS blob %s: %w", rel, err)
		}
		entries = append(entries, TreeEntry{Path: rel, Kind: EntryFile, Mode: mode, Size: info.Size(), Blob: blob})
		return nil
	})
	if err != nil {
		return Ref{}, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	manifest := TreeManifest{APIVersion: APIVersion, Entries: entries}
	if err := validateManifest(manifest); err != nil {
		return Ref{}, err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return Ref{}, fmt.Errorf("marshal CAS tree manifest: %w", err)
	}
	if int64(len(data)) > maxManifestBytes {
		return Ref{}, fmt.Errorf("CAS tree manifest exceeds %d bytes", maxManifestBytes)
	}
	ref := Ref{Kind: KindTree, Digest: digestBytes(data), Size: total}
	if err := s.writeObject(ref, bytes.NewReader(data)); err != nil {
		return Ref{}, err
	}
	return ref, nil
}

func (s Store) Verify(ref Ref) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	if err := s.require(); err != nil {
		return err
	}
	if err := requireDirectory(filepath.Dir(s.objectPath(ref))); err != nil {
		return err
	}
	switch ref.Kind {
	case KindBlob:
		data, err := safefile.ReadBoundedRegular(s.objectPath(ref), maxBlobBytes)
		if err != nil {
			return fmt.Errorf("read CAS blob %s: %w", ref.Digest, err)
		}
		if int64(len(data)) != ref.Size {
			return fmt.Errorf("CAS blob %s size mismatch", ref.Digest)
		}
		if digestBytes(data) != ref.Digest {
			return fmt.Errorf("CAS blob %s digest mismatch", ref.Digest)
		}
		return nil
	case KindTree:
		manifest, err := s.loadTree(ref)
		if err != nil {
			return err
		}
		var total int64
		for _, entry := range manifest.Entries {
			if entry.Kind != EntryFile {
				continue
			}
			if err := s.Verify(entry.Blob); err != nil {
				return fmt.Errorf("verify CAS tree entry %q: %w", entry.Path, err)
			}
			total += entry.Size
		}
		if total != ref.Size {
			return fmt.Errorf("CAS tree %s size mismatch", ref.Digest)
		}
		return nil
	default:
		return fmt.Errorf("unsupported CAS object kind %q", ref.Kind)
	}
}

// Reachable returns the deterministic closure used as a future garbage-
// collection mark set. No deletion is performed by this package.
func (s Store) Reachable(root Ref) ([]Ref, error) {
	if err := s.Verify(root); err != nil {
		return nil, err
	}
	seen := map[string]Ref{}
	var visit func(Ref) error
	visit = func(ref Ref) error {
		key := string(ref.Kind) + "\x00" + ref.Digest
		if _, ok := seen[key]; ok {
			return nil
		}
		seen[key] = ref
		if ref.Kind != KindTree {
			return nil
		}
		manifest, err := s.loadTree(ref)
		if err != nil {
			return err
		}
		for _, entry := range manifest.Entries {
			if entry.Kind == EntryFile {
				if err := visit(entry.Blob); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	result := make([]Ref, 0, len(seen))
	for _, ref := range seen {
		result = append(result, ref)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Digest < result[j].Digest
	})
	return result, nil
}

// Materialize reconstructs one immutable object at a new destination. It never
// replaces or merges an existing path. Tree destinations are published through
// exclusive directory and entry creation rather than rename, because os.Rename
// can replace a concurrently-created destination on Unix. If a tree restore
// fails after reserving its destination, the partial directory is retained with
// an incomplete marker so ASM never deletes content that may have appeared
// concurrently.
func (s Store) Materialize(ref Ref, destination string) error {
	if err := s.Verify(ref); err != nil {
		return err
	}
	parent := filepath.Dir(destination)
	if err := ensureDirectory(parent); err != nil {
		return err
	}
	switch ref.Kind {
	case KindBlob:
		data, err := safefile.ReadBoundedRegular(s.objectPath(ref), maxBlobBytes)
		if err != nil {
			return err
		}
		if err := writeNewFile(destination, data, 0o600); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("CAS materialization destination already exists: %s", destination)
			}
			return err
		}
		return nil
	case KindTree:
		return s.materializeTree(ref, destination)
	default:
		return fmt.Errorf("unsupported CAS object kind %q", ref.Kind)
	}
}

func (s Store) materializeTree(ref Ref, destination string) error {
	manifest, err := s.loadTree(ref)
	if err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("CAS materialization destination already exists: %s", destination)
		}
		return err
	}
	marker, err := os.CreateTemp(destination, ".agentstack-restore-incomplete-*")
	if err != nil {
		return fmt.Errorf("initialize CAS restore; incomplete destination retained at %s: %w", destination, err)
	}
	markerPath := marker.Name()
	if _, err := marker.WriteString("agentstack CAS restore incomplete\n"); err != nil {
		_ = marker.Close()
		return fmt.Errorf("write CAS restore marker; incomplete destination retained at %s: %w", destination, err)
	}
	if err := marker.Sync(); err != nil {
		_ = marker.Close()
		return fmt.Errorf("sync CAS restore marker; incomplete destination retained at %s: %w", destination, err)
	}
	if err := marker.Close(); err != nil {
		return fmt.Errorf("close CAS restore marker; incomplete destination retained at %s: %w", destination, err)
	}

	if err := s.populateTree(manifest, destination); err != nil {
		return fmt.Errorf("%w; incomplete destination retained at %s", err, destination)
	}
	if err := os.Remove(markerPath); err != nil {
		return fmt.Errorf("remove CAS restore marker; completed content retained at %s: %w", destination, err)
	}
	return nil
}

func (s Store) populateTree(manifest TreeManifest, destination string) error {
	for _, entry := range manifest.Entries {
		target := filepath.Join(destination, filepath.FromSlash(entry.Path))
		if !pathWithin(destination, target) {
			return fmt.Errorf("unsafe CAS materialization path %q", entry.Path)
		}
		if entry.Kind == EntryDirectory {
			if err := os.Mkdir(target, 0o700); err != nil {
				return fmt.Errorf("create CAS restore directory %q: %w", entry.Path, err)
			}
			continue
		}
		data, err := safefile.ReadBoundedRegular(s.objectPath(entry.Blob), maxBlobBytes)
		if err != nil {
			return fmt.Errorf("read CAS restore file %q: %w", entry.Path, err)
		}
		if err := writeNewFile(target, data, os.FileMode(entry.Mode)); err != nil {
			return fmt.Errorf("create CAS restore file %q: %w", entry.Path, err)
		}
	}
	for index := len(manifest.Entries) - 1; index >= 0; index-- {
		entry := manifest.Entries[index]
		if entry.Kind == EntryDirectory {
			if err := os.Chmod(filepath.Join(destination, filepath.FromSlash(entry.Path)), os.FileMode(entry.Mode)); err != nil {
				return fmt.Errorf("apply CAS restore mode to %q: %w", entry.Path, err)
			}
		}
	}
	return nil
}

func (s Store) loadTree(ref Ref) (TreeManifest, error) {
	if ref.Kind != KindTree {
		return TreeManifest{}, fmt.Errorf("CAS object %s is not a tree", ref.Digest)
	}
	data, err := safefile.ReadBoundedRegular(s.objectPath(ref), maxManifestBytes)
	if err != nil {
		return TreeManifest{}, fmt.Errorf("read CAS tree %s: %w", ref.Digest, err)
	}
	if digestBytes(data) != ref.Digest {
		return TreeManifest{}, fmt.Errorf("CAS tree %s digest mismatch", ref.Digest)
	}
	var manifest TreeManifest
	if err := strictjson.Decode(data, &manifest); err != nil {
		return TreeManifest{}, fmt.Errorf("decode CAS tree %s: %w", ref.Digest, err)
	}
	if err := validateManifest(manifest); err != nil {
		return TreeManifest{}, err
	}
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return TreeManifest{}, err
	}
	if !bytes.Equal(canonical, data) {
		return TreeManifest{}, fmt.Errorf("CAS tree %s is not canonically encoded", ref.Digest)
	}
	return manifest, nil
}

func validateManifest(manifest TreeManifest) error {
	if manifest.APIVersion != APIVersion {
		return fmt.Errorf("unsupported CAS tree apiVersion %q", manifest.APIVersion)
	}
	if len(manifest.Entries) > maxTreeFiles*2 {
		return fmt.Errorf("CAS tree manifest has too many entries")
	}
	seen := map[string]EntryKind{}
	var previous string
	var total int64
	files := 0
	for index, entry := range manifest.Entries {
		if err := validateRelativePath(entry.Path); err != nil {
			return fmt.Errorf("unsafe tree path %q: %w", entry.Path, err)
		}
		if index > 0 && entry.Path <= previous {
			return fmt.Errorf("CAS tree entries are not strictly sorted at %q", entry.Path)
		}
		previous = entry.Path
		if entry.Mode > 0o777 {
			return fmt.Errorf("CAS tree entry %q has invalid mode", entry.Path)
		}
		parent := pathParent(entry.Path)
		if parent != "" {
			kind, ok := seen[parent]
			if !ok || kind != EntryDirectory {
				return fmt.Errorf("CAS tree entry %q has missing directory parent %q", entry.Path, parent)
			}
		}
		switch entry.Kind {
		case EntryDirectory:
			if entry.Size != 0 || entry.Blob != (Ref{}) {
				return fmt.Errorf("CAS directory entry %q contains file data", entry.Path)
			}
		case EntryFile:
			files++
			if files > maxTreeFiles {
				return fmt.Errorf("CAS tree contains more than %d files", maxTreeFiles)
			}
			if entry.Size < 0 || entry.Size > maxBlobBytes || entry.Blob.Kind != KindBlob || entry.Blob.Size != entry.Size {
				return fmt.Errorf("CAS file entry %q has invalid blob reference", entry.Path)
			}
			if err := validateRef(entry.Blob); err != nil {
				return fmt.Errorf("CAS file entry %q: %w", entry.Path, err)
			}
			total += entry.Size
			if total > maxTreeBytes {
				return fmt.Errorf("CAS tree exceeds %d total bytes", maxTreeBytes)
			}
		default:
			return fmt.Errorf("CAS tree entry %q has invalid kind %q", entry.Path, entry.Kind)
		}
		seen[entry.Path] = entry.Kind
	}
	return nil
}

func validateRef(ref Ref) error {
	if ref.Kind != KindBlob && ref.Kind != KindTree {
		return fmt.Errorf("invalid CAS object kind %q", ref.Kind)
	}
	if !validDigest(ref.Digest) {
		return fmt.Errorf("invalid CAS digest %q", ref.Digest)
	}
	if ref.Size < 0 || (ref.Kind == KindBlob && ref.Size > maxBlobBytes) || (ref.Kind == KindTree && ref.Size > maxTreeBytes) {
		return fmt.Errorf("invalid CAS object size %d", ref.Size)
	}
	return nil
}

func (s Store) writeObject(ref Ref, reader io.Reader) error {
	if err := validateRef(ref); err != nil {
		return err
	}
	if err := s.ensure(); err != nil {
		return err
	}
	path := s.objectPath(ref)
	if err := ensureDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return s.Verify(ref)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.CreateTemp(filepath.Dir(path), ".agentstack-cas-object-*")
	if err != nil {
		return err
	}
	stagePath := stage.Name()
	cleanup := func() { _ = os.Remove(stagePath) }
	defer cleanup()
	limit := int64(maxBlobBytes)
	if ref.Kind == KindTree {
		limit = maxManifestBytes
	}
	written, copyErr := io.Copy(stage, io.LimitReader(reader, limit+1))
	if copyErr == nil && written > limit {
		copyErr = fmt.Errorf("CAS object exceeds %d bytes", limit)
	}
	if copyErr == nil {
		copyErr = stage.Sync()
	}
	closeErr := stage.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	data, err := safefile.ReadBoundedRegular(stagePath, limit)
	if err != nil {
		return err
	}
	if digestBytes(data) != ref.Digest {
		return fmt.Errorf("CAS object digest mismatch before install")
	}
	if err := os.Chmod(stagePath, 0o600); err != nil {
		return err
	}
	if err := os.Link(stagePath, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return s.Verify(ref)
		}
		return fmt.Errorf("publish CAS object without replacement: %w", err)
	}
	if err := os.Remove(stagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("CAS object published but temporary link cleanup failed: %w", err)
	}
	return s.Verify(ref)
}

func (s Store) require() error {
	if strings.TrimSpace(s.Root) == "" {
		return fmt.Errorf("CAS root is empty")
	}
	for _, path := range []string{s.Root, filepath.Join(s.Root, "objects"), filepath.Join(s.Root, "objects", "sha256")} {
		if err := requireDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func requireDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("CAS directory is a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("CAS path is not a directory: %s", path)
	}
	return nil
}

func (s Store) ensure() error {
	if strings.TrimSpace(s.Root) == "" {
		return fmt.Errorf("CAS root is empty")
	}
	for _, path := range []string{s.Root, filepath.Join(s.Root, "objects"), filepath.Join(s.Root, "objects", "sha256")} {
		if err := ensureDirectory(path); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("CAS directory is a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("CAS path is not a directory: %s", path)
	}
	return nil
}

func (s Store) objectPath(ref Ref) string {
	hexDigest := strings.TrimPrefix(ref.Digest, "sha256:")
	extension := ".blob"
	if ref.Kind == KindTree {
		extension = ".tree.json"
	}
	return filepath.Join(s.Root, "objects", "sha256", hexDigest[:2], hexDigest[2:]+extension)
}

func writeNewFile(path string, data []byte, mode os.FileMode) error {
	if err := ensureDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".agentstack-cas-materialize-*")
	if err != nil {
		return err
	}
	stagePath := file.Name()
	defer os.Remove(stagePath)
	_, writeErr := file.Write(data)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(stagePath, mode.Perm()); err != nil {
		return err
	}
	if err := os.Link(stagePath, path); err != nil {
		return err
	}
	if err := os.Remove(stagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("materialized file published but temporary link cleanup failed: %w", err)
	}
	return nil
}

func validateRelativePath(value string) error {
	if value == "" || value == "." || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "/") {
		return fmt.Errorf("invalid relative path")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean != value || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(filepath.FromSlash(value)) {
		return fmt.Errorf("path escapes tree root")
	}
	return nil
}

func pathParent(value string) string {
	index := strings.LastIndexByte(value, '/')
	if index < 0 {
		return ""
	}
	return value[:index]
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, r := range strings.TrimPrefix(value, "sha256:") {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
