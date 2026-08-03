package routines

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/redact"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const (
	maxRoutineSteps       = 32
	maxRoutineArguments   = 128
	maxRoutineParameters  = 64
	maxRoutineDefinition  = 1 << 20
	maxRoutineCommand     = 4 << 10
	maxRoutineArgument    = 16 << 10
	maxRoutineParamKey    = 256
	maxRoutineParamValue  = 64 << 10
	maxRoutineStoreBytes  = 64 << 20
	maxRoutineReportBytes = 16 << 20
	MaxRoutineRunDuration = 24 * time.Hour
	maxStoredRuns         = 4096
	routinesSchema        = "agentstack.routines"
	routinesStoreVersion  = 1
	runReportSchema       = "agentstack.routine-run"
	runReportVersion      = 1
)

type routinesEnvelope struct {
	Schema  string             `json:"schema"`
	Version int                `json:"version"`
	Items   map[string]Routine `json:"items"`
}

var ErrConfirmationRequired = errors.New("explicit confirmation is required before running a routine")

type Executor interface {
	Execute(context.Context, Routine, Step) (any, error)
}

type Manager struct {
	Root       string
	Clock      func() time.Time
	RunTimeout time.Duration
}

func New(root string) Manager {
	return Manager{Root: root, Clock: func() time.Time { return time.Now().UTC() }, RunTimeout: MaxRoutineRunDuration}
}

func (m Manager) now() time.Time {
	if m.Clock == nil {
		return time.Now().UTC()
	}
	return m.Clock().UTC()
}

func (m Manager) runTimeout() time.Duration {
	if m.RunTimeout <= 0 || m.RunTimeout > MaxRoutineRunDuration {
		return MaxRoutineRunDuration
	}
	return m.RunTimeout
}

func (m Manager) Put(routine Routine) (Routine, error) {
	if err := validateRoutine(routine); err != nil {
		return Routine{}, err
	}
	routines, err := m.load()
	if err != nil {
		return Routine{}, err
	}
	now := m.now()
	if previous, ok := routines[routine.ID]; ok {
		routine.CreatedAt = previous.CreatedAt
		if routine.LastRun.IsZero() {
			routine.LastRun = previous.LastRun
		}
	} else {
		routine.CreatedAt = now
	}
	routine.UpdatedAt = now
	routine.Steps = cloneSteps(routine.Steps)
	routine.NextRun, err = nextForRoutine(routine, now)
	if err != nil {
		return Routine{}, err
	}
	routines[routine.ID] = routine
	if err := m.save(routines); err != nil {
		return Routine{}, err
	}
	return routine, nil
}

func (m Manager) Get(id string) (Routine, error) {
	routines, err := m.load()
	if err != nil {
		return Routine{}, err
	}
	routine, ok := routines[id]
	if !ok {
		return Routine{}, fmt.Errorf("unknown routine %q", id)
	}
	return routine, nil
}

func (m Manager) List() ([]Routine, error) {
	routines, err := m.load()
	if err != nil {
		return nil, err
	}
	result := make([]Routine, 0, len(routines))
	for _, routine := range routines {
		result = append(result, routine)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].NextRun.Equal(result[j].NextRun) {
			return result[i].ID < result[j].ID
		}
		if result[i].NextRun.IsZero() {
			return false
		}
		if result[j].NextRun.IsZero() {
			return true
		}
		return result[i].NextRun.Before(result[j].NextRun)
	})
	return result, nil
}

func (m Manager) Delete(id string) error {
	routines, err := m.load()
	if err != nil {
		return err
	}
	if _, ok := routines[id]; !ok {
		return nil
	}
	delete(routines, id)
	return m.save(routines)
}

