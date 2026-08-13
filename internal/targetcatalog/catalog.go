package targetcatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

type Catalog struct {
	schema CatalogSchema
	byID   map[string]TargetDefinition
	byName map[string]TargetDefinition
}

// DefaultTargets returns the canonical list of built-in target definitions.
func DefaultTargets() []TargetDefinition {
	return []TargetDefinition{
		{
			ID:                 "codex",
			DisplayName:        "Codex CLI / IDE",
			Vendor:             "OpenAI",
			Scope:              ScopeGlobal,
			Aliases:            []string{".codex"},
			GlobalRootPattern:  "~/.codex",
			ProjectRootPattern: ".codex",
			Subpath:            "skills",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindPrompt, resourcehub.KindRule},
			OwnershipMarkers:   []string{"config.toml", ".asm-managed"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "claude",
			DisplayName:        "Claude Desktop / Code",
			Vendor:             "Anthropic",
			Scope:              ScopeGlobal,
			Aliases:            []string{".claude", "claude-desktop"},
			GlobalRootPattern:  "~/.claude",
			ProjectRootPattern: ".claude",
			Subpath:            "commands",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindCommand, resourcehub.KindMCPServer, resourcehub.KindPrompt},
			OwnershipMarkers:   []string{"claude_desktop_config.json", ".asm-managed"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "opencode",
			DisplayName:        "OpenCode Agent",
			Vendor:             "OpenCode Systems",
			Scope:              ScopeProject,
			Aliases:            []string{".opencode"},
			GlobalRootPattern:  "~/.opencode",
			ProjectRootPattern: ".opencode",
			Subpath:            "skills",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindAgent, resourcehub.KindRule, resourcehub.KindCommand, resourcehub.KindMCPServer},
			OwnershipMarkers:   []string{"opencode.json", ".asm-managed"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "cursor",
			DisplayName:        "Cursor IDE",
			Vendor:             "Anysphere",
			Scope:              ScopeProject,
			Aliases:            []string{".cursor", ".cursorrules"},
			GlobalRootPattern:  "~/.cursor",
			ProjectRootPattern: ".cursor",
			Subpath:            "rules",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindRule, resourcehub.KindPrompt, resourcehub.KindSkill},
			OwnershipMarkers:   []string{".cursorrules", "rules"},
			RecursionAllowed:   false,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "windsurf",
			DisplayName:        "Windsurf / Cascade",
			Vendor:             "Codeium",
			Scope:              ScopeProject,
			Aliases:            []string{".windsurf", ".codeium"},
			GlobalRootPattern:  "~/.codeium/windsurf",
			ProjectRootPattern: ".windsurf",
			Subpath:            "memories",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindRule, resourcehub.KindContext, resourcehub.KindPrompt},
			OwnershipMarkers:   []string{"cascade_rules.json"},
			RecursionAllowed:   false,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "gemini-cli",
			DisplayName:        "Gemini CLI",
			Vendor:             "Google DeepMind",
			Scope:              ScopeGlobal,
			Aliases:            []string{".gemini-cli"},
			GlobalRootPattern:  "~/.gemini",
			ProjectRootPattern: ".gemini",
			Subpath:            "antigravity-cli/skills",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindPrompt, resourcehub.KindRule},
			OwnershipMarkers:   []string{"config.json"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "antigravity-cli",
			DisplayName:        "Antigravity CLI Agent",
			Vendor:             "Google DeepMind",
			Scope:              ScopeGlobal,
			Aliases:            []string{".antigravity-cli", "gemini-antigravity"},
			GlobalRootPattern:  "~/.gemini/antigravity-cli",
			ProjectRootPattern: ".gemini/antigravity-cli",
			Subpath:            "skills",
			DiscoveryOnlyPaths: []string{"~/.gemini/antigravity-cli/brain"},
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindRule, resourcehub.KindCommand, resourcehub.KindMCPServer},
			OwnershipMarkers:   []string{"config.json", ".asm-managed"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "antigravity-ide",
			DisplayName:        "Antigravity IDE",
			Vendor:             "Google DeepMind",
			Scope:              ScopeGlobal,
			Aliases:            []string{".antigravity-ide"},
			GlobalRootPattern:  "~/.gemini/antigravity-ide",
			ProjectRootPattern: ".gemini/antigravity-ide",
			Subpath:            "extensions",
			DiscoveryOnlyPaths: []string{"~/.gemini/antigravity-ide/workspaces"},
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindContext, resourcehub.KindMCPServer},
			OwnershipMarkers:   []string{"settings.json"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "kiro",
			DisplayName:        "Kiro Agent",
			Vendor:             "Kiro",
			Scope:              ScopeProject,
			Aliases:            []string{".kiro"},
			GlobalRootPattern:  "~/.kiro",
			ProjectRootPattern: ".kiro",
			Subpath:            "steering",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindRule, resourcehub.KindPrompt, resourcehub.KindSkill},
			OwnershipMarkers:   []string{"steering", "specs"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "roo-code",
			DisplayName:        "Roo Code",
			Vendor:             "Roo Code",
			Scope:              ScopeProject,
			Aliases:            []string{".roo", ".clinerules"},
			GlobalRootPattern:  "~/.roo",
			ProjectRootPattern: ".roo",
			Subpath:            "rules",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindRule, resourcehub.KindPrompt},
			OwnershipMarkers:   []string{".clinerules"},
			RecursionAllowed:   false,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "continue",
			DisplayName:        "Continue Dev",
			Vendor:             "Continue",
			Scope:              ScopeGlobal,
			Aliases:            []string{".continue"},
			GlobalRootPattern:  "~/.continue",
			ProjectRootPattern: ".continue",
			Subpath:            "prompts",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindPrompt, resourcehub.KindRule, resourcehub.KindMCPServer},
			OwnershipMarkers:   []string{"config.json"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "goose",
			DisplayName:        "Block Goose",
			Vendor:             "Block",
			Scope:              ScopeGlobal,
			Aliases:            []string{".goose"},
			GlobalRootPattern:  "~/.config/goose",
			ProjectRootPattern: ".goose",
			Subpath:            "hints",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindRule, resourcehub.KindPrompt},
			OwnershipMarkers:   []string{"config.yaml"},
			RecursionAllowed:   false,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "copilot",
			DisplayName:        "GitHub Copilot Workspace",
			Vendor:             "GitHub",
			Scope:              ScopeProject,
			Aliases:            []string{".github/copilot-instructions.md"},
			GlobalRootPattern:  "~/.copilot",
			ProjectRootPattern: ".github",
			Subpath:            "copilot-instructions.md",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindRule},
			OwnershipMarkers:   []string{"copilot-instructions.md"},
			RecursionAllowed:   false,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
		{
			ID:                 "hermes",
			DisplayName:        "Hermes Agent",
			Vendor:             "Nous Research",
			Scope:              ScopeGlobal,
			Aliases:            []string{".hermes"},
			GlobalRootPattern:  "~/.hermes",
			ProjectRootPattern: ".hermes",
			Subpath:            "skills",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindPrompt},
			OwnershipMarkers:   []string{"config.json"},
			RecursionAllowed:   true,
			ReadOnly:           true,
			Confidence:         ConfidenceMedium,
		},
		{
			ID:                 "nara",
			DisplayName:        "Nara AI",
			Vendor:             "Nara",
			Scope:              ScopeGlobal,
			Aliases:            []string{".nara"},
			GlobalRootPattern:  "~/.nara",
			ProjectRootPattern: ".nara",
			Subpath:            "rules",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindRule},
			OwnershipMarkers:   []string{"nara.json"},
			RecursionAllowed:   false,
			ReadOnly:           true,
			Confidence:         ConfidenceUnverified,
		},
		{
			ID:                 "openhuman",
			DisplayName:        "OpenHuman Agent",
			Vendor:             "OpenHuman",
			Scope:              ScopeGlobal,
			Aliases:            []string{".openhuman"},
			GlobalRootPattern:  "~/.openhuman",
			ProjectRootPattern: ".openhuman",
			Subpath:            "context",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindContext, resourcehub.KindPrompt},
			OwnershipMarkers:   []string{"openhuman.json"},
			RecursionAllowed:   false,
			ReadOnly:           true,
			Confidence:         ConfidenceUnverified,
		},
		{
			ID:                 "openclaw",
			DisplayName:        "OpenClaw Agent",
			Vendor:             "OpenClaw",
			Scope:              ScopeGlobal,
			Aliases:            []string{".openclaw"},
			GlobalRootPattern:  "~/.openclaw",
			ProjectRootPattern: ".openclaw",
			Subpath:            "skills",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill},
			OwnershipMarkers:   []string{"config.json"},
			RecursionAllowed:   false,
			ReadOnly:           true,
			Confidence:         ConfidenceUnverified,
		},
		{
			ID:                 "qwen",
			DisplayName:        "Qwen Agent",
			Vendor:             "Alibaba",
			Scope:              ScopeGlobal,
			Aliases:            []string{".qwen"},
			GlobalRootPattern:  "~/.qwen",
			ProjectRootPattern: ".qwen",
			Subpath:            "prompts",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindPrompt, resourcehub.KindRule},
			OwnershipMarkers:   []string{"qwen.json"},
			RecursionAllowed:   false,
			ReadOnly:           true,
			Confidence:         ConfidenceUnverified,
		},
		{
			ID:                 "kimi",
			DisplayName:        "Kimi Moonshot",
			Vendor:             "Moonshot AI",
			Scope:              ScopeGlobal,
			Aliases:            []string{".kimi"},
			GlobalRootPattern:  "~/.kimi",
			ProjectRootPattern: ".kimi",
			Subpath:            "context",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindContext, resourcehub.KindPrompt},
			OwnershipMarkers:   []string{"kimi.json"},
			RecursionAllowed:   false,
			ReadOnly:           true,
			Confidence:         ConfidenceUnverified,
		},
		{
			ID:                 "generic",
			DisplayName:        "Generic Agent Target",
			Vendor:             "Generic",
			Scope:              ScopeProject,
			Aliases:            []string{".agents", ".agent"},
			GlobalRootPattern:  "~/.agents",
			ProjectRootPattern: ".agents",
			Subpath:            "skills",
			SupportedKinds:     []resourcehub.Kind{resourcehub.KindSkill, resourcehub.KindRule, resourcehub.KindCommand, resourcehub.KindPrompt, resourcehub.KindContext},
			OwnershipMarkers:   []string{".asm-managed"},
			RecursionAllowed:   true,
			ReadOnly:           false,
			Confidence:         ConfidenceHigh,
		},
	}
}

