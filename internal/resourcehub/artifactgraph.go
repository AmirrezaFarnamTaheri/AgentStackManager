package resourcehub

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/agentstack/agentstack/internal/artifactgraph"
)

const resourceArtifactMediaType = "application/vnd.agentstack.resource.v1"

type resourceHubExtension struct {
	Entry    string            `json:"entry"`
	Enabled  bool              `json:"enabled"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// CanonicalSnapshot exposes the current Resource Hub registry through ASM's
// canonical artifact envelope. This is a read-only compatibility view: the
// version-1 Resource Hub registry remains authoritative until a reviewed,
// reversible storage migration is implemented.
func (m Manager) CanonicalSnapshot() (artifactgraph.Snapshot, error) {
	registry, err := m.LoadRegistry()
	if err != nil {
		return artifactgraph.Snapshot{}, err
	}
	ids := make([]string, 0, len(registry.Resources))
	for id := range registry.Resources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	artifacts := make([]artifactgraph.Artifact, 0, len(ids))
	for _, id := range ids {
		artifact, err := canonicalArtifact(registry.Resources[id])
		if err != nil {
			return artifactgraph.Snapshot{}, fmt.Errorf("canonicalize resource %q: %w", id, err)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifactgraph.NewSnapshot(artifacts)
}

func canonicalArtifact(resource Resource) (artifactgraph.Artifact, error) {
	kind, executionClass, capabilities, err := canonicalResourceKind(resource.Kind)
	if err != nil {
		return artifactgraph.Artifact{}, err
	}
	contentRef := "resourcehub://" + resource.ID + "/" + resource.Entry
	source := artifactgraph.SourceReference{Type: "managed", URI: "resourcehub://" + resource.ID, Revision: resource.Digest}
	if resource.Source != "" {
		source.Type = "local-path"
		source.URI = resource.Source
	}
	targets := make([]artifactgraph.TargetBinding, 0, len(resource.Targets))
	for _, target := range resource.Targets {
		targets = append(targets, artifactgraph.TargetBinding{Target: string(target), Scope: resource.Scope})
	}
	extension, err := json.Marshal(resourceHubExtension{
		Entry:    resource.Entry,
		Enabled:  resource.Enabled,
		Metadata: cloneMetadata(resource.Metadata),
	})
	if err != nil {
		return artifactgraph.Artifact{}, fmt.Errorf("marshal resource extension: %w", err)
	}
	return artifactgraph.Seal(artifactgraph.Artifact{
		ID:   "local/" + string(kind) + "/" + resource.ID,
		Kind: kind,
		Metadata: artifactgraph.Metadata{
			Namespace:   "local",
			Name:        resource.ID,
			DisplayName: resource.Name,
			Description: resource.Description,
			Scope:       resource.Scope,
			Tags:        append([]string(nil), resource.Tags...),
			Labels:      cloneMetadata(resource.Metadata),
		},
		Content: artifactgraph.ContentReference{
			Ref:       contentRef,
			Digest:    resource.Digest,
			MediaType: resourceArtifactMediaType,
		},
		Source: source,
		Security: artifactgraph.SecurityClassification{
			ExecutionClass: executionClass,
			Capabilities:   capabilities,
		},
		Targets: targets,
		Provenance: artifactgraph.Provenance{
			Origin:     "asm.resourcehub/v1",
			ImportedBy: "resourcehub",
			ImportedAt: resource.ImportedAt,
			UpdatedAt:  resource.UpdatedAt,
			Fields: map[string]artifactgraph.FieldProvenance{
				"/content":              {Source: "resourcehub", Path: resource.Entry, Transform: "reference-preserved"},
				"/content/digest":       {Source: "resourcehub", Path: "digest", Transform: "preserved"},
				"/metadata/description": {Source: "resourcehub", Path: "description", Transform: "preserved"},
				"/metadata/labels":      {Source: "resourcehub", Path: "metadata", Transform: "preserved"},
				"/metadata/scope":       {Source: "resourcehub", Path: "scope", Transform: "preserved"},
				"/metadata/tags":        {Source: "resourcehub", Path: "tags", Transform: "sorted-deduplicated"},
				"/targets":              {Source: "resourcehub", Path: "targets", Transform: "typed-bindings"},
			},
		},
		Extensions: map[string]json.RawMessage{"asm.resourcehub.v1": extension},
	})
}

func canonicalResourceKind(kind Kind) (artifactgraph.Kind, artifactgraph.ExecutionClass, []string, error) {
	switch kind {
	case KindSkill:
		return artifactgraph.KindSkill, artifactgraph.ExecutionSandboxed, []string{"filesystem.read", "process.spawn.possible"}, nil
	case KindAgent:
		return artifactgraph.KindAgent, artifactgraph.ExecutionDeclarative, []string{"tool.invoke.possible"}, nil
	case KindRule:
		return artifactgraph.KindRule, artifactgraph.ExecutionDeclarative, nil, nil
	case KindCommand:
		return artifactgraph.KindCommand, artifactgraph.ExecutionInterpreted, []string{"tool.invoke.possible"}, nil
	case KindPrompt:
		return artifactgraph.KindPrompt, artifactgraph.ExecutionDeclarative, nil, nil
	case KindMCPServer:
		return artifactgraph.KindMCPServer, artifactgraph.ExecutionPrivileged, []string{"network.possible", "process.spawn"}, nil
	case KindContext:
		return artifactgraph.KindContextResource, artifactgraph.ExecutionDeclarative, nil, nil
	default:
		return "", "", nil, fmt.Errorf("unsupported resource kind %q", kind)
	}
}

func cloneMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
