// Package builtin contains ASM's reviewed in-process adapters for the target
// clients already supported by Resource Hub and mcplink. They are deterministic
// projection codecs, not mutation authorities.
package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
)

const adapterVersion = "1.1.0"

const (
	TargetCodex    = "codex"
	TargetClaude   = "claude"
	TargetCursor   = "cursor"
	TargetAgy      = "agy"
	TargetOpenCode = "opencode"
	TargetCopilot  = "github-copilot"
	TargetGeneric  = "generic"
)

type pathRule struct {
	directory         string
	extension         string
	directoryArtifact bool
	support           adapters.SupportLevel
	format            string
	lossCode          string
	lossReason        string
}

type descriptor struct {
	target      string
	aliases     []string
	rules       map[artifactgraph.Kind]pathRule
	mcpMode     adapters.MCPRegistrationMode
	mcpLocation func(adapters.Environment) (string, error)
}

type Adapter struct{ descriptor descriptor }

func All() []adapters.Adapter {
	result := make([]adapters.Adapter, 0, len(descriptors()))
	for _, item := range descriptors() {
		result = append(result, Adapter{descriptor: item})
	}
	return result
}

func NewRegistry() (*adapters.Registry, error) { return adapters.NewRegistry(All()...) }

func MustRegistry() *adapters.Registry {
	registry, err := NewRegistry()
	if err != nil {
		panic(err)
	}
	return registry
}

func (a Adapter) ID() string            { return a.descriptor.target }
func (a Adapter) Aliases() []string     { return append([]string(nil), a.descriptor.aliases...) }
func (a Adapter) SchemaVersion() string { return adapters.ContractVersion }

func (a Adapter) Capabilities(_ context.Context, env adapters.Environment) (adapters.CapabilitySet, error) {
	location := ""
	rootKey := ""
	entryName := ""
	mcpSupport := adapters.SupportUnsupported
	mcpMode := adapters.MCPRegistrationNone
	if a.descriptor.mcpMode != "" && a.descriptor.mcpMode != adapters.MCPRegistrationNone {
		var err error
		location, err = a.descriptor.mcpLocation(env)
		if err != nil {
			return adapters.CapabilitySet{}, err
		}
		mcpSupport = adapters.SupportNative
		mcpMode = a.descriptor.mcpMode
		rootKey = "mcpServers"
		entryName = "agentstack-router"
	}
	artifacts := make(map[artifactgraph.Kind]adapters.ArtifactCapability, len(a.descriptor.rules))
	for kind, rule := range a.descriptor.rules {
		artifacts[kind] = adapters.ArtifactCapability{
			Support: rule.support, Scopes: []string{"project", "global"}, Directory: filepath.ToSlash(rule.directory), Format: rule.format,
			Fields: fieldMatrix(rule.support),
		}
	}
	return adapters.SealCapabilitySet(adapters.CapabilitySet{
		AdapterID: "asm.builtin." + a.descriptor.target, AdapterVersion: adapterVersion,
		Target: a.descriptor.target, Aliases: a.descriptor.aliases, TargetVersionRange: "*",
		Artifacts: artifacts, DeploymentModes: []string{"copy", "link"},
		MCP: adapters.MCPClientCapability{Support: mcpSupport, RegistrationMode: mcpMode, Location: location, RootKey: rootKey, EntryName: entryName, Transports: transportList(mcpSupport)},
	})
}

func (a Adapter) Discover(_ context.Context, req adapters.DiscoverRequest) ([]adapters.ObservedArtifact, error) {
	result := append([]adapters.ObservedArtifact(nil), req.Candidates...)
	for i := range result {
		result[i].ArtifactID = strings.TrimSpace(result[i].ArtifactID)
		result[i].Location = strings.TrimSpace(result[i].Location)
		result[i].Digest = strings.TrimSpace(result[i].Digest)
		result[i].BaseDigest = strings.TrimSpace(result[i].BaseDigest)
		if result[i].ArtifactID == "" || result[i].Location == "" {
			return nil, fmt.Errorf("invalid observed artifact at index %d", i)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Location == result[j].Location {
			return result[i].ArtifactID < result[j].ArtifactID
		}
		return result[i].Location < result[j].Location
	})
	return result, nil
}

