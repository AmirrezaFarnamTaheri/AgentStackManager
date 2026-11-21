package runner

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/model"
)

func (e Engine) applyParallel(parent context.Context, plan model.Plan, options ApplyOptions) model.Transaction {
	if e.Commands == nil {
		e.Commands = ExecRunner{}
	}
	if e.Path == nil {
		e.Path = defaultPathRefresher{}
	}
	if planHasInstallerActions(plan) {
		_ = e.Path.Refresh()
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	tx := model.Transaction{ID: newID(), CorrelationID: options.CorrelationID, StartedAt: time.Now().UTC(), DryRun: false, Status: model.TransactionRunning}
	if err := publish(options.OnUpdate, tx); err != nil {
		tx.Status = model.TransactionFailed
		tx.FinishedAt = time.Now().UTC()
		tx.Actions = append(tx.Actions, journalFailureAction(err))
		return tx
	}

	records := make([]model.TransactionAction, len(plan.Actions))
	completed := make([]bool, len(plan.Actions))
	indexByID := make(map[string]int, len(plan.Actions))
	doneByID := make(map[string]chan struct{}, len(plan.Actions))
	for index, action := range plan.Actions {
		indexByID[action.ComponentID] = index
		doneByID[action.ComponentID] = make(chan struct{})
	}
	globalLimit := e.MaxParallel
	if globalLimit < 2 {
		globalLimit = 2
	}
	global := make(chan struct{}, globalLimit)
	kindSemaphores := map[model.InstallKind]chan struct{}{}
	for _, action := range plan.Actions {
		kind := action.Install.Kind
		if _, ok := kindSemaphores[kind]; ok {
			continue
		}
		kindSemaphores[kind] = make(chan struct{}, e.installerLimit(kind))
	}

	var stateMu sync.Mutex
	var publishMu sync.Mutex
	var startMu sync.Mutex
	var pathMu sync.Mutex
	unavailable := map[model.InstallKind]bool{}
	var publishErr error

	publishState := func(index int, record model.TransactionAction) {
		stateMu.Lock()
		records[index] = record
		completed[index] = true
		snapshot := tx
		snapshot.Actions = make([]model.TransactionAction, 0, len(records))
		for i := range records {
			if completed[i] {
				snapshot.Actions = append(snapshot.Actions, records[i])
			}
		}
		stateMu.Unlock()
		publishMu.Lock()
		if publishErr == nil {
			if err := publish(options.OnUpdate, snapshot); err != nil {
				publishErr = err
				cancel()
			}
		}
		publishMu.Unlock()
	}

	var wg sync.WaitGroup
	for index, action := range plan.Actions {
		index, action := index, action
		wg.Add(1)
		go func() {
			defer wg.Done()
			record := model.TransactionAction{ComponentID: action.ComponentID, Kind: action.Kind, StartedAt: time.Now().UTC()}
			defer func() {
				record.FinishedAt = time.Now().UTC()
				publishState(index, record)
				close(doneByID[action.ComponentID])
			}()

			if component, ok := e.Catalog.ComponentByID(action.ComponentID); ok {
				for _, dependency := range component.DependsOn {
					if dependencyDone, exists := doneByID[dependency]; exists {
						select {
						case <-dependencyDone:
						case <-ctx.Done():
							record.ExitCode = -1
							record.Error = "operation cancelled before dependency completed"
							record.Verification = "not started"
							return
						}
						dependencyIndex := indexByID[dependency]
						stateMu.Lock()
						dependencyRecord := records[dependencyIndex]
						stateMu.Unlock()
						if dependencyRecord.Error != "" || !dependencyRecord.Verified {
							record.ExitCode = -1
							record.Error = fmt.Sprintf("dependency %s failed: %s", dependency, dependencyRecord.Error)
							record.Verification = "dependency did not complete successfully"
							return
						}
					}
				}
			}

			if options.OnActionStart != nil {
				startMu.Lock()
				err := options.OnActionStart(action)
				startMu.Unlock()
				if err != nil {
					record.ExitCode = -1
					record.Error = "start action callback: " + err.Error()
					record.Verification = "action was not started"
					return
				}
			}
			if action.Kind != model.ActionInstall && action.Kind != model.ActionRepair {
				record.Verified = true
				record.Verification = "no installer action required"
				return
			}
			select {
			case <-ctx.Done():
				record.ExitCode = -1
				record.Error = "operation cancelled before installation started"
				record.Verification = "not started"
				return
			default:
			}

			stateMu.Lock()
			installerUnavailable := unavailable[action.Install.Kind]
			stateMu.Unlock()
			if installerUnavailable {
				record.ExitCode = -1
				record.Error = fmt.Sprintf("installer prerequisite %s unavailable", action.Install.Kind)
				record.Verification = "installer unavailable"
				return
			}

			kindSemaphore := kindSemaphores[action.Install.Kind]
			select {
			case kindSemaphore <- struct{}{}:
				defer func() { <-kindSemaphore }()
			case <-ctx.Done():
				record.ExitCode = -1
				record.Error = "operation cancelled while waiting for installer capacity"
				record.Verification = "not started"
				return
			}
			select {
			case global <- struct{}{}:
				defer func() { <-global }()
			case <-ctx.Done():
				record.ExitCode = -1
				record.Error = "operation cancelled while waiting for global capacity"
				record.Verification = "not started"
				return
			}

			var result Result
			if action.Install.Kind == model.InstallSkillPack {
				component, ok := e.Catalog.ComponentByID(action.ComponentID)
				if !ok {
					result = Result{ExitCode: -1, Err: fmt.Errorf("component %s missing from catalog", action.ComponentID)}
				} else if e.Skills == nil {
					result = Result{ExitCode: -1, Err: fmt.Errorf("skill installer unavailable")}
				} else {
					result = e.Skills.Install(ctx, component)
				}
			} else {
				invocation, err := invocationFor(action)
				if err != nil {
					result = Result{ExitCode: -1, Err: err}
				} else {
					record.Command, record.Args = invocation.Command, invocation.Args
					result = e.Commands.Run(ctx, invocation)
				}
			}
			record.ExitCode, record.Output, record.OutputTruncated = result.ExitCode, result.Stdout, result.Truncated
			if result.Err == nil && action.Install.Kind == model.InstallWinget {
				pathMu.Lock()
				err := e.Path.Refresh()
				pathMu.Unlock()
				if err != nil {
					result.Err = fmt.Errorf("refresh process PATH: %w", err)
					result.ExitCode = -1
					record.ExitCode = -1
				}
			}
			if result.Err != nil {
				if errors.Is(result.Err, exec.ErrNotFound) {
					stateMu.Lock()
					unavailable[action.Install.Kind] = true
					stateMu.Unlock()
				}
				record.Error = result.Err.Error()
				if result.Stderr != "" {
					record.Error += ": " + result.Stderr
				}
				record.Verification = "installer did not complete"
				return
			}
			if e.Verifier != nil {
				component, ok := e.Catalog.ComponentByID(action.ComponentID)
				if !ok {
					record.Error = "component missing from catalog during postcondition verification"
					record.Verification = "verification unavailable"
					return
				}
				verification := e.Verifier.Verify(ctx, component, action, result)
				record.Verified = verification.OK
				record.Verification = verification.Message
				if !verification.OK {
					record.Error = "postcondition verification failed: " + verification.Message
				}
				return
			}
			record.Verified = true
			record.Verification = "command completed; no verifier configured"
		}()
	}
	wg.Wait()

	stateMu.Lock()
	tx.Actions = append([]model.TransactionAction(nil), records...)
	stateMu.Unlock()
	failed := false
	for _, record := range tx.Actions {
		if record.Error != "" || !record.Verified {
			failed = true
			break
		}
	}
	publishMu.Lock()
	finalPublishErr := publishErr
	publishMu.Unlock()
	if finalPublishErr != nil {
		failed = true
		tx.Actions = append(tx.Actions, journalFailureAction(finalPublishErr))
	}
	switch {
	case failed:
		tx.Status = model.TransactionFailed
	case parent.Err() != nil:
		tx.Status = model.TransactionInterrupted
	default:
		tx.Status = model.TransactionSucceeded
	}
	tx.FinishedAt = time.Now().UTC()
	if err := publish(options.OnUpdate, tx); err != nil && finalPublishErr == nil {
		tx.Status = model.TransactionFailed
		tx.Actions = append(tx.Actions, journalFailureAction(err))
	}
	return tx
}

func (e Engine) installerLimit(kind model.InstallKind) int {
	if limit := e.InstallerParallelism[kind]; limit > 0 {
		return limit
	}
	switch kind {
	case model.InstallUVTool:
		return 2
	case model.InstallWinget, model.InstallNPMGlobal, model.InstallSkillPack, model.InstallRouter, model.InstallManual:
		return 1
	default:
		return 1
	}
}
