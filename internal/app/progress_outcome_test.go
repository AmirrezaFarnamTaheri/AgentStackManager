package app

import (
	"errors"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func TestApplyProgressSeparatesProcessedFromOutcomeCounts(t *testing.T) {
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "ok", Name: "OK", Kind: model.ActionInstall},
		{ComponentID: "bad", Name: "Bad", Kind: model.ActionRepair},
		{ComponentID: "later", Name: "Later", Kind: model.ActionConfigure},
		{ComponentID: "kept", Name: "Kept", Kind: model.ActionKeep},
	}}
	tracker := newApplyProgressTracker(plan)
	tracker.updateTransaction(model.Transaction{Actions: []model.TransactionAction{
		{ComponentID: "ok", Kind: model.ActionInstall, Verified: true},
		{ComponentID: "bad", Kind: model.ActionRepair, Error: "failed"},
	}})
	progress := tracker.complete()

	if progress.Total != 3 || progress.Processed != 3 || progress.Completed != 3 {
		t.Fatalf("progress totals = %#v", progress)
	}
	if progress.Succeeded != 1 || progress.Failed != 1 || progress.Skipped != 1 {
		t.Fatalf("progress outcome counts = %#v", progress)
	}
}

func TestApplyProgressRouterFailureCountsAsFailed(t *testing.T) {
	plan := model.Plan{Actions: []model.PlanAction{{
		ComponentID: "router", Name: "Router", Kind: model.ActionConfigure,
		Install: model.InstallSpec{Kind: model.InstallRouter},
	}}}
	tracker := newApplyProgressTracker(plan)
	tracker.startRouter()
	progress := tracker.finishRouter(errors.New("failed"))
	if progress.Processed != 1 || progress.Failed != 1 || progress.Succeeded != 0 {
		t.Fatalf("router progress = %#v", progress)
	}
}
