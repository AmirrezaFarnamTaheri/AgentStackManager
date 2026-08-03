package resourcehub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

var ErrConfirmationRequired = errors.New("resource synchronization requires explicit confirmation")

func (m Manager) PlanSync(targetID string, resourceIDs []string, options PlanOptions) (SyncPlan, error) {
	registry, err := m.LoadRegistry()
	if err != nil {
		return SyncPlan{}, err
	}
	target, ok := registry.Targets[targetID]
	if !ok {
		return SyncPlan{}, fmt.Errorf("unknown target %q", targetID)
	}
	if !target.Enabled {
		return SyncPlan{}, fmt.Errorf("target %q is disabled", targetID)
	}
	if options.TTL <= 0 {
		options.TTL = 15 * time.Minute
	}
	ids := uniqueStrings(resourceIDs)
	if len(ids) == 0 {
		for id, resource := range registry.Resources {
			if resource.Enabled && supportsAgent(resource, target.Agent) {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
	}
	state, err := m.loadManagedState(targetID)
	if err != nil {
		return SyncPlan{}, err
	}
	operations := make([]SyncOperation, 0, len(ids)+len(state.Entries))
	selectedDestinations := map[string]struct{}{}
	for _, id := range ids {
		resource, exists := registry.Resources[id]
		if !exists {
			return SyncPlan{}, fmt.Errorf("unknown resource %q", id)
		}
		if !resource.Enabled {
			continue
		}
		if !supportsAgent(resource, target.Agent) {
			continue
		}
		if !options.AllowRisk {
			audit, err := m.Audit(id)
			if err != nil {
				return SyncPlan{}, err
			}
			if audit.Blocked {
				return SyncPlan{}, fmt.Errorf("resource %q is blocked by security audit", id)
			}
		}
		destination, err := targetDestination(target, resource)
		if err != nil {
			return SyncPlan{}, err
		}
		selectedDestinations[destination] = struct{}{}
		sourcePath := m.resourceSource(resource)
		op := SyncOperation{ResourceID: id, Kind: resource.Kind, Source: sourcePath, Destination: destination, DesiredDigest: resource.Digest}
		current, statErr := treeDigest(destination)
		managed, wasManaged := state.Entries[destination]
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			op.Action, op.Reason = ActionCreate, "destination is absent"
		case statErr != nil:
			return SyncPlan{}, statErr
		case effectiveTargetMode(target) == ModeLink && wasManaged && managed.ResourceID == id && linkDestinationMatches(sourcePath, destination):
			op.Action, op.CurrentDigest, op.Reason = ActionNoop, current, "managed link still points to the canonical resource"
		case current == resource.Digest:
			op.Action, op.CurrentDigest, op.Reason = ActionNoop, current, "destination already matches resource digest"
		case wasManaged && managed.ResourceID == id && current == managed.Digest:
			op.Action, op.CurrentDigest, op.Reason = ActionUpdate, current, "managed destination is unchanged and the canonical resource advanced"
		case wasManaged && managed.ResourceID == id:
			op.Action, op.CurrentDigest, op.Reason = ActionConflict, current, "managed destination changed outside AgentStack and will not be overwritten"
		default:
			op.Action, op.CurrentDigest, op.Reason = ActionConflict, current, "foreign destination differs and will not be overwritten"
		}
		operations = append(operations, op)
	}
	if options.Prune {
		for destination, managed := range state.Entries {
			if _, keep := selectedDestinations[destination]; keep {
				continue
			}
			current, err := treeDigest(destination)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return SyncPlan{}, err
			}
			action := ActionRemove
			reason := "managed resource is no longer selected"
			if current != managed.Digest {
				action = ActionConflict
				reason = "managed destination changed outside AgentStack and will not be removed"
			}
			operations = append(operations, SyncOperation{ResourceID: managed.ResourceID, Action: action, Destination: destination, CurrentDigest: current, Reason: reason})
		}
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].Destination < operations[j].Destination })
	registryDigest, err := integrity.DigestJSON(registry)
	if err != nil {
		return SyncPlan{}, err
	}
	now := m.now()
	plan := SyncPlan{ID: newID("sync", now), TargetID: targetID, GeneratedAt: now, ExpiresAt: now.Add(options.TTL), RegistryDigest: registryDigest, AllowRisk: options.AllowRisk, Prune: options.Prune, Operations: operations}
	plan.Digest, err = syncPlanDigest(plan)
	if err != nil {
		return SyncPlan{}, err
	}
	if err := writeJSON(filepath.Join(m.Root, "plans", plan.ID+".json"), plan); err != nil {
		return SyncPlan{}, err
	}
	return plan, nil
}

