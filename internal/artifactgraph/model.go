// Package artifactgraph defines ASM's versioned canonical envelope for agent
// stack artefacts. It is intentionally independent of target files and
// persistence so existing stores can migrate into it without changing their
// current authority or mutation semantics.
package artifactgraph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const APIVersion = "fabric.asm.dev/v1alpha1"

type Kind string

const (
	KindInstruction       Kind = "Instruction"
	KindRule              Kind = "Rule"
	KindSkill             Kind = "Skill"
	KindAgent             Kind = "Agent"
	KindPrompt            Kind = "Prompt"
	KindCommand           Kind = "Command"
	KindHook              Kind = "Hook"
	KindPlugin            Kind = "Plugin"
	KindMCPServer         Kind = "MCPServer"
	KindMCPResource       Kind = "MCPResource"
	KindMCPPrompt         Kind = "MCPPrompt"
	KindContextResource   Kind = "ContextResource"
	KindPolicyFragment    Kind = "PolicyFragment"
	KindRoutine           Kind = "Routine"
	KindWorkspaceTemplate Kind = "WorkspaceTemplate"
	KindAdapter           Kind = "Adapter"
)

type ExecutionClass string

const (
	ExecutionDeclarative ExecutionClass = "declarative"
	ExecutionInterpreted ExecutionClass = "interpreted"
	ExecutionSandboxed   ExecutionClass = "sandboxed"
	ExecutionPrivileged  ExecutionClass = "privileged"
	ExecutionForbidden   ExecutionClass = "forbidden"
)

type Artifact struct {
	APIVersion string                     `json:"apiVersion"`
	ID         string                     `json:"id"`
	Kind       Kind                       `json:"kind"`
	Metadata   Metadata                   `json:"metadata"`
	Content    ContentReference           `json:"content"`
	Source     SourceReference            `json:"source"`
	Security   SecurityClassification     `json:"security"`
	Targets    []TargetBinding            `json:"targets,omitempty"`
	Provenance Provenance                 `json:"provenance"`
	Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
	Digest     string                     `json:"digest"`
}

