// Package asmv1 stages the current Resource Hub version-1 state into ASM's
// immutable content-addressed store. The stage is a verified shadow copy: it
// does not change the Resource Hub registry, its reviewed plans, or target
// mutation authority.
package asmv1

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/cas"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const APIVersion = "fabric.asm.dev/migration/asm-v1/v1alpha1"
const CASExtensionKey = "asm.cas.v1"

type ResourceRecord struct {
	ResourceID   string  `json:"resourceId"`
	ArtifactID   string  `json:"artifactId"`
	LegacyDigest string  `json:"legacyDigest"`
	Object       cas.Ref `json:"object"`
}

type Receipt struct {
	APIVersion        string                 `json:"apiVersion"`
	GeneratedAt       time.Time              `json:"generatedAt"`
	SourceGraphDigest string                 `json:"sourceGraphDigest"`
	StagedGraph       artifactgraph.Snapshot `json:"stagedGraph"`
	Resources         []ResourceRecord       `json:"resources"`
	Digest            string                 `json:"digest"`
}

type casExtension struct {
	Object cas.Ref `json:"object"`
}

// Stage writes immutable shadow objects, verifies a byte-for-byte semantic
// round trip using the legacy Resource Hub digest, and emits a digest-bound
// receipt. Resource Hub remains the sole write authority.
func Stage(hub resourcehub.Manager, store cas.Store, clock func() time.Time) (Receipt, error) {
	return stage(hub, store, clock, nil)
}

func stage(hub resourcehub.Manager, store cas.Store, clock func() time.Time, beforeFinalVerify func() error) (Receipt, error) {
	if clock == nil {
		clock = time.Now
	}
	sourceGraph, err := hub.CanonicalSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	resources, err := hub.ListResources()
	if err != nil {
		return Receipt{}, err
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].ID < resources[j].ID })
	artifactsByName := make(map[string]artifactgraph.Artifact, len(sourceGraph.Artifacts))
	for _, artifact := range sourceGraph.Artifacts {
		artifactsByName[artifact.Metadata.Name] = artifact
	}

	records := make([]ResourceRecord, 0, len(resources))
	stagedArtifacts := make([]artifactgraph.Artifact, 0, len(resources))
	for _, resource := range resources {
		artifact, ok := artifactsByName[resource.ID]
		if !ok {
			return Receipt{}, fmt.Errorf("canonical artifact for resource %q is missing", resource.ID)
		}
		contentPath, err := hub.ResourceContentPath(resource.ID)
		if err != nil {
			return Receipt{}, err
		}
		object, err := store.PutTree(contentPath)
		if err != nil {
			return Receipt{}, fmt.Errorf("stage resource %q in CAS: %w", resource.ID, err)
		}
		if err := verifyRoundTrip(store, object, resource.Digest); err != nil {
			return Receipt{}, fmt.Errorf("verify resource %q CAS round trip: %w", resource.ID, err)
		}

		extension, err := json.Marshal(casExtension{Object: object})
		if err != nil {
			return Receipt{}, err
		}
		artifact.Content.Ref = object.URI()
		artifact.Extensions = cloneRawMap(artifact.Extensions)
		artifact.Extensions[CASExtensionKey] = extension
		artifact.Provenance.Fields = cloneFieldMap(artifact.Provenance.Fields)
		artifact.Provenance.Fields["/content/ref"] = artifactgraph.FieldProvenance{
			Source:    "asm-v1-migration",
			Path:      contentPath,
			Transform: "verified-shadow-cas",
		}
		artifact, err = artifactgraph.Seal(artifact)
		if err != nil {
			return Receipt{}, fmt.Errorf("seal staged artifact %q: %w", resource.ID, err)
		}
		stagedArtifacts = append(stagedArtifacts, artifact)
		records = append(records, ResourceRecord{
			ResourceID:   resource.ID,
			ArtifactID:   artifact.ID,
			LegacyDigest: resource.Digest,
			Object:       object,
		})
	}
	if beforeFinalVerify != nil {
		if err := beforeFinalVerify(); err != nil {
			return Receipt{}, err
		}
	}
	currentGraph, err := hub.CanonicalSnapshot()
	if err != nil {
		return Receipt{}, err
	}
	if currentGraph.Digest != sourceGraph.Digest {
		return Receipt{}, fmt.Errorf("resource hub graph changed during CAS staging")
	}
	stagedGraph, err := artifactgraph.NewSnapshot(stagedArtifacts)
	if err != nil {
		return Receipt{}, err
	}
	return SealReceipt(Receipt{
		APIVersion:        APIVersion,
		GeneratedAt:       clock().UTC(),
		SourceGraphDigest: sourceGraph.Digest,
		StagedGraph:       stagedGraph,
		Resources:         records,
	})
}

