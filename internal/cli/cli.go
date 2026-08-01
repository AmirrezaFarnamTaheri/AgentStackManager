package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/catalog"
	"github.com/agentstack/agentstack/internal/mcp"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/releasepack"
	"github.com/agentstack/agentstack/internal/selfinstall"
	"github.com/agentstack/agentstack/internal/session"
	"github.com/agentstack/agentstack/internal/state"
	"github.com/agentstack/agentstack/internal/supplychain"
	"github.com/agentstack/agentstack/internal/ui"
)

type CLI struct {
	Service     *app.Service
	Out         io.Writer
	Err         io.Writer
	Starter     session.Starter
	Version     string
	Revision    string
	InstallSelf func() (selfinstall.Report, error)
}

func New(service *app.Service, version string, revision ...string) *CLI {
	value := ""
	if len(revision) > 0 {
		value = revision[0]
	}
	return &CLI{Service: service, Out: os.Stdout, Err: os.Stderr, Starter: session.ExecStarter{}, Version: version, Revision: value, InstallSelf: selfinstall.InstallSelf}
}

func (c *CLI) Run(ctx context.Context, args []string) int {
	if c.Service == nil {
		fmt.Fprintln(c.errWriter(), "AgentStack service is unavailable")
		return 1
	}
	if len(args) == 0 {
		return c.runUI(ctx, nil)
	}
	switch args[0] {
	case "help", "--help", "-h":
		c.printHelp()
		return 0
	case "version", "--version":
		if c.Revision == "" {
			fmt.Fprintln(c.outWriter(), c.Version)
		} else {
			fmt.Fprintf(c.outWriter(), "%s (%s)\n", c.Version, c.Revision)
		}
		return 0
	case "ui":
		return c.runUI(ctx, args[1:])
	case "setup":
		return c.runSetup(ctx, args[1:])
	case "catalog":
		return c.printJSON(c.Service.CatalogSnapshot())
	case "profiles":
		return c.printJSON(map[string]any{"profiles": c.Service.CatalogSnapshot().Profiles})
	case "integrations":
		result, err := c.Service.GuidedIntegrations(ctx)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"integrations": result, "secretStorage": "AgentStack never stores provider credentials"})
	case "status":
		return c.runStatus(ctx)
	case "inventory":
		result, err := c.Service.Inventory(ctx)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "plan":
		request, _, err := parseSelection(args[1:])
		if err != nil {
			return c.failUsage(err)
		}
		result, err := c.Service.Plan(ctx, request)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "apply":
		fs := flag.NewFlagSet("apply", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		planID := fs.String("plan-id", "", "reviewed plan ID")
		digest := fs.String("digest", "", "reviewed plan digest")
		yes := fs.Bool("yes", false, "confirm applying the exact reviewed plan")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *planID == "" || *digest == "" {
			return c.failUsage(fmt.Errorf("apply requires --plan-id ID --digest SHA256 --yes after reviewing the emitted plan"))
		}
		if !*yes {
			return c.failUsage(fmt.Errorf("apply requires --yes after reviewing the exact plan ID and digest"))
		}
		result, err := c.Service.ApplyPlanned(ctx, *planID, *digest, true)
		if err != nil {
			if result.Plan.ID != "" || result.Transaction.ID != "" || result.Router != nil {
				_ = c.printJSON(map[string]any{"ok": false, "error": err.Error(), "report": result})
			}
			return c.fail(err)
		}
		return c.printJSON(result)
	case "mcp":
		return c.runMCP(ctx, args[1:])
	case "mcp-router":
		return c.runRouter(ctx, args[1:])
	case "codex", "agy":
		return c.runSession(ctx, args[0], args[1:])
	case "install-self":
		report, err := c.selfInstaller()()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(report)
	case "backup":
		return c.runBackup(ctx, args[1:])
	case "data":
		return c.runData(args[1:])
	case "diagnostics":
		return c.runDiagnostics(ctx, args[1:])
	case "owned":
		return c.runOwned(ctx, args[1:])
	case "cleanup":
		return c.cleanupPreview(ctx, args[1:])
	case "sbom":
		return c.runSBOM(args[1:])
	case "releasepack", "release-pack":
		return c.runReleasepack(args[1:])
	default:
		return c.failUsage(fmt.Errorf("unknown command %q", args[0]))
	}
}

