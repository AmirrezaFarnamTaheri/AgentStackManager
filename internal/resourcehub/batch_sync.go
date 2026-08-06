package resourcehub

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

func (m Manager) PlanBatchSync(targetIDs, resourceIDs []string, options PlanOptions, maxParallel int) (BatchSyncPlan, error) {
	if err := m.ensure(); err != nil {
		return BatchSyncPlan{}, err
	}
	if len(targetIDs) == 0 || len(targetIDs) > 64 {
		return BatchSyncPlan{}, fmt.Errorf("select between 1 and 64 targets")
	}
	if maxParallel <= 0 {
		maxParallel = 3
	}
	if maxParallel > 16 {
		return BatchSyncPlan{}, fmt.Errorf("batch sync parallelism cannot exceed 16")
	}
	uniqueTargets := uniqueStrings(targetIDs)
	if len(uniqueTargets) != len(targetIDs) {
		return BatchSyncPlan{}, fmt.Errorf("batch sync target ids must be unique")
	}
	sort.Strings(uniqueTargets)
	children := make([]SyncPlan, 0, len(uniqueTargets))
	for _, targetID := range uniqueTargets {
		child, err := m.PlanSync(targetID, resourceIDs, options)
		if err != nil {
			return BatchSyncPlan{}, fmt.Errorf("plan target %s: %w", targetID, err)
		}
		children = append(children, child)
	}
	now := m.now()
	plan := BatchSyncPlan{ID: newID("batch-sync", now), GeneratedAt: now, MaxParallel: maxParallel, Children: children}
	if len(children) > 0 {
		plan.ExpiresAt = children[0].ExpiresAt
		for _, child := range children[1:] {
			if child.ExpiresAt.Before(plan.ExpiresAt) {
				plan.ExpiresAt = child.ExpiresAt
			}
		}
	}
	var err error
	plan.Digest, err = batchSyncPlanDigest(plan)
	if err != nil {
		return BatchSyncPlan{}, err
	}
	if err := writeJSON(filepath.Join(m.Root, "plans", plan.ID+".json"), plan); err != nil {
		return BatchSyncPlan{}, err
	}
	return plan, nil
}

