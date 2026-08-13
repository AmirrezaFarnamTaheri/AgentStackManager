package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/external"
	"github.com/agentstack/agentstack/internal/mcplink"
	"github.com/agentstack/agentstack/internal/migrations/asmv1"
	"github.com/agentstack/agentstack/internal/processctl"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/routines"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
	"github.com/agentstack/agentstack/internal/workspace"
)

const maxStrictJSONInputBytes = 1 << 20
const maxMigrationReceiptBytes = 32 << 20

func (c *CLI) runHub(args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("hub requires list, graph, adapters, adapter-conformance, adapter-external-conformance, cas-stage, cas-verify, cas-restore, db-stage, db-inspect, db-verify, db-backup, import, audit, targets, target-add, backups, restore, plan-sync, apply-sync, plan-refresh, apply-refresh, or remove"))
	}
	switch args[0] {
	case "list":
		if len(args) != 1 {
			return c.failUsage(fmt.Errorf("hub list does not accept arguments"))
		}
		items, err := c.Service.ListResources()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"resources": items})
	case "graph":
		if len(args) != 1 {
			return c.failUsage(fmt.Errorf("hub graph does not accept arguments"))
		}
		snapshot, err := c.Service.CanonicalResourceGraph()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(snapshot)
	case "adapters":
		fs := flag.NewFlagSet("hub adapters", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		projectRoot := fs.String("project-root", ".", "project root used to resolve project-local client paths")
		targetRoot := fs.String("target-root", "", "target root used for resource projections; defaults to project root")
		var targets stringValues
		fs.Var(&targets, "target", "adapter target; repeat or comma-separate")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected hub adapters arguments"))
		}
		capabilities, err := c.Service.AdapterCapabilities(*projectRoot, *targetRoot, targets.Values())
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"contract": adapters.ContractVersion, "adapters": capabilities})
	case "adapter-conformance":
		fs := flag.NewFlagSet("hub adapter-conformance", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		projectRoot := fs.String("project-root", ".", "project root used to resolve project-local client paths")
		targetRoot := fs.String("target-root", "", "target root used for resource projections; defaults to project root")
		var targets stringValues
		fs.Var(&targets, "target", "adapter target or alias; repeat or comma-separate")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected hub adapter-conformance arguments"))
		}
		report, err := c.Service.AdapterConformance(*projectRoot, *targetRoot, targets.Values())
		if err != nil {
			return c.fail(err)
		}
		if code := c.printJSON(report); code != 0 {
			return code
		}
		if !report.Passed() {
			return 1
		}
		return 0
	case "adapter-external-conformance":
		fs := flag.NewFlagSet("hub adapter-external-conformance", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		executable := fs.String("executable", "", "absolute path to a standalone external adapter executable")
		executableDigest := fs.String("sha256", "", "pinned executable digest in sha256:<hex> form")
		target := fs.String("target", "", "reviewed target adapter or alias to emulate")
		timeout := fs.Duration("timeout", external.DefaultLimits().Timeout, "per-request deadline; maximum 30s")
		memoryBytes := fs.Uint64("memory-bytes", 0, "optional hard process-tree memory ceiling; Linux cgroup v2 or Windows Job Object")
		cpuPercent := fs.Uint("cpu-percent", 0, "optional hard CPU ceiling from 1 through 100")
		maxProcesses := fs.Uint("max-processes", 0, "optional hard active-process ceiling")
		var fixedArguments []string
		fs.Func("arg", "fixed executable argument; repeat to preserve exact argument boundaries", func(value string) error {
			fixedArguments = append(fixedArguments, value)
			return nil
		})
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *executable == "" || *executableDigest == "" || *target == "" {
			return c.failUsage(fmt.Errorf("hub adapter-external-conformance requires --executable PATH --sha256 SHA256 --target TARGET"))
		}
		processLimits := processctl.Limits{MemoryBytes: *memoryBytes, CPUPercent: uint32(*cpuPercent), ActiveProcesses: uint32(*maxProcesses)}
		report, err := c.Service.ExternalAdapterConformance(*executable, *executableDigest, *target, fixedArguments, *timeout, processLimits)
		if err != nil {
			return c.fail(err)
		}
		if code := c.printJSON(report); code != 0 {
			return code
		}
		if !report.Passed {
			return 1
		}
		return 0
	case "cas-stage":
		fs := flag.NewFlagSet("hub cas-stage", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		root := fs.String("root", "", "CAS root; defaults to the AgentStack data root")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected hub cas-stage arguments"))
		}
		receipt, err := c.Service.StageResourceHubCAS(*root)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(receipt)
	case "cas-verify":
		fs := flag.NewFlagSet("hub cas-verify", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		root := fs.String("root", "", "CAS root; defaults to the AgentStack data root")
		receiptPath := fs.String("receipt", "", "ASM v1 migration receipt JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *receiptPath == "" {
			return c.failUsage(fmt.Errorf("hub cas-verify requires --receipt FILE"))
		}
		var receipt asmv1.Receipt
		if err := readMigrationReceipt(*receiptPath, &receipt); err != nil {
			return c.fail(err)
		}
		if err := c.Service.VerifyResourceHubCAS(*root, receipt); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"verified": true, "receiptDigest": receipt.Digest, "sourceGraphDigest": receipt.SourceGraphDigest, "stagedGraphDigest": receipt.StagedGraph.Digest})
	case "cas-restore":
		fs := flag.NewFlagSet("hub cas-restore", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		root := fs.String("root", "", "CAS root; defaults to the AgentStack data root")
		receiptPath := fs.String("receipt", "", "ASM v1 migration receipt JSON")
		resourceID := fs.String("resource", "", "Resource Hub ID to restore")
		destination := fs.String("destination", "", "new restore destination")
		yes := fs.Bool("yes", false, "confirm materialization to the new destination")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *receiptPath == "" || *resourceID == "" || *destination == "" || !*yes {
			return c.failUsage(fmt.Errorf("hub cas-restore requires --receipt FILE --resource ID --destination PATH --yes"))
		}
		var receipt asmv1.Receipt
		if err := readMigrationReceipt(*receiptPath, &receipt); err != nil {
			return c.fail(err)
		}
		if err := c.Service.RestoreResourceHubCAS(*root, receipt, *resourceID, *destination, true); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"restored": *resourceID, "destination": *destination, "receiptDigest": receipt.Digest})
	case "db-stage":
		fs := flag.NewFlagSet("hub db-stage", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		databasePath := fs.String("db", "", "SQLite metadata path; defaults to the AgentStack data root")
		casRoot := fs.String("cas-root", "", "CAS root; defaults to the AgentStack data root")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected hub db-stage arguments"))
		}
		summary, err := c.Service.StageResourceHubSQLite(*databasePath, *casRoot)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(summary)
	case "db-inspect":
		fs := flag.NewFlagSet("hub db-inspect", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		databasePath := fs.String("db", "", "SQLite metadata path; defaults to the AgentStack data root")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected hub db-inspect arguments"))
		}
		summary, err := c.Service.InspectResourceHubSQLite(*databasePath)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(summary)
	case "db-verify":
		fs := flag.NewFlagSet("hub db-verify", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		databasePath := fs.String("db", "", "SQLite metadata path; defaults to the AgentStack data root")
		casRoot := fs.String("cas-root", "", "CAS root; defaults to the AgentStack data root")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected hub db-verify arguments"))
		}
		summary, err := c.Service.VerifyResourceHubSQLite(*databasePath, *casRoot)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"verified": true, "metadata": summary})
	case "db-backup":
		fs := flag.NewFlagSet("hub db-backup", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		databasePath := fs.String("db", "", "SQLite metadata path; defaults to the AgentStack data root")
		destination := fs.String("destination", "", "new SQLite backup destination")
		yes := fs.Bool("yes", false, "confirm verified no-overwrite backup publication")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *destination == "" || !*yes {
			return c.failUsage(fmt.Errorf("hub db-backup requires --destination PATH --yes"))
		}
		summary, err := c.Service.BackupResourceHubSQLite(*databasePath, *destination, true)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(summary)
	case "targets":
		if len(args) != 1 {
			return c.failUsage(fmt.Errorf("hub targets does not accept arguments"))
		}
		items, err := c.Service.ListResourceTargets()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"targets": items})
	case "backups":
		if len(args) != 1 {
			return c.failUsage(fmt.Errorf("hub backups does not accept arguments"))
		}
		items, err := c.Service.ListResourceBackups()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"backups": items})
	case "restore":
		fs := flag.NewFlagSet("hub restore", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		backupID := fs.String("backup", "", "resource backup ID")
		yes := fs.Bool("yes", false, "confirm resource rollback")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *backupID == "" || !*yes {
			return c.failUsage(fmt.Errorf("hub restore requires --backup ID --yes"))
		}
		resource, err := c.Service.RestoreResourceBackup(*backupID, true)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(resource)
	case "import":
		fs := flag.NewFlagSet("hub import", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "resource ID")
		kind := fs.String("kind", "", "skill|agent|rule|command|prompt|mcp-server|context")
		path := fs.String("path", "", "source file or directory")
		name := fs.String("name", "", "display name")
		description := fs.String("description", "", "description")
		scope := fs.String("scope", "", "optional scope")
		replace := fs.Bool("replace", false, "replace an existing resource with the same ID")
		var tags, targets stringValues
		fs.Var(&tags, "tag", "tag; repeat or comma-separate")
		fs.Var(&targets, "target", "agent target; repeat or comma-separate")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" || *kind == "" || *path == "" {
			return c.failUsage(fmt.Errorf("hub import requires --id ID --kind KIND --path PATH"))
		}
		item, err := c.Service.ImportResource(*path, resourcehub.ImportOptions{ID: *id, Kind: resourcehub.Kind(*kind), Name: *name, Description: *description, Tags: tags.Values(), Targets: toAgents(targets.Values()), Scope: *scope, Replace: *replace})
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(item)
	case "audit":
		fs := flag.NewFlagSet("hub audit", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "resource ID")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" {
			return c.failUsage(fmt.Errorf("hub audit requires --id ID"))
		}
		result, err := c.Service.AuditResource(*id)
		if err != nil {
			return c.fail(err)
		}
		if result.Blocked {
			_ = c.printJSON(result)
			return 1
		}
		return c.printJSON(result)
	case "target-add":
		fs := flag.NewFlagSet("hub target-add", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "target ID")
		agent := fs.String("agent", "", "codex|claude|cursor|opencode|github-copilot|agy|generic")
		root := fs.String("root", "", "target root")
		mode := fs.String("mode", string(resourcehub.ModeAuto), "auto|copy|link")
		enabled := fs.Bool("enabled", true, "enable target")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" || *agent == "" || *root == "" {
			return c.failUsage(fmt.Errorf("hub target-add requires --id ID --agent AGENT --root PATH"))
		}
		target := resourcehub.Target{ID: *id, Agent: resourcehub.Agent(*agent), Root: *root, Mode: resourcehub.SyncMode(*mode), Enabled: *enabled}
		if err := c.Service.RegisterResourceTarget(target); err != nil {
			return c.fail(err)
		}
		return c.printJSON(target)
	case "plan-sync":
		fs := flag.NewFlagSet("hub plan-sync", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		target := fs.String("target", "", "registered target ID")
		prune := fs.Bool("prune", false, "remove stale AgentStack-managed resources")
		allowRisk := fs.Bool("allow-risk", false, "allow resources blocked by static audit")
		denyLoss := fs.Bool("deny-loss", false, "fail when the adapter reports any transformation or fallback loss")
		var resources stringValues
		fs.Var(&resources, "resource", "resource ID; repeat or comma-separate")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *target == "" {
			return c.failUsage(fmt.Errorf("hub plan-sync requires --target ID"))
		}
		plan, err := c.Service.PlanResourceSync(*target, resources.Values(), resourcehub.PlanOptions{TTL: 15 * time.Minute, AllowRisk: *allowRisk, Prune: *prune, DenyLoss: *denyLoss})
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(plan)
	case "plan-refresh":
		fs := flag.NewFlagSet("hub plan-refresh", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		var resources stringValues
		fs.Var(&resources, "resource", "resource ID; repeat or comma-separate; default all enabled resources")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected hub plan-refresh arguments"))
		}
		plan, err := c.Service.PlanResourceRefresh(resources.Values())
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(plan)
	case "apply-refresh":
		fs := flag.NewFlagSet("hub apply-refresh", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		planID := fs.String("plan-id", "", "reviewed resource refresh plan ID")
		digest := fs.String("digest", "", "reviewed resource refresh plan digest")
		yes := fs.Bool("yes", false, "confirm the exact reviewed refresh plan")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *planID == "" || *digest == "" || !*yes {
			return c.failUsage(fmt.Errorf("hub apply-refresh requires --plan-id ID --digest SHA --yes"))
		}
		report, err := c.Service.ApplyResourceRefresh(*planID, *digest, true)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(report)
	case "apply-sync":
		fs := flag.NewFlagSet("hub apply-sync", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		planID := fs.String("plan-id", "", "reviewed sync plan ID")
		digest := fs.String("digest", "", "reviewed sync plan digest")
		yes := fs.Bool("yes", false, "confirm the exact reviewed sync plan")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *planID == "" || *digest == "" || !*yes {
			return c.failUsage(fmt.Errorf("hub apply-sync requires --plan-id ID --digest SHA --yes"))
		}
		report, err := c.Service.ApplyResourceSync(*planID, *digest, true)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(report)
	case "remove":
		fs := flag.NewFlagSet("hub remove", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "resource ID")
		yes := fs.Bool("yes", false, "confirm deletion from the canonical resource hub")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" || !*yes {
			return c.failUsage(fmt.Errorf("hub remove requires --id ID --yes"))
		}
		if err := c.Service.RemoveResource(*id, true); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"removed": *id})
	default:
		return c.failUsage(fmt.Errorf("unknown hub command %q", args[0]))
	}
}

