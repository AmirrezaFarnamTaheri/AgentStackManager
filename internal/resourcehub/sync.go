package resourcehub

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
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
	adapter, capability, err := m.targetCapability(target)
	if err != nil {
		return SyncPlan{}, err
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
	reports := make([]adapters.LossReport, 0, len(ids)+1)
	selectedDestinations := map[string]struct{}{}
	for _, id := range ids {
		resource, exists := registry.Resources[id]
		if !exists {
			return SyncPlan{}, fmt.Errorf("unknown resource %q", id)
		}
		if !resource.Enabled || !supportsAgent(resource, target.Agent) {
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
		rendered, lossReport, err := m.renderResource(adapter, target, resource)
		if err != nil {
			return SyncPlan{}, err
		}
		reports = append(reports, lossReport)
		destination := rendered.Destination
		selectedDestinations[destination] = struct{}{}
		sourcePath := m.resourceSource(resource)
		current, statErr := treeDigest(destination)
		managed, wasManaged := state.Entries[destination]
		observed := adapters.ObservedArtifact{
			ArtifactID: rendered.ArtifactID,
			Kind:       rendered.Kind,
			Location:   destination,
			Digest:     current,
			BaseDigest: managed.Digest,
			Exists:     statErr == nil,
			Owned:      wasManaged && managed.ResourceID == id,
		}
		if errors.Is(statErr, os.ErrNotExist) {
			observed.Digest = "absent"
		} else if statErr != nil {
			return SyncPlan{}, statErr
		} else {
			observed.Equivalent = current == resource.Digest
			if effectiveTargetMode(target) == ModeLink && observed.Owned && linkDestinationMatches(sourcePath, destination) {
				observed.Equivalent = true
			}
		}
		proposals, err := adapter.Plan(context.Background(), adapters.PlanRequest{
			Environment: targetEnvironment(target), Mode: adapters.PresencePresent,
			Rendered: rendered, Observed: observed, Capability: capability, LossReport: lossReport,
		})
		if err != nil {
			return SyncPlan{}, err
		}
		if len(proposals) != 1 {
			return SyncPlan{}, fmt.Errorf("adapter %q proposed %d operations for resource %q", adapter.ID(), len(proposals), id)
		}
		action, err := syncAction(proposals[0].Action)
		if err != nil {
			return SyncPlan{}, err
		}
		op := SyncOperation{
			ResourceID: id, Kind: resource.Kind, Action: action, Source: sourcePath,
			Destination: destination, DesiredDigest: resource.Digest, Reason: proposals[0].Reason,
		}
		if observed.Exists {
			op.CurrentDigest = observed.Digest
		}
		applyAdapterMetadata(&op, proposals[0], lossReport)
		operations = append(operations, op)
	}
	emptyLossReport, err := adapters.SealLossReport(adapters.LossReport{
		Target: capability.Target, AdapterID: capability.AdapterID,
		AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest,
	})
	if err != nil {
		return SyncPlan{}, err
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
			rendered := adapters.RenderedArtifact{ArtifactID: "managed/" + managed.ResourceID, Destination: destination, DesiredDigest: "absent"}
			observed := adapters.ObservedArtifact{ArtifactID: rendered.ArtifactID, Location: destination, Digest: current, BaseDigest: managed.Digest, Exists: true, Owned: true}
			proposals, err := adapter.Plan(context.Background(), adapters.PlanRequest{
				Environment: targetEnvironment(target), Mode: adapters.PresenceAbsent,
				Rendered: rendered, Observed: observed, Capability: capability, LossReport: emptyLossReport,
			})
			if err != nil {
				return SyncPlan{}, err
			}
			if len(proposals) != 1 {
				return SyncPlan{}, fmt.Errorf("adapter %q proposed %d prune operations", adapter.ID(), len(proposals))
			}
			action, err := syncAction(proposals[0].Action)
			if err != nil {
				return SyncPlan{}, err
			}
			op := SyncOperation{ResourceID: managed.ResourceID, Action: action, Destination: destination, CurrentDigest: current, Reason: proposals[0].Reason}
			applyAdapterMetadata(&op, proposals[0], emptyLossReport)
			operations = append(operations, op)
		}
	}
	lossReport, err := adapters.MergeLossReports(capability.Target, capability.AdapterID, capability.AdapterVersion, capability.Digest, reports...)
	if err != nil {
		return SyncPlan{}, err
	}
	if options.DenyLoss && lossReport.HasLosses() {
		return SyncPlan{}, fmt.Errorf("adapter %q would produce %s fidelity with %d reported losses", adapter.ID(), lossReport.Fidelity, len(lossReport.Losses))
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].Destination < operations[j].Destination })
	registryDigest, err := integrity.DigestJSON(registry)
	if err != nil {
		return SyncPlan{}, err
	}
	now := m.now()
	plan := SyncPlan{
		ID: newID("sync", now), TargetID: targetID, GeneratedAt: now, ExpiresAt: now.Add(options.TTL),
		RegistryDigest: registryDigest, AllowRisk: options.AllowRisk, Prune: options.Prune, DenyLoss: options.DenyLoss,
		AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest,
		LossReport: lossReport, Operations: operations,
	}
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
	_, capability, err := m.targetCapability(target)
	if err != nil {
		return SyncReport{}, err
	}
	if capability.AdapterID != plan.AdapterID || capability.AdapterVersion != plan.AdapterVersion || capability.Digest != plan.CapabilityDigest {
		return SyncReport{}, fmt.Errorf("target adapter capability changed after plan review")
	}
	if err := adapters.VerifyLossReport(plan.LossReport); err != nil {
		return SyncReport{}, err
	}
	if plan.LossReport.Target != capability.Target || plan.LossReport.AdapterID != capability.AdapterID || plan.LossReport.CapabilityDigest != capability.Digest {
		return SyncReport{}, fmt.Errorf("sync plan loss report identity mismatch")
	}
	for _, operation := range plan.Operations {
		if operation.AdapterID != capability.AdapterID || operation.AdapterVersion != capability.AdapterVersion || operation.CapabilityDigest != capability.Digest {
			return SyncReport{}, fmt.Errorf("sync operation adapter identity mismatch at %s", operation.Destination)
		}
		if err := verifySyncOperationLoss(capability, operation); err != nil {
			return SyncReport{}, err
		}
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
	report := SyncReport{PlanID: plan.ID, TargetID: plan.TargetID, StartedAt: m.now(), LossReport: plan.LossReport}
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
