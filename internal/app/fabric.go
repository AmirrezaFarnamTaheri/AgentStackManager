package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/builtin"
	"github.com/agentstack/agentstack/internal/adapters/conformance"
	"github.com/agentstack/agentstack/internal/adapters/external"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/cas"
	"github.com/agentstack/agentstack/internal/contextengine"
	"github.com/agentstack/agentstack/internal/mcplink"
	"github.com/agentstack/agentstack/internal/migrations/asmv1"
	"github.com/agentstack/agentstack/internal/processctl"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/routines"
	"github.com/agentstack/agentstack/internal/runner"
	sqlitestore "github.com/agentstack/agentstack/internal/store/sqlite"
	"github.com/agentstack/agentstack/internal/workspace"
)

type FabricStatus struct {
	Resources       int       `json:"resources"`
	ResourceTargets int       `json:"resourceTargets"`
	Workspaces      int       `json:"workspaces"`
	Folders         int       `json:"folders"`
	Artifacts       int       `json:"artifacts"`
	Routines        int       `json:"routines"`
	DueRoutines     int       `json:"dueRoutines"`
	NextRoutine     time.Time `json:"nextRoutine,omitempty"`
}

const fabricLeaseStaleAfter = 26 * time.Hour

func withFabricLease[T any](s *Service, operation func() (T, error)) (result T, err error) {
	lease, err := s.Store.AcquireLease("fabric", fabricLeaseStaleAfter)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	return operation()
}

func withFabricLeaseError(s *Service, operation func() error) error {
	_, err := withFabricLease(s, func() (struct{}, error) {
		return struct{}{}, operation()
	})
	return err
}

func (s *Service) resourceHubManager() resourcehub.Manager {
	if s.ResourceHub.Root != "" {
		return s.ResourceHub
	}
	return resourcehub.New(filepath.Join(s.Paths.DataRoot, "hub"))
}

func (s *Service) contextManager() contextengine.Manager {
	if s.Context.Root != "" {
		return s.Context
	}
	return contextengine.New(filepath.Join(s.Paths.DataRoot, "context"))
}

func (s *Service) workspaceManager() workspace.Manager {
	if s.Workspaces.Root != "" {
		return s.Workspaces
	}
	return workspace.New(filepath.Join(s.Paths.DataRoot, "workspace"))
}

func (s *Service) routineManager() routines.Manager {
	if s.Routines.Root != "" {
		return s.Routines
	}
	return routines.New(filepath.Join(s.Paths.DataRoot, "routines"))
}

func (s *Service) MCPLinkManager(projectRoot string) mcplink.Manager {
	return mcplink.New(filepath.Join(s.Paths.DataRoot, "mcp-link"), mcplink.Options{
		ProjectRoot:  projectRoot,
		AgyConfig:    s.Paths.AgyConfig,
		Executable:   s.Paths.Executable,
		RouterConfig: s.Paths.RouterConfig,
		Commands:     s.Commands,
	})
}