func SealReceipt(value Receipt) (Receipt, error) {
	normalized, err := normalizeReceipt(value)
	if err != nil {
		return Receipt{}, err
	}
	normalized.Digest = ""
	digest, err := integrity.DigestJSON(normalized)
	if err != nil {
		return Receipt{}, fmt.Errorf("digest ASM v1 migration receipt: %w", err)
	}
	normalized.Digest = digest
	return normalized, nil
}

func VerifyReceipt(value Receipt) error {
	if !validDigest(value.Digest) {
		return fmt.Errorf("ASM v1 migration receipt has invalid digest")
	}
	expected, err := SealReceipt(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("ASM v1 migration receipt digest mismatch")
	}
	return nil
}

// VerifyCurrent proves that the receipt still describes the current Resource
// Hub graph and that every referenced CAS object remains intact and reversible.
func VerifyCurrent(hub resourcehub.Manager, store cas.Store, receipt Receipt) error {
	if err := VerifyReceipt(receipt); err != nil {
		return err
	}
	current, err := hub.CanonicalSnapshot()
	if err != nil {
		return err
	}
	if current.Digest != receipt.SourceGraphDigest {
		return fmt.Errorf("ASM v1 migration receipt is stale: source graph changed")
	}
	for _, record := range receipt.Resources {
		if err := store.Verify(record.Object); err != nil {
			return fmt.Errorf("verify CAS object for resource %q: %w", record.ResourceID, err)
		}
		if err := verifyRoundTrip(store, record.Object, record.LegacyDigest); err != nil {
			return fmt.Errorf("verify CAS round trip for resource %q: %w", record.ResourceID, err)
		}
	}
	return nil
}

// RestoreResource materializes one staged resource to a new path and verifies
// it against the original Resource Hub digest. It never overwrites an existing
// destination or changes Resource Hub state.
func RestoreResource(store cas.Store, receipt Receipt, resourceID, destination string) error {
	if err := VerifyReceipt(receipt); err != nil {
		return err
	}
	var record *ResourceRecord
	for index := range receipt.Resources {
		if receipt.Resources[index].ResourceID == resourceID {
			record = &receipt.Resources[index]
			break
		}
	}
	if record == nil {
		return fmt.Errorf("resource %q is not present in migration receipt", resourceID)
	}
	if err := store.Materialize(record.Object, destination); err != nil {
		return err
	}
	digest, err := resourcehub.DigestPath(destination)
	if err != nil {
		return fmt.Errorf("verify restored resource %q; destination retained at %s: %w", resourceID, destination, err)
	}
	if digest != record.LegacyDigest {
		return fmt.Errorf("restored resource %q digest mismatch; destination retained at %s", resourceID, destination)
	}
	return nil
}