type Metadata struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName,omitempty"`
	Description string            `json:"description,omitempty"`
	Version     string            `json:"version,omitempty"`
	Scope       string            `json:"scope,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type ContentReference struct {
	Ref       string `json:"ref"`
	Digest    string `json:"digest"`
	MediaType string `json:"mediaType"`
}

type SourceReference struct {
	Type     string `json:"type"`
	URI      string `json:"uri"`
	Revision string `json:"revision,omitempty"`
}

type SecurityClassification struct {
	ExecutionClass ExecutionClass `json:"executionClass"`
	Capabilities   []string       `json:"capabilities,omitempty"`
}

type TargetBinding struct {
	Target string `json:"target"`
	Scope  string `json:"scope,omitempty"`
	Mode   string `json:"mode,omitempty"`
}

type Provenance struct {
	Origin     string                     `json:"origin"`
	ImportedBy string                     `json:"importedBy"`
	ImportedAt time.Time                  `json:"importedAt"`
	UpdatedAt  time.Time                  `json:"updatedAt"`
	Fields     map[string]FieldProvenance `json:"fields,omitempty"`
}

type FieldProvenance struct {
	Source    string `json:"source"`
	Path      string `json:"path,omitempty"`
	Transform string `json:"transform,omitempty"`
}

type Snapshot struct {
	APIVersion string     `json:"apiVersion"`
	Artifacts  []Artifact `json:"artifacts"`
	Digest     string     `json:"digest"`
}

// Seal normalizes an artifact, validates its structural invariants, and binds
// it to a deterministic digest. The caller's value is not mutated.
func Seal(value Artifact) (Artifact, error) {
	normalized, err := normalizeArtifact(value)
	if err != nil {
		return Artifact{}, err
	}
	normalized.Digest = ""
	digest, err := integrity.DigestJSON(normalized)
	if err != nil {
		return Artifact{}, fmt.Errorf("digest canonical artifact: %w", err)
	}
	normalized.Digest = digest
	return normalized, nil
}

// Verify rejects malformed or tampered canonical artifacts.
func Verify(value Artifact) error {
	if !validSHA256Digest(value.Digest) {
		return fmt.Errorf("artifact %q has invalid digest", value.ID)
	}
	expected, err := Seal(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("artifact %q digest mismatch", value.ID)
	}
	return nil
}

// CanonicalJSON returns the stable JSON representation used by package,
// lockfile, and migration layers. It always reseals the artifact first.
func CanonicalJSON(value Artifact) ([]byte, error) {
	sealed, err := Seal(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(sealed)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical artifact: %w", err)
	}
	return data, nil
}

// NewSnapshot seals and sorts artifacts before binding the whole graph view to
// a deterministic digest. Duplicate canonical IDs are rejected.
func NewSnapshot(values []Artifact) (Snapshot, error) {
	artifacts := make([]Artifact, 0, len(values))
	for _, value := range values {
		sealed, err := Seal(value)
		if err != nil {
			return Snapshot{}, err
		}
		artifacts = append(artifacts, sealed)
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	for index := 1; index < len(artifacts); index++ {
		if artifacts[index-1].ID == artifacts[index].ID {
			return Snapshot{}, fmt.Errorf("duplicate canonical artifact id %q", artifacts[index].ID)
		}
	}
	snapshot := Snapshot{APIVersion: APIVersion, Artifacts: artifacts}
	digest, err := integrity.DigestJSON(snapshot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("digest artifact snapshot: %w", err)
	}
	snapshot.Digest = digest
	return snapshot, nil
}

// VerifySnapshot validates every artifact and the graph-level digest.
func VerifySnapshot(snapshot Snapshot) error {
	if snapshot.APIVersion != APIVersion {
		return fmt.Errorf("unsupported artifact snapshot apiVersion %q", snapshot.APIVersion)
	}
	if !validSHA256Digest(snapshot.Digest) {
		return fmt.Errorf("artifact snapshot has invalid digest")
	}
	for _, artifact := range snapshot.Artifacts {
		if err := Verify(artifact); err != nil {
			return err
		}
	}
	expected, err := NewSnapshot(snapshot.Artifacts)
	if err != nil {
		return err
	}
	if expected.Digest != snapshot.Digest {
		return fmt.Errorf("artifact snapshot digest mismatch")
	}
	return nil
}

func normalizeArtifact(value Artifact) (Artifact, error) {
	if value.APIVersion == "" {
		value.APIVersion = APIVersion
	}
	value.ID = strings.TrimSpace(value.ID)
	value.Metadata.Namespace = strings.TrimSpace(value.Metadata.Namespace)
	value.Metadata.Name = strings.TrimSpace(value.Metadata.Name)
	value.Metadata.DisplayName = strings.TrimSpace(value.Metadata.DisplayName)
	value.Metadata.Description = strings.TrimSpace(value.Metadata.Description)
	value.Metadata.Version = strings.TrimSpace(value.Metadata.Version)
	value.Metadata.Scope = strings.TrimSpace(value.Metadata.Scope)
	value.Content.Ref = strings.TrimSpace(value.Content.Ref)
	value.Content.Digest = strings.TrimSpace(value.Content.Digest)
	value.Content.MediaType = strings.TrimSpace(value.Content.MediaType)
	value.Source.Type = strings.TrimSpace(value.Source.Type)
	value.Source.URI = strings.TrimSpace(value.Source.URI)
	value.Source.Revision = strings.TrimSpace(value.Source.Revision)
	value.Provenance.Origin = strings.TrimSpace(value.Provenance.Origin)
	value.Provenance.ImportedBy = strings.TrimSpace(value.Provenance.ImportedBy)
	value.Provenance.ImportedAt = value.Provenance.ImportedAt.UTC()
	value.Provenance.UpdatedAt = value.Provenance.UpdatedAt.UTC()
	value.Metadata.Tags = sortedUnique(value.Metadata.Tags)
	value.Security.Capabilities = sortedUnique(value.Security.Capabilities)
	value.Metadata.Labels = cloneStringMap(value.Metadata.Labels)
	value.Provenance.Fields = cloneFieldMap(value.Provenance.Fields)
	value.Targets = normalizeTargets(value.Targets)

	if len(value.Extensions) > 0 {
		canonicalExtensions := make(map[string]json.RawMessage, len(value.Extensions))
		for key, raw := range cloneRawMap(value.Extensions) {
			trimmed := strings.TrimSpace(key)
			if trimmed != key || !validName(key) {
				return Artifact{}, fmt.Errorf("artifact %q has invalid extension key %q", value.ID, key)
			}
			canonical, err := strictjson.Canonicalize(raw)
			if err != nil {
				return Artifact{}, fmt.Errorf("artifact %q extension %q: %w", value.ID, key, err)
			}
			canonicalExtensions[key] = canonical
		}
		value.Extensions = canonicalExtensions
	}

	if err := validateStructure(value); err != nil {
		return Artifact{}, err
	}
	return value, nil
}

func validateStructure(value Artifact) error {
	if value.APIVersion != APIVersion {
		return fmt.Errorf("artifact %q has unsupported apiVersion %q", value.ID, value.APIVersion)
	}
	if !validID(value.ID) {
		return fmt.Errorf("invalid canonical artifact id %q", value.ID)
	}
	if !validKind(value.Kind) {
		return fmt.Errorf("artifact %q has unsupported kind %q", value.ID, value.Kind)
	}
	if !validName(value.Metadata.Namespace) || !validName(value.Metadata.Name) {
		return fmt.Errorf("artifact %q has invalid namespace or name", value.ID)
	}
	expectedID := value.Metadata.Namespace + "/" + string(value.Kind) + "/" + value.Metadata.Name
	if value.ID != expectedID {
		return fmt.Errorf("artifact id %q does not match canonical identity %q", value.ID, expectedID)
	}
	if value.Content.Ref == "" || value.Content.MediaType == "" || !validSHA256Digest(value.Content.Digest) {
		return fmt.Errorf("artifact %q has invalid content reference", value.ID)
	}
	if value.Source.Type == "" || value.Source.URI == "" {
		return fmt.Errorf("artifact %q has invalid source reference", value.ID)
	}
	if !validExecutionClass(value.Security.ExecutionClass) {
		return fmt.Errorf("artifact %q has invalid execution class %q", value.ID, value.Security.ExecutionClass)
	}
	if value.Provenance.Origin == "" || value.Provenance.ImportedBy == "" || value.Provenance.ImportedAt.IsZero() || value.Provenance.UpdatedAt.IsZero() {
		return fmt.Errorf("artifact %q has incomplete provenance", value.ID)
	}
	for _, target := range value.Targets {
		if !validName(target.Target) {
			return fmt.Errorf("artifact %q has invalid target %q", value.ID, target.Target)
		}
	}
	for path, field := range value.Provenance.Fields {
		if !strings.HasPrefix(path, "/") || strings.TrimSpace(field.Source) == "" {
			return fmt.Errorf("artifact %q has invalid field provenance %q", value.ID, path)
		}
	}
	return nil
}

func validKind(kind Kind) bool {
	switch kind {
	case KindInstruction, KindRule, KindSkill, KindAgent, KindPrompt, KindCommand,
		KindHook, KindPlugin, KindMCPServer, KindMCPResource, KindMCPPrompt,
		KindContextResource, KindPolicyFragment, KindRoutine, KindWorkspaceTemplate,
		KindAdapter:
		return true
	default:
		return false
	}
}

func validExecutionClass(class ExecutionClass) bool {
	switch class {
	case ExecutionDeclarative, ExecutionInterpreted, ExecutionSandboxed, ExecutionPrivileged, ExecutionForbidden:
		return true
	default:
		return false
	}
}

func validName(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func validID(value string) bool {
	if value == "" || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") || strings.Contains(value, "//") {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if !validName(part) {
			return false
		}
	}
	return true
}

func validSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, r := range strings.TrimPrefix(value, "sha256:") {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func sortedUnique(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeTargets(values []TargetBinding) []TargetBinding {
	result := make([]TargetBinding, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value.Target = strings.TrimSpace(value.Target)
		value.Scope = strings.TrimSpace(value.Scope)
		value.Mode = strings.TrimSpace(value.Mode)
		key := value.Target + "\x00" + value.Scope + "\x00" + value.Mode
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Target != result[j].Target {
			return result[i].Target < result[j].Target
		}
		if result[i].Scope != result[j].Scope {
			return result[i].Scope < result[j].Scope
		}
		return result[i].Mode < result[j].Mode
	})
	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneFieldMap(values map[string]FieldProvenance) map[string]FieldProvenance {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]FieldProvenance, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneRawMap(values map[string]json.RawMessage) map[string]json.RawMessage {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		result[key] = append(json.RawMessage(nil), value...)
	}
	return result
}