func (m Manager) ApplyBatchSync(ctx context.Context, planID, digest string, confirmed bool) (BatchSyncReport, error) {
	if !confirmed {
		return BatchSyncReport{}, ErrConfirmationRequired
	}
	plan, err := m.loadBatchSyncPlan(planID)
	if err != nil {
		return BatchSyncReport{}, err
	}
	if digest != plan.Digest {
		return BatchSyncReport{}, fmt.Errorf("batch sync plan digest mismatch")
	}
	actual, err := batchSyncPlanDigest(plan)
	if err != nil || actual != plan.Digest {
		return BatchSyncReport{}, fmt.Errorf("stored batch sync plan digest is invalid")
	}
	if !m.now().Before(plan.ExpiresAt) {
		return BatchSyncReport{}, fmt.Errorf("batch sync plan expired")
	}
	registry, err := m.LoadRegistry()
	if err != nil {
		return BatchSyncReport{}, err
	}
	rootByTarget := make(map[string]string, len(plan.Children))
	for _, child := range plan.Children {
		target, ok := registry.Targets[child.TargetID]
		if !ok || !target.Enabled {
			return BatchSyncReport{}, fmt.Errorf("target %q is unavailable", child.TargetID)
		}
		rootByTarget[child.TargetID] = filepath.Clean(target.Root)
	}

	report := BatchSyncReport{PlanID: plan.ID, StartedAt: m.now(), Results: make([]BatchSyncTargetResult, len(plan.Children))}
	limit := plan.MaxParallel
	if limit <= 0 {
		limit = 1
	}
	semaphore := make(chan struct{}, limit)
	rootLocks := map[string]*sync.Mutex{}
	var rootLocksMu sync.Mutex
	var wg sync.WaitGroup
	for index, child := range plan.Children {
		index, child := index, child
		if ctx.Err() != nil {
			report.Results[index] = BatchSyncTargetResult{TargetID: child.TargetID, Status: "cancelled", FailureCategory: "cancelled", Message: "The operation was cancelled before this target started.", Recovery: "Create a fresh reviewed plan for unfinished targets."}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				report.Results[index] = BatchSyncTargetResult{TargetID: child.TargetID, Status: "cancelled", FailureCategory: "cancelled", Message: "The operation was cancelled before this target started.", Recovery: "Create a fresh reviewed plan for unfinished targets."}
				return
			}
			root := rootByTarget[child.TargetID]
			rootLocksMu.Lock()
			lock := rootLocks[root]
			if lock == nil {
				lock = &sync.Mutex{}
				rootLocks[root] = lock
			}
			rootLocksMu.Unlock()
			lock.Lock()
			defer lock.Unlock()
			if ctx.Err() != nil {
				report.Results[index] = BatchSyncTargetResult{TargetID: child.TargetID, Status: "cancelled", FailureCategory: "cancelled", Message: "The operation was cancelled before this target started.", Recovery: "Create a fresh reviewed plan for unfinished targets."}
				return
			}
			childReport, applyErr := m.ApplySync(child.ID, child.Digest, true)
			if applyErr != nil {
				category, message, recovery := classifyBatchSyncFailure(applyErr)
				report.Results[index] = BatchSyncTargetResult{TargetID: child.TargetID, Status: "failed", FailureCategory: category, Message: message, Recovery: recovery}
				return
			}
			report.Results[index] = BatchSyncTargetResult{TargetID: child.TargetID, Status: "succeeded", Report: &childReport, Message: "Target synchronized and verified."}
		}()
	}
	wg.Wait()
	for _, result := range report.Results {
		switch result.Status {
		case "succeeded":
			report.Succeeded++
		case "cancelled":
			report.Cancelled++
		default:
			report.Failed++
		}
	}
	report.FinishedAt = m.now()
	_ = removeManagedPath(filepath.Join(m.Root, "plans", plan.ID+".json"))
	if report.Failed > 0 {
		return report, fmt.Errorf("%d target synchronization%s failed", report.Failed, pluralBatchSuffix(report.Failed))
	}
	if report.Cancelled > 0 {
		return report, context.Canceled
	}
	return report, nil
}

func (m Manager) loadBatchSyncPlan(id string) (BatchSyncPlan, error) {
	if !validID(id) {
		return BatchSyncPlan{}, fmt.Errorf("batch sync plan id is empty or invalid")
	}
	data, err := safefile.ReadBoundedRegular(filepath.Join(m.Root, "plans", id+".json"), maxResourcePlanBytes)
	if err != nil {
		return BatchSyncPlan{}, err
	}
	var plan BatchSyncPlan
	if err := strictjson.Decode(data, &plan); err != nil {
		return BatchSyncPlan{}, err
	}
	if plan.ID != id {
		return BatchSyncPlan{}, fmt.Errorf("batch sync plan identity mismatch")
	}
	return plan, nil
}

func batchSyncPlanDigest(plan BatchSyncPlan) (string, error) {
	copyValue := plan
	copyValue.Digest = ""
	return integrity.DigestJSON(copyValue)
}

func classifyBatchSyncFailure(err error) (string, string, string) {
	lower := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled", "Synchronization was cancelled.", "Create a fresh reviewed plan for unfinished targets."
	case strings.Contains(lower, "changed after plan review") || strings.Contains(lower, "appeared after plan review") || strings.Contains(lower, "digest mismatch"):
		return "stale_plan", "The target changed after the plan was reviewed.", "Scan again and approve a fresh plan."
	case strings.Contains(lower, "conflict"):
		return "content_conflict", "Managed and existing target content conflict.", "Review the conflicting resource; AgentStack will not overwrite it automatically."
	case strings.Contains(lower, "permission") || strings.Contains(lower, "access is denied"):
		return "permission_denied", "AgentStack could not write to the target.", "Review target permissions or use a writable scope, then create a fresh plan."
	default:
		return "sync_failed", "The target could not be synchronized and verified.", "Review target health, scan again, and create a fresh plan."
	}
}

func pluralBatchSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