func (c *CLI) runContext(args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("context requires scan, score, read, search, git, plan, or apply"))
	}
	fs := flag.NewFlagSet("context "+args[0], flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	root := fs.String("root", "", "project root")
	path := fs.String("path", "", "project-relative file path")
	query := fs.String("query", "", "search query")
	limit := fs.Int("limit", 50, "maximum search results")
	var targets stringValues
	fs.Var(&targets, "target", "codex|claude|cursor|opencode|github-copilot; repeat or comma-separate")
	planID := fs.String("plan-id", "", "reviewed context plan ID")
	digest := fs.String("digest", "", "reviewed context plan digest")
	yes := fs.Bool("yes", false, "confirm the exact reviewed context plan")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected context arguments"))
	}
	switch args[0] {
	case "scan":
		if *root == "" {
			return c.failUsage(fmt.Errorf("context scan requires --root PATH"))
		}
		result, err := c.Service.ContextScan(*root)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "score":
		if *root == "" {
			return c.failUsage(fmt.Errorf("context score requires --root PATH"))
		}
		result, err := c.Service.ContextScore(*root, toAgents(targets.Values()))
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "read":
		if *root == "" || *path == "" {
			return c.failUsage(fmt.Errorf("context read requires --root PATH --path RELATIVE"))
		}
		result, err := c.Service.ContextRead(*root, *path)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "search":
		if *root == "" || *query == "" {
			return c.failUsage(fmt.Errorf("context search requires --root PATH --query TEXT"))
		}
		result, err := c.Service.ContextSearch(*root, *query, *limit)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "git":
		if *root == "" {
			return c.failUsage(fmt.Errorf("context git requires --root PATH"))
		}
		result, err := c.Service.ContextGit(context.Background(), *root)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "plan":
		if *root == "" {
			return c.failUsage(fmt.Errorf("context plan requires --root PATH"))
		}
		result, err := c.Service.PlanContextRefresh(*root, toAgents(targets.Values()))
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "apply":
		if *planID == "" || *digest == "" || !*yes {
			return c.failUsage(fmt.Errorf("context apply requires --plan-id ID --digest SHA --yes"))
		}
		result, err := c.Service.ApplyContextRefresh(*planID, *digest, true)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	default:
		return c.failUsage(fmt.Errorf("unknown context command %q", args[0]))
	}
}

