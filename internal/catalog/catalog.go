package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/agentstack/agentstack/internal/model"
)

//go:embed default.json
var defaultCatalogFS embed.FS

func LoadDefault() (model.Catalog, error) {
	data, err := defaultCatalogFS.ReadFile("default.json")
	if err != nil {
		return model.Catalog{}, fmt.Errorf("read embedded catalog: %w", err)
	}
	return decode(data)
}

func LoadFile(path string) (model.Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Catalog{}, fmt.Errorf("read catalog %q: %w", path, err)
	}
	return decode(data)
}

func decode(data []byte) (model.Catalog, error) {
	var c model.Catalog
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return model.Catalog{}, fmt.Errorf("decode catalog: %w", err)
	}
	if err := Validate(c); err != nil {
		return model.Catalog{}, err
	}
	return c, nil
}

func Validate(c model.Catalog) error {
	if c.Version < 1 {
		return fmt.Errorf("catalog version must be positive")
	}
	componentIDs := make(map[string]struct{}, len(c.Components))
	preferredByCapability := make(map[string]string)
	for index, component := range c.Components {
		if strings.TrimSpace(component.ID) == "" {
			return fmt.Errorf("component at index %d has empty id", index)
		}
		if _, exists := componentIDs[component.ID]; exists {
			return fmt.Errorf("duplicate component id %q", component.ID)
		}
		componentIDs[component.ID] = struct{}{}
		if component.Preferred && component.Capability != "" {
			if prior, exists := preferredByCapability[component.Capability]; exists {
				return fmt.Errorf("capability %q has multiple preferred providers: %s and %s", component.Capability, prior, component.ID)
			}
			preferredByCapability[component.Capability] = component.ID
		}
		if component.Install.Kind == model.InstallRouter && component.Router == nil {
			return fmt.Errorf("router component %q has no router definition", component.ID)
		}
		if component.Tier != "" {
			if len(component.Platforms) == 0 {
				return fmt.Errorf("automatic catalog component %q has no supported platform declaration", component.ID)
			}
			if err := validateSupplyChainLock(component); err != nil {
				return err
			}
		}
	}
	for _, component := range c.Components {
		seenDependencies := map[string]bool{}
		for _, dependency := range component.DependsOn {
			if dependency == component.ID {
				return fmt.Errorf("component %q depends on itself", component.ID)
			}
			if seenDependencies[dependency] {
				return fmt.Errorf("component %q repeats dependency %q", component.ID, dependency)
			}
			seenDependencies[dependency] = true
			if _, exists := componentIDs[dependency]; !exists {
				return fmt.Errorf("component %q has unknown dependency %q", component.ID, dependency)
			}
		}
	}
	if err := validateDependencyCycles(c); err != nil {
		return err
	}
	profileIDs := map[string]struct{}{}
	for _, profile := range c.Profiles {
		if profile.ID == "" {
			return fmt.Errorf("profile has empty id")
		}
		if _, exists := profileIDs[profile.ID]; exists {
			return fmt.Errorf("duplicate profile id %q", profile.ID)
		}
		profileIDs[profile.ID] = struct{}{}
		for _, id := range profile.Components {
			if _, exists := componentIDs[id]; !exists {
				return fmt.Errorf("profile %q references unknown component %q", profile.ID, id)
			}
		}
	}
	return nil
}

func validateSupplyChainLock(component model.Component) error {
	install := component.Install
	switch install.Kind {
	case model.InstallWinget:
		if install.WingetID == "" || install.Source == "" || install.Version == "" || install.Publisher == "" {
			return fmt.Errorf("winget component %q must lock id, source, version, and publisher", component.ID)
		}
	case model.InstallNPMGlobal:
		if install.Package == "" || install.Source != "npm" || install.Version == "" || install.Publisher == "" || !packageHasExactNPMVersion(install.Package) {
			return fmt.Errorf("npm component %q must use an exact package version and source metadata", component.ID)
		}
	case model.InstallUVTool:
		if install.Package == "" || install.Source != "pypi" || install.Version == "" || install.Publisher == "" || !strings.Contains(install.Package, "==") {
			return fmt.Errorf("uv component %q must use an exact == version and source metadata", component.ID)
		}
	case model.InstallSkillPack:
		if install.Repository == "" || install.RepositoryRevision == "" || !strings.HasPrefix(install.ManifestDigest, "git-commit:") || len(strings.TrimPrefix(install.ManifestDigest, "git-commit:")) != 40 || len(install.ExpectedEntries) == 0 {
			return fmt.Errorf("skill pack %q must pin an immutable revision, full git commit digest, and expected skill inventory", component.ID)
		}
	case model.InstallRouter:
		if component.Router == nil {
			return fmt.Errorf("router component %q has no router definition", component.ID)
		}
		for _, value := range append(append([]string(nil), component.Router.Args...), routerWarmArgs(component.Router)...) {
			lower := strings.ToLower(value)
			if strings.Contains(lower, "@latest") {
				return fmt.Errorf("router component %q contains floating package %q", component.ID, value)
			}
		}
	case model.InstallManual:
		if component.CredentialRequired && !component.GuidedSetup {
			return fmt.Errorf("credential component %q must be labeled as guided setup", component.ID)
		}
	case model.InstallNone:
		return nil
	default:
		return fmt.Errorf("component %q has unsupported install kind %q", component.ID, install.Kind)
	}
	if component.CredentialRequired {
		if strings.TrimSpace(install.LoginHint) == "" || !strings.HasPrefix(install.DocumentationURL, "https://") {
			return fmt.Errorf("credential component %q must include an explicit login hint and HTTPS documentation URL", component.ID)
		}
	}
	return nil
}

func packageHasExactNPMVersion(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "@") {
		separator := strings.LastIndex(value, "@")
		return separator > strings.Index(value, "/") && separator < len(value)-1
	}
	separator := strings.LastIndex(value, "@")
	return separator > 0 && separator < len(value)-1
}

func routerWarmArgs(router *model.RouterServerSpec) []string {
	if router == nil || router.Warm == nil {
		return nil
	}
	return router.Warm.Args
}

func validateDependencyCycles(c model.Catalog) error {
	components := make(map[string]model.Component, len(c.Components))
	for _, component := range c.Components {
		components[component.ID] = component
	}
	const (
		unseen = iota
		visiting
		visited
	)
	state := map[string]int{}
	stack := []string{}
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case visiting:
			cycle := append(append([]string(nil), stack...), id)
			return fmt.Errorf("dependency cycle: %s", strings.Join(cycle, " -> "))
		case visited:
			return nil
		}
		state[id] = visiting
		stack = append(stack, id)
		for _, dependency := range components[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = visited
		return nil
	}
	for _, component := range c.Components {
		if state[component.ID] == unseen {
			if err := visit(component.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
