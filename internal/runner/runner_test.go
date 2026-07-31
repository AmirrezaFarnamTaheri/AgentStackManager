package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/model"
)

type fakeCommandRunner struct {
	calls []Invocation
	fail  map[string]error
}

func (f *fakeCommandRunner) Run(_ context.Context, invocation Invocation) Result {
	f.calls = append(f.calls, invocation)
	key := invocation.Command
	if len(invocation.Args) > 0 {
		key += " " + invocation.Args[0]
	}
	if err := f.fail[key]; err != nil {
		return Result{ExitCode: 1, Err: err, Stderr: err.Error()}
	}
	return Result{ExitCode: 0, Stdout: "ok"}
}

type fakeSkillInstaller struct{ calls int }

func (f *fakeSkillInstaller) Install(context.Context, model.Component) Result {
	f.calls++
	return Result{ExitCode: 0, Stdout: "skills installed"}
}

type fakePathRefresher struct {
	calls int
	err   error
}

func (f *fakePathRefresher) Refresh() error {
	f.calls++
	return f.err
}

func TestDryRunExecutesNothing(t *testing.T) {
	commands := &fakeCommandRunner{}
	engine := Engine{Commands: commands, Skills: &fakeSkillInstaller{}}
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "git", Name: "Git", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{DryRun: true})
	if len(commands.calls) != 0 {
		t.Fatalf("dry run executed %d commands", len(commands.calls))
	}
	if tx.Status != model.TransactionPlanned {
		t.Fatalf("expected planned, got %s", tx.Status)
	}
}

func TestWingetInstallUsesNoUpgradeAndAcceptanceFlags(t *testing.T) {
	commands := &fakeCommandRunner{}
	engine := Engine{Commands: commands, Skills: &fakeSkillInstaller{}}
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "git", Name: "Git", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionSucceeded {
		t.Fatalf("unexpected status %s", tx.Status)
	}
	if len(commands.calls) != 1 {
		t.Fatalf("expected one command, got %d", len(commands.calls))
	}
	args := commands.calls[0].Args
	required := []string{"install", "--id", "Git.Git", "--exact", "--silent", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity", "--no-upgrade"}
	if !equalStrings(args, required) {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestFailureIsRecordedAndDoesNotInventRollbackOfPreexistingSoftware(t *testing.T) {
	commands := &fakeCommandRunner{fail: map[string]error{"winget install": assertErr("boom")}}
	engine := Engine{Commands: commands, Skills: &fakeSkillInstaller{}}
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "git", Name: "Git", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}},
		{ComponentID: "memory-mcp", Name: "Memory", Kind: model.ActionConfigure, Install: model.InstallSpec{Kind: model.InstallRouter}},
	}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionFailed {
		t.Fatalf("expected failed, got %s", tx.Status)
	}
	if tx.Actions[0].Error == "" {
		t.Fatal("failure not recorded")
	}
	if len(commands.calls) != 1 {
		t.Fatalf("unexpected commands: %#v", commands.calls)
	}
}