func (c *CLI) runWorkspace(args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("workspace requires list, show, create, update, render, or delete"))
	}
	switch args[0] {
	case "list":
		items, err := c.Service.ListWorkspaces()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"workspaces": items})
	case "show":
		fs := flag.NewFlagSet("workspace show", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "workspace ID")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" {
			return c.failUsage(fmt.Errorf("workspace show requires --id ID"))
		}
		item, err := c.Service.GetWorkspace(*id)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(item)
	case "create":
		fs := flag.NewFlagSet("workspace create", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "workspace ID")
		name := fs.String("name", "", "workspace name")
		kind := fs.String("type", string(workspace.TypeWorkspace), "workspace|folder")
		parent := fs.String("parent", "", "parent folder ID")
		root := fs.String("root", "", "workspace project root")
		prompt := fs.String("prompt", "", "workspace prompt template")
		var vars, resources, routineIDs stringValues
		fs.Var(&vars, "var", "prompt variable key=value; repeat")
		fs.Var(&resources, "resource", "resource ID; repeat or comma-separate")
		fs.Var(&routineIDs, "routine", "routine ID; repeat or comma-separate")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" || *name == "" || (*kind == string(workspace.TypeWorkspace) && *root == "") {
			return c.failUsage(fmt.Errorf("workspace create requires --id ID --name NAME and --root PATH for workspace type"))
		}
		assignments, err := parseAssignments(vars.Values())
		if err != nil {
			return c.failUsage(err)
		}
		item, err := c.Service.CreateWorkspace(workspace.Item{ID: *id, Name: *name, Type: workspace.ItemType(*kind), ParentID: *parent, Root: *root, Prompt: *prompt, Vars: assignments, ResourceIDs: resources.Values(), RoutineIDs: routineIDs.Values()})
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(item)
	case "update":
		fs := flag.NewFlagSet("workspace update", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		file := fs.String("file", "", "strict JSON workspace document")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *file == "" {
			return c.failUsage(fmt.Errorf("workspace update requires --file workspace.json"))
		}
		var item workspace.Item
		if err := readStrictJSON(*file, &item); err != nil {
			return c.fail(err)
		}
		updated, err := c.Service.UpdateWorkspace(item)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(updated)
	case "render":
		fs := flag.NewFlagSet("workspace render", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "workspace ID")
		var vars stringValues
		fs.Var(&vars, "var", "prompt variable key=value; repeat")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" {
			return c.failUsage(fmt.Errorf("workspace render requires --id ID"))
		}
		values, err := parseAssignments(vars.Values())
		if err != nil {
			return c.failUsage(err)
		}
		prompt, err := c.Service.RenderWorkspacePrompt(*id, values, time.Now().UTC())
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"workspaceId": *id, "prompt": prompt})
	case "delete":
		fs := flag.NewFlagSet("workspace delete", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "workspace ID")
		yes := fs.Bool("yes", false, "confirm recursive workspace deletion")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" || !*yes {
			return c.failUsage(fmt.Errorf("workspace delete requires --id ID --yes"))
		}
		if err := c.Service.DeleteWorkspace(*id, true); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"deleted": *id})
	default:
		return c.failUsage(fmt.Errorf("unknown workspace command %q", args[0]))
	}
}

