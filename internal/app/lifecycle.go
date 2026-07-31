package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/mcp"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/runner"
	"github.com/agentstack/agentstack/internal/skills"
	"github.com/agentstack/agentstack/internal/state"
)

type OwnedAction struct {
	ComponentID string                 `json:"componentId"`
	Operation   string                 `json:"operation"`
	Supported   bool                   `json:"supported"`
	Reason      string                 `json:"reason,omitempty"`
	Command     string                 `json:"command,omitempty"`
	Args        []string               `json:"args,omitempty"`
	Quarantine  []string               `json:"quarantine,omitempty"`
	Status      string                 `json:"status"`
	Error       string                 `json:"error,omitempty"`
	Ownership   state.ManagedComponent `json:"ownership"`
}

type OwnedReport struct {
	Preview   bool          `json:"preview"`
	Operation string        `json:"operation"`
	Actions   []OwnedAction `json:"actions"`
}

func (s *Service) OwnedPreview(ids []string, operation string) (OwnedReport, error) {
	ownership, err := s.Store.LoadOwnership()
	if err != nil {
		return OwnedReport{}, err
	}
	selected := normalizeOwnedIDs(ids, ownership)
	report := OwnedReport{Preview: true, Operation: operation}
	for _, id := range selected {
		record, ok := ownership.ManagedComponents[id]
		if !ok {
			report.Actions = append(report.Actions, OwnedAction{ComponentID: id, Operation: operation, Status: "not-owned", Reason: "component is not recorded as AgentStack-owned"})
			continue
		}
		action := s.ownedAction(record, operation)
		report.Actions = append(report.Actions, action)
	}
	return report, nil
}

