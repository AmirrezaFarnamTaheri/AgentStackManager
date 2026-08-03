package releasepack

import (
	"archive/zip"
	"bufio"
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

	"github.com/agentstack/agentstack/internal/safefile"
)

const (
	SourceManifestName   = "SOURCE_MANIFEST.sha256"
	SourceRevisionName   = "SOURCE_REVISION"
	SourceProvenanceName = "SOURCE_PROVENANCE.json"
)

type SourceVerification struct {
	Revision  string
	FileCount int
}

type sourceEntry struct {
	Path   string
	SHA256 string
}

type sourceProvenance struct {
	SchemaVersion     int     `json:"schemaVersion"`
	Status            string  `json:"status"`
	Revision          string  `json:"revision"`
	BaseRevision      string  `json:"baseRevision"`
	CandidateRevision *string `json:"candidateRevision"`
}

var excludedRootDirs = map[string]bool{
	".git":                true,
	"dist":                true,
	"dist-dev":            true,
	".cocoindex_code":     true,
	".codegraph":          true,
	".serena":             true,
	".smart-coding-cache": true,
	"graphify-out":        true,
}

var deniedSourceFiles = map[string]bool{
	"coverage.out":          true,
	"benchmark-results.txt": true,
}

func WriteSourceManifest(root string) (SourceVerification, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return SourceVerification{}, err
	}
	revision, err := validateSourceMetadata(root)
	if err != nil {
		return SourceVerification{}, err
	}
	files, err := collectSourceFiles(root)
	if err != nil {
		return SourceVerification{}, err
	}
	entries, err := digestSourceFiles(root, files)
	if err != nil {
		return SourceVerification{}, err
	}
	manifestPath := filepath.Join(root, SourceManifestName)
	temp, err := os.CreateTemp(root, ".source-manifest-*.tmp")
	if err != nil {
		return SourceVerification{}, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	writer := bufio.NewWriter(temp)
	for _, entry := range entries {
		if _, err := fmt.Fprintf(writer, "%s  ./%s\n", entry.SHA256, filepath.ToSlash(entry.Path)); err != nil {
			_ = temp.Close()
			return SourceVerification{}, err
		}
	}
	if err := writer.Flush(); err != nil {
		_ = temp.Close()
		return SourceVerification{}, err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return SourceVerification{}, err
	}
	if err := temp.Close(); err != nil {
		return SourceVerification{}, err
	}
	if err := safefile.Replace(tempName, manifestPath); err != nil {
		return SourceVerification{}, fmt.Errorf("publish source manifest: %w", err)
	}
	return SourceVerification{Revision: revision, FileCount: len(entries)}, nil
}

func VerifySourceClosure(root string) (SourceVerification, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return SourceVerification{}, err
	}
	revision, entries, _, err := verifySourceClosure(root)
	if err != nil {
		return SourceVerification{}, err
	}
	return SourceVerification{Revision: revision, FileCount: len(entries)}, nil
}

func PackVerifiedSource(root, destination, prefix string) (SourceVerification, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return SourceVerification{}, err
	}
	revision, entries, manifestDigest, err := verifySourceClosure(root)
	if err != nil {
		return SourceVerification{}, err
	}
	files := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		files = append(files, entry.Path)
	}
	files = append(files, SourceManifestName)
	sort.Strings(files)
	if err := packRelativeFiles(root, destination, prefix, files); err != nil {
		return SourceVerification{}, err
	}
	if err := verifyPackedSourceArchive(destination, prefix, entries, manifestDigest); err != nil {
		_ = os.Remove(destination)
		return SourceVerification{}, err
	}
	return SourceVerification{Revision: revision, FileCount: len(entries)}, nil
}

func verifySourceClosure(root string) (string, []sourceEntry, string, error) {
	revision, err := validateSourceMetadata(root)
	if err != nil {
		return "", nil, "", err
	}
	manifestPath := filepath.Join(root, SourceManifestName)
	expected, err := readSourceManifest(manifestPath)
	if err != nil {
		return "", nil, "", err
	}
	manifestDigest, err := digestFile(manifestPath)
	if err != nil {
		return "", nil, "", err
	}
	actualFiles, err := collectSourceFiles(root)
	if err != nil {
		return "", nil, "", err
	}
	actual := make([]string, len(actualFiles))
	for index, value := range actualFiles {
		actual[index] = filepath.ToSlash(value)
	}
	expectedPaths := make([]string, len(expected))
	for index, entry := range expected {
		expectedPaths[index] = entry.Path
	}
	if !equalStrings(expectedPaths, actual) {
		return "", nil, "", fmt.Errorf("source manifest exact file set mismatch: expected %v, actual %v", expectedPaths, actual)
	}
	for _, entry := range expected {
		digest, err := digestFile(filepath.Join(root, filepath.FromSlash(entry.Path)))
		if err != nil {
			return "", nil, "", err
		}
		if digest != entry.SHA256 {
			return "", nil, "", fmt.Errorf("source manifest digest mismatch for %s", entry.Path)
		}
	}
	return revision, expected, manifestDigest, nil
}

func collectSourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		parts := strings.Split(relSlash, "/")
		if info.IsDir() {
			if (len(parts) == 1 && excludedRootDirs[parts[0]]) || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if _, err := sanitizeArchivePath(relSlash, "source member"); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("source tree contains unsupported filesystem node: %s", relSlash)
		}
		if relSlash == SourceManifestName {
			return nil
		}
		if deniedSourceFiles[relSlash] || strings.EqualFold(filepath.Ext(relSlash), ".exe") {
			return fmt.Errorf("source tree contains runtime artifact: %s", relSlash)
		}
		files = append(files, relSlash)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func digestSourceFiles(root string, files []string) ([]sourceEntry, error) {
	entries := make([]sourceEntry, 0, len(files))
	for _, rel := range files {
		digest, err := digestFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		entries = append(entries, sourceEntry{Path: rel, SHA256: digest})
	}
	return entries, nil
}

func digestFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readSourceManifest(path string) ([]sourceEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s is missing", SourceManifestName)
		}
		return nil, err
	}
	defer file.Close()
	var entries []sourceEntry
	seen := map[string]bool{}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if len(line) < 68 || line[64:66] != "  " || !validHexDigest(line[:64]) || !strings.HasPrefix(line[66:], "./") {
			return nil, fmt.Errorf("invalid source manifest line %d", lineNumber)
		}
		rel := strings.TrimPrefix(line[66:], "./")
		clean, pathErr := sanitizeArchivePath(rel, "source manifest member")
		if pathErr != nil || clean == "" {
			return nil, fmt.Errorf("invalid source manifest path on line %d: %s", lineNumber, rel)
		}
		if seen[rel] {
			return nil, fmt.Errorf("duplicate source manifest path: %s", rel)
		}
		seen[rel] = true
		entries = append(entries, sourceEntry{Path: rel, SHA256: strings.ToLower(line[:64])})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, errors.New("source manifest is empty")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func validHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validateSourceMetadata(root string) (string, error) {
	revisionBytes, err := os.ReadFile(filepath.Join(root, SourceRevisionName))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", SourceRevisionName, err)
	}
	revision := strings.TrimSpace(string(revisionBytes))
	parts := strings.SplitN(revision, ":", 2)
	if len(parts) != 2 || (parts[0] != "git" && parts[0] != "unreleased-base") || len(parts[1]) != 40 {
		return "", fmt.Errorf("%s must contain git:<40-hex> or unreleased-base:<40-hex>", SourceRevisionName)
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", fmt.Errorf("%s contains an invalid revision", SourceRevisionName)
	}
	data, err := os.ReadFile(filepath.Join(root, SourceProvenanceName))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", SourceProvenanceName, err)
	}
	var provenance sourceProvenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return "", fmt.Errorf("decode %s: %w", SourceProvenanceName, err)
	}
	if provenance.SchemaVersion < 1 || strings.TrimSpace(provenance.Status) == "" {
		return "", fmt.Errorf("%s lacks required schemaVersion/status", SourceProvenanceName)
	}
	switch parts[0] {
	case "git":
		candidate := provenance.Revision
		if provenance.CandidateRevision != nil {
			candidate = *provenance.CandidateRevision
		}
		if candidate != parts[1] {
			return "", fmt.Errorf("source provenance candidate revision does not match %s", SourceRevisionName)
		}
	case "unreleased-base":
		if provenance.BaseRevision != parts[1] {
			return "", fmt.Errorf("source provenance base revision does not match %s", SourceRevisionName)
		}
	}
	return revision, nil
}

func verifyPackedSourceArchive(archivePath, prefix string, entries []sourceEntry, manifestDigest string) error {
	cleanPrefix, err := sanitizePrefix(prefix)
	if err != nil {
		return err
	}
	expected := make(map[string]string, len(entries)+1)
	for _, entry := range entries {
		expected[cleanPrefix+entry.Path] = entry.SHA256
	}
	expected[cleanPrefix+SourceManifestName] = manifestDigest
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("reopen source archive: %w", err)
	}
	defer reader.Close()
	seen := map[string]bool{}
	for _, member := range reader.File {
		if seen[member.Name] {
			return fmt.Errorf("source archive contains duplicate member: %s", member.Name)
		}
		seen[member.Name] = true
		digest, ok := expected[member.Name]
		if !ok {
			return fmt.Errorf("source archive contains unlisted member: %s", member.Name)
		}
		input, err := member.Open()
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if hex.EncodeToString(hash.Sum(nil)) != digest {
			return fmt.Errorf("source archive digest mismatch: %s", member.Name)
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("source archive exact file set mismatch: expected %d members, found %d", len(expected), len(seen))
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