func (c *CLI) runMemory(args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("memory requires remember, recall, search, or forget"))
	}
	fs := flag.NewFlagSet("memory "+args[0], flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	layer := fs.String("layer", string(workspace.LayerUser), "user|project|workspace|session")
	scope := fs.String("scope", "", "layer scope")
	key := fs.String("key", "", "memory key")
	value := fs.String("value", "", "memory value")
	query := fs.String("query", "", "search query")
	workspaceID := fs.String("workspace", "", "workspace ID")
	sessionID := fs.String("session", "", "session ID")
	source := fs.String("source", "", "memory source")
	ttl := fs.Duration("ttl", 0, "optional expiry duration")
	yes := fs.Bool("yes", false, "confirm memory deletion")
	var tags stringValues
	fs.Var(&tags, "tag", "tag; repeat or comma-separate")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected memory arguments"))
	}
	switch args[0] {
	case "remember":
		if *key == "" {
			return c.failUsage(fmt.Errorf("memory remember requires --key KEY --value VALUE"))
		}
		entry := workspace.MemoryEntry{Layer: workspace.MemoryLayer(*layer), Scope: *scope, Key: *key, Value: *value, Tags: tags.Values(), Source: *source}
		if *ttl > 0 {
			entry.ExpiresAt = time.Now().UTC().Add(*ttl)
		}
		result, err := c.Service.Remember(entry)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "recall":
		if *key == "" {
			return c.failUsage(fmt.Errorf("memory recall requires --key KEY"))
		}
		result, err := c.Service.Recall(*workspaceID, *key, *sessionID)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "search":
		result, err := c.Service.SearchMemory(*query, *workspaceID, *sessionID)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"memories": result})
	case "forget":
		if *key == "" || !*yes {
			return c.failUsage(fmt.Errorf("memory forget requires --layer LAYER --scope SCOPE --key KEY --yes"))
		}
		if err := c.Service.ForgetMemory(workspace.MemoryLayer(*layer), *scope, *key, true); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"forgotten": *key})
	default:
		return c.failUsage(fmt.Errorf("unknown memory command %q", args[0]))
	}
}