func (m Manager) ApplySync(planID, digest string, confirmed bool) (SyncReport, error) {
	if !confirmed {
		return SyncReport{}, ErrConfirmationRequired
	}
	if !validID(planID) {
		return SyncReport{}, fmt.Errorf("sync plan id is empty or invalid")
	}
	plan, err := m.loadPlan(planID)
	if err != nil {
		return SyncReport{}, err
	}
	if digest != plan.Digest {
		return SyncReport{}, fmt.Errorf("sync plan digest mismatch")
	}
	actual, err := syncPlanDigest(plan)
	if err != nil {
		return SyncReport{}, err
	}
	if actual != plan.Digest {
		return SyncReport{}, fmt.Errorf("stored sync plan digest is invalid")
	}
	if !m.now().Before(plan.ExpiresAt) {
		return SyncReport{}, fmt.Errorf("sync plan expired at %s", plan.ExpiresAt.Format(time.RFC3339))
	}
	registry, err := m.LoadRegistry()
	if err != nil {
		return SyncReport{}, err
	}
	registryDigest, err := integrity.DigestJSON(registry)
	if err != nil {
		return SyncReport{}, err
	}
	if registryDigest != plan.RegistryDigest {
		return SyncReport{}, fmt.Errorf("resource registry changed after plan review")
	}
	target, ok := registry.Targets[plan.TargetID]
	if !ok || !target.Enabled {
		return SyncReport{}, fmt.Errorf("target %q is unavailable", plan.TargetID)
	}
	lock, err := m.acquireLock(plan.TargetID)
	if err != nil {
		return SyncReport{}, err
	}
	defer m.releaseLock(lock)
	state, err := m.loadManagedState(plan.TargetID)
	if err != nil {
		return SyncReport{}, err
	}

	// Revalidate every reviewed input before the first filesystem mutation. This
	// prevents a late conflict from leaving an earlier operation partially applied.
	for _, operation := range plan.Operations {
		switch operation.Action {
		case ActionNoop:
			continue
		case ActionConflict:
			return SyncReport{}, fmt.Errorf("sync conflict at %s: %s", operation.Destination, operation.Reason)
		case ActionRemove:
			entry, ok := state.Entries[operation.Destination]
			if !ok {
				return SyncReport{}, fmt.Errorf("managed state missing for removal %s", operation.Destination)
			}
			current, digestErr := treeDigest(operation.Destination)
			if digestErr != nil && !errors.Is(digestErr, os.ErrNotExist) {
				return SyncReport{}, digestErr
			}
			if digestErr == nil && current != entry.Digest {
				return SyncReport{}, fmt.Errorf("managed destination changed before removal: %s", operation.Destination)
			}
		case ActionCreate, ActionUpdate:
			resource, ok := registry.Resources[operation.ResourceID]
			if !ok {
				return SyncReport{}, fmt.Errorf("resource %q disappeared", operation.ResourceID)
			}
			currentDigest, digestErr := treeDigest(m.resourceSource(resource))
			if digestErr != nil {
				return SyncReport{}, digestErr
			}
			if currentDigest != operation.DesiredDigest || currentDigest != resource.Digest {
				return SyncReport{}, fmt.Errorf("resource %q changed after plan review", resource.ID)
			}
			if operation.Action == ActionCreate {
				if _, statErr := os.Lstat(operation.Destination); !errors.Is(statErr, os.ErrNotExist) {
					if statErr == nil {
						return SyncReport{}, fmt.Errorf("destination appeared after plan review: %s", operation.Destination)
					}
					return SyncReport{}, statErr
				}
			} else if current, digestErr := treeDigest(operation.Destination); digestErr != nil || current != operation.CurrentDigest {
				return SyncReport{}, fmt.Errorf("destination changed after plan review: %s", operation.Destination)
			}
		default:
			return SyncReport{}, fmt.Errorf("unsupported sync action %q", operation.Action)
		}
	}

	type rollbackEntry struct {
		destination string
		backup      string
		existed     bool
	}
	rollbacks := make([]rollbackEntry, 0, len(plan.Operations))
	rollbackAll := func() error {
		var rollbackErr error
		for index := len(rollbacks) - 1; index >= 0; index-- {
			entry := rollbacks[index]
			rollbackErr = errors.Join(rollbackErr, removeManagedPath(entry.destination))
			if entry.existed {
				rollbackErr = errors.Join(rollbackErr, os.Rename(entry.backup, entry.destination))
			}
		}
		return rollbackErr
	}
	report := SyncReport{PlanID: plan.ID, TargetID: plan.TargetID, StartedAt: m.now()}
	for operationIndex, operation := range plan.Operations {
		if operation.Action == ActionNoop {
			report.Skipped = append(report.Skipped, operation)
			continue
		}
		if m.beforeSyncOperation != nil {
			if hookErr := m.beforeSyncOperation(operation); hookErr != nil {
				return report, errors.Join(hookErr, rollbackAll())
			}
		}

		entry := rollbackEntry{destination: operation.Destination}
		if operation.Action == ActionUpdate || operation.Action == ActionRemove {
			entry.existed = true
			entry.backup = nextSyncRollbackPath(operation.Destination, plan.ID, operationIndex)
			if err := os.Rename(operation.Destination, entry.backup); err != nil {
				return report, errors.Join(err, rollbackAll())
			}
		}
		rollbacks = append(rollbacks, entry)

		switch operation.Action {
		case ActionRemove:
			delete(state.Entries, operation.Destination)
		case ActionCreate, ActionUpdate:
			resource := registry.Resources[operation.ResourceID]
			mode := effectiveTargetMode(target)
			applyErr := error(nil)
			if mode == ModeLink {
				applyErr = linkResource(m.resourceSource(resource), operation.Destination)
			}
			if applyErr != nil || mode == ModeCopy {
				applyErr = copyResource(m.resourceSource(resource), operation.Destination)
			}
			if applyErr != nil {
				return report, errors.Join(applyErr, rollbackAll())
			}
			installedDigest, digestErr := treeDigest(operation.Destination)
			if digestErr != nil {
				return report, errors.Join(digestErr, rollbackAll())
			}
			if mode == ModeCopy && installedDigest != resource.Digest {
				return report, errors.Join(fmt.Errorf("installed resource digest mismatch for %q", resource.ID), rollbackAll())
			}
			state.Entries[operation.Destination] = managedEntry{ResourceID: resource.ID, Destination: operation.Destination, Digest: installedDigest, ManagedAt: m.now()}
		}
		report.Applied = append(report.Applied, operation)
	}

	state.UpdatedAt = m.now()
	if err := m.saveManagedState(state); err != nil {
		return report, errors.Join(err, rollbackAll())
	}
	for _, entry := range rollbacks {
		if entry.existed {
			report.Backups = append(report.Backups, entry.backup)
		}
	}
	report.FinishedAt = m.now()
	_ = os.Remove(filepath.Join(m.Root, "plans", plan.ID+".json"))
	return report, nil
}

