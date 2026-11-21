package donormanifest_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/donormanifest"
)

const fixedTime = "2026-08-13T12:00:00Z"

func TestGeneratorEmptySnapshot(t *testing.T) {
	tempDir := t.TempDir()
	emptySubdir := filepath.Join(tempDir, "empty_root")
	if err := os.MkdirAll(emptySubdir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest, receipt, err := donormanifest.Generate("agentdns", emptySubdir, donormanifest.WithInventoryTime(fixedTime))
	if err != nil {
		t.Fatalf("unexpected error on empty snapshot: %v", err)
	}

	if !manifest.EmptySnapshot {
		t.Errorf("expected EmptySnapshot to be true")
	}
	if manifest.FileCount != 0 {
		t.Errorf("expected FileCount == 0, got %d", manifest.FileCount)
	}
	if manifest.TotalSizeBytes != 0 {
		t.Errorf("expected TotalSizeBytes == 0, got %d", manifest.TotalSizeBytes)
	}
	if len(manifest.Entries) != 0 {
		t.Errorf("expected len(Entries) == 0, got %d", len(manifest.Entries))
	}
	if !receipt.EmptySnapshot {
		t.Errorf("expected receipt.EmptySnapshot to be true")
	}
	if receipt.ManifestDigest == "" {
		t.Errorf("expected receipt.ManifestDigest to be non-empty")
	}
}

func TestGeneratorDeterminismAndSorting(t *testing.T) {
	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "donor_root")

	// Create subdirectories and files in non-alphabetical order
	if err := os.MkdirAll(filepath.Join(root, "b_dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "a_dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "z_file.txt"), []byte("content z"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a_dir", "sub_file.txt"), []byte("content sub"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b_dir", "file_b.txt"), []byte("content b"), 0644); err != nil {
		t.Fatal(err)
	}

	// Generate manifest 1
	m1, r1, err := donormanifest.Generate("test-snapshot", root, donormanifest.WithInventoryTime(fixedTime))
	if err != nil {
		t.Fatalf("first generate failed: %v", err)
	}

	// Generate manifest 2
	m2, r2, err := donormanifest.Generate("test-snapshot", root, donormanifest.WithInventoryTime(fixedTime))
	if err != nil {
		t.Fatalf("second generate failed: %v", err)
	}

	if r1.ManifestDigest != r2.ManifestDigest {
		t.Fatalf("expected identical receipt digest for same tree: got %q vs %q", r1.ManifestDigest, r2.ManifestDigest)
	}

	b1, err := donormanifest.MarshalManifestJSON(m1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := donormanifest.MarshalManifestJSON(m2)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(b1, b2) {
		t.Fatalf("manifest JSON output was not deterministic")
	}

	// Verify entries sorting
	expectedPaths := []string{
		"a_dir",
		"a_dir/sub_file.txt",
		"b_dir",
		"b_dir/file_b.txt",
		"z_file.txt",
	}

	if len(m1.Entries) != len(expectedPaths) {
		t.Fatalf("expected %d entries, got %d", len(expectedPaths), len(m1.Entries))
	}

	for i, expected := range expectedPaths {
		if m1.Entries[i].Path != expected {
			t.Errorf("entry index %d mismatch: expected %q, got %q", i, expected, m1.Entries[i].Path)
		}
	}
}

func TestGeneratorNonExistentRoot(t *testing.T) {
	tempDir := t.TempDir()
	nonExistent := filepath.Join(tempDir, "does_not_exist")

	_, _, err := donormanifest.Generate("test", nonExistent)
	if err == nil {
		t.Fatalf("expected error for non-existent root")
	}
}

func TestGeneratorInvalidInput(t *testing.T) {
	_, _, err := donormanifest.Generate("", "some/path")
	if err == nil {
		t.Errorf("expected error for empty snapshotID")
	}
	_, _, err = donormanifest.Generate("id", "")
	if err == nil {
		t.Errorf("expected error for empty sourceRoot")
	}
}

func TestGeneratorSymlinkHandling(t *testing.T) {
	tempDir := t.TempDir()
	root := filepath.Join(tempDir, "root")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(root, "target.txt")
	if err := os.WriteFile(targetFile, []byte("target file"), 0644); err != nil {
		t.Fatal(err)
	}

	linkFile := filepath.Join(root, "sym_link.txt")
	err := os.Symlink("target.txt", linkFile)
	if err != nil {
		t.Skip("Symlinks not supported on this platform/environment, skipping symlink test")
		return
	}

	manifest, _, err := donormanifest.Generate("symlink-test", root, donormanifest.WithInventoryTime(fixedTime))
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	var foundSymlink bool
	for _, entry := range manifest.Entries {
		if entry.Path == "sym_link.txt" {
			foundSymlink = true
			if entry.ObjectType != donormanifest.ObjectTypeSymlink {
				t.Errorf("expected ObjectType %q, got %q", donormanifest.ObjectTypeSymlink, entry.ObjectType)
			}
			if entry.SymlinkTarget != "target.txt" {
				t.Errorf("expected SymlinkTarget %q, got %q", "target.txt", entry.SymlinkTarget)
			}
		}
	}
	if !foundSymlink {
		t.Errorf("symlink entry was not found in manifest")
	}
}

func TestManifestsClosureAndDispositions(t *testing.T) {
	manifestsDir := filepath.Join("..", "..", "docs", "convergence", "manifests")
	indexFile := filepath.Join(manifestsDir, "INDEX.json")

	data, err := os.ReadFile(indexFile)
	if err != nil {
		t.Skipf("manifest index %s not present in test environment: %v", indexFile, err)
		return
	}

	var index struct {
		SchemaVersion  int `json:"schemaVersion"`
		TotalSnapshots int `json:"totalSnapshots"`
		Snapshots      []struct {
			ID           string `json:"id"`
			ManifestFile string `json:"manifestFile"`
			ReceiptFile  string `json:"receiptFile"`
		} `json:"snapshots"`
	}

	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatalf("failed to decode manifest INDEX.json: %v", err)
	}

	if index.TotalSnapshots != 8 {
		t.Fatalf("expected 8 total snapshots, got %d", index.TotalSnapshots)
	}

	validDispositions := map[string]bool{
		"adopt":          true,
		"adapt":          true,
		"fixture":        true,
		"inspire":        true,
		"defer":          true,
		"reject":         true,
		"duplicate-of":   true,
		"not-applicable": true,
	}

	for _, snap := range index.Snapshots {
		mPath := filepath.Join(manifestsDir, snap.ManifestFile)
		mData, err := os.ReadFile(mPath)
		if err != nil {
			t.Fatalf("snapshot %s manifest file %s missing: %v", snap.ID, snap.ManifestFile, err)
		}

		var manifest donormanifest.Manifest
		if err := json.Unmarshal(mData, &manifest); err != nil {
			t.Fatalf("failed to decode manifest %s: %v", snap.ManifestFile, err)
		}

		for _, entry := range manifest.Entries {
			if !validDispositions[entry.Disposition] {
				t.Fatalf("snapshot %s entry %s has invalid disposition %q", snap.ID, entry.Path, entry.Disposition)
			}
			if entry.Disposition == "adopt" || entry.Disposition == "adapt" || entry.Disposition == "fixture" {
				if entry.Destination == "" {
					t.Fatalf("snapshot %s entry %s (disposition %s) lacks required destination", snap.ID, entry.Path, entry.Disposition)
				}
			}
			if entry.Disposition == "reject" || entry.Disposition == "defer" || entry.Disposition == "inspire" || entry.Disposition == "not-applicable" {
				if entry.Reason == "" {
					t.Fatalf("snapshot %s entry %s (disposition %s) lacks required reason", snap.ID, entry.Path, entry.Disposition)
				}
			}
		}
	}
}