func (c *CLI) runSBOM(args []string) int {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	version := fs.String("version", c.Version, "AgentStack version")
	licensesPath := fs.String("licenses", "supply-chain/component-licenses.json", "reviewed license inventory")
	out := fs.String("out", "agentstack-catalog.cdx.json", "CycloneDX output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected sbom positional argument: %s", fs.Arg(0)))
	}
	cat, err := catalog.LoadDefault()
	if err != nil {
		return c.fail(err)
	}
	licenses, err := supplychain.LoadLicenses(*licensesPath)
	if err != nil && !filepath.IsAbs(*licensesPath) {
		if exe, e := os.Executable(); e == nil {
			alt := filepath.Join(filepath.Dir(exe), *licensesPath)
			if l, e2 := supplychain.LoadLicenses(alt); e2 == nil {
				licenses = l
				err = nil
			}
		}
	}
	if err != nil {
		return c.fail(err)
	}
	bom, err := supplychain.Generate(cat, *version, licenses)
	if err != nil {
		return c.fail(err)
	}
	data, err := json.MarshalIndent(bom, "", "  ")
	if err != nil {
		return c.fail(err)
	}
	data = append(data, '\n')
	if info, lerr := os.Lstat(*out); lerr == nil && info.Mode()&os.ModeSymlink != 0 {
		return c.fail(fmt.Errorf("refusing to write SBOM to symlink target: %s", *out))
	}
	if err := os.WriteFile(*out, data, 0o600); err != nil {
		return c.fail(err)
	}
	return 0
}

func (c *CLI) runReleasepack(args []string) int {
	fs := flag.NewFlagSet("releasepack", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	root := fs.String("root", "", "root directory")
	out := fs.String("out", "", "output ZIP")
	prefix := fs.String("prefix", "", "archive prefix")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected releasepack positional argument: %s", fs.Arg(0)))
	}
	if *root == "" || *out == "" {
		return c.failUsage(fmt.Errorf("--root and --out are required"))
	}
	if err := releasepack.Pack(*root, *out, *prefix); err != nil {
		return c.fail(err)
	}
	return 0
}

func (c *CLI) runSetup(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	noLaunch := fs.Bool("no-launch", false, "install without opening the manager")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected setup arguments: %s", strings.Join(fs.Args(), " ")))
	}
	report, err := c.selfInstaller()()
	if err != nil {
		return c.fail(err)
	}
	if report.Destination != "" {
		c.Service.Paths.Executable = report.Destination
	}
	if *noLaunch {
		return c.printJSON(report)
	}
	return c.runUI(ctx, nil)
}

func (c *CLI) selfInstaller() func() (selfinstall.Report, error) {
	if c.InstallSelf != nil {
		return c.InstallSelf
	}
	return selfinstall.InstallSelf
}

func (c *CLI) runUI(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	noOpen := fs.Bool("no-open", false, "do not open the browser automatically")
	listen := fs.String("listen", "127.0.0.1:0", "loopback listen address")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	err := ui.Run(ctx, ui.HandlerOptions{Backend: c.Service, Version: c.Version, Revision: c.Revision}, ui.RunOptions{ListenAddress: *listen, OpenBrowser: !*noOpen})
	if errors.Is(err, context.Canceled) {
		return 0
	}
	if err != nil {
		return c.fail(err)
	}
	return 0
}

func (c *CLI) runStatus(ctx context.Context) int {
	current, err := c.Service.Inventory(ctx)
	if err != nil {
		return c.fail(err)
	}
	installed := 0
	for _, item := range current.Items {
		if item.Installed {
			installed++
		}
	}
	status := map[string]any{
		"version":            c.Version,
		"paths":              c.Service.Paths,
		"catalogComponents":  len(c.Service.Catalog.Components),
		"detectedComponents": installed,
		"externalSources":    sortedExternalKeys(current.External),
	}
	if _, err := os.Stat(c.Service.Paths.RouterConfig); err == nil {
		doctor, doctorErr := c.Service.MCPDoctor(ctx)
		status["mcpDoctor"] = doctor
		if doctorErr != nil {
			status["mcpDoctorError"] = doctorErr.Error()
		}
	} else {
		status["mcpDoctor"] = map[string]any{"healthy": false, "message": "router is not initialized"}
	}
	return c.printJSON(status)
}

