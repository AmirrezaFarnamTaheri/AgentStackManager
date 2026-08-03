package reviewedplan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/runner"
	"github.com/agentstack/agentstack/internal/state"
)

var ErrConfirmationRequired = errors.New("explicit confirmation is required before applying changes")
var ErrPlanStale = errors.New("reviewed plan is stale; rebuild and review the plan")
var ErrPlanMismatch = errors.New("reviewed plan digest does not match")

type Request struct {
	PlanID    string
	Digest    string
	Confirmed bool
}

type Result struct {
	Saved       state.SavedPlan
	Transaction model.Transaction
}

// Executor owns the complete authorization-to-journal transaction boundary for
// a reviewed plan. Callers supply inventory and ownership hooks, but cannot
// bypass digest, expiry, catalog, inventory, lease, or single-use checks.
type Executor struct {
	Catalog                  model.Catalog
	Store                    state.Store
	Installer                runner.Engine
	Inventory                func(context.Context) (model.Inventory, error)
	RecordSuccessfulInstalls func(model.Plan, model.Transaction) error
	LogEvent                 func(state.Event)
	LeaseTTL                 time.Duration
	Now                      func() time.Time
}

func (e Executor) Execute(ctx context.Context, request Request) (Result, error) {
	if !request.Confirmed {
		return Result{}, ErrConfirmationRequired
	}
	leaseTTL := e.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = 6 * time.Hour
	}
	lease, err := e.Store.AcquireLease("mutation", leaseTTL)
	if err != nil {
		return Result{}, err
	}
	defer lease.Close()
	e.emit(state.Event{Level: "info", Type: "apply.started", CorrelationID: request.PlanID})

	saved, err := e.Store.LoadPlan(request.PlanID)
	if err != nil {
		return Result{}, err
	}
	plan := saved.Plan
	if plan.ID != request.PlanID {
		return Result{}, ErrPlanMismatch
	}
	if request.Digest == "" || request.Digest != plan.Digest {
		return Result{}, ErrPlanMismatch
	}
	computedDigest, err := planner.PlanDigest(plan)
	if err != nil || computedDigest != plan.Digest {
		return Result{}, ErrPlanMismatch
	}
	if e.now().After(plan.ExpiresAt) {
		return Result{}, ErrPlanStale
	}
	catalogHash, err := integrity.DigestJSON(e.Catalog)
	if err != nil || catalogHash != plan.CatalogHash {
		return Result{}, ErrPlanStale
	}
	if e.Inventory == nil {
		return Result{}, errors.New("reviewed plan executor inventory loader is unavailable")
	}
	current, err := e.Inventory(ctx)
	if err != nil {
		return Result{}, err
	}
	inventoryHash, err := planner.InventoryDigest(current)
	if err != nil || inventoryHash != plan.InventoryHash {
		return Result{}, ErrPlanStale
	}
	if err := lease.Touch(); err != nil {
		return Result{}, err
	}
	// Consume approval before first external mutation. A partial or failed run
	// must be reviewed again against resulting machine state.
	if err := e.Store.DeletePlan(request.PlanID); err != nil {
		return Result{}, fmt.Errorf("consume reviewed plan before mutation: %w", err)
	}

	transaction := e.Installer.Apply(ctx, plan, runner.ApplyOptions{
		CorrelationID: plan.ID,
		OnUpdate: func(tx model.Transaction) error {
			if err := lease.Touch(); err != nil {
				return err
			}
			return e.Store.SaveTransaction(tx)
		},
	})
	result := Result{Saved: saved, Transaction: transaction}
	if err := e.Store.SaveTransaction(transaction); err != nil {
		return result, err
	}
	if e.RecordSuccessfulInstalls != nil {
		if err := e.RecordSuccessfulInstalls(plan, transaction); err != nil {
			return result, err
		}
	}
	if transaction.Status == model.TransactionFailed {
		return result, fmt.Errorf("one or more selected installations failed; see transaction %s", transaction.ID)
	}
	return result, nil
}

func (e Executor) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

func (e Executor) emit(event state.Event) {
	if e.LogEvent != nil {
		e.LogEvent(event)
	}
}