func (m Manager) Due(now time.Time) ([]Routine, error) {
	if now.IsZero() {
		now = m.now()
	}
	all, err := m.List()
	if err != nil {
		return nil, err
	}
	due := make([]Routine, 0)
	for _, routine := range all {
		if routine.Enabled && !routine.NextRun.IsZero() && !routine.NextRun.After(now.UTC()) {
			due = append(due, routine)
		}
	}
	return due, nil
}

func (m Manager) Run(ctx context.Context, id string, confirmed bool, executor Executor) (RunReport, error) {
	if !confirmed {
		return RunReport{}, ErrConfirmationRequired
	}
	if executor == nil {
		return RunReport{}, fmt.Errorf("routine executor is required")
	}
	routines, err := m.load()
	if err != nil {
		return RunReport{}, err
	}
	routine, ok := routines[id]
	if !ok {
		return RunReport{}, fmt.Errorf("unknown routine %q", id)
	}
	if !routine.Enabled {
		return RunReport{}, fmt.Errorf("routine %q is disabled", id)
	}
	runCtx, cancel := context.WithTimeout(ctx, m.runTimeout())
	defer cancel()
	started := m.now()
	report := RunReport{
		Schema:      runReportSchema,
		Version:     runReportVersion,
		ID:          runID(started),
		RoutineID:   routine.ID,
		WorkspaceID: routine.WorkspaceID,
		Status:      RunRunning,
		StartedAt:   started,
		Steps:       make([]StepReport, 0, len(routine.Steps)),
	}
	for _, step := range routine.Steps {
		stepStarted := m.now()
		output, stepErr := executor.Execute(runCtx, routine, step)
		stepReport := StepReport{StepID: step.ID, Kind: step.Kind, StartedAt: stepStarted, FinishedAt: m.now(), Output: redact.Value(output)}
		if stepErr != nil {
			stepReport.Error = redact.Text(stepErr.Error())
			report.Steps = append(report.Steps, stepReport)
			report.Status = RunFailed
			report.Error = redact.Text(fmt.Sprintf("step %s failed: %v", step.ID, stepErr))
			report.FinishedAt = m.now()
			routine.LastRun = report.FinishedAt
			routine.UpdatedAt = report.FinishedAt
			routine.NextRun, err = nextForRoutine(routine, report.FinishedAt)
			if err != nil {
				return report, errors.Join(stepErr, fmt.Errorf("advance failed routine schedule: %w", err))
			}
			routines[id] = routine
			if persistErr := m.persistRunAndRoutines(report, routines); persistErr != nil {
				return report, errors.Join(stepErr, persistErr)
			}
			return report, stepErr
		}
		report.Steps = append(report.Steps, stepReport)
	}
	report.Status = RunSucceeded
	report.FinishedAt = m.now()
	routine.LastRun = report.FinishedAt
	routine.UpdatedAt = report.FinishedAt
	routine.NextRun, err = nextForRoutine(routine, report.FinishedAt)
	if err != nil {
		return report, err
	}
	routines[id] = routine
	if err := m.persistRunAndRoutines(report, routines); err != nil {
		return report, err
	}
	return report, nil
}

func (m Manager) persistRunAndRoutines(report RunReport, routines map[string]Routine) error {
	if err := m.ensure(); err != nil {
		return err
	}
	report.Schema = runReportSchema
	report.Version = runReportVersion
	if err := writeJSON(filepath.Join(m.Root, "runs", report.ID+".json"), report); err != nil {
		return err
	}
	if err := m.save(routines); err != nil {
		return err
	}
	return m.pruneRuns(maxStoredRuns)
}

func (m Manager) ListRuns(routineID string, limit int) ([]RunReport, error) {
	if err := m.ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(m.Root, "runs"))
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxStoredRuns {
		limit = maxStoredRuns
	}
	reports := make([]RunReport, 0, min(limit, len(entries)))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		report, err := m.loadRunReport(filepath.Join(m.Root, "runs", entry.Name()))
		if err != nil {
			return nil, err
		}
		if routineID == "" || report.RoutineID == routineID {
			reports = append(reports, report)
		}
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].FinishedAt.Equal(reports[j].FinishedAt) {
			return reports[i].ID > reports[j].ID
		}
		return reports[i].FinishedAt.After(reports[j].FinishedAt)
	})
	if len(reports) > limit {
		reports = reports[:limit]
	}
	return reports, nil
}

