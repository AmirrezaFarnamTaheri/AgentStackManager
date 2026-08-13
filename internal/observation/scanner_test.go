package observation_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/observation"
)

func TestDirectRootScan(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-obs-direct-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content 1"), 0o644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "file2.txt"), []byte("content 2"), 0o644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	result, err := scanner.Scan(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("scan root: %v", err)
	}

	if result.FilesScanned != 3 { // file1.txt, sub, sub/file2.txt
		t.Errorf("expected 3 files scanned, got %d", result.FilesScanned)
	}

	if err := observation.VerifyScanResult(result); err != nil {
		t.Errorf("verify scan result: %v", err)
	}
}

func TestExcludedNativeStatePaths(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-obs-excluded-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Valid skill file
	if err := os.WriteFile(filepath.Join(tmpDir, "valid_skill.md"), []byte("# Skill"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	// Excluded directories (.omx, skills_old, node_modules)
	omxDir := filepath.Join(tmpDir, ".omx")
	if err := os.MkdirAll(omxDir, 0o755); err != nil {
		t.Fatalf("mkdir .omx: %v", err)
	}
	if err := os.WriteFile(filepath.Join(omxDir, "secret.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write omx secret: %v", err)
	}

	oldDir := filepath.Join(tmpDir, "skills_old")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir skills_old: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "old.md"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old skill: %v", err)
	}

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	result, err := scanner.Scan(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("scan root: %v", err)
	}

	// Only valid_skill.md should be observed; .omx and skills_old must be excluded
	if result.FilesScanned != 1 {
		t.Errorf("expected 1 file scanned (excluding .omx and skills_old), got %d", result.FilesScanned)
	}

	if len(result.Diagnostics) < 2 {
		t.Errorf("expected at least 2 excluded path diagnostics, got %d", len(result.Diagnostics))
	}
}

func TestSymlinkDeduplicationAndAliasEdges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-obs-symlink-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	targetFile := filepath.Join(tmpDir, "original.txt")
	if err := os.WriteFile(targetFile, []byte("shared content"), 0o644); err != nil {
		t.Fatalf("write original: %v", err)
	}

	linkFile := filepath.Join(tmpDir, "alias.txt")
	err = os.Symlink(targetFile, linkFile)
	if err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	result, err := scanner.Scan(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("scan root: %v", err)
	}

	// Symlink should deduplicate into original observation as an alias edge
	if len(result.Observations) != 1 {
		t.Errorf("expected 1 physical observation, got %d", len(result.Observations))
	}

	if len(result.Observations[0].Aliases) != 1 {
		t.Errorf("expected 1 alias edge, got %d", len(result.Observations[0].Aliases))
	}
}

func TestBrokenJunctionOrLinkDiagnostic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-obs-broken-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	linkFile := filepath.Join(tmpDir, "broken_link.txt")
	err = os.Symlink(filepath.Join(tmpDir, "nonexistent.txt"), linkFile)
	if err != nil {
		t.Skipf("symlinks not supported on this platform: %v", err)
	}

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	result, err := scanner.Scan(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("scan root: %v", err)
	}

	var hasBrokenDiag bool
	for _, diag := range result.Diagnostics {
		if diag.Kind == observation.DiagBrokenJunction {
			hasBrokenDiag = true
			break
		}
	}

	if !hasBrokenDiag {
		t.Errorf("expected broken junction/link diagnostic")
	}
}

func TestScannerContextCancellation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-obs-cancel-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	for i := 0; i < 100; i++ {
		os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("file_%d.txt", i)), []byte("test"), 0o644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancelled

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	_, err = scanner.Scan(ctx, tmpDir)
	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}
}

func TestSyntheticTreeScan(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-obs-synthetic-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a synthetic tree with 200 files
	for i := 0; i < 10; i++ {
		sub := filepath.Join(tmpDir, fmt.Sprintf("dir_%d", i))
		os.MkdirAll(sub, 0o755)
		for j := 0; j < 20; j++ {
			os.WriteFile(filepath.Join(sub, fmt.Sprintf("skill_%d.md", j)), []byte(fmt.Sprintf("# Skill %d-%d", i, j)), 0o644)
		}
	}

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	start := time.Now()
	result, err := scanner.Scan(context.Background(), tmpDir)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("scan synthetic tree: %v", err)
	}

	if result.FilesScanned != 210 { // 10 dirs + 200 files
		t.Errorf("expected 210 files scanned, got %d", result.FilesScanned)
	}

	t.Logf("scanned %d synthetic items in %v", result.FilesScanned, duration)
}

func TestScanResultJSONSerialization(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-obs-json-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "item.txt"), []byte("sample"), 0o644)

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	result, err := scanner.Scan(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	data, err := observation.SealScanResult(result)
	if err != nil {
		t.Fatalf("seal result: %v", err)
	}

	// Verify unsupported API version is rejected
	badJSON := []byte(strings.Replace(string(data.Digest), "observation.asm.dev/v1alpha1", "observation.asm.dev/v2", 1))
	_, err = observation.UnmarshalScanResultJSON(badJSON)
	if err == nil {
		t.Errorf("expected unmarshal error for unsupported API version")
	}
}