func TestEngineRefreshesPathAfterSuccessfulWingetInstall(t *testing.T) {
	commands := &fakeCommandRunner{}
	paths := &fakePathRefresher{}
	engine := Engine{Commands: commands, Path: paths}
	plan := model.Plan{Actions: []model.PlanAction{{
		ComponentID: "node",
		Kind:        model.ActionInstall,
		Install:     model.InstallSpec{Kind: model.InstallWinget, WingetID: "OpenJS.NodeJS.LTS"},
	}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionSucceeded {
		t.Fatalf("unexpected transaction: %#v", tx)
	}
	if paths.calls != 1 {
		t.Fatalf("expected one PATH refresh, got %d", paths.calls)
	}
}

func TestEngineReportsPathRefreshFailure(t *testing.T) {
	commands := &fakeCommandRunner{}
	paths := &fakePathRefresher{err: assertErr("registry unavailable")}
	engine := Engine{Commands: commands, Path: paths}
	plan := model.Plan{Actions: []model.PlanAction{{
		ComponentID: "node",
		Kind:        model.ActionInstall,
		Install:     model.InstallSpec{Kind: model.InstallWinget, WingetID: "OpenJS.NodeJS.LTS"},
	}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionFailed || !strings.Contains(tx.Actions[0].Error, "refresh process PATH") {
		t.Fatalf("expected PATH refresh failure, got %#v", tx)
	}
}

func TestEngineExecutesRepairAction(t *testing.T) {
	commands := &fakeCommandRunner{}
	engine := Engine{Commands: commands, Path: &fakePathRefresher{}}
	plan := model.Plan{Actions: []model.PlanAction{{
		ComponentID: "gitleaks",
		Kind:        model.ActionRepair,
		Install:     model.InstallSpec{Kind: model.InstallWinget, WingetID: "Gitleaks.Gitleaks"},
	}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionSucceeded || len(commands.calls) != 1 {
		t.Fatalf("repair was not executed: tx=%#v calls=%#v", tx, commands.calls)
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEngineDoesNotRunDependentAfterPrerequisiteFailure(t *testing.T) {
	commands := &fakeCommandRunner{fail: map[string]error{"winget install": assertErr("node failed")}}
	catalog := model.Catalog{Version: 1, Components: []model.Component{
		{ID: "node", Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "OpenJS.NodeJS.LTS"}},
		{ID: "ast-grep", DependsOn: []string{"node"}, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "@ast-grep/cli"}},
	}}
	engine := Engine{Commands: commands, Path: &fakePathRefresher{}, Catalog: catalog}
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "node", Kind: model.ActionInstall, Install: catalog.Components[0].Install},
		{ComponentID: "ast-grep", Kind: model.ActionInstall, Install: catalog.Components[1].Install},
	}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if len(commands.calls) != 1 {
		t.Fatalf("dependent command must not run after prerequisite failure: %#v", commands.calls)
	}
	if len(tx.Actions) != 2 || !strings.Contains(tx.Actions[1].Error, "dependency node failed") {
		t.Fatalf("expected explicit dependent skip record, got %#v", tx.Actions)
	}
}

type fakeVerifier struct {
	ok bool
}

func (v fakeVerifier) Verify(context.Context, model.Component, model.PlanAction, Result) VerificationResult {
	return VerificationResult{OK: v.ok, Message: "probe result"}
}

func TestEngineFailsWhenPostconditionFails(t *testing.T) {
	commands := &fakeCommandRunner{}
	catalog := model.Catalog{Components: []model.Component{{ID: "git", Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}}
	engine := Engine{Commands: commands, Catalog: catalog, Verifier: fakeVerifier{ok: false}}
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "git", Kind: model.ActionInstall, Install: catalog.Components[0].Install}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{})
	if tx.Status != model.TransactionFailed || tx.Actions[0].Verified || !strings.Contains(tx.Actions[0].Error, "postcondition") {
		t.Fatalf("expected postcondition failure, got %#v", tx)
	}
}

func TestEnginePublishesIncrementalJournal(t *testing.T) {
	commands := &fakeCommandRunner{}
	catalog := model.Catalog{Components: []model.Component{{ID: "git", Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "Git.Git"}}}}
	engine := Engine{Commands: commands, Catalog: catalog, Verifier: fakeVerifier{ok: true}}
	var snapshots []model.Transaction
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "git", Kind: model.ActionInstall, Install: catalog.Components[0].Install}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{OnUpdate: func(value model.Transaction) error {
		snapshots = append(snapshots, value)
		return nil
	}})
	if tx.Status != model.TransactionSucceeded {
		t.Fatalf("unexpected transaction %#v", tx)
	}
	if len(snapshots) < 3 || snapshots[0].Status != model.TransactionRunning || snapshots[len(snapshots)-1].Status != model.TransactionSucceeded {
		t.Fatalf("journal did not receive running/action/final snapshots: %#v", snapshots)
	}
}