func normalizeReceipt(value Receipt) (Receipt, error) {
	if value.APIVersion == "" {
		value.APIVersion = APIVersion
	}
	value.GeneratedAt = value.GeneratedAt.UTC()
	value.SourceGraphDigest = strings.TrimSpace(value.SourceGraphDigest)
	value.Resources = append([]ResourceRecord(nil), value.Resources...)
	sort.Slice(value.Resources, func(i, j int) bool { return value.Resources[i].ResourceID < value.Resources[j].ResourceID })
	if value.APIVersion != APIVersion {
		return Receipt{}, fmt.Errorf("unsupported ASM v1 migration receipt apiVersion %q", value.APIVersion)
	}
	if value.GeneratedAt.IsZero() || !validDigest(value.SourceGraphDigest) {
		return Receipt{}, fmt.Errorf("ASM v1 migration receipt has incomplete identity")
	}
	if err := artifactgraph.VerifySnapshot(value.StagedGraph); err != nil {
		return Receipt{}, fmt.Errorf("verify staged artifact graph: %w", err)
	}
	if len(value.Resources) != len(value.StagedGraph.Artifacts) {
		return Receipt{}, fmt.Errorf("ASM v1 migration receipt resource count does not match staged graph")
	}
	artifacts := make(map[string]artifactgraph.Artifact, len(value.StagedGraph.Artifacts))
	for _, artifact := range value.StagedGraph.Artifacts {
		artifacts[artifact.ID] = artifact
	}
	seen := map[string]struct{}{}
	seenArtifacts := map[string]struct{}{}
	for _, record := range value.Resources {
		if strings.TrimSpace(record.ResourceID) == "" || strings.TrimSpace(record.ArtifactID) == "" || !validDigest(record.LegacyDigest) {
			return Receipt{}, fmt.Errorf("ASM v1 migration receipt has invalid resource record")
		}
		if _, exists := seen[record.ResourceID]; exists {
			return Receipt{}, fmt.Errorf("duplicate migration resource %q", record.ResourceID)
		}
		seen[record.ResourceID] = struct{}{}
		artifact, ok := artifacts[record.ArtifactID]
		if !ok || artifact.Metadata.Name != record.ResourceID {
			return Receipt{}, fmt.Errorf("migration resource %q does not match staged artifact %q", record.ResourceID, record.ArtifactID)
		}
		if _, exists := seenArtifacts[record.ArtifactID]; exists {
			return Receipt{}, fmt.Errorf("staged artifact %q is referenced more than once", record.ArtifactID)
		}
		seenArtifacts[record.ArtifactID] = struct{}{}
		if record.Object.Kind != cas.KindTree || cas.ValidateRef(record.Object) != nil {
			return Receipt{}, fmt.Errorf("migration resource %q has invalid CAS object", record.ResourceID)
		}
		if artifact.Content.Ref != record.Object.URI() || artifact.Content.Digest != record.LegacyDigest {
			return Receipt{}, fmt.Errorf("migration resource %q content reference mismatch", record.ResourceID)
		}
		var extension casExtension
		raw, ok := artifact.Extensions[CASExtensionKey]
		if !ok || strictjson.Decode(raw, &extension) != nil || extension.Object != record.Object {
			return Receipt{}, fmt.Errorf("migration resource %q CAS extension mismatch", record.ResourceID)
		}
	}
	if len(seenArtifacts) != len(artifacts) {
		return Receipt{}, fmt.Errorf("ASM v1 migration receipt leaves staged artifacts unreferenced")
	}
	return value, nil
}

func verifyRoundTrip(store cas.Store, object cas.Ref, legacyDigest string) error {
	root, err := os.MkdirTemp("", "agentstack-cas-roundtrip-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	destination := filepath.Join(root, "content")
	if err := store.Materialize(object, destination); err != nil {
		return err
	}
	digest, err := resourcehub.DigestPath(destination)
	if err != nil {
		return err
	}
	if digest != legacyDigest {
		return fmt.Errorf("legacy digest mismatch: got %s want %s", digest, legacyDigest)
	}
	return nil
}

func cloneRawMap(values map[string]json.RawMessage) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(values)+1)
	for key, value := range values {
		result[key] = append(json.RawMessage(nil), value...)
	}
	return result
}

func cloneFieldMap(values map[string]artifactgraph.FieldProvenance) map[string]artifactgraph.FieldProvenance {
	result := make(map[string]artifactgraph.FieldProvenance, len(values)+1)
	for key, value := range values {
		result[key] = value
	}
	return result
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