func (c *CLI) runMCP(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("mcp requires init, doctor, or list"))
	}
	switch args[0] {
	case "init":
		remaining, noWarm := removeBoolFlag(args[1:], "--no-warm")
		remaining, noRegister := removeBoolFlag(remaining, "--no-register")
		remaining, yes := removeBoolFlag(remaining, "--yes")
		if !yes {
			return c.failUsage(fmt.Errorf("mcp init requires --yes because it may write managed configuration and client registrations"))
		}
		request, rest, err := parseSelection(remaining)
		if err != nil {
			return c.failUsage(err)
		}
		if len(rest) != 0 {
			return c.failUsage(fmt.Errorf("unexpected mcp init arguments: %s", strings.Join(rest, " ")))
		}
		report, err := c.Service.MCPInit(ctx, app.MCPInitOptions{Request: request, Warm: !noWarm, RegisterClients: !noRegister, Confirm: true})
		if err != nil {
			_ = c.printJSON(report)
			return c.fail(err)
		}
		return c.printJSON(report)
	case "doctor":
		report, err := c.Service.MCPDoctor(ctx)
		if err != nil {
			_ = c.printJSON(report)
			return c.fail(err)
		}
		if !report.Healthy {
			_ = c.printJSON(report)
			return 1
		}
		return c.printJSON(report)
	case "list":
		config, err := mcp.LoadRouterConfig(c.Service.Paths.RouterConfig)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(config)
	default:
		return c.failUsage(fmt.Errorf("unknown mcp command %q", args[0]))
	}
}

func (c *CLI) runRouter(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("mcp-router", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	configPath := fs.String("config", c.Service.Paths.RouterConfig, "router configuration path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	config, err := mcp.LoadRouterConfig(*configPath)
	if err != nil {
		return c.fail(err)
	}
	children := &mcp.PooledChildClient{Base: mcp.StdIOChildClient{Observer: c.Service.MCPChildObserver()}}
	router := mcp.Router{Config: config, Children: children}
	if err := router.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		return c.fail(err)
	}
	return 0
}

func (c *CLI) runSession(ctx context.Context, target string, args []string) int {
	request, forwarded, err := parseSelection(args)
	if err != nil {
		return c.failUsage(err)
	}
	initializer := func(initCtx context.Context) error {
		_, initErr := c.Service.MCPInit(initCtx, app.MCPInitOptions{Request: request, Warm: true, RegisterClients: true, Confirm: true})
		return initErr
	}
	if err := session.Launch(ctx, target, forwarded, initializer, c.Starter); err != nil {
		return c.fail(err)
	}
	return 0
}

func (c *CLI) runBackup(ctx context.Context, args []string) int {
	if len(args) == 0 || args[0] == "list" {
		backups, err := c.Service.Backups()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"backups": backups})
	}
	if args[0] != "restore" {
		return c.failUsage(fmt.Errorf("backup requires list or restore"))
	}
	fs := flag.NewFlagSet("backup restore", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	id := fs.String("id", "", "backup ID")
	target := fs.String("target", "", "exact original target path; optional")
	preview := fs.Bool("preview", false, "verify the backup and show the exact restore target without writing")
	yes := fs.Bool("yes", false, "confirm validated restore")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *id == "" || fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("backup restore requires --id ID and either --preview or --yes"))
	}
	if *preview {
		if *yes {
			return c.failUsage(fmt.Errorf("choose either --preview or --yes"))
		}
		report, err := c.Service.PreviewRestore(*id, *target)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(report)
	}
	if !*yes {
		return c.failUsage(fmt.Errorf("backup restore requires --yes after reviewing --preview"))
	}
	record, err := c.Service.RestoreBackup(ctx, *id, *target, true)
	if err != nil {
		return c.fail(err)
	}
	return c.printJSON(record)
}