func (c *CLI) runArtifact(args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("artifact requires add, list, verify, or remove"))
	}
	fs := flag.NewFlagSet("artifact "+args[0], flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	id := fs.String("id", "", "artifact ID")
	workspaceID := fs.String("workspace", "", "workspace ID")
	path := fs.String("path", "", "source file")
	name := fs.String("name", "", "display name")
	mediaType := fs.String("media-type", "", "media type")
	replace := fs.Bool("replace", false, "replace existing artifact")
	yes := fs.Bool("yes", false, "confirm artifact deletion")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return c.failUsage(fmt.Errorf("unexpected artifact arguments"))
	}
	switch args[0] {
	case "add":
		if *id == "" || *workspaceID == "" || *path == "" {
			return c.failUsage(fmt.Errorf("artifact add requires --workspace ID --id ID --path FILE"))
		}
		result, err := c.Service.AddArtifact(*workspaceID, *path, workspace.ArtifactOptions{ID: *id, Name: *name, MediaType: *mediaType, Replace: *replace})
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "list":
		result, err := c.Service.ListArtifacts(*workspaceID)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"artifacts": result})
	case "verify":
		if *id == "" {
			return c.failUsage(fmt.Errorf("artifact verify requires --id ID"))
		}
		ok, err := c.Service.VerifyArtifact(*id)
		if err != nil {
			return c.fail(err)
		}
		if !ok {
			_ = c.printJSON(map[string]any{"id": *id, "verified": false})
			return 1
		}
		return c.printJSON(map[string]any{"id": *id, "verified": true})
	case "remove":
		if *id == "" || !*yes {
			return c.failUsage(fmt.Errorf("artifact remove requires --id ID --yes"))
		}
		if err := c.Service.RemoveArtifact(*id, true); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"removed": *id})
	default:
		return c.failUsage(fmt.Errorf("unknown artifact command %q", args[0]))
	}
}

