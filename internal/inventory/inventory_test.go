package inventory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/model"
)

type fakeLocator map[string]string

func (f fakeLocator) LookPath(name string) (string, error) {
	if path, ok := f[name]; ok {
		return path, nil
	}
	return "", errors.New("not found")
}

type fakeProbe struct {
	outputs map[string]CommandResult
}

func (f fakeProbe) Run(_ context.Context, command string, args ...string) CommandResult {
	key := command
	for _, arg := range args {
		key += " " + arg
	}
	return f.outputs[key]
}

func TestScannerDetectsCatalogCommandsAndMinimizesRawInventoryByDefault(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "git", DetectCommands: []string{"git"}},
		{ID: "fd", DetectCommands: []string{"fd"}},
	}}
	scanner := Scanner{
		Locator: fakeLocator{"git": `C:\Tools\git.exe`, "npm": `C:\Node\npm.cmd`},
		Probe: fakeProbe{outputs: map[string]CommandResult{
			"npm list --global --depth=0 --json": {Stdout: `{"dependencies":{"mystery-tool":{"version":"2.1.0"}}}`, ExitCode: 0},
		}},
	}

	result := scanner.Scan(context.Background(), c, nil)
	if !result.Items["git"].Installed {
		t.Fatal("git should be detected")
	}
	if result.Items["fd"].Installed {
		t.Fatal("fd should be missing")
	}
	if len(result.External["npm"]) != 1 || result.External["npm"][0].Name != "mystery-tool" {
		t.Fatalf("unknown npm package should be inventoried, got %#v", result.External["npm"])
	}
	if result.RawSources != nil {
		t.Fatalf("raw inventory should be omitted by default, got %#v", result.RawSources)
	}
	if result.Revision == "" {
		t.Fatal("inventory should have a deterministic revision")
	}
}

func TestScannerPreservesRawInventoryOnlyWhenRequested(t *testing.T) {
	c := model.Catalog{Version: 1}
	scanner := Scanner{
		IncludeRaw: true,
		Locator:    fakeLocator{"npm": `C:\Node\npm.cmd`},
		Probe: fakeProbe{outputs: map[string]CommandResult{
			"npm list --global --depth=0 --json": {Stdout: `{"dependencies":{"mystery-tool":{"version":"2.1.0"}}}`, ExitCode: 0},
		}},
	}
	result := scanner.Scan(context.Background(), c, nil)
	if result.RawSources["npm"] == "" {
		t.Fatal("raw npm inventory should be retained only in diagnostic mode")
	}
	if Minimized(result).RawSources != nil {
		t.Fatal("minimization should remove all raw command output")
	}
}

func TestScannerMarksManagedRouterAsReconciliationRequired(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "memory-mcp", Install: model.InstallSpec{Kind: model.InstallRouter}},
	}}
	scanner := Scanner{Locator: fakeLocator{}, Probe: fakeProbe{outputs: map[string]CommandResult{}}}
	result := scanner.Scan(context.Background(), c, map[string]bool{"memory-mcp": true})
	item := result.Items["memory-mcp"]
	if item.Installed || !item.Managed || !item.Broken {
		t.Fatalf("managed router should require reconciliation instead of overstating presence: %#v", item)
	}
}

func TestScannerAdoptsInstalledNPMAndUVPackagesWithoutCommands(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "markdownlint", Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "markdownlint-cli2@0.23.2"}},
		{ID: "graphify", Install: model.InstallSpec{Kind: model.InstallUVTool, Package: "graphifyy[mcp]==0.9.25"}},
	}}
	scanner := Scanner{
		Locator: fakeLocator{"npm": `C:\Node\npm.cmd`, "uv": `C:\Tools\uv.exe`},
		Probe: fakeProbe{outputs: map[string]CommandResult{
			"npm list --global --depth=0 --json": {Stdout: `{"dependencies":{"markdownlint-cli2":{"version":"0.18.1"}}}`, ExitCode: 0},
			"uv tool list":                       {Stdout: "graphifyy v0.9.25\n- graphify\n", ExitCode: 0},
		}},
	}
	result := scanner.Scan(context.Background(), c, nil)
	if !result.Items["markdownlint"].Installed || result.Items["markdownlint"].Version != "0.18.1" {
		t.Fatalf("npm package should be adopted, got %#v", result.Items["markdownlint"])
	}
	if !result.Items["graphify"].Installed || result.Items["graphify"].Version != "0.9.25" {
		t.Fatalf("uv package should be adopted, got %#v", result.Items["graphify"])
	}
}