func (m Manager) loadRunReport(path string) (RunReport, error) {
	data, err := safefile.ReadBoundedRegular(path, maxRoutineReportBytes)
	if err != nil {
		return RunReport{}, err
	}
	var report RunReport
	if err := strictjson.Decode(data, &report); err != nil {
		return RunReport{}, fmt.Errorf("decode routine run %s: %w", filepath.Base(path), err)
	}
	if report.Schema != "" && report.Schema != runReportSchema {
		return RunReport{}, fmt.Errorf("unexpected routine run schema %q", report.Schema)
	}
	if report.Version != 0 && report.Version != runReportVersion {
		return RunReport{}, fmt.Errorf("unsupported routine run schema version %d", report.Version)
	}
	if report.ID == "" || report.RoutineID == "" || report.FinishedAt.IsZero() {
		return RunReport{}, fmt.Errorf("routine run receipt is incomplete: %s", filepath.Base(path))
	}
	return report, nil
}

func (m Manager) reconcileRuns(routines map[string]Routine) (bool, error) {
	reports, err := m.ListRuns("", maxStoredRuns)
	if err != nil {
		return false, err
	}
	changed := false
	seen := map[string]struct{}{}
	for _, report := range reports {
		if _, ok := seen[report.RoutineID]; ok {
			continue
		}
		seen[report.RoutineID] = struct{}{}
		routine, ok := routines[report.RoutineID]
		if !ok || !report.FinishedAt.After(routine.LastRun) {
			continue
		}
		routine.LastRun = report.FinishedAt
		routine.UpdatedAt = report.FinishedAt
		next, nextErr := nextForRoutine(routine, report.FinishedAt)
		if nextErr != nil {
			return false, nextErr
		}
		routine.NextRun = next
		routines[routine.ID] = routine
		changed = true
	}
	return changed, nil
}