func (a Adapter) Import(ctx context.Context, req adapters.ImportRequest) (artifactgraph.Artifact, adapters.LossReport, error) {
	candidate, err := artifactgraph.Seal(req.Candidate)
	if err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	capability, err := a.Capabilities(ctx, req.Environment)
	if err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	report, err := a.lossReport(capability, candidate)
	if err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	return candidate, report, nil
}

func (a Adapter) Render(ctx context.Context, req adapters.RenderRequest) (adapters.RenderedSet, adapters.LossReport, error) {
	if err := artifactgraph.Verify(req.Artifact); err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	capability, err := a.Capabilities(ctx, req.Environment)
	if err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	rule, ok := a.descriptor.rules[req.Artifact.Kind]
	if !ok || rule.support == adapters.SupportUnsupported {
		report, reportErr := adapters.SealLossReport(adapters.LossReport{
			Target: capability.Target, AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest,
			Losses: []adapters.Loss{{ArtifactID: req.Artifact.ID, Field: "/", Kind: adapters.LossUnsupported, Code: "artifact-kind-unsupported", Reason: "target adapter does not support this artifact kind", Required: true}},
		})
		if reportErr != nil {
			return adapters.RenderedSet{}, adapters.LossReport{}, reportErr
		}
		return adapters.RenderedSet{}, report, fmt.Errorf("artifact kind %q is unsupported for target %q", req.Artifact.Kind, capability.Target)
	}
	root, err := absoluteRoot(req.Environment.TargetRoot)
	if err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	relative := destination(rule, req.Artifact.Metadata.Name)
	absolute := filepath.Join(root, filepath.FromSlash(relative))
	if !within(root, absolute) {
		return adapters.RenderedSet{}, adapters.LossReport{}, fmt.Errorf("rendered destination escapes target root")
	}
	rendered, err := adapters.SealRenderedSet(adapters.RenderedSet{
		AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, Target: capability.Target,
		Outputs: []adapters.RenderedArtifact{{ArtifactID: req.Artifact.ID, Kind: req.Artifact.Kind, SourcePath: req.SourcePath, Destination: absolute, RelativeDestination: filepath.ToSlash(relative), DesiredDigest: req.Artifact.Content.Digest, Support: rule.support}},
	})
	if err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	report, err := a.lossReport(capability, req.Artifact)
	if err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	return rendered, report, nil
}

func (a Adapter) Plan(_ context.Context, req adapters.PlanRequest) ([]adapters.ProposedOperation, error) {
	return adapters.Plan(req)
}

func (a Adapter) Verify(_ context.Context, req adapters.VerifyRequest) (adapters.VerificationResult, error) {
	return adapters.Verify(req), nil
}

func (a Adapter) lossReport(capability adapters.CapabilitySet, artifact artifactgraph.Artifact) (adapters.LossReport, error) {
	rule, ok := a.descriptor.rules[artifact.Kind]
	losses := []adapters.Loss{}
	if !ok || rule.support == adapters.SupportUnsupported {
		losses = append(losses, adapters.Loss{ArtifactID: artifact.ID, Field: "/", Kind: adapters.LossUnsupported, Code: "artifact-kind-unsupported", Reason: "target adapter does not support this artifact kind", Required: true})
	} else {
		switch rule.support {
		case adapters.SupportPassthrough:
			losses = append(losses, adapters.Loss{ArtifactID: artifact.ID, Field: "/content", Kind: adapters.LossTransformation, Code: rule.lossCode, Reason: rule.lossReason})
			if artifact.Metadata.Description != "" {
				losses = append(losses, adapters.Loss{ArtifactID: artifact.ID, Field: "/metadata/description", Kind: adapters.LossOmission, Code: "metadata-description-omitted", Reason: "canonical description is retained by ASM but is not projected into the target artifact"})
			}
			if len(artifact.Metadata.Labels) > 0 {
				losses = append(losses, adapters.Loss{ArtifactID: artifact.ID, Field: "/metadata/labels", Kind: adapters.LossOmission, Code: "metadata-labels-omitted", Reason: "canonical labels are retained by ASM but are not projected into the target artifact"})
			}
		case adapters.SupportFallback:
			losses = append(losses, adapters.Loss{ArtifactID: artifact.ID, Field: "/destination", Kind: adapters.LossFallback, Code: rule.lossCode, Reason: rule.lossReason})
		}
	}
	return adapters.SealLossReport(adapters.LossReport{Target: capability.Target, AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest, Losses: losses})
}