func (s *Service) FabricStatus(now time.Time) (FabricStatus, error) {
	resources, err := s.resourceHubManager().ListResources()
	if err != nil {
		return FabricStatus{}, err
	}
	targets, err := s.resourceHubManager().ListTargets()
	if err != nil {
		return FabricStatus{}, err
	}
	items, err := s.workspaceManager().List()
	if err != nil {
		return FabricStatus{}, err
	}
	artifacts, err := s.workspaceManager().ListArtifacts("")
	if err != nil {
		return FabricStatus{}, err
	}
	routineItems, err := s.routineManager().List()
	if err != nil {
		return FabricStatus{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	due, err := s.routineManager().Due(now)
	if err != nil {
		return FabricStatus{}, err
	}
	status := FabricStatus{
		Resources: len(resources), ResourceTargets: len(targets), Artifacts: len(artifacts),
		Routines: len(routineItems), DueRoutines: len(due),
	}
	for _, item := range items {
		if item.Type == workspace.TypeFolder {
			status.Folders++
		} else {
			status.Workspaces++
		}
	}
	for _, routine := range routineItems {
		if routine.Enabled && !routine.NextRun.IsZero() && (status.NextRoutine.IsZero() || routine.NextRun.Before(status.NextRoutine)) {
			status.NextRoutine = routine.NextRun
		}
	}
	return status, nil
}

func (s *Service) ListResources() ([]resourcehub.Resource, error) {
	return s.resourceHubManager().ListResources()
}
func (s *Service) CanonicalResourceGraph() (artifactgraph.Snapshot, error) {
	return s.resourceHubManager().CanonicalSnapshot()
}

func (s *Service) AdapterCapabilities(projectRoot, targetRoot string, targets []string) ([]adapters.CapabilitySet, error) {
	if strings.TrimSpace(projectRoot) == "" {
		projectRoot = "."
	}
	if strings.TrimSpace(targetRoot) == "" {
		targetRoot = projectRoot
	}
	registry := builtin.MustRegistry()
	environment := builtin.RuntimeEnvironment(projectRoot, targetRoot, "", s.Paths.AgyConfig)
	return registry.Capabilities(context.Background(), environment, targets)
}

func (s *Service) AdapterConformance(projectRoot, targetRoot string, targets []string) (conformance.Report, error) {
	if strings.TrimSpace(projectRoot) == "" {
		projectRoot = "."
	}
	if strings.TrimSpace(targetRoot) == "" {
		targetRoot = projectRoot
	}
	home := filepath.Join(targetRoot, ".agentstack-conformance-home")
	agyConfig := strings.TrimSpace(s.Paths.AgyConfig)
	if agyConfig == "" {
		agyConfig = filepath.Join(home, ".gemini", "config", "mcp_config.json")
	}
	environment := builtin.RuntimeEnvironment(projectRoot, targetRoot, home, agyConfig)
	return conformance.RunEmbedded(context.Background(), builtin.MustRegistry(), conformance.RunOptions{Environment: environment, Targets: targets})
}

// ExternalAdapterConformance admits pinned executable bytes into a private,
// synthetic environment and differentially tests them against the reviewed
// built-in target adapter. It never registers or activates the executable.
func (s *Service) ExternalAdapterConformance(executable, executableDigest, target string, arguments []string, timeout time.Duration, processLimits processctl.Limits) (external.ConformanceReport, error) {
	registry := builtin.MustRegistry()
	reference, err := registry.Get(target)
	if err != nil {
		return external.ConformanceReport{}, err
	}
	sessionRoot, err := os.MkdirTemp("", "agentstack-external-conformance-")
	if err != nil {
		return external.ConformanceReport{}, fmt.Errorf("create external adapter conformance environment: %w", err)
	}
	defer os.RemoveAll(sessionRoot)
	if err := os.Chmod(sessionRoot, 0o700); err != nil {
		return external.ConformanceReport{}, fmt.Errorf("harden external adapter conformance environment: %w", err)
	}
	projectRoot := filepath.Join(sessionRoot, "project")
	targetRoot := filepath.Join(sessionRoot, "target")
	home := filepath.Join(sessionRoot, "home")
	agyConfig := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	for _, directory := range []string{projectRoot, targetRoot, filepath.Dir(agyConfig)} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return external.ConformanceReport{}, fmt.Errorf("create external adapter conformance directory: %w", err)
		}
	}
	limits := external.DefaultLimits()
	if timeout != 0 {
		limits.Timeout = timeout
	}
	limits.Process = processLimits
	environment := builtin.RuntimeEnvironment(projectRoot, targetRoot, home, agyConfig)
	admission := external.Admission{
		Executable: executable, ExecutableDigest: executableDigest, Arguments: arguments,
		Target: reference.ID(), SandboxRoot: sessionRoot, Environment: environment, Limits: limits,
	}
	return external.RunConformance(context.Background(), admission, reference)
}

