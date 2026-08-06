// Package conformance provides ASM's embedded, machine-readable target adapter
// conformance corpus. The corpus is an oracle for the reviewed in-process
// adapters; it does not grant adapters mutation authority or load external code.
package conformance

import (
	_ "embed"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const (
	CorpusAPIVersion = "fabric.asm.dev/adapter-conformance/v1alpha1"
	ReportAPIVersion = "fabric.asm.dev/adapter-conformance-report/v1alpha1"
)

//go:embed testdata/corpus.json
var embeddedCorpus []byte

type Corpus struct {
	APIVersion      string          `json:"apiVersion"`
	AdapterContract string          `json:"adapterContract"`
	Targets         []TargetFixture `json:"targets"`
	Digest          string          `json:"digest"`
}

type TargetFixture struct {
	Target             string            `json:"target"`
	AdapterID          string            `json:"adapterId"`
	AdapterVersion     string            `json:"adapterVersion"`
	TargetVersionRange string            `json:"targetVersionRange"`
	Aliases            []string          `json:"aliases,omitempty"`
	DeploymentModes    []string          `json:"deploymentModes"`
	MCP                MCPFixture        `json:"mcp"`
	Artifacts          []ArtifactFixture `json:"artifacts,omitempty"`
}

type MCPFixture struct {
	Support          adapters.SupportLevel        `json:"support"`
	RegistrationMode adapters.MCPRegistrationMode `json:"registrationMode"`
	LocationKind     string                       `json:"locationKind"`
	Location         string                       `json:"location,omitempty"`
	RootKey          string                       `json:"rootKey,omitempty"`
	EntryName        string                       `json:"entryName,omitempty"`
	Transports       []string                     `json:"transports,omitempty"`
}

type ArtifactFixture struct {
	Kind                artifactgraph.Kind               `json:"kind"`
	Support             adapters.SupportLevel            `json:"support"`
	Directory           string                           `json:"directory,omitempty"`
	Format              string                           `json:"format,omitempty"`
	Fields              map[string]adapters.FieldSupport `json:"fields,omitempty"`
	RelativeDestination string                           `json:"relativeDestination,omitempty"`
	Fidelity            adapters.Fidelity                `json:"fidelity"`
	Losses              []LossFixture                    `json:"losses,omitempty"`
}

type LossFixture struct {
	Field    string            `json:"field"`
	Kind     adapters.LossKind `json:"kind"`
	Code     string            `json:"code"`
	Required bool              `json:"required,omitempty"`
}

func LoadEmbedded() (Corpus, error) {
	var value Corpus
	if err := strictjson.Decode(embeddedCorpus, &value); err != nil {
		return Corpus{}, fmt.Errorf("decode embedded adapter conformance corpus: %w", err)
	}
	return SealCorpus(value)
}

func SealCorpus(value Corpus) (Corpus, error) {
	if value.APIVersion == "" {
		value.APIVersion = CorpusAPIVersion
	}
	value.AdapterContract = strings.TrimSpace(value.AdapterContract)
	if value.APIVersion != CorpusAPIVersion {
		return Corpus{}, fmt.Errorf("unsupported conformance corpus apiVersion %q", value.APIVersion)
	}
	if value.AdapterContract != adapters.ContractVersion {
		return Corpus{}, fmt.Errorf("unsupported adapter contract %q", value.AdapterContract)
	}
	if len(value.Targets) == 0 {
		return Corpus{}, fmt.Errorf("conformance corpus has no targets")
	}
	value.Targets = append([]TargetFixture(nil), value.Targets...)
	seenTargets := map[string]struct{}{}
	seenNames := map[string]string{}
	for i := range value.Targets {
		target := &value.Targets[i]
		target.Target = strings.TrimSpace(target.Target)
		target.AdapterID = strings.TrimSpace(target.AdapterID)
		target.AdapterVersion = strings.TrimSpace(target.AdapterVersion)
		target.TargetVersionRange = strings.TrimSpace(target.TargetVersionRange)
		if target.Target == "" || target.AdapterID == "" || target.AdapterVersion == "" || target.TargetVersionRange == "" {
			return Corpus{}, fmt.Errorf("target fixture at index %d has incomplete identity", i)
		}
		if _, duplicate := seenTargets[target.Target]; duplicate {
			return Corpus{}, fmt.Errorf("duplicate target fixture %q", target.Target)
		}
		seenTargets[target.Target] = struct{}{}
		if owner, duplicate := seenNames[target.Target]; duplicate {
			return Corpus{}, fmt.Errorf("target name %q collides with %q", target.Target, owner)
		}
		seenNames[target.Target] = target.Target
		target.Aliases = sortedUnique(target.Aliases)
		for _, alias := range target.Aliases {
			if alias == target.Target {
				return Corpus{}, fmt.Errorf("target %q repeats itself as an alias", target.Target)
			}
			if owner, duplicate := seenNames[alias]; duplicate {
				return Corpus{}, fmt.Errorf("adapter name %q collides between %q and %q", alias, owner, target.Target)
			}
			seenNames[alias] = target.Target
		}
		target.DeploymentModes = sortedUnique(target.DeploymentModes)
		if len(target.DeploymentModes) == 0 {
			return Corpus{}, fmt.Errorf("target %q has no deployment modes", target.Target)
		}
		if err := normalizeMCPFixture(&target.MCP); err != nil {
			return Corpus{}, fmt.Errorf("target %q MCP fixture: %w", target.Target, err)
		}
		target.Artifacts = append([]ArtifactFixture(nil), target.Artifacts...)
		seenKinds := map[artifactgraph.Kind]struct{}{}
		for j := range target.Artifacts {
			fixture := &target.Artifacts[j]
			if !validArtifactKind(fixture.Kind) {
				return Corpus{}, fmt.Errorf("target %q has invalid artifact kind %q", target.Target, fixture.Kind)
			}
			if _, duplicate := seenKinds[fixture.Kind]; duplicate {
				return Corpus{}, fmt.Errorf("target %q repeats artifact kind %q", target.Target, fixture.Kind)
			}
			seenKinds[fixture.Kind] = struct{}{}
			if err := normalizeArtifactFixture(fixture); err != nil {
				return Corpus{}, fmt.Errorf("target %q artifact %q: %w", target.Target, fixture.Kind, err)
			}
		}
		sort.Slice(target.Artifacts, func(i, j int) bool { return target.Artifacts[i].Kind < target.Artifacts[j].Kind })
	}
	sort.Slice(value.Targets, func(i, j int) bool { return value.Targets[i].Target < value.Targets[j].Target })
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return Corpus{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyCorpus(value Corpus) error {
	if !validDigest(value.Digest) {
		return fmt.Errorf("invalid conformance corpus digest")
	}
	expected, err := SealCorpus(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("conformance corpus digest mismatch")
	}
	return nil
}

func normalizeMCPFixture(value *MCPFixture) error {
	value.LocationKind = strings.TrimSpace(value.LocationKind)
	value.Location = strings.TrimSpace(value.Location)
	value.RootKey = strings.TrimSpace(value.RootKey)
	value.EntryName = strings.TrimSpace(value.EntryName)
	value.Transports = sortedUnique(value.Transports)
	if !validSupport(value.Support) {
		return fmt.Errorf("invalid support %q", value.Support)
	}
	switch value.RegistrationMode {
	case adapters.MCPRegistrationCommand, adapters.MCPRegistrationJSONFile:
		if value.Support == adapters.SupportUnsupported || value.LocationKind == "" || value.Location == "" || value.EntryName == "" {
			return fmt.Errorf("supported registration is missing structural fields")
		}
	case adapters.MCPRegistrationNone:
		if value.Support != adapters.SupportUnsupported || value.LocationKind != "none" || value.Location != "" || value.RootKey != "" || value.EntryName != "" || len(value.Transports) != 0 {
			return fmt.Errorf("none registration must be empty and unsupported")
		}
	default:
		return fmt.Errorf("invalid registration mode %q", value.RegistrationMode)
	}
	switch value.LocationKind {
	case "literal", "project-relative", "agy-config", "none":
	default:
		return fmt.Errorf("invalid location kind %q", value.LocationKind)
	}
	if value.LocationKind == "project-relative" && !validRelative(value.Location) {
		return fmt.Errorf("invalid project-relative location %q", value.Location)
	}
	return nil
}

func normalizeArtifactFixture(value *ArtifactFixture) error {
	value.Directory = strings.ReplaceAll(strings.TrimSpace(value.Directory), "\\", "/")
	value.Format = strings.TrimSpace(value.Format)
	value.RelativeDestination = strings.ReplaceAll(strings.TrimSpace(value.RelativeDestination), "\\", "/")
	if !validSupport(value.Support) || !validFidelity(value.Fidelity) {
		return fmt.Errorf("invalid support or fidelity")
	}
	if value.Support == adapters.SupportUnsupported {
		if value.Directory != "" || value.Format != "" || value.RelativeDestination != "" || len(value.Fields) != 0 || value.Fidelity != adapters.FidelityBlocked {
			return fmt.Errorf("unsupported fixture must not declare a projection")
		}
	} else {
		if value.Directory == "" || value.Format == "" || !validRelativeTemplate(value.RelativeDestination) {
			return fmt.Errorf("supported fixture has an invalid projection")
		}
	}
	if len(value.Fields) > 0 {
		fields := make(map[string]adapters.FieldSupport, len(value.Fields))
		for field, support := range value.Fields {
			field = strings.TrimSpace(field)
			if field == "" || !validFieldSupport(support) {
				return fmt.Errorf("invalid field support for %q", field)
			}
			fields[field] = support
		}
		value.Fields = fields
	}
	value.Losses = append([]LossFixture(nil), value.Losses...)
	for i := range value.Losses {
		loss := &value.Losses[i]
		loss.Field = strings.TrimSpace(loss.Field)
		loss.Code = strings.TrimSpace(loss.Code)
		if loss.Field == "" || loss.Code == "" || !validLossKind(loss.Kind) {
			return fmt.Errorf("invalid loss at index %d", i)
		}
	}
	sort.Slice(value.Losses, func(i, j int) bool {
		if value.Losses[i].Field != value.Losses[j].Field {
			return value.Losses[i].Field < value.Losses[j].Field
		}
		if value.Losses[i].Kind != value.Losses[j].Kind {
			return value.Losses[i].Kind < value.Losses[j].Kind
		}
		return value.Losses[i].Code < value.Losses[j].Code
	})
	for i := 1; i < len(value.Losses); i++ {
		left, right := value.Losses[i-1], value.Losses[i]
		if left.Field == right.Field && left.Kind == right.Kind && left.Code == right.Code {
			return fmt.Errorf("duplicate loss %q", right.Code)
		}
	}
	if derivedFixtureFidelity(value.Losses) != value.Fidelity {
		return fmt.Errorf("declared fidelity %q does not match fixture losses", value.Fidelity)
	}
	return nil
}

func validRelative(value string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	clean := path.Clean(normalized)
	return clean == normalized && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/")
}

func validRelativeTemplate(value string) bool {
	if strings.Count(value, "{{name}}") != 1 {
		return false
	}
	return validRelative(strings.Replace(value, "{{name}}", "fixture", 1))
}

func validArtifactKind(value artifactgraph.Kind) bool {
	switch value {
	case artifactgraph.KindInstruction, artifactgraph.KindRule, artifactgraph.KindSkill, artifactgraph.KindAgent,
		artifactgraph.KindPrompt, artifactgraph.KindCommand, artifactgraph.KindHook, artifactgraph.KindPlugin,
		artifactgraph.KindMCPServer, artifactgraph.KindMCPResource, artifactgraph.KindMCPPrompt,
		artifactgraph.KindContextResource, artifactgraph.KindPolicyFragment, artifactgraph.KindRoutine,
		artifactgraph.KindWorkspaceTemplate, artifactgraph.KindAdapter:
		return true
	default:
		return false
	}
}

func validSupport(value adapters.SupportLevel) bool {
	switch value {
	case adapters.SupportNative, adapters.SupportPassthrough, adapters.SupportFallback, adapters.SupportUnsupported:
		return true
	default:
		return false
	}
}

func validFieldSupport(value adapters.FieldSupport) bool {
	switch value {
	case adapters.FieldNative, adapters.FieldMapped, adapters.FieldDefaulted, adapters.FieldOmitted, adapters.FieldUnsupported:
		return true
	default:
		return false
	}
}

func validFidelity(value adapters.Fidelity) bool {
	switch value {
	case adapters.FidelityFull, adapters.FidelityPartial, adapters.FidelityLossy, adapters.FidelityBlocked:
		return true
	default:
		return false
	}
}

func validLossKind(value adapters.LossKind) bool {
	switch value {
	case adapters.LossTransformation, adapters.LossFallback, adapters.LossOmission, adapters.LossUnsupported:
		return true
	default:
		return false
	}
}

func derivedFixtureFidelity(losses []LossFixture) adapters.Fidelity {
	result := adapters.FidelityFull
	for _, loss := range losses {
		switch loss.Kind {
		case adapters.LossUnsupported:
			return adapters.FidelityBlocked
		case adapters.LossOmission, adapters.LossFallback:
			result = adapters.FidelityLossy
		case adapters.LossTransformation:
			if result == adapters.FidelityFull {
				result = adapters.FidelityPartial
			}
		}
	}
	return result
}

func sortedUnique(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil
	}
	return result
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
