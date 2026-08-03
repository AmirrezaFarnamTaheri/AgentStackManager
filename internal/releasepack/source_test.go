package releasepack

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSourceMetadata(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, SourceRevisionName), []byte("unreleased-base:2356a0290239f3a7551a6db9dd7bb76f563fa96d\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provenance := `{"schemaVersion":1,"status":"unreleased-test","baseRevision":"2356a0290239f3a7551a6db9dd7bb76f563fa96d","candidateRevision":null}`
	if err := os.WriteFile(filepath.Join(root, SourceProvenanceName), []byte(provenance), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestWriteAndVerifySourceManifestClosesExactFileSet(t *testing.T) {
	root := t.TempDir()
	writeSourceMetadata(t, root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "demo", "demo.go"), []byte("package demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSourceManifest(root); err != nil {
		t.Fatalf("WriteSourceManifest() error = %v", err)
	}
	verification, err := VerifySourceClosure(root)
	if err != nil {
		t.Fatalf("VerifySourceClosure() error = %v", err)
	}
	if verification.FileCount != 3 {
		t.Fatalf("FileCount = %d, want 3", verification.FileCount)
	}

	if err := os.WriteFile(filepath.Join(root, "unlisted.txt"), []byte("injected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifySourceClosure(root); err == nil || !strings.Contains(err.Error(), "exact file set") {
		t.Fatalf("VerifySourceClosure() error = %v, want exact-set failure", err)
	}
}

func TestWriteSourceManifestRejectsRuntimeArtifacts(t *testing.T) {
	root := t.TempDir()
	writeSourceMetadata(t, root)
	if err := os.WriteFile(filepath.Join(root, "agentstack.exe"), []byte("MZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSourceManifest(root); err == nil || !strings.Contains(err.Error(), "runtime artifact") {
		t.Fatalf("WriteSourceManifest() error = %v", err)
	}
}

func TestPackVerifiedSourceReopensClosedArchive(t *testing.T) {
	root := t.TempDir()
	writeSourceMetadata(t, root)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSourceManifest(root); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "source.zip")
	verification, err := PackVerifiedSource(root, out, "source")
	if err != nil {
		t.Fatalf("PackVerifiedSource() error = %v", err)
	}
	if verification.FileCount != 3 {
		t.Fatalf("FileCount = %d, want 3", verification.FileCount)
	}
	reader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	seenManifest := false
	for _, file := range reader.File {
		if file.Name == "source/"+SourceManifestName {
			seenManifest = true
		}
	}
	if !seenManifest {
		t.Fatal("packed source omitted manifest")
	}
}

func TestPackVerifiedSourceIncludesOnlyManifestedFiles(t *testing.T) {
	root := t.TempDir()
	writeSourceMetadata(t, root)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "demo"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "demo", "junk.js"), []byte("junk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSourceManifest(root); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "source.zip")
	if _, err := PackVerifiedSource(root, out, "source"); err != nil {
		t.Fatalf("PackVerifiedSource() error = %v", err)
	}
	reader, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if strings.Contains(file.Name, "node_modules") {
			t.Fatalf("archive included excluded working-tree file %q", file.Name)
		}
	}
}

func TestPackedArchiveIsCheckedAgainstManifestDigests(t *testing.T) {
	root := t.TempDir()
	writeSourceMetadata(t, root)
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("reviewed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSourceManifest(root); err != nil {
		t.Fatal(err)
	}
	_, entries, manifestDigest, err := verifySourceClosure(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readme, []byte("changed after verification"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := make([]string, 0, len(entries)+1)
	for _, entry := range entries {
		files = append(files, entry.Path)
	}
	files = append(files, SourceManifestName)
	out := filepath.Join(t.TempDir(), "tampered.zip")
	if err := packRelativeFiles(root, out, "source", files); err != nil {
		t.Fatal(err)
	}
	if err := verifyPackedSourceArchive(out, "source", entries, manifestDigest); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("verifyPackedSourceArchive() error = %v, want manifest digest mismatch", err)
	}
}

func TestWriteSourceManifestValidatesMetadataBeforeReplacement(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, SourceManifestName)
	if err := os.WriteFile(manifest, []byte("sentinel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, SourceRevisionName), []byte("invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSourceManifest(root); err == nil {
		t.Fatal("WriteSourceManifest() unexpectedly accepted invalid metadata")
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sentinel\n" {
		t.Fatalf("manifest changed after metadata rejection: %q", data)
	}
}

func TestVerifySourceClosureRejectsCrossPlatformUnsafeManifestPaths(t *testing.T) {
	for _, unsafePath := range []string{"../escape.txt", `nested\\escape.txt`, "C:/escape.txt", "//server/share.txt"} {
		t.Run(strings.ReplaceAll(unsafePath, "/", "_"), func(t *testing.T) {
			root := t.TempDir()
			writeSourceMetadata(t, root)
			manifestLine := strings.Repeat("0", 64) + "  ./" + unsafePath + "\n"
			if err := os.WriteFile(filepath.Join(root, SourceManifestName), []byte(manifestLine), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifySourceClosure(root); err == nil || !strings.Contains(err.Error(), "invalid source manifest path") {
				t.Fatalf("VerifySourceClosure() error = %v", err)
			}
		})
	}
}