func (c *CLI) runData(args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("data requires policy, export, or clear"))
	}
	switch args[0] {
	case "policy":
		if len(args) != 1 {
			return c.failUsage(fmt.Errorf("data policy does not accept arguments"))
		}
		policy := state.DefaultRetentionPolicy()
		return c.printJSON(map[string]any{
			"plans":        policy.Plans.String(),
			"transactions": policy.Transactions.String(),
			"diagnostics":  policy.Diagnostics.String(),
			"events":       policy.Events.String(),
			"backups":      "retained until explicit user deletion",
			"ownership":    "retained until explicit user deletion",
			"memory":       "retained until explicit user deletion",
		})
	case "export":
		fs := flag.NewFlagSet("data export", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		out := fs.String("out", "agentstack-data-export.zip", "destination ZIP")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected data export arguments"))
		}
		if err := c.Service.ExportData(*out); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"export": *out})
	case "clear":
		fs := flag.NewFlagSet("data clear", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		scope := fs.String("scope", string(state.ClearOperational), "operational|memory|all")
		yes := fs.Bool("yes", false, "confirm deletion of the selected AgentStack data scope")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if !*yes || fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("data clear requires --scope operational|memory|all --yes"))
		}
		removed, err := c.Service.ClearData(state.ClearScope(*scope), true)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"scope": *scope, "removed": removed})
	default:
		return c.failUsage(fmt.Errorf("unknown data command %q", args[0]))
	}
}

func (c *CLI) runDiagnostics(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("diagnostics", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	out := fs.String("out", fmt.Sprintf("agentstack-diagnostics-%s.zip", time.Now().UTC().Format("20060102T150405Z")), "sanitized diagnostic bundle")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected diagnostics arguments"))
	}
	if err := c.Service.CreateDiagnostics(ctx, *out, c.Version); err != nil {
		return c.fail(err)
	}
	return c.printJSON(map[string]any{"diagnostics": *out})
}

func (c *CLI) runOwned(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("owned requires preview, deactivate, or remove"))
	}
	operation := args[0]
	fs := flag.NewFlagSet("owned "+operation, flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	var components stringValues
	fs.Var(&components, "component", "AgentStack-owned component ID; repeat or comma-separate")
	yes := fs.Bool("yes", false, "confirm the exact ownership-scoped operation")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected owned arguments"))
	}
	ids := components.Values()
	switch operation {
	case "preview":
		report, err := c.Service.OwnedPreview(ids, "remove")
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(report)
	case "deactivate":
		if !*yes {
			return c.failUsage(fmt.Errorf("owned deactivate requires --yes"))
		}
		report, err := c.Service.DeactivateOwned(ctx, ids, true)
		if err != nil {
			_ = c.printJSON(report)
			return c.fail(err)
		}
		return c.printJSON(report)
	case "remove":
		if !*yes {
			return c.failUsage(fmt.Errorf("owned remove requires --yes"))
		}
		report, err := c.Service.RemoveOwned(ctx, ids, true)
		if err != nil {
			_ = c.printJSON(report)
			return c.fail(err)
		}
		return c.printJSON(report)
	default:
		return c.failUsage(fmt.Errorf("unknown owned command %q", operation))
	}
}

func (c *CLI) cleanupPreview(ctx context.Context, args []string) int {
	if len(args) != 1 || args[0] != "--preview" {
		return c.failUsage(fmt.Errorf("cleanup supports only --preview; AgentStack never uninstalls automatically"))
	}
	request := planner.Request{Profile: "full-local"}
	plan, err := c.Service.Plan(ctx, request)
	if err != nil {
		return c.fail(err)
	}
	inactive := make([]any, 0)
	for _, action := range plan.Actions {
		if action.Kind == "preserve-inactive" || action.Kind == "skip-dominated" {
			inactive = append(inactive, action)
		}
	}
	return c.printJSON(map[string]any{
		"mode":    "preview-only",
		"message": "No software will be removed. These providers are inactive or dominated in the managed profile.",
		"items":   inactive,
	})
}