func (s *Service) StageResourceHubCAS(root string) (asmv1.Receipt, error) {
	store, err := s.fabricCASStore(root)
	if err != nil {
		return asmv1.Receipt{}, err
	}
	return withFabricLease(s, func() (asmv1.Receipt, error) {
		return asmv1.Stage(s.resourceHubManager(), store, time.Now)
	})
}

func (s *Service) VerifyResourceHubCAS(root string, receipt asmv1.Receipt) error {
	store, err := s.fabricCASStore(root)
	if err != nil {
		return err
	}
	return asmv1.VerifyCurrent(s.resourceHubManager(), store, receipt)
}

func (s *Service) RestoreResourceHubCAS(root string, receipt asmv1.Receipt, resourceID, destination string, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("CAS resource restore requires explicit confirmation")
	}
	store, err := s.fabricCASStore(root)
	if err != nil {
		return err
	}
	return withFabricLeaseError(s, func() error {
		return asmv1.RestoreResource(store, receipt, resourceID, destination)
	})
}

// StageResourceHubSQLite creates or advances the reversible SQLite shadow
// metadata head from a freshly verified Resource Hub/CAS receipt. It does not
// redirect Resource Hub reads or target mutation authority.
func (s *Service) StageResourceHubSQLite(databasePath, casRoot string) (sqlitestore.Summary, error) {
	objectStore, err := s.fabricCASStore(casRoot)
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	metadataStore, err := s.fabricSQLiteStore(databasePath)
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	return withFabricLease(s, func() (sqlitestore.Summary, error) {
		receipt, err := asmv1.Stage(s.resourceHubManager(), objectStore, time.Now)
		if err != nil {
			return sqlitestore.Summary{}, err
		}
		summary, err := metadataStore.Stage(receipt)
		if err != nil {
			return sqlitestore.Summary{}, err
		}
		inspection, err := metadataStore.Inspect()
		if err != nil {
			return sqlitestore.Summary{}, err
		}
		if err := asmv1.VerifyCurrent(s.resourceHubManager(), objectStore, inspection.Receipt); err != nil {
			return sqlitestore.Summary{}, err
		}
		return summary, nil
	})
}

func (s *Service) InspectResourceHubSQLite(databasePath string) (sqlitestore.Summary, error) {
	metadataStore, err := s.fabricSQLiteStore(databasePath)
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	inspection, err := metadataStore.Inspect()
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	return inspection.Summary, nil
}

func (s *Service) VerifyResourceHubSQLite(databasePath, casRoot string) (sqlitestore.Summary, error) {
	metadataStore, err := s.fabricSQLiteStore(databasePath)
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	objectStore, err := s.fabricCASStore(casRoot)
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	inspection, err := metadataStore.Inspect()
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	if err := asmv1.VerifyCurrent(s.resourceHubManager(), objectStore, inspection.Receipt); err != nil {
		return sqlitestore.Summary{}, err
	}
	return inspection.Summary, nil
}

func (s *Service) BackupResourceHubSQLite(databasePath, destination string, confirmed bool) (sqlitestore.Summary, error) {
	if !confirmed {
		return sqlitestore.Summary{}, fmt.Errorf("SQLite metadata backup requires explicit confirmation")
	}
	metadataStore, err := s.fabricSQLiteStore(databasePath)
	if err != nil {
		return sqlitestore.Summary{}, err
	}
	return withFabricLease(s, func() (sqlitestore.Summary, error) {
		return metadataStore.Backup(destination)
	})
}

func (s *Service) fabricCASStore(root string) (cas.Store, error) {
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(s.Paths.DataRoot, "fabric", "cas")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return cas.Store{}, err
	}
	return cas.New(absolute), nil
}