func (m Manager) pruneRuns(keep int) error {
	entries, err := os.ReadDir(filepath.Join(m.Root, "runs"))
	if err != nil {
		return err
	}
	type candidate struct {
		path string
		when time.Time
	}
	files := make([]candidate, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		files = append(files, candidate{path: filepath.Join(m.Root, "runs", entry.Name()), when: info.ModTime()})
	}
	if len(files) <= keep {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].when.Equal(files[j].when) {
			return files[i].path < files[j].path
		}
		return files[i].when.Before(files[j].when)
	})
	for _, file := range files[:len(files)-keep] {
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (m Manager) ensure() error {
	for _, path := range []string{m.Root, filepath.Join(m.Root, "runs")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (m Manager) load() (map[string]Routine, error) {
	if err := m.ensure(); err != nil {
		return nil, err
	}
	path := filepath.Join(m.Root, "routines.json")
	data, err := safefile.ReadBoundedRegular(path, maxRoutineStoreBytes)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Routine{}, nil
	}
	if err != nil {
		return nil, err
	}
	var top map[string]json.RawMessage
	if err := strictjson.Decode(data, &top); err != nil {
		return nil, fmt.Errorf("decode routines: %w", err)
	}
	if rawSchema, ok := top["schema"]; ok {
		var storedSchema string
		if json.Unmarshal(rawSchema, &storedSchema) == nil && strings.HasPrefix(storedSchema, "agentstack.") {
			if storedSchema != routinesSchema {
				return nil, fmt.Errorf("unexpected persistence schema %q; expected %q", storedSchema, routinesSchema)
			}
			var envelope routinesEnvelope
			if err := strictjson.Decode(data, &envelope); err != nil {
				return nil, fmt.Errorf("decode routines envelope: %w", err)
			}
			if envelope.Version != routinesStoreVersion {
				return nil, fmt.Errorf("unsupported %s schema version %d", routinesSchema, envelope.Version)
			}
			if envelope.Items == nil {
				envelope.Items = map[string]Routine{}
			}
			if err := validatePersistedRoutines(envelope.Items); err != nil {
				return nil, err
			}
			changed, err := m.reconcileRuns(envelope.Items)
			if err != nil {
				return nil, err
			}
			if changed {
				if err := m.save(envelope.Items); err != nil {
					return nil, err
				}
			}
			return envelope.Items, nil
		}
	}
	var routines map[string]Routine
	if err := strictjson.Decode(data, &routines); err != nil {
		return nil, fmt.Errorf("decode legacy routines: %w", err)
	}
	if routines == nil {
		routines = map[string]Routine{}
	}
	if err := validatePersistedRoutines(routines); err != nil {
		return nil, err
	}
	changed, err := m.reconcileRuns(routines)
	if err != nil {
		return nil, err
	}
	if changed {
		if err := m.save(routines); err != nil {
			return nil, err
		}
	}
	return routines, nil
}

func validatePersistedRoutines(routines map[string]Routine) error {
	for key, routine := range routines {
		if key != routine.ID {
			return fmt.Errorf("persisted routine key %q does not match id %q", key, routine.ID)
		}
		if err := validateRoutine(routine); err != nil {
			return fmt.Errorf("persisted routine %q is invalid: %w", key, err)
		}
	}
	return nil
}

func (m Manager) save(routines map[string]Routine) error {
	if err := m.ensure(); err != nil {
		return err
	}
	if routines == nil {
		routines = map[string]Routine{}
	}
	return writeJSON(filepath.Join(m.Root, "routines.json"), routinesEnvelope{
		Schema: routinesSchema, Version: routinesStoreVersion, Items: routines,
	})
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data)
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".agentstack-routine-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return safefile.Replace(temporaryPath, path)
}

func validateRoutine(routine Routine) error {
	if !validID(routine.ID) {
		return fmt.Errorf("routine id is empty or invalid")
	}
	if strings.TrimSpace(routine.Name) == "" {
		return fmt.Errorf("routine name is required")
	}
	if len(routine.Steps) == 0 || len(routine.Steps) > maxRoutineSteps {
		return fmt.Errorf("routine must define between 1 and %d steps", maxRoutineSteps)
	}
	encoded, err := json.Marshal(routine)
	if err != nil {
		return err
	}
	if len(encoded) > maxRoutineDefinition {
		return fmt.Errorf("routine definition exceeds %d bytes", maxRoutineDefinition)
	}
	if _, err := NextRun(routine.Schedule, time.Now().UTC(), routine.LastRun); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, step := range routine.Steps {
		if !validID(step.ID) {
			return fmt.Errorf("routine step id is empty or invalid")
		}
		if _, duplicate := seen[step.ID]; duplicate {
			return fmt.Errorf("duplicate routine step id %q", step.ID)
		}
		seen[step.ID] = struct{}{}
		if err := validateStep(step); err != nil {
			return fmt.Errorf("step %s: %w", step.ID, err)
		}
	}
	return nil
}

func validateStep(step Step) error {
	switch step.Kind {
	case StepInventory, StepMCPDoctor, StepContextScan, StepContextScore, StepMemorySearch, StepPromptRender, StepArtifactVerify, StepResourceAudit, StepResourceRefreshPlan:
		if strings.TrimSpace(step.Command) != "" || len(step.Args) != 0 {
			return fmt.Errorf("non-command steps cannot define command or args")
		}
	case StepCommand:
		if strings.TrimSpace(step.Command) == "" {
			return fmt.Errorf("command step requires command")
		}
		if len(step.Command) > maxRoutineCommand || strings.ContainsAny(step.Command, "\x00\r\n") {
			return fmt.Errorf("command is invalid or exceeds %d bytes", maxRoutineCommand)
		}
	default:
		return fmt.Errorf("unsupported routine step kind %q", step.Kind)
	}
	if len(step.Args) > maxRoutineArguments {
		return fmt.Errorf("command step exceeds %d arguments", maxRoutineArguments)
	}
	if len(step.Params) > maxRoutineParameters {
		return fmt.Errorf("routine step exceeds %d parameters", maxRoutineParameters)
	}
	maxSeconds := int(MaxRoutineRunDuration / time.Second)
	if step.TimeoutSeconds < 0 || step.TimeoutSeconds > maxSeconds {
		return fmt.Errorf("timeoutSeconds must be between 0 and %d", maxSeconds)
	}
	for key, value := range step.Params {
		if key == "" || len(key) > maxRoutineParamKey || strings.ContainsAny(key, "\x00\r\n") {
			return fmt.Errorf("parameter key is invalid or exceeds %d bytes", maxRoutineParamKey)
		}
		if len(value) > maxRoutineParamValue || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("parameter %q exceeds %d bytes or contains NUL", key, maxRoutineParamValue)
		}
		if redact.SensitiveKey(key) && !credentialReferenceKey(key) {
			return fmt.Errorf("parameter %q would persist credential material; use an environment, file, or reference key", key)
		}
		if redact.Text(value) != value {
			return fmt.Errorf("parameter %q contains credential-like material", key)
		}
	}
	for index := 0; index < len(step.Args); index++ {
		argument := step.Args[index]
		if len(argument) > maxRoutineArgument || strings.ContainsRune(argument, '\x00') {
			return fmt.Errorf("command argument exceeds %d bytes or contains NUL", maxRoutineArgument)
		}
		if redact.Text(argument) != argument {
			return fmt.Errorf("command argument contains credential-like material")
		}
		if strings.HasPrefix(argument, "-") {
			parts := strings.SplitN(strings.TrimLeft(argument, "-"), "=", 2)
			flag := parts[0]
			if redact.SensitiveKey(flag) && !credentialReferenceKey(flag) {
				return fmt.Errorf("command flag %q would persist credential material; use an environment, file, or reference flag", flag)
			}
			if redact.SensitiveKey(flag) && credentialReferenceKey(flag) {
				value := ""
				if len(parts) == 2 {
					value = parts[1]
				} else if index+1 < len(step.Args) {
					index++
					value = step.Args[index]
				}
				if err := validateCredentialReference(flag, value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateCredentialReference(key, value string) error {
	if value == "" || len(value) > maxRoutineParamValue || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("credential reference %q is empty or invalid", key)
	}
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
	if strings.HasSuffix(normalized, "env") {
		for index, r := range value {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' || (index > 0 && r >= '0' && r <= '9') {
				continue
			}
			return fmt.Errorf("credential environment reference %q is invalid", value)
		}
	}
	return nil
}

func credentialReferenceKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
	for _, suffix := range []string{"env", "file", "path", "ref", "reference"} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

func nextForRoutine(routine Routine, now time.Time) (time.Time, error) {
	if !routine.Enabled {
		return time.Time{}, nil
	}
	return NextRun(routine.Schedule, now, routine.LastRun)
}

func validID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func cloneSteps(steps []Step) []Step {
	result := make([]Step, len(steps))
	for i, step := range steps {
		result[i] = step
		result[i].Args = append([]string(nil), step.Args...)
		if step.Params != nil {
			result[i].Params = make(map[string]string, len(step.Params))
			for key, value := range step.Params {
				result[i].Params[key] = value
			}
		}
	}
	return result
}

func runID(now time.Time) string {
	var entropy [6]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return fmt.Sprintf("run-%d", now.UnixNano())
	}
	return fmt.Sprintf("run-%d-%s", now.UnixNano(), hex.EncodeToString(entropy[:]))
}