func effectiveTargetMode(target Target) SyncMode {
	mode := target.Mode
	if mode == "" || mode == ModeAuto {
		mode = ModeCopy
		if runtime.GOOS != "windows" {
			mode = ModeLink
		}
	}
	return mode
}

func linkDestinationMatches(source, destination string) bool {
	info, err := os.Lstat(destination)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return false
	}
	resolvedDestination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		return false
	}
	sourceAbs, err := filepath.Abs(resolvedSource)
	if err != nil {
		return false
	}
	destinationAbs, err := filepath.Abs(resolvedDestination)
	if err != nil {
		return false
	}
	return filepath.Clean(sourceAbs) == filepath.Clean(destinationAbs)
}

func nextSyncRollbackPath(destination, planID string, operationIndex int) string {
	base := fmt.Sprintf("%s.agentstack-rollback-%s-%d", destination, planID, operationIndex)
	for suffix := 0; ; suffix++ {
		candidate := base
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func supportsAgent(resource Resource, agent Agent) bool {
	if len(resource.Targets) == 0 {
		return true
	}
	for _, candidate := range resource.Targets {
		if candidate == agent {
			return true
		}
	}
	return false
}

func targetDestination(target Target, resource Resource) (string, error) {
	base := target.Root
	fileName := resource.ID + ".md"
	switch target.Agent {
	case AgentCodex:
		switch resource.Kind {
		case KindSkill:
			return filepath.Join(base, ".agents", "skills", resource.ID), nil
		case KindAgent:
			return filepath.Join(base, ".codex", "agents", fileName), nil
		case KindRule, KindContext:
			return filepath.Join(base, ".agents", "rules", fileName), nil
		case KindCommand, KindPrompt:
			return filepath.Join(base, ".codex", "prompts", fileName), nil
		case KindMCPServer:
			return filepath.Join(base, ".agentstack", "mcp", resource.ID+".json"), nil
		}
	case AgentClaude:
		switch resource.Kind {
		case KindSkill:
			return filepath.Join(base, ".claude", "skills", resource.ID), nil
		case KindAgent:
			return filepath.Join(base, ".claude", "agents", fileName), nil
		case KindRule, KindContext:
			return filepath.Join(base, ".claude", "rules", fileName), nil
		case KindCommand, KindPrompt:
			return filepath.Join(base, ".claude", "commands", fileName), nil
		case KindMCPServer:
			return filepath.Join(base, ".agentstack", "mcp", resource.ID+".json"), nil
		}
	case AgentCursor:
		switch resource.Kind {
		case KindSkill:
			return filepath.Join(base, ".cursor", "skills", resource.ID), nil
		case KindAgent:
			return filepath.Join(base, ".cursor", "agents", fileName), nil
		case KindRule, KindContext:
			return filepath.Join(base, ".cursor", "rules", resource.ID+".mdc"), nil
		case KindCommand, KindPrompt:
			return filepath.Join(base, ".cursor", "commands", fileName), nil
		case KindMCPServer:
			return filepath.Join(base, ".agentstack", "mcp", resource.ID+".json"), nil
		}
	case AgentOpenCode:
		switch resource.Kind {
		case KindSkill:
			return filepath.Join(base, ".opencode", "skills", resource.ID), nil
		case KindAgent:
			return filepath.Join(base, ".opencode", "agents", fileName), nil
		case KindRule, KindContext:
			return filepath.Join(base, ".opencode", "rules", fileName), nil
		case KindCommand, KindPrompt:
			return filepath.Join(base, ".opencode", "commands", fileName), nil
		case KindMCPServer:
			return filepath.Join(base, ".agentstack", "mcp", resource.ID+".json"), nil
		}
	case AgentCopilot:
		switch resource.Kind {
		case KindRule, KindContext:
			return filepath.Join(base, ".github", "instructions", resource.ID+".instructions.md"), nil
		case KindPrompt, KindCommand:
			return filepath.Join(base, ".github", "prompts", resource.ID+".prompt.md"), nil
		case KindSkill, KindAgent, KindMCPServer:
			return filepath.Join(base, ".agentstack", string(resource.Kind), resource.ID), nil
		}
	case AgentGeneric:
		return filepath.Join(base, ".agentstack", string(resource.Kind), resource.ID), nil
	}
	return "", fmt.Errorf("resource kind %q is unsupported for target %q", resource.Kind, target.Agent)
}

func (m Manager) loadPlan(id string) (SyncPlan, error) {
	data, err := safefile.ReadBoundedRegular(filepath.Join(m.Root, "plans", id+".json"), maxResourcePlanBytes)
	if err != nil {
		return SyncPlan{}, err
	}
	var plan SyncPlan
	if err := strictjson.Decode(data, &plan); err != nil {
		return SyncPlan{}, err
	}
	if plan.ID != id {
		return SyncPlan{}, fmt.Errorf("sync plan identity mismatch")
	}
	return plan, nil
}

func syncPlanDigest(plan SyncPlan) (string, error) {
	copyValue := plan
	copyValue.Digest = ""
	return integrity.DigestJSON(copyValue)
}

func newID(prefix string, now time.Time) string {
	return fmt.Sprintf("%s-%s", prefix, now.UTC().Format("20060102T150405.000000000Z"))
}

func (m Manager) loadManagedState(targetID string) (managedState, error) {
	path := filepath.Join(m.Root, "sync-state", targetID+".json")
	data, err := safefile.ReadBoundedRegular(path, maxManagedStateBytes)
	if errors.Is(err, os.ErrNotExist) {
		return managedState{Version: 1, TargetID: targetID, Entries: map[string]managedEntry{}}, nil
	}
	if err != nil {
		return managedState{}, err
	}
	var state managedState
	if err := strictjson.Decode(data, &state); err != nil {
		return managedState{}, err
	}
	if state.TargetID != targetID {
		return managedState{}, fmt.Errorf("managed state identity mismatch")
	}
	if state.Entries == nil {
		state.Entries = map[string]managedEntry{}
	}
	return state, nil
}

func (m Manager) saveManagedState(state managedState) error {
	state.Version = 1
	return writeJSON(filepath.Join(m.Root, "sync-state", state.TargetID+".json"), state)
}

func (m Manager) acquireLock(targetID string) (*os.File, error) {
	path := filepath.Join(m.Root, "locks", targetID+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("resource sync is already active for target %q", targetID)
		}
		return nil, err
	}
	_, _ = file.WriteString(fmt.Sprintf("pid=%d\n", os.Getpid()))
	return file, nil
}
func (m Manager) releaseLock(file *os.File) {
	if file == nil {
		return
	}
	path := file.Name()
	_ = file.Close()
	_ = os.Remove(path)
}

func normalizeDestination(path string) string { return strings.ToLower(filepath.Clean(path)) }