func (c *CLI) runRoutine(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("routine requires put, list, due, history, run, run-due, or remove"))
	}
	switch args[0] {
	case "put":
		fs := flag.NewFlagSet("routine put", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		file := fs.String("file", "", "strict JSON routine document")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *file == "" {
			return c.failUsage(fmt.Errorf("routine put requires --file routine.json"))
		}
		var routine routines.Routine
		if err := readStrictJSON(*file, &routine); err != nil {
			return c.fail(err)
		}
		result, err := c.Service.PutRoutine(routine)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(result)
	case "list":
		items, err := c.Service.ListRoutines()
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"routines": items})
	case "due":
		items, err := c.Service.DueRoutines(time.Now().UTC())
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"routines": items})
	case "history":
		fs := flag.NewFlagSet("routine history", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "optional routine ID filter")
		limit := fs.Int("limit", 100, "maximum receipts to return")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *limit < 1 {
			return c.failUsage(fmt.Errorf("routine history accepts --id ID --limit N"))
		}
		reports, err := c.Service.ListRoutineRuns(*id, *limit)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"reports": reports})
	case "run", "run-due":
		fs := flag.NewFlagSet("routine "+args[0], flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "routine ID")
		yes := fs.Bool("yes", false, "confirm execution of routine steps")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || !*yes || (args[0] == "run" && *id == "") {
			return c.failUsage(fmt.Errorf("routine %s requires %s--yes", args[0], map[bool]string{true: "--id ID ", false: ""}[args[0] == "run"]))
		}
		ids := []string{*id}
		if args[0] == "run-due" {
			due, err := c.Service.DueRoutines(time.Now().UTC())
			if err != nil {
				return c.fail(err)
			}
			ids = ids[:0]
			for _, routine := range due {
				ids = append(ids, routine.ID)
			}
		}
		reports := make([]routines.RunReport, 0, len(ids))
		failures := map[string]string{}
		for _, routineID := range ids {
			report, err := c.Service.RunRoutine(ctx, routineID, true)
			reports = append(reports, report)
			if err != nil {
				failures[routineID] = err.Error()
			}
		}
		_ = c.printJSON(map[string]any{"reports": reports, "failures": failures})
		if len(failures) > 0 {
			return 1
		}
		return 0
	case "remove":
		fs := flag.NewFlagSet("routine remove", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		id := fs.String("id", "", "routine ID")
		yes := fs.Bool("yes", false, "confirm routine deletion")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *id == "" || !*yes {
			return c.failUsage(fmt.Errorf("routine remove requires --id ID --yes"))
		}
		if err := c.Service.DeleteRoutine(*id, true); err != nil {
			return c.fail(err)
		}
		return c.printJSON(map[string]any{"removed": *id})
	default:
		return c.failUsage(fmt.Errorf("unknown routine command %q", args[0]))
	}
}

