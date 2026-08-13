package resourcehub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const OwnershipAPIVersion = "resourcehub.asm.dev/ownership/v1alpha1"
const OwnershipMarkerFilename = ".asm-managed"

type OwnershipManifest struct {
	APIVersion        string    `json:"apiVersion"`
	Owner             string    `json:"owner"`
	SchemaVersion     int       `json:"schemaVersion"`
	ResourceID        string    `json:"resourceId"`
	ArtifactID        string    `json:"artifactId"`
	ArtifactDigest    string    `json:"artifactDigest"`
	TargetID          string    `json:"targetId"`
	ProjectionRoot    string    `json:"projectionRoot"`
	GenerationReceipt string    `json:"generationReceipt"`
	GeneratedAt       time.Time `json:"generatedAt"`
	Digest            string    `json:"digest"`
}

// CreateOwnershipMarker initializes and seals an ownership manifest.
func CreateOwnershipMarker(resourceID, artifactID, artifactDigest, targetID, projectionRoot, generationReceipt string, now time.Time) (OwnershipManifest, error) {
	if strings.TrimSpace(resourceID) == "" {
		return OwnershipManifest{}, fmt.Errorf("resource ID cannot be empty")
	}
	if strings.TrimSpace(artifactID) == "" {
		return OwnershipManifest{}, fmt.Errorf("artifact ID cannot be empty")
	}
	if strings.TrimSpace(artifactDigest) == "" {
		return OwnershipManifest{}, fmt.Errorf("artifact digest cannot be empty")
	}
	if strings.TrimSpace(targetID) == "" {
		return OwnershipManifest{}, fmt.Errorf("target ID cannot be empty")
	}

	manifest := OwnershipManifest{
		APIVersion:        OwnershipAPIVersion,
		Owner:             "AgentStackManager",
		SchemaVersion:     1,
		ResourceID:        resourceID,
		ArtifactID:        artifactID,
		ArtifactDigest:    artifactDigest,
		TargetID:          targetID,
		ProjectionRoot:    filepath.ToSlash(projectionRoot),
		GenerationReceipt: generationReceipt,
		GeneratedAt:       now.UTC(),
	}

	return SealOwnershipManifest(manifest)
}

// SealOwnershipManifest normalizes fields and computes SHA-256 digest.
func SealOwnershipManifest(manifest OwnershipManifest) (OwnershipManifest, error) {
	if manifest.APIVersion == "" {
		manifest.APIVersion = OwnershipAPIVersion
	}
	manifest.Digest = ""
	digest, err := integrity.DigestJSON(manifest)
	if err != nil {
		return OwnershipManifest{}, fmt.Errorf("digest ownership manifest: %w", err)
	}
	manifest.Digest = digest
	return manifest, nil
}

// VerifyOwnershipManifest validates that the manifest digest and owner match ASM authority contracts.
func VerifyOwnershipManifest(manifest OwnershipManifest) error {
	if manifest.APIVersion != OwnershipAPIVersion {
		return fmt.Errorf("unsupported ownership manifest API version %q", manifest.APIVersion)
	}
	if manifest.Owner != "AgentStackManager" {
		return fmt.Errorf("unsupported owner %q: must be 'AgentStackManager'", manifest.Owner)
	}
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("unsupported ownership schema version %d", manifest.SchemaVersion)
	}
	if manifest.Digest == "" {
		return fmt.Errorf("ownership manifest digest is empty")
	}
	expected, err := SealOwnershipManifest(manifest)
	if err != nil {
		return err
	}
	if expected.Digest != manifest.Digest {
		return fmt.Errorf("ownership manifest digest mismatch: got %q, expected %q", manifest.Digest, expected.Digest)
	}
	return nil
}

// WriteOwnershipMarker writes a sealed ownership manifest to the target projection root.
func WriteOwnershipMarker(projectionRoot string, manifest OwnershipManifest) error {
	sealed, err := SealOwnershipManifest(manifest)
	if err != nil {
		return fmt.Errorf("seal ownership manifest: %w", err)
	}
	markerPath := filepath.Join(projectionRoot, OwnershipMarkerFilename)
	data, err := json.MarshalIndent(sealed, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ownership manifest: %w", err)
	}
	return os.WriteFile(markerPath, data, 0o644)
}

// InspectManagedProjection inspects a path for a valid ASM ownership marker.
func InspectManagedProjection(projectionRoot string) (bool, OwnershipManifest, error) {
	markerPath := filepath.Join(projectionRoot, OwnershipMarkerFilename)
	data, err := safefile.ReadBoundedRegular(markerPath, 1<<20)
	if err != nil {
		if os.IsNotExist(err) {
			return false, OwnershipManifest{}, nil // Missing marker -> unmanaged/foreign
		}
		return false, OwnershipManifest{}, fmt.Errorf("read ownership marker: %w", err)
	}

	var manifest OwnershipManifest
	if err := strictjson.Decode(data, &manifest); err != nil {
		return false, OwnershipManifest{}, fmt.Errorf("forged or invalid ownership marker JSON: %w", err)
	}

	if err := VerifyOwnershipManifest(manifest); err != nil {
		return false, manifest, fmt.Errorf("invalid or tampered ownership marker: %w", err)
	}

	return true, manifest, nil
}

// ShouldExcludeFromImport checks if a target path is an ASM managed projection that should be excluded from re-import.
func ShouldExcludeFromImport(path string) bool {
	managed, _, err := InspectManagedProjection(path)
	if err != nil || !managed {
		return false
	}
	return true
}