func (s *Service) fabricSQLiteStore(path string) (sqlitestore.Store, error) {
	if strings.TrimSpace(path) == "" {
		path = filepath.Join(s.Paths.DataRoot, "fabric", "metadata.db")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return sqlitestore.Store{}, err
	}
	return sqlitestore.New(absolute), nil
}
func (s *Service) ImportResource(source string, options resourcehub.ImportOptions) (resourcehub.Resource, error) {
	return withFabricLease(s, func() (resourcehub.Resource, error) {
		return s.resourceHubManager().Import(source, options)
	})
}
func (s *Service) AuditResource(id string) (resourcehub.AuditResult, error) {
	return s.resourceHubManager().Audit(id)
}
func (s *Service) RegisterResourceTarget(target resourcehub.Target) error {
	return withFabricLeaseError(s, func() error { return s.resourceHubManager().RegisterTarget(target) })
}
func (s *Service) InspectResourceSync() (resourcehub.SyncInspection, error) {
	return s.resourceHubManager().Inspect()
}

func (s *Service) ListResourceTargets() ([]resourcehub.Target, error) {
	return s.resourceHubManager().ListTargets()
}
func (s *Service) ListResourceBackups() ([]resourcehub.BackupInfo, error) {
	return s.resourceHubManager().ListBackups()
}
func (s *Service) RestoreResourceBackup(id string, confirmed bool) (resourcehub.Resource, error) {
	return withFabricLease(s, func() (resourcehub.Resource, error) {
		return s.resourceHubManager().RestoreBackup(id, confirmed)
	})
}
func (s *Service) PlanResourceSync(targetID string, resourceIDs []string, options resourcehub.PlanOptions) (resourcehub.SyncPlan, error) {
	return s.resourceHubManager().PlanSync(targetID, resourceIDs, options)
}
func (s *Service) ApplyResourceSync(planID, digest string, confirmed bool) (resourcehub.SyncReport, error) {
	return withFabricLease(s, func() (resourcehub.SyncReport, error) {
		return s.resourceHubManager().ApplySync(planID, digest, confirmed)
	})
}
func (s *Service) PlanResourceBatchSync(targetIDs, resourceIDs []string, maxParallel int) (resourcehub.BatchSyncPlan, error) {
	return s.resourceHubManager().PlanBatchSync(targetIDs, resourceIDs, resourcehub.PlanOptions{TTL: 15 * time.Minute, AllowRisk: true}, maxParallel)
}
func (s *Service) ApplyResourceBatchSync(ctx context.Context, planID, digest string, confirmed bool) (resourcehub.BatchSyncReport, error) {
	return withFabricLease(s, func() (resourcehub.BatchSyncReport, error) {
		return s.resourceHubManager().ApplyBatchSync(ctx, planID, digest, confirmed)
	})
}
func (s *Service) PlanResourceRefresh(resourceIDs []string) (resourcehub.RefreshPlan, error) {
	return s.resourceHubManager().PlanRefresh(resourceIDs, 15*time.Minute)
}
func (s *Service) ApplyResourceRefresh(planID, digest string, confirmed bool) (resourcehub.RefreshReport, error) {
	return withFabricLease(s, func() (resourcehub.RefreshReport, error) {
		return s.resourceHubManager().ApplyRefresh(planID, digest, confirmed)
	})
}
func (s *Service) RemoveResource(id string, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("resource removal requires explicit confirmation")
	}
	return withFabricLeaseError(s, func() error { return s.resourceHubManager().RemoveResource(id) })
}

func (s *Service) ContextScan(root string) (contextengine.Snapshot, error) {
	return s.contextManager().Scan(root)
}
func (s *Service) ContextScore(root string, targets []resourcehub.Agent) (contextengine.ScoreResult, error) {
	return s.contextManager().Score(root, targets)
}
func (s *Service) ContextRead(root, relative string) (contextengine.FileView, error) {
	return s.contextManager().ReadFile(root, relative)
}
func (s *Service) ContextSearch(root, query string, limit int) (contextengine.SearchResult, error) {
	return s.contextManager().Search(root, query, limit)
}
func (s *Service) ContextGit(ctx context.Context, root string) (contextengine.GitContext, error) {
	manager := s.contextManager()
	manager.Commands = s.Commands
	return manager.Git(ctx, root)
}
func (s *Service) PlanContextRefresh(root string, targets []resourcehub.Agent) (contextengine.RefreshPlan, error) {
	return s.contextManager().PlanRefresh(root, targets, 15*time.Minute)
}
func (s *Service) ApplyContextRefresh(planID, digest string, confirmed bool) (contextengine.RefreshReport, error) {
	return withFabricLease(s, func() (contextengine.RefreshReport, error) {
		return s.contextManager().ApplyRefresh(planID, digest, confirmed)
	})
}