func descriptors() []descriptor {
	markdownPassthrough := func(directory, extension string) pathRule {
		return pathRule{directory: directory, extension: extension, support: adapters.SupportPassthrough, format: "markdown", lossCode: "content-passthrough", lossReason: "content is preserved byte-for-byte without target-specific semantic normalization"}
	}
	nativeSkill := func(directory string) pathRule {
		return pathRule{directory: directory, directoryArtifact: true, support: adapters.SupportNative, format: "agent-skill-directory"}
	}
	fallbackMCP := pathRule{directory: ".agentstack/mcp", extension: ".json", support: adapters.SupportFallback, format: "json", lossCode: "target-native-registration-not-rendered", lossReason: "MCP definition is preserved in ASM fallback storage and is not registered in the target by Resource Hub"}
	fallback := func(kind string) pathRule {
		return pathRule{directory: ".agentstack/" + kind, support: adapters.SupportFallback, format: "opaque", lossCode: "target-native-projection-unavailable", lossReason: "artifact is preserved in ASM fallback storage because the target-native representation is not implemented"}
	}
	return []descriptor{
		{target: TargetCodex, aliases: []string{"openai-codex"}, mcpMode: adapters.MCPRegistrationCommand, mcpLocation: func(adapters.Environment) (string, error) { return "codex:mcp:agentstack-router", nil }, rules: map[artifactgraph.Kind]pathRule{
			artifactgraph.KindSkill: nativeSkill(".agents/skills"), artifactgraph.KindAgent: markdownPassthrough(".codex/agents", ".md"), artifactgraph.KindRule: markdownPassthrough(".agents/rules", ".md"), artifactgraph.KindContextResource: markdownPassthrough(".agents/rules", ".md"), artifactgraph.KindCommand: markdownPassthrough(".codex/prompts", ".md"), artifactgraph.KindPrompt: markdownPassthrough(".codex/prompts", ".md"), artifactgraph.KindMCPServer: fallbackMCP,
		}},
		{target: TargetClaude, aliases: []string{"claude-code"}, mcpMode: adapters.MCPRegistrationJSONFile, mcpLocation: projectPath(".mcp.json"), rules: map[artifactgraph.Kind]pathRule{
			artifactgraph.KindSkill: nativeSkill(".claude/skills"), artifactgraph.KindAgent: markdownPassthrough(".claude/agents", ".md"), artifactgraph.KindRule: markdownPassthrough(".claude/rules", ".md"), artifactgraph.KindContextResource: markdownPassthrough(".claude/rules", ".md"), artifactgraph.KindCommand: markdownPassthrough(".claude/commands", ".md"), artifactgraph.KindPrompt: markdownPassthrough(".claude/commands", ".md"), artifactgraph.KindMCPServer: fallbackMCP,
		}},
		{target: TargetCursor, mcpMode: adapters.MCPRegistrationJSONFile, mcpLocation: projectPath(".cursor", "mcp.json"), rules: map[artifactgraph.Kind]pathRule{
			artifactgraph.KindSkill: nativeSkill(".cursor/skills"), artifactgraph.KindAgent: markdownPassthrough(".cursor/agents", ".md"), artifactgraph.KindRule: markdownPassthrough(".cursor/rules", ".mdc"), artifactgraph.KindContextResource: markdownPassthrough(".cursor/rules", ".mdc"), artifactgraph.KindCommand: markdownPassthrough(".cursor/commands", ".md"), artifactgraph.KindPrompt: markdownPassthrough(".cursor/commands", ".md"), artifactgraph.KindMCPServer: fallbackMCP,
		}},
		{target: TargetAgy, aliases: []string{"gemini", "gemini-cli", "antigravity"}, mcpMode: adapters.MCPRegistrationJSONFile, mcpLocation: agyPath, rules: map[artifactgraph.Kind]pathRule{}},
		{target: TargetOpenCode, mcpMode: adapters.MCPRegistrationJSONFile, mcpLocation: projectPath("opencode.json"), rules: map[artifactgraph.Kind]pathRule{
			artifactgraph.KindSkill: nativeSkill(".opencode/skills"), artifactgraph.KindAgent: markdownPassthrough(".opencode/agents", ".md"), artifactgraph.KindRule: markdownPassthrough(".opencode/rules", ".md"), artifactgraph.KindContextResource: markdownPassthrough(".opencode/rules", ".md"), artifactgraph.KindCommand: markdownPassthrough(".opencode/commands", ".md"), artifactgraph.KindPrompt: markdownPassthrough(".opencode/commands", ".md"), artifactgraph.KindMCPServer: fallbackMCP,
		}},
		{target: TargetCopilot, aliases: []string{"copilot"}, mcpMode: adapters.MCPRegistrationNone, rules: map[artifactgraph.Kind]pathRule{
			artifactgraph.KindRule: markdownPassthrough(".github/instructions", ".instructions.md"), artifactgraph.KindContextResource: markdownPassthrough(".github/instructions", ".instructions.md"), artifactgraph.KindPrompt: markdownPassthrough(".github/prompts", ".prompt.md"), artifactgraph.KindCommand: markdownPassthrough(".github/prompts", ".prompt.md"), artifactgraph.KindSkill: fallback("skill"), artifactgraph.KindAgent: fallback("agent"), artifactgraph.KindMCPServer: fallback("mcp-server"),
		}},
		{target: TargetGeneric, mcpMode: adapters.MCPRegistrationNone, rules: map[artifactgraph.Kind]pathRule{
			artifactgraph.KindSkill: fallback("skill"), artifactgraph.KindAgent: fallback("agent"), artifactgraph.KindRule: fallback("rule"), artifactgraph.KindContextResource: fallback("context"), artifactgraph.KindPrompt: fallback("prompt"), artifactgraph.KindCommand: fallback("command"), artifactgraph.KindMCPServer: fallback("mcp-server"),
		}},
	}
}

