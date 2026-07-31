package catalog

import (
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func lockedComponent(id string, kind model.InstallKind, pkg, version string) model.Component {
	return model.Component{ID: id, Tier: model.TierEssential, Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: kind, Package: pkg, Source: map[model.InstallKind]string{model.InstallNPMGlobal: "npm", model.InstallUVTool: "pypi"}[kind], Version: version, Publisher: "example"}}
}

func TestCatalogRejectsFloatingOrMismatchedPackageSpecs(t *testing.T) {
	cases := []model.Component{
		lockedComponent("npm-range", model.InstallNPMGlobal, "example@^1.2.3", "1.2.3"),
		lockedComponent("npm-tag", model.InstallNPMGlobal, "example@beta", "1.2.3"),
		lockedComponent("npm-mismatch", model.InstallNPMGlobal, "example@1.2.3", "9.9.9"),
		lockedComponent("uv-wildcard", model.InstallUVTool, "example==1.2.*", "1.2.3"),
		lockedComponent("uv-mismatch", model.InstallUVTool, "example==1.2.3", "9.9.9"),
	}
	for _, component := range cases {
		t.Run(component.ID, func(t *testing.T) {
			if err := Validate(model.Catalog{Version: 1, Components: []model.Component{component}}); err == nil {
				t.Fatalf("accepted floating or mismatched package: %+v", component.Install)
			}
		})
	}
}

func TestCatalogAcceptsExactScopedNPMAndPEP440PreRelease(t *testing.T) {
	cases := []model.Component{
		lockedComponent("npm", model.InstallNPMGlobal, "@scope/example@1.2.3", "1.2.3"),
		lockedComponent("uv", model.InstallUVTool, "example[cli]==0.0.1a4", "0.0.1a4"),
	}
	for _, component := range cases {
		if err := Validate(model.Catalog{Version: 1, Components: []model.Component{component}}); err != nil {
			t.Fatalf("rejected exact package %s: %v", component.ID, err)
		}
	}
}

func TestCatalogRejectsUnversionedRouterAcquisition(t *testing.T) {
	component := model.Component{ID: "router-floating", Tier: model.TierEssential, Platforms: []string{"windows"}, Install: model.InstallSpec{Kind: model.InstallRouter}, Router: &model.RouterServerSpec{Command: "cmd.exe", Args: []string{"/c", "npx", "-y", "example-mcp"}, Limits: model.ProcessLimits{MemoryBytes: 64 << 20, CPUPercent: 10, ActiveProcesses: 1}}}
	err := Validate(model.Catalog{Version: 1, Components: []model.Component{component}})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "exact") {
		t.Fatalf("unversioned router acquisition was accepted: %v", err)
	}
}

func TestCatalogRejectsMalformedPackageNamesAndHiddenFloatingAcquisitions(t *testing.T) {
	cases := []model.Component{
		lockedComponent("npm-empty-name", model.InstallNPMGlobal, "@1.2.3", "1.2.3"),
		lockedComponent("npm-double-at", model.InstallNPMGlobal, "example@@1.2.3", "1.2.3"),
		lockedComponent("uv-url-name", model.InstallUVTool, "https://example.test/pkg==1.2.3", "1.2.3"),
		{
			ID: "router-hidden-floating", Tier: model.TierEssential, Platforms: []string{"windows"},
			Install: model.InstallSpec{Kind: model.InstallRouter},
			Router: &model.RouterServerSpec{
				Command: "npx", Args: []string{"--package=hidden@latest", "visible@1.2.3"},
				Limits: model.ProcessLimits{MemoryBytes: 64 << 20, CPUPercent: 10, ActiveProcesses: 1},
			},
		},
		{
			ID: "router-hidden-uv-floating", Tier: model.TierEssential, Platforms: []string{"windows"},
			Install: model.InstallSpec{Kind: model.InstallRouter},
			Router: &model.RouterServerSpec{
				Command: "uvx", Args: []string{"--from=hidden==1.*", "visible==1.2.3"},
				Limits: model.ProcessLimits{MemoryBytes: 64 << 20, CPUPercent: 10, ActiveProcesses: 1},
			},
		},
	}
	for _, component := range cases {
		t.Run(component.ID, func(t *testing.T) {
			if err := Validate(model.Catalog{Version: 1, Components: []model.Component{component}}); err == nil {
				t.Fatalf("unsafe acquisition was accepted: %+v", component)
			}
		})
	}
}

func TestRouterAcquisitionValidatesExplicitPackageFlags(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		wantErr bool
	}{
		{name: "npx long separate", command: "npx", args: []string{"--package", "helper@1.2.3", "server@2.0.0"}},
		{name: "npx short inline", command: "npx", args: []string{"-p=helper@1.2.3", "server@2.0.0"}},
		{name: "npx incomplete", command: "npx", args: []string{"--package"}, wantErr: true},
		{name: "npx invalid separate", command: "npx", args: []string{"-p", "helper@latest", "server@2.0.0"}, wantErr: true},
		{name: "uvx separate", command: "uvx", args: []string{"--from", "helper==1.2.3", "server==2.0.0"}},
		{name: "uvx incomplete", command: "uvx", args: []string{"--from"}, wantErr: true},
		{name: "uvx invalid positional", command: "uvx", args: []string{"server"}, wantErr: true},
		{name: "npx no acquisition", command: "npx", args: []string{"--yes"}, wantErr: true},
		{name: "unmanaged command", command: "custom-router", args: []string{"server"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateRouterAcquisition(test.name, test.command, test.args)
			if test.wantErr && err == nil {
				t.Fatalf("expected acquisition rejection for %q", test.args)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("rejected exact acquisition %q: %v", test.args, err)
			}
		})
	}
}
