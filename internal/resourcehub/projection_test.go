package resourcehub_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestValidOwnershipMarker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-projection-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	now := time.Now().UTC()
	marker, err := resourcehub.CreateOwnershipMarker("res-001", "art-001", "sha256:1111111111111111111111111111111111111111111111111111111111111111", "opencode", tmpDir, "rcpt-001", now)
	if err != nil {
		t.Fatalf("create ownership marker: %v", err)
	}

	if err := resourcehub.VerifyOwnershipManifest(marker); err != nil {
		t.Errorf("verify ownership marker: %v", err)
	}

	if err := resourcehub.WriteOwnershipMarker(tmpDir, marker); err != nil {
		t.Fatalf("write ownership marker: %v", err)
	}

	managed, readMarker, err := resourcehub.InspectManagedProjection(tmpDir)
	if err != nil {
		t.Fatalf("inspect managed projection: %v", err)
	}
	if !managed {
		t.Errorf("expected projection to be marked managed")
	}
	if readMarker.ResourceID != "res-001" {
		t.Errorf("expected resource ID 'res-001', got %s", readMarker.ResourceID)
	}

	if !resourcehub.ShouldExcludeFromImport(tmpDir) {
		t.Errorf("expected managed projection to be excluded from re-import")
	}
}

func TestTamperedOwnershipMarker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-projection-tamper-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	now := time.Now().UTC()
	marker, err := resourcehub.CreateOwnershipMarker("res-002", "art-002", "sha256:2222222222222222222222222222222222222222222222222222222222222222", "claude", tmpDir, "rcpt-002", now)
	if err != nil {
		t.Fatalf("create marker: %v", err)
	}

	if err := resourcehub.WriteOwnershipMarker(tmpDir, marker); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	// Tamper marker file content (change resourceId without updating digest)
	markerPath := filepath.Join(tmpDir, ".asm-managed")
	tamperedJSON := []byte(`{
		"apiVersion": "resourcehub.asm.dev/ownership/v1alpha1",
		"owner": "AgentStackManager",
		"schemaVersion": 1,
		"resourceId": "TAMPERED_RESOURCE_ID",
		"artifactId": "art-002",
		"artifactDigest": "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		"targetId": "claude",
		"projectionRoot": "` + filepath.ToSlash(tmpDir) + `",
		"generationReceipt": "rcpt-002",
		"generatedAt": "` + now.Format(time.RFC3339Nano) + `",
		"digest": "` + marker.Digest + `"
	}`)
	os.WriteFile(markerPath, tamperedJSON, 0o644)

	managed, _, err := resourcehub.InspectManagedProjection(tmpDir)
	if err == nil {
		t.Fatalf("expected error for tampered marker, got nil")
	}
	if managed {
		t.Errorf("tampered projection must not be marked managed")
	}
	if resourcehub.ShouldExcludeFromImport(tmpDir) {
		t.Errorf("tampered projection must not be excluded as valid managed projection")
	}
}

func TestForgedOrNonASMOwnerMarker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-projection-forged-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	markerPath := filepath.Join(tmpDir, ".asm-managed")
	forgedJSON := []byte(`{
		"apiVersion": "resourcehub.asm.dev/ownership/v1alpha1",
		"owner": "FakeManager",
		"schemaVersion": 1,
		"resourceId": "res-999",
		"artifactId": "art-999",
		"artifactDigest": "sha256:9999999999999999999999999999999999999999999999999999999999999999",
		"targetId": "unknown",
		"projectionRoot": "tmp",
		"generationReceipt": "rcpt-999",
		"generatedAt": "2026-08-13T00:00:00Z",
		"digest": "fake"
	}`)
	os.WriteFile(markerPath, forgedJSON, 0o644)

	managed, _, err := resourcehub.InspectManagedProjection(tmpDir)
	if err == nil {
		t.Fatalf("expected error for forged owner marker, got nil")
	}
	if managed {
		t.Errorf("forged projection must not be marked managed")
	}
}

func TestMissingMarker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-projection-missing-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	managed, _, err := resourcehub.InspectManagedProjection(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error inspecting missing marker: %v", err)
	}
	if managed {
		t.Errorf("directory without marker must not be marked managed")
	}
}