func destination(rule pathRule, name string) string {
	name = strings.TrimSpace(name)
	if rule.directoryArtifact {
		return filepath.ToSlash(filepath.Join(rule.directory, name))
	}
	return filepath.ToSlash(filepath.Join(rule.directory, name+rule.extension))
}

func fieldMatrix(support adapters.SupportLevel) map[string]adapters.FieldSupport {
	switch support {
	case adapters.SupportNative:
		return map[string]adapters.FieldSupport{"content": adapters.FieldNative, "metadata.name": adapters.FieldNative, "metadata.scope": adapters.FieldMapped}
	case adapters.SupportPassthrough:
		return map[string]adapters.FieldSupport{"content": adapters.FieldMapped, "metadata.name": adapters.FieldMapped, "metadata.description": adapters.FieldOmitted, "metadata.labels": adapters.FieldOmitted}
	case adapters.SupportFallback:
		return map[string]adapters.FieldSupport{"content": adapters.FieldNative, "metadata": adapters.FieldNative}
	default:
		return map[string]adapters.FieldSupport{"content": adapters.FieldUnsupported}
	}
}

func transportList(support adapters.SupportLevel) []string {
	if support == adapters.SupportUnsupported {
		return nil
	}
	return []string{"stdio"}
}

func projectPath(parts ...string) func(adapters.Environment) (string, error) {
	return func(env adapters.Environment) (string, error) {
		root := strings.TrimSpace(env.ProjectRoot)
		if root == "" {
			root = "."
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		return filepath.Join(append([]string{absolute}, parts...)...), nil
	}
}

func agyPath(env adapters.Environment) (string, error) {
	if strings.TrimSpace(env.AgyConfig) != "" {
		return filepath.Abs(env.AgyConfig)
	}
	home := strings.TrimSpace(env.Home)
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home for AGY MCP config: %w", err)
		}
	}
	return filepath.Join(home, ".gemini", "config", "mcp_config.json"), nil
}

func absoluteRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func RuntimeEnvironment(projectRoot, targetRoot, home, agyConfig string) adapters.Environment {
	return adapters.Environment{OS: runtime.GOOS, Architecture: runtime.GOARCH, ProjectRoot: projectRoot, TargetRoot: targetRoot, Home: home, AgyConfig: agyConfig}
}
