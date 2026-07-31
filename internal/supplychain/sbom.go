package supplychain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/model"
)

type LicenseInventory map[string]string

type BOM struct {
	Schema       string          `json:"$schema,omitempty"`
	BOMFormat    string          `json:"bomFormat"`
	SpecVersion  string          `json:"specVersion"`
	Serial       string          `json:"serialNumber"`
	Version      int             `json:"version"`
	Metadata     BOMMetadata     `json:"metadata"`
	Components   []BOMComponent  `json:"components"`
	Dependencies []BOMDependency `json:"dependencies,omitempty"`
}

type BOMMetadata struct {
	Component  BOMComponent `json:"component"`
	Properties []Property   `json:"properties,omitempty"`
}

type BOMComponent struct {
	Type       string     `json:"type"`
	BOMRef     string     `json:"bom-ref"`
	Name       string     `json:"name"`
	Version    string     `json:"version,omitempty"`
	Publisher  string     `json:"publisher,omitempty"`
	PURL       string     `json:"purl,omitempty"`
	Licenses   []License  `json:"licenses,omitempty"`
	Properties []Property `json:"properties,omitempty"`
}

type License struct {
	Expression string `json:"expression"`
}
type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type BOMDependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

func LoadLicenses(path string) (LicenseInventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var values LicenseInventory
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func Generate(c model.Catalog, productVersion string, licenses LicenseInventory) (BOM, error) {
	if productVersion == "" {
		productVersion = "dev"
	}
	components := append([]model.Component(nil), c.Components...)
	sort.Slice(components, func(i, j int) bool { return components[i].ID < components[j].ID })
	serial, err := catalogSerial(productVersion, components)
	if err != nil {
		return BOM{}, err
	}
	bom := BOM{
		Schema: "https://cyclonedx.org/schema/bom-1.6.schema.json", BOMFormat: "CycloneDX", SpecVersion: "1.6", Serial: serial, Version: 1,
		Metadata: BOMMetadata{Component: BOMComponent{Type: "application", BOMRef: "pkg:generic/agentstack-manager@" + productVersion, Name: "AgentStack Manager", Version: productVersion, Publisher: "AgentStack"}},
	}
	refs := map[string]string{}
	for _, component := range components {
		if component.Install.Kind == model.InstallManual || component.Install.Kind == model.InstallNone {
			continue
		}
		license := strings.TrimSpace(licenses[component.ID])
		if license == "" {
			return BOM{}, fmt.Errorf("automatic component %q has no reviewed license entry", component.ID)
		}
		version, purl := componentIdentity(component)
		ref := "component:" + component.ID
		refs[component.ID] = ref
		item := BOMComponent{Type: "application", BOMRef: ref, Name: component.Name, Version: version, Publisher: component.Install.Publisher, PURL: purl, Licenses: []License{{Expression: license}}, Properties: []Property{
			{Name: "agentstack:component-id", Value: component.ID}, {Name: "agentstack:install-kind", Value: string(component.Install.Kind)}, {Name: "agentstack:source", Value: component.Install.Source}, {Name: "agentstack:platforms", Value: strings.Join(component.Platforms, ",")},
		}}
		bom.Components = append(bom.Components, item)
	}
	for _, component := range components {
		ref, ok := refs[component.ID]
		if !ok {
			continue
		}
		dep := BOMDependency{Ref: ref}
		for _, id := range component.DependsOn {
			if child, ok := refs[id]; ok {
				dep.DependsOn = append(dep.DependsOn, child)
			}
		}
		sort.Strings(dep.DependsOn)
		bom.Dependencies = append(bom.Dependencies, dep)
	}
	return bom, nil
}

func componentIdentity(component model.Component) (string, string) {
	install := component.Install
	switch install.Kind {
	case model.InstallNPMGlobal:
		name, version := splitNPM(install.Package)
		return version, npmPURL(name, version)
	case model.InstallUVTool:
		name, version := splitPython(install.Package)
		return version, "pkg:pypi/" + name + "@" + version
	case model.InstallSkillPack:
		commit := strings.TrimPrefix(install.ManifestDigest, "git-commit:")
		return install.Version, "pkg:github/obra/superpowers@" + commit
	case model.InstallRouter:
		name, version, ecosystem := routerPackage(component)
		if ecosystem == "npm" {
			return version, npmPURL(name, version)
		}
		if ecosystem == "pypi" {
			return version, "pkg:pypi/" + name + "@" + version
		}
		return version, "pkg:generic/" + component.ID + "@" + version
	default:
		return install.Version, "pkg:generic/" + install.WingetID + "@" + install.Version + "?source=" + install.Source
	}
}

func splitNPM(value string) (string, string) {
	if strings.HasPrefix(value, "@") {
		if i := strings.LastIndex(value, "@"); i > strings.Index(value, "/") {
			return value[:i], value[i+1:]
		}
	}
	if i := strings.LastIndex(value, "@"); i > 0 {
		return value[:i], value[i+1:]
	}
	return value, "unknown"
}
func splitPython(value string) (string, string) {
	if i := strings.Index(value, "=="); i >= 0 {
		name := value[:i]
		if j := strings.Index(name, "["); j >= 0 {
			name = name[:j]
		}
		return name, value[i+2:]
	}
	return value, "unknown"
}
func routerPackage(component model.Component) (string, string, string) {
	if component.Router == nil {
		return component.ID, "unknown", "generic"
	}
	for _, arg := range component.Router.Args {
		if strings.Contains(arg, "==") {
			n, v := splitPython(arg)
			return n, v, "pypi"
		}
		if strings.Contains(arg, "@") && !strings.HasPrefix(arg, "/") {
			n, v := splitNPM(arg)
			if v != "unknown" {
				return n, v, "npm"
			}
		}
	}
	return component.ID, "unknown", "generic"
}

func catalogSerial(productVersion string, components []model.Component) (string, error) {
	payload, err := json.Marshal(struct {
		ProductVersion string            `json:"productVersion"`
		Components     []model.Component `json:"components"`
	}{ProductVersion: productVersion, Components: components})
	if err != nil {
		return "", fmt.Errorf("serialize catalog for SBOM identity: %w", err)
	}
	digest := sha256.Sum256(payload)
	id := append([]byte(nil), digest[:16]...)
	id[6] = (id[6] & 0x0f) | 0x50
	id[8] = (id[8] & 0x3f) | 0x80
	hexID := hex.EncodeToString(id)
	return fmt.Sprintf("urn:uuid:%s-%s-%s-%s-%s", hexID[0:8], hexID[8:12], hexID[12:16], hexID[16:20], hexID[20:32]), nil
}

func npmPURL(name, version string) string {
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(strings.TrimPrefix(name, "@"), "/", 2)
		if len(parts) == 2 {
			return "pkg:npm/%40" + parts[0] + "/" + parts[1] + "@" + version
		}
	}
	return "pkg:npm/" + name + "@" + version
}