func (c *CLI) runMCPLink(ctx context.Context, args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("mcp clients requires plan or apply"))
	}
	switch args[0] {
	case "plan":
		fs := flag.NewFlagSet("mcp clients plan", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		root := fs.String("root", ".", "project root for project-local clients")
		mode := fs.String("mode", string(mcplink.ModeLink), "link|unlink")
		var clients stringValues
		fs.Var(&clients, "client", "codex|claude|cursor|agy|opencode; repeat or comma-separate")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 {
			return c.failUsage(fmt.Errorf("unexpected mcp clients plan arguments"))
		}
		plan, err := c.Service.PlanMCPClientLinks(*root, mcplink.Mode(*mode), toClients(clients.Values()))
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(plan)
	case "apply":
		fs := flag.NewFlagSet("mcp clients apply", flag.ContinueOnError)
		fs.SetOutput(c.errWriter())
		root := fs.String("root", ".", "same project root used to create the plan")
		planID := fs.String("plan-id", "", "reviewed MCP client plan ID")
		digest := fs.String("digest", "", "reviewed MCP client plan digest")
		yes := fs.Bool("yes", false, "confirm the exact reviewed MCP client plan")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() != 0 || *planID == "" || *digest == "" || !*yes {
			return c.failUsage(fmt.Errorf("mcp clients apply requires --plan-id ID --digest SHA --yes"))
		}
		report, err := c.Service.ApplyMCPClientLinks(ctx, *root, *planID, *digest, true)
		if err != nil {
			return c.fail(err)
		}
		return c.printJSON(report)
	default:
		return c.failUsage(fmt.Errorf("unknown mcp clients command %q", args[0]))
	}
}

func readMigrationReceipt(path string, destination *asmv1.Receipt) error {
	data, err := safefile.ReadBoundedRegular(path, maxMigrationReceiptBytes)
	if err != nil {
		return err
	}
	if err := strictjson.Decode(data, destination); err != nil {
		return err
	}
	return asmv1.VerifyReceipt(*destination)
}

func readStrictJSON(path string, destination any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("strict JSON input must be a regular file")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxStrictJSONInputBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxStrictJSONInputBytes {
		return fmt.Errorf("strict JSON input exceeds %d bytes", maxStrictJSONInputBytes)
	}
	return strictjson.Decode(data, destination)
}

func parseAssignments(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, value := range values {
		parts := strings.SplitN(value, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, fmt.Errorf("expected key=value, got %q", value)
		}
		result[strings.TrimSpace(parts[0])] = parts[1]
	}
	return result, nil
}

func toAgents(values []string) []resourcehub.Agent {
	result := make([]resourcehub.Agent, 0, len(values))
	for _, value := range values {
		result = append(result, resourcehub.Agent(value))
	}
	return result
}

func toClients(values []string) []mcplink.ClientKind {
	result := make([]mcplink.ClientKind, 0, len(values))
	for _, value := range values {
		result = append(result, mcplink.ClientKind(value))
	}
	return result
}
