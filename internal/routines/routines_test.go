package routines

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeExecutor struct {
	calls []Step
	fail  string
}

func (f *fakeExecutor) Execute(_ context.Context, _ Routine, step Step) (any, error) {
	f.calls = append(f.calls, step)
	if step.ID == f.fail {
		return nil, fmt.Errorf("boom")
	}
	return map[string]string{"step": step.ID}, nil
}

func TestDailyRoutineDueAndRun(t *testing.T) {
	now := time.Date(2026, 8, 3, 7, 30, 0, 0, time.UTC)
	manager := New(t.TempDir())
	manager.Clock = func() time.Time { return now }
	routine, err := manager.Put(Routine{ID: "morning", Name: "Morning brief", Enabled: true, Schedule: Schedule{Kind: ScheduleDaily, At: "08:00", Timezone: "UTC"}, Steps: []Step{{ID: "inventory", Kind: StepInventory}, {ID: "context", Kind: StepContextScan}}})
	if err != nil {
		t.Fatal(err)
	}
	if !routine.NextRun.Equal(time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected next run: %s", routine.NextRun)
	}
	due, err := manager.Due(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("not due yet: %#v", due)
	}
	now = time.Date(2026, 8, 3, 8, 1, 0, 0, time.UTC)
	due, err = manager.Due(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("expected due routine: %#v", due)
	}
	executor := &fakeExecutor{}
	report, err := manager.Run(context.Background(), "morning", true, executor)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != RunSucceeded || len(executor.calls) != 2 {
		t.Fatalf("unexpected report: %#v calls=%d", report, len(executor.calls))
	}
	updated, err := manager.Get("morning")
	if err != nil {
		t.Fatal(err)
	}
	if !updated.NextRun.After(now) {
		t.Fatalf("next run did not advance: %s", updated.NextRun)
	}
}

func TestRoutineRunRequiresConfirmationAndStopsOnFailure(t *testing.T) {
	manager := New(t.TempDir())
	if _, err := manager.Put(Routine{ID: "r", Name: "R", Enabled: true, Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "a", Kind: StepMemorySearch}, {ID: "b", Kind: StepCommand, Command: "test-tool"}, {ID: "c", Kind: StepInventory}}}); err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{fail: "b"}
	if _, err := manager.Run(context.Background(), "r", false, executor); err == nil {
		t.Fatal("expected confirmation error")
	}
	report, err := manager.Run(context.Background(), "r", true, executor)
	if err == nil {
		t.Fatal("expected failure")
	}
	if report.Status != RunFailed || len(executor.calls) != 2 {
		t.Fatalf("expected stop on first failure: %#v calls=%d", report, len(executor.calls))
	}
}

func TestWeekdayScheduleSkipsWeekend(t *testing.T) {
	friday := time.Date(2026, 8, 7, 18, 0, 0, 0, time.UTC)
	next, err := NextRun(Schedule{Kind: ScheduleWeekdays, At: "09:00", Timezone: "UTC"}, friday, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %s want %s", next, want)
	}
}

