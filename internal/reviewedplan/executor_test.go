package reviewedplan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/runner"
	"github.com/agentstack/agentstack/internal/state"
)

func sealedPlan(t *testing.T, catalog model.Catalog, inventory model.Inventory) model.Plan {
	t.Helper()
	plan, err := planner.Seal(catalog, inventory, model.Plan{
		Profile: "essential",
		Actions: []model.PlanAction{{ComponentID: "tool", Kind: model.ActionKeep}},
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestExecutorConsumesValidatedPlanAndPersistsTransaction(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{{ID: "tool", Name: "Tool"}}}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{"tool": {ComponentID: "tool", Installed: true}}}
	plan := sealedPlan(t, catalog, inventory)
	store := state.NewStore(t.TempDir())
	if err := store.SavePlan(state.SavedPlan{Plan: plan, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	recorded := false
	executor := Executor{
		Catalog:   catalog,
		Store:     store,
		Installer: runner.Engine{Catalog: catalog},
		Inventory: func(context.Context) (model.Inventory, error) { return inventory, nil },
		RecordSuccessfulInstalls: func(model.Plan, model.Transaction) error {
			recorded = true
			return nil
		},
	}
	result, err := executor.Execute(context.Background(), Request{PlanID: plan.ID, Digest: plan.Digest, Confirmed: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Transaction.Status != model.TransactionSucceeded {
		t.Fatalf("transaction status = %s", result.Transaction.Status)
	}
	if !recorded {
		t.Fatal("ownership recorder was not called")
	}
	if _, err := store.LoadPlan(plan.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consumed plan still loadable: %v", err)
	}
	if _, err := store.LoadTransaction(result.Transaction.ID); err != nil {
		t.Fatalf("persisted transaction missing: %v", err)
	}
}

func TestExecutorRejectsDigestMismatchWithoutConsumingPlan(t *testing.T) {
	catalog := model.Catalog{Version: 1}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	plan := sealedPlan(t, catalog, inventory)
	store := state.NewStore(t.TempDir())
	if err := store.SavePlan(state.SavedPlan{Plan: plan}); err != nil {
		t.Fatal(err)
	}
	executor := Executor{Catalog: catalog, Store: store, Inventory: func(context.Context) (model.Inventory, error) { return inventory, nil }}
	_, err := executor.Execute(context.Background(), Request{PlanID: plan.ID, Digest: "sha256:wrong", Confirmed: true})
	if !errors.Is(err, ErrPlanMismatch) {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := store.LoadPlan(plan.ID); err != nil {
		t.Fatalf("plan was consumed after rejected digest: %v", err)
	}
}

func TestExecutorRejectsInventoryDriftBeforeConsumption(t *testing.T) {
	catalog := model.Catalog{Version: 1}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	plan := sealedPlan(t, catalog, inventory)
	store := state.NewStore(t.TempDir())
	if err := store.SavePlan(state.SavedPlan{Plan: plan}); err != nil {
		t.Fatal(err)
	}
	drifted := model.Inventory{Items: map[string]model.InventoryItem{"new": {ComponentID: "new", Installed: true}}}
	executor := Executor{Catalog: catalog, Store: store, Inventory: func(context.Context) (model.Inventory, error) { return drifted, nil }}
	_, err := executor.Execute(context.Background(), Request{PlanID: plan.ID, Digest: plan.Digest, Confirmed: true})
	if !errors.Is(err, ErrPlanStale) {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := store.LoadPlan(plan.ID); err != nil {
		t.Fatalf("stale plan was consumed: %v", err)
	}
}

func TestExecutorRequiresConfirmationBeforeLeaseOrLoad(t *testing.T) {
	executor := Executor{}
	_, err := executor.Execute(context.Background(), Request{})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecutorRejectsExpiredPlanWithoutConsumption(t *testing.T) {
	catalog := model.Catalog{Version: 1}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	plan := sealedPlan(t, catalog, inventory)
	store := state.NewStore(t.TempDir())
	if err := store.SavePlan(state.SavedPlan{Plan: plan}); err != nil {
		t.Fatal(err)
	}
	executor := Executor{
		Catalog:   catalog,
		Store:     store,
		Inventory: func(context.Context) (model.Inventory, error) { return inventory, nil },
		Now:       func() time.Time { return plan.ExpiresAt.Add(time.Second) },
	}
	_, err := executor.Execute(context.Background(), Request{PlanID: plan.ID, Digest: plan.Digest, Confirmed: true})
	if !errors.Is(err, ErrPlanStale) {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := store.LoadPlan(plan.ID); err != nil {
		t.Fatalf("expired plan was consumed: %v", err)
	}
}

func TestExecutorRejectsCatalogDriftWithoutInventoryScan(t *testing.T) {
	catalog := model.Catalog{Version: 1}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	plan := sealedPlan(t, catalog, inventory)
	store := state.NewStore(t.TempDir())
	if err := store.SavePlan(state.SavedPlan{Plan: plan}); err != nil {
		t.Fatal(err)
	}
	called := false
	executor := Executor{
		Catalog: model.Catalog{Version: 2},
		Store:   store,
		Inventory: func(context.Context) (model.Inventory, error) {
			called = true
			return inventory, nil
		},
	}
	_, err := executor.Execute(context.Background(), Request{PlanID: plan.ID, Digest: plan.Digest, Confirmed: true})
	if !errors.Is(err, ErrPlanStale) {
		t.Fatalf("Execute() error = %v", err)
	}
	if called {
		t.Fatal("inventory scan ran after catalog drift was already proven")
	}
}

func TestExecutorRejectsPlanWhoseStoredIdentityDiffersFromRequest(t *testing.T) {
	catalog := model.Catalog{Version: 1}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	plan := sealedPlan(t, catalog, inventory)
	store := state.NewStore(t.TempDir())
	storedID := "renamed-plan"
	if err := os.MkdirAll(filepath.Join(store.Root, "plans"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state.SavedPlan{Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Root, "plans", storedID+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	executor := Executor{
		Catalog:   catalog,
		Store:     store,
		Inventory: func(context.Context) (model.Inventory, error) { return inventory, nil },
	}
	_, err = executor.Execute(context.Background(), Request{PlanID: storedID, Digest: plan.Digest, Confirmed: true})
	if !errors.Is(err, ErrPlanMismatch) {
		t.Fatalf("Execute() error = %v, want ErrPlanMismatch", err)
	}
	if _, err := store.LoadPlan(storedID); err != nil {
		t.Fatalf("identity-mismatched plan was consumed: %v", err)
	}
}

func TestExecutorMissingPlanReturnsUnavailableWithoutPathLeak(t *testing.T) {
	catalog := model.Catalog{Version: 1}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	store := state.NewStore(t.TempDir())
	executor := Executor{
		Catalog:   catalog,
		Store:     store,
		Inventory: func(context.Context) (model.Inventory, error) { return inventory, nil },
	}
	_, err := executor.Execute(context.Background(), Request{PlanID: "missing-plan", Digest: "sha256:missing", Confirmed: true})
	if !errors.Is(err, ErrPlanUnavailable) {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(err.Error(), store.Root) || strings.Contains(strings.ToLower(err.Error()), "plans/") || strings.Contains(strings.ToLower(err.Error()), `plans\\`) {
		t.Fatalf("private store path leaked: %v", err)
	}
}

func TestExecutorProgressCallbacksRunInsideReviewedBoundary(t *testing.T) {
	catalog := model.Catalog{Version: 1, Components: []model.Component{{ID: "tool", Name: "Tool", Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Vendor.Tool"}}}}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	plan, err := planner.Seal(catalog, inventory, model.Plan{Profile: "essential", Actions: []model.PlanAction{{ComponentID: "tool", Name: "Tool", Kind: model.ActionInstall, Install: catalog.Components[0].Install}}}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store := state.NewStore(t.TempDir())
	if err := store.SavePlan(state.SavedPlan{Plan: plan, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var events []string
	executor := Executor{
		Catalog:   catalog,
		Store:     store,
		Installer: runner.Engine{Catalog: catalog, Commands: &fakeReviewedCommandRunner{}},
		Inventory: func(context.Context) (model.Inventory, error) { return inventory, nil },
		OnPlanReady: func(ready model.Plan) error {
			if _, err := store.LoadPlan(ready.ID); err != nil {
				t.Fatalf("plan was consumed before readiness callback: %v", err)
			}
			events = append(events, "ready:"+ready.ID)
			return nil
		},
		OnActionStart: func(action model.PlanAction) error {
			events = append(events, "start:"+action.ComponentID)
			return nil
		},
		OnTransaction: func(tx model.Transaction) error {
			events = append(events, "tx:"+string(tx.Status))
			return nil
		},
	}
	result, err := executor.Execute(context.Background(), Request{PlanID: plan.ID, Digest: plan.Digest, Confirmed: true})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Transaction.Status != model.TransactionSucceeded {
		t.Fatalf("transaction = %#v", result.Transaction)
	}
	if len(events) < 4 || events[0] != "ready:"+plan.ID || events[1] != "tx:running" || events[2] != "start:tool" {
		t.Fatalf("callback order = %#v", events)
	}
	if _, err := store.LoadPlan(plan.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plan was not consumed: %v", err)
	}
}

func TestExecutorReadinessCallbackFailureDoesNotConsumePlan(t *testing.T) {
	catalog := model.Catalog{Version: 1}
	inventory := model.Inventory{Items: map[string]model.InventoryItem{}}
	plan := sealedPlan(t, catalog, inventory)
	store := state.NewStore(t.TempDir())
	if err := store.SavePlan(state.SavedPlan{Plan: plan}); err != nil {
		t.Fatal(err)
	}
	executor := Executor{
		Catalog:     catalog,
		Store:       store,
		Inventory:   func(context.Context) (model.Inventory, error) { return inventory, nil },
		OnPlanReady: func(model.Plan) error { return errors.New("progress sink unavailable") },
	}
	_, err := executor.Execute(context.Background(), Request{PlanID: plan.ID, Digest: plan.Digest, Confirmed: true})
	if err == nil || !strings.Contains(err.Error(), "progress sink unavailable") {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := store.LoadPlan(plan.ID); err != nil {
		t.Fatalf("plan was consumed after readiness callback failure: %v", err)
	}
}

type fakeReviewedCommandRunner struct{}

func (fakeReviewedCommandRunner) Run(context.Context, runner.Invocation) runner.Result {
	return runner.Result{ExitCode: 0, Stdout: "ok"}
}
