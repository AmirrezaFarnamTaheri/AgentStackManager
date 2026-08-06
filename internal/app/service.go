package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/catalog"
	"github.com/agentstack/agentstack/internal/contextengine"
	"github.com/agentstack/agentstack/internal/diagnostics"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/inventory"
	"github.com/agentstack/agentstack/internal/mcp"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/reviewedplan"
	"github.com/agentstack/agentstack/internal/routines"
	"github.com/agentstack/agentstack/internal/runner"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/security"
	"github.com/agentstack/agentstack/internal/skills"
	"github.com/agentstack/agentstack/internal/state"
	"github.com/agentstack/agentstack/internal/workspace"
)

var ErrConfirmationRequired = reviewedplan.ErrConfirmationRequired
var ErrPlanStale = reviewedplan.ErrPlanStale
var ErrPlanMismatch = reviewedplan.ErrPlanMismatch

type Service struct {
	Catalog    model.Catalog
	Scanner    inventory.Scanner
	Store      state.Store
	Installer  runner.Engine
	Commands   runner.CommandRunner
	Paths      Paths
	LookPath   inventory.Locator
	SkillRoots []string
	PlanTTL    time.Duration
	DoctorTTL  time.Duration

	ResourceHub resourcehub.Manager
	Context     contextengine.Manager
	Workspaces  workspace.Manager
	Routines    routines.Manager

	doctorMu      sync.Mutex
	doctorCached  DoctorReport
	doctorCacheAt time.Time
}

type ApplyReport struct {
	Plan        model.Plan        `json:"plan"`
	Transaction model.Transaction `json:"transaction"`
	Router      *MCPInitReport    `json:"router,omitempty"`
}