func TestLegacyRoutinesMigrateToVersionedEnvelope(t *testing.T) {
	root := t.TempDir()
	legacy := `{"legacy":{"id":"legacy","name":"Legacy","enabled":true,"schedule":{"kind":"manual"},"steps":[{"id":"inventory","kind":"inventory"}],"createdAt":"2026-08-03T00:00:00Z","updatedAt":"2026-08-03T00:00:00Z"}}`
	if err := os.WriteFile(filepath.Join(root, "routines.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(root)
	if _, err := manager.Get("legacy"); err != nil {
		t.Fatalf("legacy routine should load: %v", err)
	}
	if _, err := manager.Put(Routine{ID: "new", Name: "New", Enabled: true, Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "scan", Kind: StepContextScan}}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "routines.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schema": "`+routinesSchema+`"`) || !strings.Contains(string(data), `"version": 1`) {
		t.Fatalf("routines were not migrated to a versioned envelope: %s", data)
	}
}

func TestRoutinesRejectUnsupportedSchemaVersion(t *testing.T) {
	root := t.TempDir()
	data := `{"schema":"agentstack.routines","version":99,"items":{}}`
	if err := os.WriteFile(filepath.Join(root, "routines.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).List(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported schema version error, got %v", err)
	}
}

func TestRoutinesRejectInvalidPersistedDefinition(t *testing.T) {
	root := t.TempDir()
	data := `{"schema":"agentstack.routines","version":1,"items":{"invalid":{"id":"invalid","name":"Invalid","enabled":true,"schedule":{"kind":"manual"},"steps":[{"id":"command","kind":"command","command":"tool","args":["--token=secret-value"]}]}}}`
	if err := os.WriteFile(filepath.Join(root, "routines.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root).List(); err == nil || !strings.Contains(err.Error(), "persisted routine") {
		t.Fatalf("expected invalid persisted routine rejection, got %v", err)
	}
}

func TestRoutineLoadReconcilesPersistedRunReceipt(t *testing.T) {
	root := t.TempDir()
	manager := New(root)
	created, err := manager.Put(Routine{ID: "daily", Name: "Daily", Enabled: true, Schedule: Schedule{Kind: ScheduleDaily, At: "08:00", Timezone: "UTC"}, Steps: []Step{{ID: "scan", Kind: StepContextScan}}})
	if err != nil {
		t.Fatal(err)
	}
	finished := created.UpdatedAt.Add(2 * time.Hour)
	report := RunReport{Schema: runReportSchema, Version: runReportVersion, ID: "run-recovery", RoutineID: "daily", Status: RunSucceeded, StartedAt: finished.Add(-time.Minute), FinishedAt: finished}
	if err := writeJSON(filepath.Join(root, "runs", report.ID+".json"), report); err != nil {
		t.Fatal(err)
	}

	reconciled, err := manager.Get("daily")
	if err != nil {
		t.Fatal(err)
	}
	if !reconciled.LastRun.Equal(finished) {
		t.Fatalf("last run was not reconciled: got %s want %s", reconciled.LastRun, finished)
	}
	if !reconciled.NextRun.After(finished) {
		t.Fatalf("next run was not advanced: %s", reconciled.NextRun)
	}
}

func TestListRunsFiltersAndLimitsNewestFirst(t *testing.T) {
	root := t.TempDir()
	manager := New(root)
	if err := manager.ensure(); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 3, 8, 0, 0, 0, time.UTC)
	for index, routineID := range []string{"a", "b", "a"} {
		report := RunReport{Schema: runReportSchema, Version: runReportVersion, ID: fmt.Sprintf("run-%d", index), RoutineID: routineID, Status: RunSucceeded, StartedAt: base.Add(time.Duration(index) * time.Minute), FinishedAt: base.Add(time.Duration(index)*time.Minute + time.Second)}
		if err := writeJSON(filepath.Join(root, "runs", report.ID+".json"), report); err != nil {
			t.Fatal(err)
		}
	}
	reports, err := manager.ListRuns("a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].ID != "run-2" {
		t.Fatalf("unexpected run history: %#v", reports)
	}
}

func TestRoutineRunReceiptsRedactSecrets(t *testing.T) {
	manager := New(t.TempDir())
	if _, err := manager.Put(Routine{ID: "secret", Name: "Secret", Enabled: true, Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "emit", Kind: StepCommand, Command: "test-tool"}}}); err != nil {
		t.Fatal(err)
	}
	executor := executorFunc(func(context.Context, Routine, Step) (any, error) {
		return map[string]any{"stdout": "token=super-secret-value", "nested": map[string]any{"password": "hunter2"}}, fmt.Errorf("authorization: Bearer abcdefghijklmnopqrstuvwxyz")
	})
	report, err := manager.Run(context.Background(), "secret", true, executor)
	if err == nil {
		t.Fatal("expected executor failure")
	}
	encoded := fmt.Sprintf("%#v", report)
	for _, secret := range []string{"super-secret-value", "hunter2", "abcdefghijklmnopqrstuvwxyz"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("secret persisted in routine receipt: %s", encoded)
		}
	}
	runs, listErr := manager.ListRuns("secret", 1)
	if listErr != nil || len(runs) != 1 {
		t.Fatalf("runs=%#v err=%v", runs, listErr)
	}
	stored := fmt.Sprintf("%#v", runs[0])
	if strings.Contains(stored, "super-secret-value") || !strings.Contains(stored, "[REDACTED]") {
		t.Fatalf("stored receipt was not redacted: %s", stored)
	}
}

type executorFunc func(context.Context, Routine, Step) (any, error)

func (f executorFunc) Execute(ctx context.Context, routine Routine, step Step) (any, error) {
	return f(ctx, routine, step)
}

func TestRoutineRunHasOverallDeadline(t *testing.T) {
	manager := New(t.TempDir())
	manager.RunTimeout = 10 * time.Millisecond
	if _, err := manager.Put(Routine{ID: "bounded", Name: "Bounded", Enabled: true, Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "wait", Kind: StepCommand, Command: "test-tool"}}}); err != nil {
		t.Fatal(err)
	}
	executor := executorFunc(func(ctx context.Context, _ Routine, _ Step) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	report, err := manager.Run(context.Background(), "bounded", true, executor)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected routine deadline, got %v", err)
	}
	if report.Status != RunFailed || !strings.Contains(report.Error, context.DeadlineExceeded.Error()) {
		t.Fatalf("deadline was not recorded in report: %#v", report)
	}
}

func TestRoutineRejectsPersistedSecretArguments(t *testing.T) {
	manager := New(t.TempDir())
	for _, step := range []Step{
		{ID: "flag", Kind: StepCommand, Command: "tool", Args: []string{"--password", "plain-secret"}},
		{ID: "assignment", Kind: StepCommand, Command: "tool", Args: []string{"api_key=plain-secret"}},
		{ID: "param", Kind: StepMemorySearch, Params: map[string]string{"access_token": "plain-secret"}},
	} {
		_, err := manager.Put(Routine{ID: "secret-" + step.ID, Name: "Secret", Enabled: true, Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{step}})
		if err == nil {
			t.Fatalf("routine accepted persisted secret material: %#v", step)
		}
	}
	if _, err := manager.Put(Routine{
		ID: "secret-reference", Name: "Reference", Enabled: true, Schedule: Schedule{Kind: ScheduleManual},
		Steps: []Step{{ID: "run", Kind: StepCommand, Command: "tool", Args: []string{"--token-env", "TOOL_TOKEN"}, Params: map[string]string{"credential_file": "secrets/tool.token"}}},
	}); err != nil {
		t.Fatalf("credential references should remain valid: %v", err)
	}
}

func TestRoutineAdmissionBoundsCommandShapeAndDefinitionSize(t *testing.T) {
	manager := New(t.TempDir())
	cases := []Routine{
		{ID: "missing-command", Name: "Missing", Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "run", Kind: StepCommand}}},
		{ID: "confused", Name: "Confused", Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "scan", Kind: StepContextScan, Command: "tool"}}},
		{ID: "too-many-args", Name: "Args", Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "run", Kind: StepCommand, Command: "tool", Args: make([]string, maxRoutineArguments+1)}}},
		{ID: "huge-param", Name: "Param", Schedule: Schedule{Kind: ScheduleManual}, Steps: []Step{{ID: "scan", Kind: StepContextScan, Params: map[string]string{"root": strings.Repeat("x", maxRoutineParamValue+1)}}}},
	}
	for _, routine := range cases {
		if _, err := manager.Put(routine); err == nil {
			t.Fatalf("invalid routine was accepted: %s", routine.ID)
		}
	}
	if _, err := manager.Put(Routine{
		ID: "bad-reference", Name: "Bad ref", Schedule: Schedule{Kind: ScheduleManual},
		Steps: []Step{{ID: "run", Kind: StepCommand, Command: "tool", Args: []string{"--token-env", "not valid"}}},
	}); err == nil {
		t.Fatal("invalid environment reference was accepted")
	}
}