func (s *Service) CreateWorkspace(item workspace.Item) (workspace.Item, error) {
	return withFabricLease(s, func() (workspace.Item, error) { return s.workspaceManager().Create(item) })
}
func (s *Service) UpdateWorkspace(item workspace.Item) (workspace.Item, error) {
	return withFabricLease(s, func() (workspace.Item, error) { return s.workspaceManager().Update(item) })
}
func (s *Service) GetWorkspace(id string) (workspace.Item, error) {
	return s.workspaceManager().Get(id)
}
func (s *Service) ListWorkspaces() ([]workspace.Item, error) { return s.workspaceManager().List() }
func (s *Service) DeleteWorkspace(id string, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("workspace deletion requires explicit confirmation")
	}
	return withFabricLeaseError(s, func() error { return s.workspaceManager().Delete(id) })
}
func (s *Service) RenderWorkspacePrompt(id string, values map[string]string, now time.Time) (string, error) {
	return s.workspaceManager().RenderPrompt(id, values, now)
}
func (s *Service) Remember(entry workspace.MemoryEntry) (workspace.MemoryEntry, error) {
	return withFabricLease(s, func() (workspace.MemoryEntry, error) { return s.workspaceManager().Remember(entry) })
}
func (s *Service) Recall(workspaceID, key, sessionID string) (workspace.MemoryEntry, error) {
	return s.workspaceManager().Recall(workspaceID, key, sessionID)
}
func (s *Service) SearchMemory(query, workspaceID, sessionID string) ([]workspace.MemoryEntry, error) {
	return s.workspaceManager().SearchMemory(query, workspaceID, sessionID)
}
func (s *Service) ForgetMemory(layer workspace.MemoryLayer, scope, key string, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("memory deletion requires explicit confirmation")
	}
	return withFabricLeaseError(s, func() error { return s.workspaceManager().Forget(layer, scope, key) })
}
func (s *Service) AddArtifact(workspaceID, source string, options workspace.ArtifactOptions) (workspace.Artifact, error) {
	return withFabricLease(s, func() (workspace.Artifact, error) {
		return s.workspaceManager().AddArtifact(workspaceID, source, options)
	})
}
func (s *Service) ListArtifacts(workspaceID string) ([]workspace.Artifact, error) {
	return s.workspaceManager().ListArtifacts(workspaceID)
}
func (s *Service) VerifyArtifact(id string) (bool, error) {
	return s.workspaceManager().VerifyArtifact(id)
}
func (s *Service) RemoveArtifact(id string, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("artifact removal requires explicit confirmation")
	}
	return withFabricLeaseError(s, func() error { return s.workspaceManager().RemoveArtifact(id) })
}

func (s *Service) PutRoutine(routine routines.Routine) (routines.Routine, error) {
	return withFabricLease(s, func() (routines.Routine, error) { return s.routineManager().Put(routine) })
}
func (s *Service) ListRoutines() ([]routines.Routine, error) { return s.routineManager().List() }
func (s *Service) ListRoutineRuns(routineID string, limit int) ([]routines.RunReport, error) {
	return s.routineManager().ListRuns(routineID, limit)
}
func (s *Service) DueRoutines(now time.Time) ([]routines.Routine, error) {
	return s.routineManager().Due(now)
}
func (s *Service) RunRoutine(ctx context.Context, id string, confirmed bool) (routines.RunReport, error) {
	return withFabricLease(s, func() (routines.RunReport, error) {
		return s.routineManager().Run(ctx, id, confirmed, s)
	})
}
func (s *Service) DeleteRoutine(id string, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("routine deletion requires explicit confirmation")
	}
	return withFabricLeaseError(s, func() error { return s.routineManager().Delete(id) })
}