type WarmResult struct {
	Server   string `json:"server"`
	Command  string `json:"command,omitempty"`
	ExitCode int    `json:"exitCode,omitempty"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

type MCPInitOptions struct {
	Request         planner.Request `json:"request"`
	RegisterClients bool            `json:"registerClients"`
	Warm            bool            `json:"warm"`
	Confirm         bool            `json:"confirm"`
}

type MCPInitReport struct {
	RouterConfigPath string                  `json:"routerConfigPath"`
	RouterChanged    bool                    `json:"routerChanged"`
	Servers          []string                `json:"servers"`
	Warm             []WarmResult            `json:"warm,omitempty"`
	Codex            *mcp.RegistrationResult `json:"codex,omitempty"`
	Agy              *mcp.MergeResult        `json:"agy,omitempty"`
	BackupPath       string                  `json:"backupPath,omitempty"`
}

type DoctorReport struct {
	Healthy        bool                      `json:"healthy"`
	Config         string                    `json:"config"`
	Servers        map[string]mcp.DoctorItem `json:"servers"`
	CheckedAt      time.Time                 `json:"checkedAt"`
	Duration       time.Duration             `json:"duration"`
	Cached         bool                      `json:"cached,omitempty"`
	HealthyCount   int                       `json:"healthyCount"`
	UnhealthyCount int                       `json:"unhealthyCount"`
}

type RestoreReport struct {
	Backup              state.BackupRecord `json:"backup"`
	Validated           bool               `json:"validated"`
	StructuralValidated bool               `json:"structuralValidated"`
	LiveValidated       bool               `json:"liveValidated,omitempty"`
	Validation          string             `json:"validation"`
	Doctor              *DoctorReport      `json:"doctor,omitempty"`
	RolledBack          bool               `json:"rolledBack,omitempty"`
	Rollback            string             `json:"rollback,omitempty"`
}

type RestorePreview struct {
	Backup              state.BackupRecord `json:"backup"`
	Target              string             `json:"target"`
	DigestVerified      bool               `json:"digestVerified"`
	StructuralValidated bool               `json:"structuralValidated"`
	Validation          string             `json:"validation"`
}

func NewDefault() (*Service, error) {
	c, err := catalog.LoadDefault()
	if err != nil {
		return nil, err
	}
	paths, err := DefaultPaths()
	if err != nil {
		return nil, err
	}
	if err := security.EnsurePrivateDir(paths.DataRoot); err != nil {
		return nil, err
	}
	commands := runner.ExecRunner{}
	skillRoots := skills.DefaultTargets()
	skillInstaller := skills.Installer{Commands: commands, Targets: skillRoots}
	service := &Service{
		Catalog:     c,
		Scanner:     inventory.NewScanner(),
		Store:       state.NewStore(paths.DataRoot),
		Installer:   runner.Engine{Commands: commands, Skills: skillInstaller, Catalog: c, MaxParallel: 4},
		Commands:    commands,
		Paths:       paths,
		LookPath:    defaultLocator{},
		SkillRoots:  skillRoots,
		PlanTTL:     planner.DefaultPlanTTL,
		DoctorTTL:   30 * time.Second,
		ResourceHub: resourcehub.New(filepath.Join(paths.DataRoot, "hub")),
		Context:     contextengine.New(filepath.Join(paths.DataRoot, "context")),
		Workspaces:  workspace.New(filepath.Join(paths.DataRoot, "workspace")),
		Routines:    routines.New(filepath.Join(paths.DataRoot, "routines")),
	}
	service.Installer.Verifier = inventory.Verifier{Scanner: service.Scanner, Catalog: c}
	if err := prepareStore(service.Store, time.Now().UTC()); err != nil {
		return nil, err
	}
	return service, nil
}

func (s *Service) Inventory(ctx context.Context) (model.Inventory, error) {
	ownership, err := s.Store.LoadOwnership()
	if err != nil {
		return model.Inventory{}, err
	}
	managed := make(map[string]bool, len(ownership.ManagedComponents))
	for id := range ownership.ManagedComponents {
		managed[id] = true
	}
	result := s.Scanner.Scan(ctx, s.Catalog, managed)
	s.applyManagedPathHealth(&result, ownership)
	s.applyManagedRouterHealth(&result, ownership)
	if err := s.reconcileOwnershipHealth(result, ownership); err != nil {
		return model.Inventory{}, err
	}
	if err := s.Store.SaveInventory(inventory.Minimized(result)); err != nil {
		return model.Inventory{}, err
	}
	_ = s.logEvent(state.Event{Level: "info", Type: "inventory.completed", Fields: map[string]any{"components": len(result.Items), "externalSources": len(result.External)}})
	return result, nil
}

func (s *Service) reconcileOwnershipHealth(result model.Inventory, ownership state.Ownership) error {
	changed := false
	now := time.Now().UTC()
	for id, record := range ownership.ManagedComponents {
		item, exists := result.Items[id]
		if !exists {
			continue
		}
		healthy := item.Installed && !item.Broken && !item.Incompatible
		if record.Healthy != healthy || record.Version != item.Version || record.LastVerified.IsZero() || now.Sub(record.LastVerified) >= time.Minute {
			record.Healthy = healthy
			record.Version = item.Version
			record.LastVerified = now
			ownership.ManagedComponents[id] = record
			changed = true
		}
	}
	if changed {
		return s.Store.SaveOwnership(ownership)
	}
	return nil
}

func (s *Service) applyManagedRouterHealth(result *model.Inventory, ownership state.Ownership) {
	config, err := mcp.LoadRouterConfig(s.Paths.RouterConfig)
	for id, record := range ownership.ManagedComponents {
		if record.Source != "agentstack-router" {
			continue
		}
		item, exists := result.Items[id]
		if !exists {
			continue
		}
		if err == nil {
			if _, configured := config.Servers[id]; configured {
				item.Installed = true
				item.Broken = false
				item.HealthMessage = "managed router entry is present"
			} else {
				item.Installed = false
				item.Broken = true
				item.HealthMessage = "managed router entry is absent from the active router configuration"
			}
		} else {
			item.Installed = false
			item.Broken = true
			item.HealthMessage = "managed router configuration is missing or invalid"
		}
		result.Items[id] = item
	}
}

func (s *Service) Plan(ctx context.Context, request planner.Request) (model.Plan, error) {
	current, err := s.Inventory(ctx)
	if err != nil {
		return model.Plan{}, err
	}
	plan, err := planner.Build(s.Catalog, current, request)
	if err != nil {
		return model.Plan{}, err
	}
	plan, err = planner.Seal(s.Catalog, current, plan, s.PlanTTL)
	if err != nil {
		return model.Plan{}, err
	}
	if err := s.Store.SavePlan(state.SavedPlan{Plan: plan, Request: request, CreatedAt: time.Now().UTC()}); err != nil {
		return model.Plan{}, err
	}
	_ = s.logEvent(state.Event{Level: "info", Type: "plan.created", CorrelationID: plan.ID, Fields: map[string]any{"profile": plan.Profile, "actions": len(plan.Actions), "expiresAt": plan.ExpiresAt}})
	return plan, nil
}

func (s *Service) ApplyPlanned(ctx context.Context, planID, digest string, confirmed bool) (ApplyReport, error) {
	return s.ApplyPlannedWithProgress(ctx, planID, digest, confirmed, nil)
}

func (s *Service) ApplyPlannedWithProgress(ctx context.Context, planID, digest string, confirmed bool, onProgress func(ApplyProgress)) (ApplyReport, error) {
	var tracker *applyProgressTracker
	emit := func(progress ApplyProgress) {
		if onProgress != nil {
			onProgress(progress)
		}
	}
	execution, err := (reviewedplan.Executor{
		Catalog:                  s.Catalog,
		Store:                    s.Store,
		Installer:                s.Installer,
		Inventory:                s.Inventory,
		RecordSuccessfulInstalls: s.recordSuccessfulInstalls,
		LogEvent: func(event state.Event) {
			_ = s.logEvent(event)
		},
		OnPlanReady: func(plan model.Plan) error {
			tracker = newApplyProgressTracker(plan)
			emit(tracker.snapshot("preparing", ""))
			return nil
		},
		OnActionStart: func(action model.PlanAction) error {
			if tracker != nil {
				emit(tracker.start(action))
			}
			return nil
		},
		OnTransaction: func(transaction model.Transaction) error {
			if tracker != nil {
				emit(tracker.updateTransaction(transaction))
			}
			return nil
		},
	}).Execute(ctx, reviewedplan.Request{PlanID: planID, Digest: digest, Confirmed: confirmed})
	report := ApplyReport{Plan: execution.Saved.Plan, Transaction: execution.Transaction}
	if err != nil {
		if tracker != nil {
			emit(tracker.complete())
		}
		if execution.Transaction.Status == model.TransactionFailed {
			_ = s.logEvent(state.Event{Level: "error", Type: "apply.failed", CorrelationID: planID, Fields: map[string]any{"transactionId": execution.Transaction.ID, "status": execution.Transaction.Status}})
		}
		return report, err
	}
	plan := execution.Saved.Plan
	transaction := execution.Transaction
	if planHasRouterActions(s.Catalog, plan) {
		if tracker != nil {
			emit(tracker.startRouter())
		}
		routerReport, routerErr := s.configureRouter(ctx, plan, MCPInitOptions{Request: execution.Saved.Request, RegisterClients: true, Warm: true})
		report.Router = &routerReport
		if tracker != nil {
			emit(tracker.finishRouter(routerErr))
		}
		if routerErr != nil {
			transaction.Status = model.TransactionPartial
			transaction.FinishedAt = time.Now().UTC()
			report.Transaction = transaction
			if saveErr := s.Store.SaveTransaction(transaction); saveErr != nil {
				if tracker != nil {
					emit(tracker.complete())
				}
				return report, fmt.Errorf("router configuration failed after installation (%v), and partial transaction persistence failed: %w", routerErr, saveErr)
			}
			_ = s.logEvent(state.Event{Level: "error", Type: "apply.partial", CorrelationID: plan.ID, Message: routerErr.Error(), Fields: map[string]any{"transactionId": transaction.ID, "status": transaction.Status}})
			if tracker != nil {
				emit(tracker.complete())
			}
			return report, routerErr
		}
	}
	if tracker != nil {
		emit(tracker.verifying())
		emit(tracker.complete())
	}
	_ = s.logEvent(state.Event{Level: "info", Type: "apply.completed", CorrelationID: plan.ID, Fields: map[string]any{"transactionId": transaction.ID, "status": transaction.Status}})
	return report, nil
}

func (s *Service) recordSuccessfulInstalls(plan model.Plan, transaction model.Transaction) error {
	ownership, err := s.Store.LoadOwnership()
	if err != nil {
		return err
	}
	if ownership.ManagedComponents == nil {
		ownership.ManagedComponents = map[string]state.ManagedComponent{}
	}
	for _, action := range transaction.Actions {
		if (action.Kind != model.ActionInstall && action.Kind != model.ActionRepair) || action.Error != "" {
			continue
		}
		record := state.ManagedComponent{ID: action.ComponentID, Source: "agentstack", InstalledAt: time.Now().UTC(), LastVerified: time.Now().UTC(), Active: true, Healthy: action.Verified}
		if prior, exists := ownership.ManagedComponents[action.ComponentID]; exists {
			record.Paths = append(record.Paths, prior.Paths...)
		}
		component, exists := s.Catalog.ComponentByID(action.ComponentID)
		if exists {
			record.InstallKind = component.Install.Kind
			record.Package = component.Install.Package
			record.WingetID = component.Install.WingetID
			record.PackageSource = component.Install.Source
			record.Version = component.Install.Version
			record.ManifestDigest = component.Install.ManifestDigest
		}
		if exists && component.Install.Kind == model.InstallSkillPack && action.Output != "" {
			var report skills.Report
			if json.Unmarshal([]byte(action.Output), &report) == nil {
				record.Paths = appendUnique(record.Paths, report.Added...)
			}
		}
		ownership.ManagedComponents[action.ComponentID] = record
	}
	return s.Store.SaveOwnership(ownership)
}

func (s *Service) applyManagedPathHealth(result *model.Inventory, ownership state.Ownership) {
	for id, managed := range ownership.ManagedComponents {
		if len(managed.Paths) == 0 {
			continue
		}
		item, exists := result.Items[id]
		if !exists {
			continue
		}
		missing := 0
		present := 0
		for _, path := range managed.Paths {
			if _, err := os.Stat(path); err == nil {
				present++
			} else if errors.Is(err, os.ErrNotExist) {
				missing++
			} else {
				missing++
			}
		}
		if missing > 0 {
			item.Broken = true
			item.Installed = present > 0
			item.HealthMessage = fmt.Sprintf("%d of %d AgentStack-owned paths are missing", missing, len(managed.Paths))
			result.Items[id] = item
		}
	}
}

func appendUnique(existing []string, values ...string) []string {
	seen := make(map[string]bool, len(existing)+len(values))
	result := make([]string, 0, len(existing)+len(values))
	for _, value := range append(append([]string(nil), existing...), values...) {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (s *Service) MCPInit(ctx context.Context, options MCPInitOptions) (MCPInitReport, error) {
	if !options.Confirm {
		return MCPInitReport{}, ErrConfirmationRequired
	}
	lease, err := s.Store.AcquireLease("mutation", 6*time.Hour)
	if err != nil {
		return MCPInitReport{}, err
	}
	defer lease.Close()
	plan, err := s.Plan(ctx, options.Request)
	if err != nil {
		return MCPInitReport{}, err
	}
	if err := s.Store.DeletePlan(plan.ID); err != nil {
		return MCPInitReport{}, fmt.Errorf("consume transient MCP plan before mutation: %w", err)
	}
	return s.configureRouter(ctx, plan, options)
}

func (s *Service) configureRouter(ctx context.Context, plan model.Plan, options MCPInitOptions) (MCPInitReport, error) {
	config, err := mcp.BuildRouterConfig(s.Catalog, plan, s.Paths.DataRoot)
	if err != nil {
		return MCPInitReport{}, err
	}
	report := MCPInitReport{RouterConfigPath: s.Paths.RouterConfig, Servers: mcp.SortedServerNames(config)}
	if err := os.MkdirAll(filepath.Join(s.Paths.DataRoot, "memory"), 0o700); err != nil {
		return report, err
	}
	existing, loadErr := mcp.LoadRouterConfig(s.Paths.RouterConfig)
	switch {
	case loadErr == nil && mcp.RouterConfigEquivalent(existing, config):
		// Keep an equivalent managed configuration byte-for-byte so session
		// initialization does not create backup churn or needless writes.
	case loadErr == nil:
		backup, backupErr := s.Store.BackupFile(s.Paths.RouterConfig, "router")
		if backupErr != nil {
			return report, backupErr
		}
		report.BackupPath = backup
		if err := mcp.WriteRouterConfig(s.Paths.RouterConfig, config); err != nil {
			return report, err
		}
		report.RouterChanged = true
	case errors.Is(loadErr, os.ErrNotExist):
		if err := mcp.WriteRouterConfig(s.Paths.RouterConfig, config); err != nil {
			return report, err
		}
		report.RouterChanged = true
	default:
		return report, fmt.Errorf("read existing managed router config without replacing it: %w", loadErr)
	}
	if err := s.recordConfiguredRouters(config); err != nil {
		return report, err
	}
	s.invalidateDoctorCache()
	if options.Warm {
		report.Warm = s.warmServers(ctx, config)
	}
	if options.RegisterClients {
		if s.commandExists("codex") {
			value, registerErr := mcp.RegisterCodex(ctx, s.commandRunner(), s.Paths.Executable, s.Paths.RouterConfig)
			report.Codex = &value
			if registerErr != nil {
				return report, registerErr
			}
		}
		if s.commandExists("agy") {
			value, mergeErr := mcp.MergeAgyConfig(s.Paths.AgyConfig, s.Paths.Executable, []string{"mcp-router", "--config", s.Paths.RouterConfig}, s.Paths.BackupRoot)
			report.Agy = &value
			if mergeErr != nil {
				return report, mergeErr
			}
		}
	}
	for _, warm := range report.Warm {
		if warm.Status == "failed" {
			return report, fmt.Errorf("MCP warm-up failed for %s: %s", warm.Server, warm.Message)
		}
	}
	return report, nil
}

func (s *Service) recordConfiguredRouters(config mcp.RouterConfig) error {
	ownership, err := s.Store.LoadOwnership()
	if err != nil {
		return err
	}
	if ownership.ManagedComponents == nil {
		ownership.ManagedComponents = map[string]state.ManagedComponent{}
	}
	for id := range config.Servers {
		component, _ := s.Catalog.ComponentByID(id)
		ownership.ManagedComponents[id] = state.ManagedComponent{ID: id, Source: "agentstack-router", InstalledAt: time.Now().UTC(), LastVerified: time.Now().UTC(), Active: true, Healthy: true, InstallKind: model.InstallRouter, Package: component.Install.Package, Version: component.Install.Version}
	}
	for id, record := range ownership.ManagedComponents {
		if record.Source == "agentstack-router" {
			if _, active := config.Servers[id]; !active {
				record.Active = false
				ownership.ManagedComponents[id] = record
			}
		}
	}
	return s.Store.SaveOwnership(ownership)
}

func (s *Service) warmServers(ctx context.Context, config mcp.RouterConfig) []WarmResult {
	results := make([]WarmResult, 0, len(config.Servers))
	commands := s.commandRunner()
	for _, name := range mcp.SortedServerNames(config) {
		server := config.Servers[name]
		if server.Warm == nil {
			results = append(results, WarmResult{Server: name, Status: "skipped", Message: "no warm-up command"})
			continue
		}
		invocation := runner.Invocation{Command: server.Warm.Command, Args: append([]string(nil), server.Warm.Args...), Env: server.Warm.Env}
		outcome := commands.Run(ctx, invocation)
		item := WarmResult{Server: name, Command: invocation.Command, ExitCode: outcome.ExitCode, Status: "ok"}
		if outcome.Err != nil || outcome.ExitCode != 0 {
			item.Status = "failed"
			item.Message = resultError(outcome)
		}
		results = append(results, item)
	}
	return results
}

func (s *Service) MCPDoctor(ctx context.Context) (DoctorReport, error) {
	s.doctorMu.Lock()
	if s.DoctorTTL > 0 && !s.doctorCacheAt.IsZero() && time.Since(s.doctorCacheAt) < s.DoctorTTL {
		cached := s.doctorCached
		cached.Cached = true
		s.doctorMu.Unlock()
		return cached, nil
	}
	s.doctorMu.Unlock()
	started := time.Now()
	config, err := mcp.LoadRouterConfig(s.Paths.RouterConfig)
	if err != nil {
		return DoctorReport{Healthy: false, Config: s.Paths.RouterConfig, CheckedAt: time.Now().UTC(), Duration: time.Since(started)}, err
	}
	child := mcp.NewManagedChildRuntime(mcp.ChildRuntimeOptions{Observer: s.MCPChildObserver()})
	defer child.Close()
	report := DoctorReport{Healthy: true, Config: s.Paths.RouterConfig, Servers: map[string]mcp.DoctorItem{}, CheckedAt: time.Now().UTC()}
	for _, name := range mcp.SortedServerNames(config) {
		item := child.Doctor(ctx, config.Servers[name])
		report.Servers[name] = item
		if item.Status != "ok" {
			report.Healthy = false
			report.UnhealthyCount++
		} else {
			report.HealthyCount++
		}
	}
	report.Duration = time.Since(started)
	s.doctorMu.Lock()
	s.doctorCached = report
	s.doctorCacheAt = time.Now()
	s.doctorMu.Unlock()
	_ = s.logEvent(state.Event{Level: map[bool]string{true: "info", false: "error"}[report.Healthy], Type: "mcp.doctor", Fields: map[string]any{"healthy": report.Healthy, "healthyCount": report.HealthyCount, "unhealthyCount": report.UnhealthyCount, "durationMs": report.Duration.Milliseconds()}})
	return report, nil
}

func (s *Service) invalidateDoctorCache() {
	s.doctorMu.Lock()
	s.doctorCacheAt = time.Time{}
	s.doctorCached = DoctorReport{}
	s.doctorMu.Unlock()
}

func (s *Service) commandRunner() runner.CommandRunner {
	if s.Commands != nil {
		return s.Commands
	}
	return runner.ExecRunner{}
}

func (s *Service) commandExists(name string) bool {
	if s.LookPath == nil {
		s.LookPath = defaultLocator{}
	}
	_, err := s.LookPath.LookPath(name)
	return err == nil
}

func planHasRouterActions(c model.Catalog, plan model.Plan) bool {
	for _, action := range plan.Actions {
		component, ok := c.ComponentByID(action.ComponentID)
		if ok && component.Install.Kind == model.InstallRouter && (action.Kind == model.ActionConfigure || action.Kind == model.ActionKeep) {
			return true
		}
	}
	return false
}

func resultError(result runner.Result) string {
	if result.Err != nil {
		if result.Stderr != "" {
			return result.Err.Error() + ": " + result.Stderr
		}
		return result.Err.Error()
	}
	return result.Stderr
}

func (s *Service) logEvent(event state.Event) error {
	return s.Store.AppendEvent(event)
}

func (s *Service) MCPChildObserver() mcp.ChildObserver {
	return func(event mcp.ChildEvent) {
		level := "info"
		if event.Status != "ok" {
			level = "error"
		}
		_ = s.logEvent(state.Event{
			Level:         level,
			Type:          "mcp." + event.Type,
			CorrelationID: event.ServerKey,
			Message:       event.Message,
			Fields: map[string]any{
				"command":    event.Command,
				"status":     event.Status,
				"method":     event.Method,
				"durationMs": event.Duration.Milliseconds(),
			},
		})
	}
}

func (s *Service) Backups() ([]state.BackupRecord, error) {
	return s.Store.ListBackups()
}

func (s *Service) PreviewRestore(id, target string) (RestorePreview, error) {
	record, resolvedTarget, err := s.Store.ResolveBackup(id, target)
	if err != nil {
		return RestorePreview{}, err
	}
	validated, validation, err := s.validateRestoreCandidate(resolvedTarget, record.Path)
	preview := RestorePreview{
		Backup:              record,
		Target:              resolvedTarget,
		DigestVerified:      true,
		StructuralValidated: validated,
		Validation:          validation,
	}
	return preview, err
}

func (s *Service) RestoreBackup(ctx context.Context, id, target string, confirmed bool) (RestoreReport, error) {
	if !confirmed {
		return RestoreReport{}, ErrConfirmationRequired
	}
	preview, err := s.PreviewRestore(id, target)
	if err != nil {
		return RestoreReport{}, err
	}
	lease, err := s.Store.AcquireLease("mutation", 30*time.Minute)
	if err != nil {
		return RestoreReport{}, err
	}
	defer lease.Close()
	rollbackPath, targetExisted, err := stageRestoreRollback(preview.Target)
	if err != nil {
		return RestoreReport{}, err
	}
	if rollbackPath != "" {
		defer os.Remove(rollbackPath)
	}
	record, err := s.Store.RestoreBackup(id, target)
	report := RestoreReport{Backup: record, StructuralValidated: preview.StructuralValidated}
	if err == nil {
		report.Validated, report.LiveValidated, report.Validation, report.Doctor, err = s.validateRestoredTarget(ctx, record.Source)
	}
	if err != nil && record.Source != "" {
		rollbackErr := restorePreviousTarget(record.Source, rollbackPath, targetExisted)
		if rollbackErr == nil {
			report.RolledBack = true
			report.Rollback = "previous target restored after validation failure"
		} else {
			report.Rollback = "automatic rollback failed: " + rollbackErr.Error()
			err = fmt.Errorf("%w; %s", err, report.Rollback)
		}
	}
	level := "info"
	if err != nil {
		level = "error"
	}
	_ = s.logEvent(state.Event{Level: level, Type: "backup.restore", CorrelationID: id, Fields: map[string]any{"target": filepath.Base(record.Source), "success": err == nil, "validated": report.Validated, "structuralValidated": report.StructuralValidated, "liveValidated": report.LiveValidated, "validation": report.Validation}})
	return report, err
}

func stageRestoreRollback(target string) (path string, existed bool, err error) {
	input, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return "", false, err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".agentstack-restore-rollback-*.tmp")
	if err != nil {
		return "", false, err
	}
	name := temp.Name()
	cleanup := func(cause error) (string, bool, error) {
		_ = temp.Close()
		_ = os.Remove(name)
		return "", false, cause
	}
	if _, err := io.Copy(temp, input); err != nil {
		return cleanup(err)
	}
	if err := temp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return cleanup(err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return "", false, err
	}
	return name, true, nil
}

func restorePreviousTarget(target, rollbackPath string, existed bool) error {
	if !existed {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if rollbackPath == "" {
		return fmt.Errorf("rollback copy is unavailable")
	}
	return safefile.Replace(rollbackPath, target)
}

func (s *Service) validateRestoreCandidate(target, candidate string) (bool, string, error) {
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return false, "restore target path is invalid", err
	}
	routerPath, err := filepath.Abs(s.Paths.RouterConfig)
	if err != nil {
		return false, "router configuration path is invalid", err
	}
	agyPath, err := filepath.Abs(s.Paths.AgyConfig)
	if err != nil {
		return false, "AGY configuration path is invalid", err
	}
	switch cleanTarget {
	case routerPath:
		if _, err := mcp.LoadRouterConfig(candidate); err != nil {
			return false, "router JSON is invalid", err
		}
		return true, "router configuration parsed successfully", nil
	case agyPath:
		data, err := os.ReadFile(candidate)
		if err != nil {
			return false, "AGY configuration could not be read", err
		}
		var document map[string]any
		if err := json.Unmarshal(data, &document); err != nil {
			return false, "AGY configuration is invalid JSON", err
		}
		if servers, ok := document["mcpServers"]; !ok || servers == nil {
			return false, "AGY configuration lacks mcpServers", fmt.Errorf("restored AGY configuration lacks mcpServers")
		}
		return true, "AGY configuration parsed and contains mcpServers", nil
	default:
		return true, "backup digest and candidate bytes verified", nil
	}
}

func (s *Service) validateRestoredTarget(ctx context.Context, target string) (validated, liveValidated bool, message string, doctor *DoctorReport, err error) {
	structural, structuralMessage, structuralErr := s.validateRestoreCandidate(target, target)
	if structuralErr != nil {
		return false, false, structuralMessage, nil, structuralErr
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return false, false, "restored target path is invalid", nil, err
	}
	routerPath, err := filepath.Abs(s.Paths.RouterConfig)
	if err != nil {
		return false, false, "router configuration path is invalid", nil, err
	}
	if cleanTarget != routerPath {
		return structural, false, structuralMessage, nil, nil
	}
	s.invalidateDoctorCache()
	report, doctorErr := s.MCPDoctor(ctx)
	if doctorErr != nil {
		return false, false, "router restored but live MCP validation failed: " + doctorErr.Error(), nil, doctorErr
	}
	if !report.Healthy {
		return false, false, fmt.Sprintf("router restored but %d child MCP server(s) failed live validation", report.UnhealthyCount), &report, fmt.Errorf("restored router failed live MCP validation")
	}
	return true, true, fmt.Sprintf("router parsed and %d child MCP server(s) completed initialize and tools/list", report.HealthyCount), &report, nil
}

func (s *Service) ExportData(destination string) error {
	lease, err := s.Store.AcquireLease("data-export", 30*time.Minute)
	if err != nil {
		return err
	}
	defer lease.Close()
	err = s.Store.ExportData(destination)
	_ = s.logEvent(state.Event{Level: eventLevel(err), Type: "privacy.export", Fields: map[string]any{"destination": filepath.Base(destination), "success": err == nil}})
	return err
}

func (s *Service) ClearData(scope state.ClearScope, confirmed bool) ([]string, error) {
	if !confirmed {
		return nil, ErrConfirmationRequired
	}
	lease, err := s.Store.AcquireLease("mutation", 30*time.Minute)
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	removed, err := s.Store.ClearData(scope)
	if scope != state.ClearAll {
		_ = s.logEvent(state.Event{Level: eventLevel(err), Type: "privacy.clear", Fields: map[string]any{"scope": scope, "removed": len(removed), "success": err == nil}})
	}
	return removed, err
}

func (s *Service) CreateDiagnostics(ctx context.Context, destination, version string) error {
	current, err := s.Inventory(ctx)
	if err != nil {
		return err
	}
	doctor, doctorErr := s.MCPDoctor(ctx)
	events, eventsErr := s.Store.RecentEvents(500)
	if eventsErr != nil {
		return eventsErr
	}
	catalogDigest, err := integrity.DigestJSON(s.Catalog)
	if err != nil {
		return err
	}
	err = diagnostics.Create(diagnostics.Input{Destination: destination, Version: version, CatalogDigest: catalogDigest, Inventory: current, Events: events, Healthy: doctorErr == nil && doctor.Healthy})
	_ = s.logEvent(state.Event{Level: eventLevel(err), Type: "diagnostics.created", Fields: map[string]any{"destination": filepath.Base(destination), "success": err == nil}})
	return err
}

func eventLevel(err error) string {
	if err != nil {
		return "error"
	}
	return "info"
}

func (s *Service) CatalogSnapshot() model.Catalog {
	return s.Catalog
}