func normalizeOwnedIDs(ids []string, ownership state.Ownership) []string {
	seen := map[string]bool{}
	result := []string{}
	if len(ids) == 0 {
		for id := range ownership.ManagedComponents {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func (s *Service) ownedAction(record state.ManagedComponent, operation string) OwnedAction {
	action := OwnedAction{ComponentID: record.ID, Operation: operation, Ownership: record, Status: "planned"}
	if record.Source != "agentstack" && record.Source != "agentstack-router" {
		action.Reason = "ownership source is not eligible for AgentStack lifecycle management"
		return action
	}
	if operation == "deactivate" {
		action.Supported = true
		action.Reason = "deactivate managed routing/exposure without uninstalling third-party software"
		return action
	}
	if operation != "remove" {
		action.Reason = "unknown lifecycle operation"
		return action
	}
	if record.Source == "agentstack-router" || record.InstallKind == model.InstallRouter {
		action.Supported = true
		action.Reason = "remove only the AgentStack-managed router entry"
		return action
	}
	switch record.InstallKind {
	case model.InstallWinget:
		if record.WingetID == "" {
			action.Reason = "missing recorded WinGet package identity"
			return action
		}
		action.Supported = true
		action.Command = "winget"
		action.Args = []string{"uninstall", "--id", record.WingetID, "--exact", "--silent", "--disable-interactivity"}
		if record.PackageSource != "" {
			action.Args = append(action.Args, "--source", record.PackageSource)
		}
	case model.InstallNPMGlobal:
		name := npmPackageName(record.Package)
		if name == "" {
			action.Reason = "missing recorded npm package identity"
			return action
		}
		action.Supported = true
		action.Command = "npm"
		action.Args = []string{"uninstall", "--global", name}
	case model.InstallUVTool:
		name := uvToolName(record.Package)
		if name == "" {
			action.Reason = "missing recorded uv tool identity"
			return action
		}
		action.Supported = true
		action.Command = "uv"
		action.Args = []string{"tool", "uninstall", name}
	case model.InstallSkillPack:
		if len(record.Paths) == 0 {
			action.Reason = "no AgentStack-owned skill paths were recorded"
			return action
		}
		action.Supported = true
		action.Reason = "move AgentStack-owned skill paths to recoverable quarantine"
	case model.InstallManual, model.InstallNone, "":
		action.Reason = "manual or unclassified installations are never removed automatically"
	default:
		action.Reason = fmt.Sprintf("unsupported install kind %q", record.InstallKind)
	}
	return action
}

func (s *Service) DeactivateOwned(ctx context.Context, ids []string, confirmed bool) (OwnedReport, error) {
	if !confirmed {
		return OwnedReport{}, ErrConfirmationRequired
	}
	return s.applyOwned(ctx, ids, "deactivate")
}

func (s *Service) RemoveOwned(ctx context.Context, ids []string, confirmed bool) (OwnedReport, error) {
	if !confirmed {
		return OwnedReport{}, ErrConfirmationRequired
	}
	return s.applyOwned(ctx, ids, "remove")
}

func (s *Service) applyOwned(ctx context.Context, ids []string, operation string) (OwnedReport, error) {
	lease, err := s.Store.AcquireLease("mutation", 2*time.Hour)
	if err != nil {
		return OwnedReport{}, err
	}
	defer lease.Close()
	ownership, err := s.Store.LoadOwnership()
	if err != nil {
		return OwnedReport{}, err
	}
	selected := normalizeOwnedIDs(ids, ownership)
	report := OwnedReport{Preview: false, Operation: operation}
	var firstErr error
	for _, id := range selected {
		if err := lease.Touch(); err != nil {
			firstErr = fmt.Errorf("refresh mutation lease before %s: %w", id, err)
			break
		}
		record, ok := ownership.ManagedComponents[id]
		if !ok {
			report.Actions = append(report.Actions, OwnedAction{ComponentID: id, Operation: operation, Status: "not-owned", Reason: "component is not recorded as AgentStack-owned"})
			continue
		}
		action := s.ownedAction(record, operation)
		if !action.Supported {
			action.Status = "refused"
			report.Actions = append(report.Actions, action)
			continue
		}
		var actionErr error
		if operation == "deactivate" {
			actionErr = s.deactivateRecord(id, record, &ownership)
		} else {
			actionErr = s.removeRecord(ctx, id, record, &ownership, &action)
		}
		if actionErr != nil {
			action.Status = "failed"
			action.Error = actionErr.Error()
			if firstErr == nil {
				firstErr = actionErr
			}
		} else {
			action.Status = "succeeded"
			if err := s.Store.SaveOwnership(ownership); err != nil {
				action.Status = "failed"
				action.Error = "operation completed but ownership checkpoint failed: " + err.Error()
				if firstErr == nil {
					firstErr = fmt.Errorf("checkpoint ownership after %s: %w", id, err)
				}
			}
		}
		report.Actions = append(report.Actions, action)
		if firstErr != nil {
			break
		}
		if err := lease.Touch(); err != nil {
			firstErr = fmt.Errorf("refresh mutation lease after %s: %w", id, err)
			break
		}
	}
	_ = s.logEvent(state.Event{Level: eventLevel(firstErr), Type: "ownership." + operation, Fields: map[string]any{"components": len(selected), "success": firstErr == nil}})
	return report, firstErr
}

func (s *Service) deactivateRecord(id string, record state.ManagedComponent, ownership *state.Ownership) error {
	if record.Source == "agentstack-router" || record.InstallKind == model.InstallRouter {
		if err := s.removeRouterEntry(id); err != nil {
			return err
		}
	}
	record.Active = false
	record.LastVerified = time.Now().UTC()
	ownership.ManagedComponents[id] = record
	return nil
}

func (s *Service) removeRecord(ctx context.Context, id string, record state.ManagedComponent, ownership *state.Ownership, action *OwnedAction) error {
	if record.Source == "agentstack-router" || record.InstallKind == model.InstallRouter {
		if err := s.removeRouterEntry(id); err != nil {
			return err
		}
		delete(ownership.ManagedComponents, id)
		return nil
	}
	if record.InstallKind == model.InstallSkillPack {
		paths, err := s.quarantineOwnedPaths(id, record.Paths)
		action.Quarantine = paths
		if err != nil {
			return err
		}
		delete(ownership.ManagedComponents, id)
		return nil
	}
	planned := s.ownedAction(record, "remove")
	result := s.commandRunner().Run(ctx, runner.Invocation{Command: planned.Command, Args: planned.Args, Timeout: 20 * time.Minute, MaxOutputBytes: 1 << 20})
	if result.Err != nil || result.ExitCode != 0 {
		return fmt.Errorf("remove %s: %s", id, resultError(result))
	}
	delete(ownership.ManagedComponents, id)
	return nil
}

func (s *Service) removeRouterEntry(id string) error {
	config, err := mcp.LoadRouterConfig(s.Paths.RouterConfig)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, exists := config.Servers[id]; !exists {
		return nil
	}
	if _, err := s.Store.BackupFile(s.Paths.RouterConfig, "router-before-deactivate"); err != nil {
		return err
	}
	delete(config.Servers, id)
	return mcp.WriteRouterConfig(s.Paths.RouterConfig, config)
}

func (s *Service) quarantineOwnedPaths(id string, paths []string) ([]string, error) {
	root := filepath.Join(s.Paths.DataRoot, "quarantine", time.Now().UTC().Format("20060102T150405Z"), safeComponentName(id))
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	var moved []string
	for index, source := range paths {
		validated, err := s.validateOwnedSkillPath(source)
		if err != nil {
			return moved, err
		}
		info, err := os.Lstat(validated)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return moved, err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return moved, fmt.Errorf("refuse unsafe owned skill path %q: expected a real directory", validated)
		}
		name := filepath.Base(validated)
		if name == "." || name == string(filepath.Separator) || name == "" {
			name = fmt.Sprintf("item-%d", index)
		}
		destination := filepath.Join(root, name)
		if err := os.Rename(validated, destination); err != nil {
			return moved, fmt.Errorf("atomically quarantine %q: %w; source was preserved", validated, err)
		}
		moved = append(moved, destination)
	}
	return moved, nil
}

func (s *Service) validateOwnedSkillPath(source string) (string, error) {
	roots := s.SkillRoots
	if len(roots) == 0 {
		roots = skills.DefaultTargets()
	}
	cleanSource, err := filepath.Abs(source)
	if err != nil {
		return "", fmt.Errorf("resolve owned skill path %q: %w", source, err)
	}
	for _, root := range roots {
		cleanRoot, rootErr := filepath.Abs(root)
		if rootErr != nil {
			continue
		}
		if evaluated, evalErr := filepath.EvalSymlinks(cleanRoot); evalErr == nil {
			cleanRoot = evaluated
		}
		candidate := cleanSource
		if resolved, resolveErr := filepath.EvalSymlinks(cleanSource); resolveErr == nil {
			candidate = resolved
		} else if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", fmt.Errorf("resolve owned skill path %q: %w", source, resolveErr)
		}
		relative, relErr := filepath.Rel(cleanRoot, candidate)
		if relErr != nil || relative == "." || relative == "" || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if strings.Contains(relative, string(filepath.Separator)) {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("refuse owned skill path %q: it is not an immediate child of a recognized AgentStack skill root", source)
}

func npmPackageName(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "@") {
		if slash := strings.Index(value, "/"); slash >= 0 {
			if at := strings.LastIndex(value, "@"); at > slash {
				return value[:at]
			}
		}
		return value
	}
	if at := strings.LastIndex(value, "@"); at > 0 {
		return value[:at]
	}
	return value
}

func uvToolName(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, "["); index >= 0 {
		value = value[:index]
	}
	if index := strings.Index(value, "=="); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func safeComponentName(value string) string {
	value = strings.NewReplacer("/", "-", "\\", "-", "..", "-").Replace(value)
	return strings.Trim(value, ".- ")
}
