package resourcehub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

func (m Manager) PlanRefresh(resourceIDs []string, ttl time.Duration) (RefreshPlan, error) {
	registry, err := m.LoadRegistry()
	if err != nil {
		return RefreshPlan{}, err
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	selected := uniqueStrings(resourceIDs)
	if len(selected) == 0 {
		for id, resource := range registry.Resources {
			if resource.Enabled {
				selected = append(selected, id)
			}
		}
		sort.Strings(selected)
	}
	operations := make([]RefreshOperation, 0, len(selected))
	for _, id := range selected {
		resource, ok := registry.Resources[id]
		if !ok {
			return RefreshPlan{}, fmt.Errorf("unknown resource %q", id)
		}
		operation := RefreshOperation{ResourceID: id, Source: resource.Source, BeforeDigest: resource.Digest}
		if resource.Source == "" {
			operation.Action = RefreshConflict
			operation.Reason = "resource predates source tracking; re-import it to enable refresh"
			operations = append(operations, operation)
			continue
		}
		digest, digestErr := treeDigest(resource.Source)
		if digestErr != nil {
			operation.Action = RefreshConflict
			operation.Reason = fmt.Sprintf("source is unavailable: %v", digestErr)
			operations = append(operations, operation)
			continue
		}
		operation.SourceDigest = digest
		if digest == resource.Digest {
			operation.Action = RefreshNoop
			operation.Reason = "source content matches the canonical resource"
		} else {
			operation.Action = RefreshUpdate
			operation.Reason = "source content changed"
		}
		operations = append(operations, operation)
	}
	registryDigest, err := integrity.DigestJSON(registry)
	if err != nil {
		return RefreshPlan{}, err
	}
	now := m.now()
	plan := RefreshPlan{ID: "resource-refresh-" + now.Format("20060102T150405.000000000Z"), RegistryDigest: registryDigest, GeneratedAt: now, ExpiresAt: now.Add(ttl), Operations: operations}
	plan.Digest, err = resourceRefreshDigest(plan)
	if err != nil {
		return RefreshPlan{}, err
	}
	if err := writeJSON(filepath.Join(m.Root, "plans", plan.ID+".json"), plan); err != nil {
		return RefreshPlan{}, err
	}
	return plan, nil
}

func (m Manager) ApplyRefresh(planID, digest string, confirmed bool) (RefreshReport, error) {
	if !confirmed {
		return RefreshReport{}, fmt.Errorf("resource refresh requires explicit confirmation")
	}
	data, err := safefile.ReadBoundedRegular(filepath.Join(m.Root, "plans", planID+".json"), maxResourcePlanBytes)
	if err != nil {
		return RefreshReport{}, err
	}
	var plan RefreshPlan
	if err := strictjson.Decode(data, &plan); err != nil {
		return RefreshReport{}, err
	}
	if plan.ID != planID || plan.Digest != digest {
		return RefreshReport{}, fmt.Errorf("resource refresh plan identity or digest mismatch")
	}
	actual, err := resourceRefreshDigest(plan)
	if err != nil || actual != plan.Digest {
		return RefreshReport{}, fmt.Errorf("stored resource refresh plan digest is invalid")
	}
	if !m.now().Before(plan.ExpiresAt) {
		return RefreshReport{}, fmt.Errorf("resource refresh plan expired")
	}
	registry, err := m.LoadRegistry()
	if err != nil {
		return RefreshReport{}, err
	}
	registryDigest, err := integrity.DigestJSON(registry)
	if err != nil || registryDigest != plan.RegistryDigest {
		return RefreshReport{}, fmt.Errorf("resource registry changed after review")
	}
	lock, err := m.acquireLock("resource-refresh")
	if err != nil {
		return RefreshReport{}, err
	}
	defer m.releaseLock(lock)

	for _, operation := range plan.Operations {
		resource, ok := registry.Resources[operation.ResourceID]
		if !ok || resource.Digest != operation.BeforeDigest || resource.Source != operation.Source {
			return RefreshReport{}, fmt.Errorf("resource %q changed after review", operation.ResourceID)
		}
		if operation.Action == RefreshConflict {
			return RefreshReport{}, fmt.Errorf("resource %q cannot refresh: %s", operation.ResourceID, operation.Reason)
		}
		canonicalDigest, canonicalErr := treeDigest(m.resourceSource(resource))
		if canonicalErr != nil || canonicalDigest != resource.Digest {
			return RefreshReport{}, fmt.Errorf("canonical resource changed after review: %s", operation.ResourceID)
		}
		if operation.Action == RefreshUpdate {
			current, sourceErr := treeDigest(operation.Source)
			if sourceErr != nil || current != operation.SourceDigest {
				return RefreshReport{}, fmt.Errorf("resource source changed after review: %s", operation.ResourceID)
			}
		}
	}

	type stagedRefresh struct {
		operation RefreshOperation
		resource  Resource
		stage     string
		backup    string
		installed bool
	}
	staged := make([]stagedRefresh, 0, len(plan.Operations))
	cleanupStages := func() {
		for _, entry := range staged {
			if !entry.installed {
				_ = os.RemoveAll(entry.stage)
			}
		}
	}
	defer cleanupStages()

	for _, operation := range plan.Operations {
		if operation.Action == RefreshNoop {
			continue
		}
		resource := registry.Resources[operation.ResourceID]
		stage, updated, stageErr := m.stageRefreshedResource(resource, operation)
		if stageErr != nil {
			return RefreshReport{}, stageErr
		}
		staged = append(staged, stagedRefresh{operation: operation, resource: updated, stage: stage})
	}

	rollback := func() error {
		var rollbackErr error
		for index := len(staged) - 1; index >= 0; index-- {
			entry := &staged[index]
			if !entry.installed {
				continue
			}
			resourceDir := filepath.Join(m.Root, "resources", entry.resource.ID)
			rollbackErr = errors.Join(rollbackErr, os.RemoveAll(resourceDir))
			rollbackErr = errors.Join(rollbackErr, os.Rename(entry.backup, resourceDir))
			entry.installed = false
		}
		return rollbackErr
	}

	report := RefreshReport{PlanID: plan.ID, StartedAt: m.now()}
	for index := range staged {
		entry := &staged[index]
		if m.beforeRefreshOperation != nil {
			if hookErr := m.beforeRefreshOperation(entry.operation); hookErr != nil {
				return report, errors.Join(hookErr, rollback())
			}
		}
		resourceDir := filepath.Join(m.Root, "resources", entry.resource.ID)
		entry.backup = m.nextBackupPath(entry.resource.ID, "refresh", m.now())
		if err := os.Rename(resourceDir, entry.backup); err != nil {
			return report, errors.Join(err, rollback())
		}
		if err := os.Rename(entry.stage, resourceDir); err != nil {
			_ = os.Rename(entry.backup, resourceDir)
			return report, errors.Join(err, rollback())
		}
		entry.installed = true
		registry.Resources[entry.resource.ID] = entry.resource
		report.Applied = append(report.Applied, entry.operation)
		report.Backups = append(report.Backups, entry.backup)
	}
	for _, operation := range plan.Operations {
		if operation.Action == RefreshNoop {
			report.Skipped = append(report.Skipped, operation)
		}
	}
	if err := m.saveRegistry(registry); err != nil {
		return report, errors.Join(err, rollback())
	}
	report.FinishedAt = m.now()
	if err := os.Remove(filepath.Join(m.Root, "plans", plan.ID+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return report, err
	}
	return report, nil
}

func (m Manager) stageRefreshedResource(resource Resource, operation RefreshOperation) (string, Resource, error) {
	info, err := os.Lstat(operation.Source)
	if err != nil {
		return "", Resource{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || (!info.Mode().IsRegular() && !info.IsDir()) {
		return "", Resource{}, fmt.Errorf("resource source must remain a regular file or directory")
	}
	stage, err := os.MkdirTemp(filepath.Join(m.Root, "resources"), ".refresh-*")
	if err != nil {
		return "", Resource{}, err
	}
	content := filepath.Join(stage, "content")
	if info.IsDir() {
		err = copyTree(operation.Source, content)
	} else {
		if err = os.MkdirAll(content, 0o700); err == nil {
			err = copyFile(operation.Source, filepath.Join(content, filepath.Base(operation.Source)), info.Mode().Perm())
		}
	}
	if err != nil {
		_ = os.RemoveAll(stage)
		return "", Resource{}, err
	}
	contentDigest, err := treeDigest(content)
	if err != nil || contentDigest != operation.SourceDigest {
		_ = os.RemoveAll(stage)
		return "", Resource{}, fmt.Errorf("staged resource digest mismatch: %s", operation.ResourceID)
	}
	updated := resource
	updated.Digest = contentDigest
	updated.UpdatedAt = m.now()
	if err := writeJSON(filepath.Join(stage, "resource.json"), updated); err != nil {
		_ = os.RemoveAll(stage)
		return "", Resource{}, err
	}
	return stage, updated, nil
}

func resourceRefreshDigest(plan RefreshPlan) (string, error) {
	copyValue := plan
	copyValue.Digest = ""
	return integrity.DigestJSON(copyValue)
}