func TestScannerParsesAndReconcilesWingetExport(t *testing.T) {
	temp := t.TempDir()
	exportPath := filepath.Join(temp, "packages.json")
	payload := `{"Sources":[{"SourceDetails":{"Name":"winget","Identifier":"Microsoft.Winget.Source_8wekyb3d8bbwe"},"Packages":[{"PackageIdentifier":"Git.Git","Version":"2.50.1"},{"PackageIdentifier":"Vendor.Unknown","Version":"1.2.3"}]}]}`
	probe := fileWritingProbe{path: exportPath, payload: payload}
	c := model.Catalog{Version: 1, Components: []model.Component{{ID: "git", Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git", Publisher: "Git Development Community"}}}}
	scanner := Scanner{Locator: fakeLocator{"winget": `C:\Windows\winget.exe`}, Probe: probe}
	// scanWinget creates its own path, so exercise its fallback parser with JSON stdout.
	probe.payloadToStdout = true
	scanner.Probe = probe
	result := scanner.Scan(context.Background(), c, nil)
	item := result.Items["git"]
	if !item.Installed || item.Version != "2.50.1" || item.PackageSource != "winget" {
		t.Fatalf("winget package should be reconciled, got %#v", item)
	}
	if len(result.External["winget"]) != 2 {
		t.Fatalf("all winget packages should be inventoried, got %#v", result.External["winget"])
	}
}

type fileWritingProbe struct {
	path            string
	payload         string
	payloadToStdout bool
}

func (p fileWritingProbe) Run(_ context.Context, command string, args ...string) CommandResult {
	if command != "winget" {
		return CommandResult{}
	}
	if p.payloadToStdout {
		return CommandResult{Stdout: p.payload, ExitCode: 0}
	}
	_ = os.WriteFile(p.path, []byte(p.payload), 0o600)
	return CommandResult{ExitCode: 0}
}

func TestScannerVersionPolicyRejectsUnsupportedRuntime(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{
		ID: "node", DetectCommands: []string{"node"},
		VersionPolicy: &model.VersionPolicy{Minimum: "20.0.0", Probe: model.CommandSpec{Command: "node", Args: []string{"--version"}}},
	}}}
	scanner := Scanner{
		Locator: fakeLocator{"node": `C:\Node\node.exe`},
		Probe:   fakeProbe{outputs: map[string]CommandResult{"node --version": {Stdout: "v18.19.0", ExitCode: 0}}},
	}
	item := scanner.Scan(context.Background(), c, nil).Items["node"]
	if !item.Installed || !item.Incompatible || item.Compatible {
		t.Fatalf("unsupported runtime should be adopted but marked incompatible: %#v", item)
	}
}

func TestScannerMarksMissingManagedExecutableAsBroken(t *testing.T) {
	c := model.Catalog{Version: 1, Components: []model.Component{{
		ID:             "gitleaks",
		DetectCommands: []string{"gitleaks"},
		Install:        model.InstallSpec{Kind: model.InstallWinget, WingetID: "Gitleaks.Gitleaks"},
	}}}
	scanner := Scanner{Locator: fakeLocator{}, Probe: fakeProbe{outputs: map[string]CommandResult{}}}
	result := scanner.Scan(context.Background(), c, map[string]bool{"gitleaks": true})
	item := result.Items["gitleaks"]
	if item.Installed || !item.Managed || !item.Broken {
		t.Fatalf("missing managed executable should be repairable, got %#v", item)
	}
}

func TestMinimizedInventoryDropsExecutablePathsAndRawOutput(t *testing.T) {
	input := model.Inventory{RawSources: map[string]string{"winget": "sensitive"}, Items: map[string]model.InventoryItem{
		"git": {ComponentID: "git", Installed: true, ExecutablePath: `C:\\Users\\Example\\bin\\git.exe`, Version: "1.2.3"},
	}}
	minimized := Minimized(input)
	if minimized.RawSources != nil || minimized.Items["git"].ExecutablePath != "" {
		t.Fatalf("inventory was not minimized: %+v", minimized)
	}
	if !minimized.Items["git"].Installed || minimized.Items["git"].Version != "1.2.3" {
		t.Fatal("minimization removed planning facts")
	}
}

func TestInventoryProbeHelperProcess(t *testing.T) {
	mode := os.Getenv("AGENTSTACK_INVENTORY_HELPER")
	if mode == "" {
		return
	}
	if mode == "sleep" {
		time.Sleep(time.Minute)
		return
	}
	_, _ = os.Stdout.WriteString(strings.Repeat("p", 2048))
}

func TestOSProbeBoundsOutputAndTimeout(t *testing.T) {
	// Race instrumentation can make even a no-op helper process take more than a
	// second to start and exit on a busy CI host. Keep the success-path timeout
	// deliberately generous; the explicit sleep case below is the contract test
	// for cancellation behavior.
	probe := OSProbe{Timeout: 10 * time.Second, MaxOutputBytes: 64}
	result := probe.Run(context.Background(), os.Args[0], "-test.run=^TestInventoryProbeHelperProcess$")
	// No helper environment means a clean test process with bounded ordinary output.
	if result.ExitCode != 0 || result.Err != nil {
		t.Fatalf("unexpected baseline probe: %+v", result)
	}
	result = runProbeWithHelper(t, "output", 10*time.Second, 64)
	if !result.Truncated || len(result.Stdout) != 64 {
		t.Fatalf("probe output was not bounded: %+v len=%d", result, len(result.Stdout))
	}
	result = runProbeWithHelper(t, "sleep", 100*time.Millisecond, 64)
	if result.Err == nil || result.ExitCode != -1 {
		t.Fatalf("probe timeout was not enforced: %+v", result)
	}
}

func runProbeWithHelper(t *testing.T, mode string, timeout time.Duration, limit int) CommandResult {
	t.Helper()
	old, had := os.LookupEnv("AGENTSTACK_INVENTORY_HELPER")
	if err := os.Setenv("AGENTSTACK_INVENTORY_HELPER", mode); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if had {
			_ = os.Setenv("AGENTSTACK_INVENTORY_HELPER", old)
		} else {
			_ = os.Unsetenv("AGENTSTACK_INVENTORY_HELPER")
		}
	}()
	return (OSProbe{Timeout: timeout, MaxOutputBytes: limit}).Run(context.Background(), os.Args[0], "-test.run=^TestInventoryProbeHelperProcess$")
}