func (s *Service) PlanMCPClientLinks(projectRoot string, mode mcplink.Mode, clients []mcplink.ClientKind) (mcplink.Plan, error) {
	return s.MCPLinkManager(projectRoot).Plan(mode, clients, 15*time.Minute)
}
func (s *Service) ApplyMCPClientLinks(ctx context.Context, projectRoot, planID, digest string, confirmed bool) (mcplink.Report, error) {
	return withFabricLease(s, func() (mcplink.Report, error) {
		return s.MCPLinkManager(projectRoot).Apply(ctx, planID, digest, confirmed)
	})
}

// Execute implements routines.Executor. Every step is bounded by the routine
// confirmation gate; command steps invoke a binary directly and never a shell.
func (s *Service) Execute(ctx context.Context, routine routines.Routine, step routines.Step) (any, error) {
	stepCtx := ctx
	if step.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, time.Duration(step.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	root := strings.TrimSpace(step.Params["root"])
	if root == "" && routine.WorkspaceID != "" {
		if item, err := s.GetWorkspace(routine.WorkspaceID); err == nil {
			root = item.Root
		}
	}
	switch step.Kind {
	case routines.StepInventory:
		return s.Inventory(stepCtx)
	case routines.StepMCPDoctor:
		return s.MCPDoctor(stepCtx)
	case routines.StepContextScan:
		if root == "" {
			return nil, fmt.Errorf("context-scan routine step requires root or workspace")
		}
		return s.ContextScan(root)
	case routines.StepContextScore:
		if root == "" {
			return nil, fmt.Errorf("context-score routine step requires root or workspace")
		}
		return s.ContextScore(root, parseAgentList(step.Params["targets"]))
	case routines.StepMemorySearch:
		return s.SearchMemory(step.Params["query"], routine.WorkspaceID, step.Params["session"])
	case routines.StepPromptRender:
		if routine.WorkspaceID == "" {
			return nil, fmt.Errorf("prompt-render routine step requires workspaceId")
		}
		values := map[string]string{}
		for key, value := range step.Params {
			if strings.HasPrefix(key, "var.") {
				values[strings.TrimPrefix(key, "var.")] = value
			}
		}
		return s.RenderWorkspacePrompt(routine.WorkspaceID, values, time.Now().UTC())
	case routines.StepArtifactVerify:
		return s.VerifyArtifact(step.Params["id"])
	case routines.StepResourceAudit:
		return s.AuditResource(step.Params["id"])
	case routines.StepResourceRefreshPlan:
		ids := strings.Split(step.Params["ids"], ",")
		if strings.TrimSpace(step.Params["ids"]) == "" {
			ids = nil
		}
		return s.PlanResourceRefresh(ids)
	case routines.StepCommand:
		if strings.TrimSpace(step.Command) == "" {
			return nil, fmt.Errorf("command routine step requires command")
		}
		result := s.Commands.Run(stepCtx, runner.Invocation{Command: step.Command, Args: append([]string(nil), step.Args...), Timeout: time.Duration(step.TimeoutSeconds) * time.Second, MaxOutputBytes: 1 << 20})
		output := map[string]any{"command": step.Command, "args": step.Args, "stdout": result.Stdout, "stderr": result.Stderr, "exitCode": result.ExitCode, "truncated": result.Truncated}
		if result.Err != nil {
			return output, result.Err
		}
		return output, nil
	default:
		return nil, fmt.Errorf("unsupported routine step kind %q", step.Kind)
	}
}

func parseAgentList(value string) []resourcehub.Agent {
	var result []resourcehub.Agent
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, resourcehub.Agent(item))
		}
	}
	return result
}
