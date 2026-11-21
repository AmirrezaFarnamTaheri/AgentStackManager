package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/model"
)

type concurrencyRunner struct {
	mu     sync.Mutex
	active int
	peak   int
	starts []string
	delay  time.Duration
}

func (r *concurrencyRunner) Run(_ context.Context, invocation Invocation) Result {
	r.mu.Lock()
	r.active++
	if r.active > r.peak {
		r.peak = r.active
	}
	r.starts = append(r.starts, invocation.Command)
	r.mu.Unlock()
	time.Sleep(r.delay)
	r.mu.Lock()
	r.active--
	r.mu.Unlock()
	return Result{ExitCode: 0}
}

func TestParallelEngineOverlapsIndependentInstallerFamilies(t *testing.T) {
	runner := &concurrencyRunner{delay: 40 * time.Millisecond}
	engine := Engine{Commands: runner, MaxParallel: 4, Catalog: model.Catalog{Components: []model.Component{{ID: "npm"}, {ID: "uv"}}}}
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "npm", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "pkg-a"}},
		{ComponentID: "uv", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallUVTool, Package: "pkg-b"}},
	}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionSucceeded || runner.peak < 2 {
		t.Fatalf("tx=%#v peak=%d", tx, runner.peak)
	}
	if len(tx.Actions) != 2 || tx.Actions[0].ComponentID != "npm" || tx.Actions[1].ComponentID != "uv" {
		t.Fatalf("action order=%#v", tx.Actions)
	}
}

func TestParallelEngineSerializesSameInstallerFamily(t *testing.T) {
	runner := &concurrencyRunner{delay: 30 * time.Millisecond}
	engine := Engine{Commands: runner, MaxParallel: 4}
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "a", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "a"}},
		{ComponentID: "b", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "b"}},
	}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionSucceeded || runner.peak != 1 {
		t.Fatalf("tx=%#v peak=%d", tx, runner.peak)
	}
}

func TestParallelEngineWaitsForDependencies(t *testing.T) {
	runner := &concurrencyRunner{delay: 10 * time.Millisecond}
	catalog := model.Catalog{Components: []model.Component{
		{ID: "foundation"},
		{ID: "dependent", DependsOn: []string{"foundation"}},
	}}
	engine := Engine{Commands: runner, MaxParallel: 4, Catalog: catalog}
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "foundation", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "foundation"}},
		{ComponentID: "dependent", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallUVTool, Package: "dependent"}},
	}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionSucceeded {
		t.Fatalf("tx=%#v", tx)
	}
	if len(runner.starts) != 2 || runner.starts[0] != "npm" || runner.starts[1] != "uv" {
		t.Fatalf("start order=%#v", runner.starts)
	}
}
