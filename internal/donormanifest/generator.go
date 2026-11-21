package donormanifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type GeneratorOptions struct {
	InventoryTime string
	Rules         []DispositionRule
}

type Option func(*GeneratorOptions)

func WithInventoryTime(t string) Option {
	return func(o *GeneratorOptions) {
		o.InventoryTime = t
	}
}

func WithDispositionRules(rules []DispositionRule) Option {
	return func(o *GeneratorOptions) {
		o.Rules = rules
	}
}

// Generate creates a deterministic Manifest for a snapshot root.
func Generate(snapshotID, sourceRoot string, opts ...Option) (*Manifest, *RootReceipt, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return nil, nil, errors.New("snapshotID cannot be empty")
	}
	if strings.TrimSpace(sourceRoot) == "" {
		return nil, nil, errors.New("sourceRoot cannot be empty")
	}

	options := GeneratorOptions{
		InventoryTime: time.Now().UTC().Format(time.RFC3339),
	}
	for _, opt := range opts {
		opt(&options)
	}

	absRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve abs sourceRoot: %w", err)
	}
	absRoot = filepath.Clean(absRoot)

	info, err := os.Stat(absRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("sourceRoot %q does not exist: %w", sourceRoot, err)
		}
		return nil, nil, fmt.Errorf("stat sourceRoot %q: %w", sourceRoot, err)
	}
	if !info.IsDir() {
		return nil, nil, fmt.Errorf("sourceRoot %q is not a directory", sourceRoot)
	}

	var entries []ManifestEntry
	var fileCount int
	var totalSizeBytes int64
	seenCase := make(map[string]string)

	err = filepath.Walk(absRoot, func(current string, currentInfo os.FileInfo, walkErr error) error {
		if walkErr != nil {
			rel, relErr := filepath.Rel(absRoot, current)
			if relErr == nil && rel != "." {
				relSlash := filepath.ToSlash(rel)
				entries = append(entries, ManifestEntry{
					Path:        relSlash,
					ObjectType:  ObjectTypeReadError,
					ReadError:   walkErr.Error(),
					Disposition: DispositionUnreviewed,
				})
			}
			return nil
		}

		rel, err := filepath.Rel(absRoot, current)
		if err != nil {
			return fmt.Errorf("relative path calculation failed for %s: %w", current, err)
		}
		if rel == "." {
			return nil
		}

		relSlash := filepath.ToSlash(rel)

		if strings.HasPrefix(relSlash, "../") || relSlash == ".." || filepath.IsAbs(rel) {
			return fmt.Errorf("path traversal attempt detected: %s", relSlash)
		}

		lower := strings.ToLower(relSlash)
		if existing, exists := seenCase[lower]; exists && existing != relSlash {
			return fmt.Errorf("case collision detected: %q and %q normalize to %q", existing, relSlash, lower)
		}
		seenCase[lower] = relSlash

		lInfo, lErr := os.Lstat(current)
		if lErr != nil {
			entries = append(entries, ManifestEntry{
				Path:        relSlash,
				ObjectType:  ObjectTypeReadError,
				ReadError:   lErr.Error(),
				Disposition: DispositionUnreviewed,
			})
			return nil
		}

		disposition, destination, reason := getDisposition(relSlash, options.Rules)

		mode := lInfo.Mode()
		if mode&os.ModeSymlink != 0 {
			target, rErr := os.Readlink(current)
			targetStr := ""
			if rErr != nil {
				targetStr = fmt.Sprintf("readlink error: %v", rErr)
			} else {
				targetStr = filepath.ToSlash(target)
			}
			entries = append(entries, ManifestEntry{
				Path:          relSlash,
				ObjectType:    ObjectTypeSymlink,
				SizeBytes:     lInfo.Size(),
				SymlinkTarget: targetStr,
				Disposition:   disposition,
				Destination:   destination,
				Reason:        reason,
			})
			if lInfo.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if mode.IsDir() {
			entries = append(entries, ManifestEntry{
				Path:        relSlash,
				ObjectType:  ObjectTypeDir,
				SizeBytes:   0,
				Disposition: disposition,
				Destination: destination,
				Reason:      reason,
			})
			return nil
		}

		if mode.IsRegular() {
			digest, size, dErr := digestFile(current)
			if dErr != nil {
				entries = append(entries, ManifestEntry{
					Path:        relSlash,
					ObjectType:  ObjectTypeReadError,
					SizeBytes:   lInfo.Size(),
					ReadError:   dErr.Error(),
					Disposition: disposition,
					Destination: destination,
					Reason:      reason,
				})
			} else {
				fileCount++
				totalSizeBytes += size
				entries = append(entries, ManifestEntry{
					Path:        relSlash,
					ObjectType:  ObjectTypeFile,
					SizeBytes:   size,
					SHA256:      digest,
					Disposition: disposition,
					Destination: destination,
					Reason:      reason,
				})
			}
			return nil
		}

		entries = append(entries, ManifestEntry{
			Path:        relSlash,
			ObjectType:  ObjectTypeUnknown,
			SizeBytes:   lInfo.Size(),
			Disposition: disposition,
			Destination: destination,
			Reason:      reason,
		})
		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	emptySnapshot := (fileCount == 0 && len(entries) == 0)

	manifest := &Manifest{
		SchemaVersion:    SchemaVersion,
		SnapshotID:       snapshotID,
		SourceRoot:       filepath.ToSlash(sourceRoot),
		PhysicalIdentity: absRoot,
		InventoryTime:    options.InventoryTime,
		EmptySnapshot:    emptySnapshot,
		FileCount:        fileCount,
		TotalSizeBytes:   totalSizeBytes,
		Entries:          entries,
	}

	receipt, err := GenerateReceipt(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("generate receipt: %w", err)
	}

	return manifest, receipt, nil
}

// GenerateReceipt computes a RootReceipt for a Manifest.
func GenerateReceipt(manifest *Manifest) (*RootReceipt, error) {
	manifestBytes, err := MarshalManifestJSON(manifest)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(manifestBytes)
	digest := hex.EncodeToString(hash[:])

	return &RootReceipt{
		SchemaVersion:    SchemaVersion,
		SnapshotID:       manifest.SnapshotID,
		SourceRoot:       manifest.SourceRoot,
		ManifestDigest:   digest,
		PhysicalIdentity: manifest.PhysicalIdentity,
		InventoryTime:    manifest.InventoryTime,
		EmptySnapshot:    manifest.EmptySnapshot,
		FileCount:        manifest.FileCount,
		TotalSizeBytes:   manifest.TotalSizeBytes,
	}, nil
}

// MarshalManifestJSON marshals the Manifest deterministically with indent.
func MarshalManifestJSON(m *Manifest) ([]byte, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// MarshalReceiptJSON marshals the RootReceipt deterministically with indent.
func MarshalReceiptJSON(r *RootReceipt) ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func digestFile(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func getDisposition(relSlash string, rules []DispositionRule) (string, string, string) {
	for _, rule := range rules {
		pattern := rule.Pattern
		if pattern == "." || pattern == "" {
			return rule.Disposition, rule.Destination, rule.Reason
		}
		if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
			if matched, _ := filepath.Match(pattern, relSlash); matched {
				return rule.Disposition, rule.Destination, rule.Reason
			}
			if matched, _ := filepath.Match(pattern, filepath.Base(relSlash)); matched {
				return rule.Disposition, rule.Destination, rule.Reason
			}
		}
		if relSlash == pattern || strings.HasPrefix(relSlash, strings.TrimSuffix(pattern, "/")+"/") {
			return rule.Disposition, rule.Destination, rule.Reason
		}
	}

	if relSlash == ".github" || strings.HasPrefix(relSlash, ".github/") || relSlash == ".vscode" || strings.HasPrefix(relSlash, ".vscode/") || relSlash == ".githooks" || strings.HasPrefix(relSlash, ".githooks/") || relSlash == ".husky" || strings.HasPrefix(relSlash, ".husky/") {
		return "not-applicable", "", "repository workspace configuration"
	}

	if strings.HasPrefix(relSlash, "docs/") {
		return "inspire", "docs/convergence", "reference documentation"
	}
	if strings.HasPrefix(relSlash, "tests/") || strings.HasPrefix(relSlash, "fixtures/") {
		return "fixture", "internal/adapters/conformance", "conformance test fixtures"
	}
	if strings.HasPrefix(relSlash, "packages/") {
		return "adapt", "internal/cli and target adapters", "monorepo packages and CLI integration"
	}
	if strings.HasPrefix(relSlash, "skills/") {
		return "adapt", "Resource Hub governance contracts", "donor skill manifests and assets"
	}
	if strings.HasPrefix(relSlash, "assets/") || strings.HasPrefix(relSlash, "flatpak/") || strings.HasPrefix(relSlash, "images/") || strings.HasPrefix(relSlash, "icons/") || strings.HasPrefix(relSlash, "img/") || strings.HasPrefix(relSlash, "media/") || strings.HasPrefix(relSlash, "static/") || strings.HasPrefix(relSlash, "public/") {
		return "not-applicable", "", "static assets or OS packaging"
	}
	if strings.HasPrefix(relSlash, "scripts/") || strings.HasPrefix(relSlash, "bin/") || strings.HasPrefix(relSlash, "tools/") {
		return "not-applicable", "", "build scripts or binary tooling"
	}

	base := filepath.Base(relSlash)
	ext := strings.ToLower(filepath.Ext(base))
	if ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".svg" || ext == ".ico" || ext == ".gif" || ext == ".webp" || ext == ".woff" || ext == ".woff2" || ext == ".ttf" || ext == ".eot" {
		return "not-applicable", "", "binary image or font asset"
	}
	if !strings.Contains(relSlash, "/") {
		return "not-applicable", "", "root repository metadata and build tool configuration"
	}
	if ext == ".md" || ext == ".txt" || ext == ".json" || ext == ".toml" || ext == ".yaml" || ext == ".yml" {
		return "inspire", "docs/convergence", "reference document"
	}

	return DispositionUnreviewed, "", "unmapped file path"
}