func parseSelection(args []string) (planner.Request, []string, error) {
	fs := flag.NewFlagSet("selection", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	profile := fs.String("profile", "core", "profile")
	allowCredentials := fs.Bool("allow-credentials", false, "allow selected credential integrations")
	allowUpgrades := fs.Bool("allow-upgrades", false, "allow explicit upgrades required by the selected profile")
	var includes, excludes, providers stringValues
	fs.Var(&includes, "include", "comma-separated component IDs")
	fs.Var(&excludes, "exclude", "comma-separated component IDs")
	fs.Var(&providers, "provider", "capability=component-id")
	if err := fs.Parse(args); err != nil {
		return planner.Request{}, nil, err
	}
	overrides := map[string]string{}
	for _, raw := range providers.Values() {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return planner.Request{}, nil, fmt.Errorf("invalid --provider %q; expected capability=component-id", raw)
		}
		overrides[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return planner.Request{
		Profile:           *profile,
		Include:           includes.Values(),
		Exclude:           excludes.Values(),
		AllowCredentialed: *allowCredentials,
		AllowUpgrades:     *allowUpgrades,
		ProviderOverrides: overrides,
	}, fs.Args(), nil
}

type stringValues []string

func (s *stringValues) String() string { return strings.Join(*s, ",") }
func (s *stringValues) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}
func (s stringValues) Values() []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(s))
	for _, value := range s {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func removeBoolFlag(args []string, name string) ([]string, bool) {
	result := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == name {
			found = true
			continue
		}
		result = append(result, arg)
	}
	return result, found
}

func sortedExternalKeys(values map[string][]model.ExternalPackage) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c *CLI) printJSON(value any) int {
	encoder := json.NewEncoder(c.outWriter())
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return c.fail(err)
	}
	return 0
}
func (c *CLI) fail(err error) int { fmt.Fprintln(c.errWriter(), "error:", err); return 1 }
func (c *CLI) failUsage(err error) int {
	fmt.Fprintln(c.errWriter(), "error:", err)
	fmt.Fprintln(c.errWriter(), "Run 'agentstack help' for usage.")
	return 2
}
func (c *CLI) outWriter() io.Writer {
	if c.Out != nil {
		return c.Out
	}
	return os.Stdout
}
func (c *CLI) errWriter() io.Writer {
	if c.Err != nil {
		return c.Err
	}
	return os.Stderr
}

func (c *CLI) printHelp() {
	profiles := c.Service.CatalogSnapshot().Profiles
	profileLines := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		profileLines = append(profileLines, fmt.Sprintf("    %-18s %s", profile.ID, profile.Description))
	}
	fmt.Fprintf(c.outWriter(), `AgentStack Manager

Usage:
  agentstack [ui]
  agentstack setup [--no-launch]
  agentstack status | inventory | catalog | profiles | integrations
  agentstack sbom [--version VERSION] [--licenses PATH] [--out FILE]
  agentstack releasepack --root DIR --out ZIP [--prefix NAME]
  agentstack plan [selection options]
  agentstack apply --plan-id ID --digest SHA256 --yes
  agentstack mcp init [selection options] --yes [--no-warm] [--no-register]
  agentstack mcp doctor | mcp list
  agentstack mcp-router [--config PATH]
  agentstack codex [selection options] -- [codex arguments]
  agentstack agy [selection options] -- [agy arguments]
  agentstack install-self
  agentstack backup [list]
  agentstack backup restore --id ID [--target PATH] --yes
  agentstack data policy
  agentstack data export [--out FILE]
  agentstack data clear --scope operational|memory|all --yes
  agentstack diagnostics [--out FILE]
  agentstack owned preview [--component ID]
  agentstack owned deactivate [--component ID] --yes
  agentstack owned remove [--component ID] --yes
  agentstack cleanup --preview

Selection options:
  --profile ID
  --include id,id
  --exclude id,id
  --provider capability=component-id
  --allow-credentials
  --allow-upgrades

Profiles:
%s

Safety:
  Apply consumes only an unexpired plan ID and digest emitted by 'agentstack plan'.
  Existing packages, skills, MCP entries, and unrelated configuration are not
  automatically removed. Removal commands operate only on recorded AgentStack-owned
  resources, require --yes, and preserve skill content in quarantine. Credential
  integrations and incompatible-runtime upgrades require explicit consent.
`, strings.Join(profileLines, "\n"))
}