// New creates a initialized Catalog populated with DefaultTargets.
func New() (*Catalog, error) {
	targets := DefaultTargets()
	schema := CatalogSchema{
		APIVersion: APIVersion,
		Targets:    targets,
	}
	sealed, err := SealCatalogSchema(schema)
	if err != nil {
		return nil, fmt.Errorf("seal default catalog: %w", err)
	}

	cat := &Catalog{
		schema: sealed,
		byID:   make(map[string]TargetDefinition),
		byName: make(map[string]TargetDefinition),
	}

	for _, t := range sealed.Targets {
		cat.byID[t.ID] = t
		cat.byName[strings.ToLower(t.ID)] = t
		for _, alias := range t.Aliases {
			cat.byName[strings.ToLower(alias)] = t
		}
	}

	return cat, nil
}

func (c *Catalog) Schema() CatalogSchema {
	return c.schema
}

func (c *Catalog) Lookup(id string) (TargetDefinition, bool) {
	t, ok := c.byID[id]
	if !ok {
		t, ok = c.byName[strings.ToLower(id)]
	}
	return t, ok
}

// DetectTarget identifies matching target definitions for an observed path.
func (c *Catalog) DetectTarget(observedPath string) (DetectedTarget, bool) {
	clean := filepath.ToSlash(filepath.Clean(observedPath))
	base := filepath.Base(clean)
	lowerBase := strings.ToLower(base)

	for _, target := range c.schema.Targets {
		// Check ID match or alias match
		if strings.EqualFold(target.ID, base) || strings.EqualFold(target.ProjectRootPattern, base) {
			isProject := target.Scope == ScopeProject || strings.HasPrefix(target.ProjectRootPattern, ".")
			managed := isManagedRoot(observedPath, target.OwnershipMarkers)
			return DetectedTarget{
				Definition: target,
				RootPath:   clean,
				FoundPath:  clean,
				IsProject:  isProject,
				Managed:    managed,
			}, true
		}

		for _, alias := range target.Aliases {
			if strings.EqualFold(alias, base) || strings.EqualFold(alias, lowerBase) {
				isProject := target.Scope == ScopeProject || strings.HasPrefix(alias, ".")
				managed := isManagedRoot(observedPath, target.OwnershipMarkers)
				return DetectedTarget{
					Definition: target,
					RootPath:   clean,
					FoundPath:  clean,
					IsProject:  isProject,
					Managed:    managed,
				}, true
			}
		}
	}

	return DetectedTarget{}, false
}

func isManagedRoot(path string, markers []string) bool {
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		checkPath := filepath.Join(path, marker)
		if _, err := os.Stat(checkPath); err == nil {
			return true
		}
	}
	return false
}