func TestExecRunnerHelperProcess(t *testing.T) {
	mode := os.Getenv("AGENTSTACK_RUNNER_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "output":
		_, _ = os.Stdout.WriteString(strings.Repeat("x", 4096))
	case "sleep":
		time.Sleep(time.Minute)
	case "exit":
		_, _ = os.Stderr.WriteString("failed")
		os.Exit(7)
	}
}

func TestExecRunnerBoundsOutputAndReportsTruncation(t *testing.T) {
	result := (ExecRunner{}).Run(context.Background(), Invocation{Command: os.Args[0], Args: []string{"-test.run=^TestExecRunnerHelperProcess$"}, Env: map[string]string{"AGENTSTACK_RUNNER_HELPER": "output"}, MaxOutputBytes: 128})
	if result.Err != nil || !result.Truncated || len(result.Stdout) != 128 {
		t.Fatalf("unexpected bounded output result: %+v len=%d", result, len(result.Stdout))
	}
}

func TestExecRunnerAppliesTimeoutAndExitCode(t *testing.T) {
	result := (ExecRunner{}).Run(context.Background(), Invocation{Command: os.Args[0], Args: []string{"-test.run=^TestExecRunnerHelperProcess$"}, Env: map[string]string{"AGENTSTACK_RUNNER_HELPER": "sleep"}, Timeout: 30 * time.Millisecond})
	if result.Err == nil || result.ExitCode != -1 {
		t.Fatalf("expected timeout result: %+v", result)
	}
	failed := (ExecRunner{}).Run(context.Background(), Invocation{Command: os.Args[0], Args: []string{"-test.run=^TestExecRunnerHelperProcess$"}, Env: map[string]string{"AGENTSTACK_RUNNER_HELPER": "exit"}})
	if failed.ExitCode != 7 || !strings.Contains(failed.Stderr, "failed") {
		t.Fatalf("expected real exit code and stderr: %+v", failed)
	}
}

func TestEnginePublishesEveryNonInstallerAndDependencyCheckpoint(t *testing.T) {
	commands := &fakeCommandRunner{fail: map[string]error{"winget install": assertErr("node failed")}}
	catalog := model.Catalog{Components: []model.Component{
		{ID: "node", Install: model.InstallSpec{Kind: model.InstallWinget, WingetID: "OpenJS.NodeJS.LTS"}},
		{ID: "ast-grep", DependsOn: []string{"node"}, Install: model.InstallSpec{Kind: model.InstallNPMGlobal, Package: "@ast-grep/cli@1.0.0"}},
	}}
	engine := Engine{Commands: commands, Catalog: catalog, Path: &fakePathRefresher{}}
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "preserved", Kind: model.ActionKeep},
		{ComponentID: "node", Kind: model.ActionInstall, Install: catalog.Components[0].Install},
		{ComponentID: "ast-grep", Kind: model.ActionInstall, Install: catalog.Components[1].Install},
	}}
	var counts []int
	tx := engine.Apply(context.Background(), plan, ApplyOptions{OnUpdate: func(value model.Transaction) error {
		counts = append(counts, len(value.Actions))
		return nil
	}})
	if tx.Status != model.TransactionFailed {
		t.Fatalf("expected failed transaction, got %#v", tx)
	}
	want := []int{0, 1, 2, 3, 3}
	if !equalInts(counts, want) {
		t.Fatalf("expected every action plus final checkpoint, got %#v", counts)
	}
}

func TestEngineReportsJournalFailure(t *testing.T) {
	calls := 0
	engine := Engine{Commands: &fakeCommandRunner{}}
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "preserved", Kind: model.ActionKeep}}}
	tx := engine.Apply(context.Background(), plan, ApplyOptions{OnUpdate: func(model.Transaction) error {
		calls++
		if calls == 2 {
			return assertErr("disk full")
		}
		return nil
	}})
	if tx.Status != model.TransactionFailed || len(tx.Actions) != 2 || !strings.Contains(tx.Actions[1].Error, "disk full") {
		t.Fatalf("expected explicit journal failure, got %#v", tx)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
