package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/processctl"
)

type Invocation struct {
	Command        string
	Args           []string
	Env            map[string]string
	Timeout        time.Duration
	MaxOutputBytes int
	LongRunning    bool
}

type Result struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Err       error
	Truncated bool
}

type CommandRunner interface {
	Run(context.Context, Invocation) Result
}
type SkillInstaller interface {
	Install(context.Context, model.Component) Result
}
type PathRefresher interface {
	Refresh() error
}

type ExecRunner struct {
	DefaultTimeout time.Duration
	MaxOutputBytes int
}

func (r ExecRunner) Run(ctx context.Context, invocation Invocation) Result {
	timeout := invocation.Timeout
	if timeout <= 0 && !invocation.LongRunning {
		timeout = r.DefaultTimeout
		if timeout <= 0 {
			timeout = 20 * time.Minute
		}
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.Command(invocation.Command, invocation.Args...)
	if len(invocation.Env) > 0 {
		env := cmd.Environ()
		for key, value := range invocation.Env {
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}
	limit := invocation.MaxOutputBytes
	if limit <= 0 {
		limit = r.MaxOutputBytes
	}
	if limit <= 0 {
		limit = 1 << 20
	}
	stdout, stderr := newLimitedBuffer(limit), newLimitedBuffer(limit)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	process, startErr := processctl.Start(cmd)
	var err error
	if startErr != nil {
		err = startErr
	} else {
		err = process.Wait(ctx)
	}
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), Err: err, Truncated: stdout.Truncated() || stderr.Truncated()}
	if err == nil {
		return result
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.ExitCode = -1
	}
	return result
}

type Engine struct {
	Commands CommandRunner
	Skills   SkillInstaller
	Catalog  model.Catalog
	Path     PathRefresher
	Verifier ComponentVerifier
}

type VerificationResult struct {
	OK      bool
	Message string
}

type ComponentVerifier interface {
	Verify(context.Context, model.Component, model.PlanAction, Result) VerificationResult
}

type ApplyOptions struct {
	DryRun        bool
	CorrelationID string
	OnUpdate      func(model.Transaction) error
}

func (e Engine) Apply(ctx context.Context, plan model.Plan, options ApplyOptions) model.Transaction {
	if e.Commands == nil {
		e.Commands = ExecRunner{}
	}
	if e.Path == nil {
		e.Path = defaultPathRefresher{}
	}
	tx := model.Transaction{ID: newID(), CorrelationID: options.CorrelationID, StartedAt: time.Now().UTC(), DryRun: options.DryRun, Status: model.TransactionRunning}
	if options.DryRun {
		tx.Status = model.TransactionPlanned
	}
	if err := publish(options.OnUpdate, tx); err != nil {
		tx.Status = model.TransactionFailed
		tx.FinishedAt = time.Now().UTC()
		tx.Actions = append(tx.Actions, journalFailureAction(err))
		return tx
	}
	checkpoint := func(record model.TransactionAction) bool {
		tx.Actions = append(tx.Actions, record)
		if err := publish(options.OnUpdate, tx); err != nil {
			tx.Status = model.TransactionFailed
			tx.Actions = append(tx.Actions, journalFailureAction(err))
			return false
		}
		return true
	}

	failed := map[string]string{}
	for _, action := range plan.Actions {
		record := model.TransactionAction{ComponentID: action.ComponentID, Kind: action.Kind, StartedAt: time.Now().UTC()}
		if action.Kind != model.ActionInstall && action.Kind != model.ActionRepair {
			record.Verified = true
			record.Verification = "no installer action required"
			record.FinishedAt = time.Now().UTC()
			if !checkpoint(record) {
				break
			}
			continue
		}
		if dependency, message := e.failedDependency(action.ComponentID, failed); dependency != "" {
			record.ExitCode = -1
			record.Error = fmt.Sprintf("dependency %s failed: %s", dependency, message)
			record.FinishedAt = time.Now().UTC()
			tx.Status = model.TransactionFailed
			failed[action.ComponentID] = record.Error
			if !checkpoint(record) {
				break
			}
			continue
		}
		if options.DryRun {
			invocation, _ := invocationFor(action)
			record.Command, record.Args = invocation.Command, invocation.Args
			record.Verified = true
			record.Verification = "dry-run action resolved"
			record.FinishedAt = time.Now().UTC()
			if !checkpoint(record) {
				break
			}
			continue
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
			if err := e.Path.Refresh(); err != nil {
				result.Err = fmt.Errorf("refresh process PATH: %w", err)
				result.ExitCode = -1
				record.ExitCode = -1
			}
		}
		if result.Err != nil {
			record.Error = result.Err.Error()
			if result.Stderr != "" {
				record.Error += ": " + result.Stderr
			}
			tx.Status = model.TransactionFailed
			failed[action.ComponentID] = record.Error
		}
		if result.Err == nil && e.Verifier != nil {
			component, ok := e.Catalog.ComponentByID(action.ComponentID)
			if !ok {
				record.Error = "component missing from catalog during postcondition verification"
				tx.Status = model.TransactionFailed
				failed[action.ComponentID] = record.Error
			} else {
				verification := e.Verifier.Verify(ctx, component, action, result)
				record.Verified = verification.OK
				record.Verification = verification.Message
				if !verification.OK {
					record.Error = "postcondition verification failed: " + verification.Message
					tx.Status = model.TransactionFailed
					failed[action.ComponentID] = record.Error
				}
			}
		} else if result.Err == nil {
			record.Verified = true
			record.Verification = "command completed; no verifier configured"
		}
		record.FinishedAt = time.Now().UTC()
		if !checkpoint(record) {
			break
		}
	}
	if tx.Status == model.TransactionRunning {
		tx.Status = model.TransactionSucceeded
	}
	tx.FinishedAt = time.Now().UTC()
	if err := publish(options.OnUpdate, tx); err != nil {
		tx.Status = model.TransactionFailed
		tx.Actions = append(tx.Actions, journalFailureAction(err))
	}
	return tx
}

func journalFailureAction(err error) model.TransactionAction {
	now := time.Now().UTC()
	return model.TransactionAction{
		ComponentID:  "agentstack-state",
		Kind:         model.ActionConfigure,
		Error:        "persist transaction journal: " + err.Error(),
		StartedAt:    now,
		FinishedAt:   now,
		Verification: "transaction state could not be durably recorded",
	}
}

func publish(callback func(model.Transaction) error, tx model.Transaction) error {
	if callback == nil {
		return nil
	}
	return callback(tx)
}

func (e Engine) failedDependency(componentID string, failed map[string]string) (string, string) {
	component, ok := e.Catalog.ComponentByID(componentID)
	if !ok {
		return "", ""
	}
	for _, dependency := range component.DependsOn {
		if message, exists := failed[dependency]; exists {
			return dependency, message
		}
	}
	return "", ""
}

func invocationFor(action model.PlanAction) (Invocation, error) {
	spec := action.Install
	switch spec.Kind {
	case model.InstallWinget:
		if spec.WingetID == "" {
			return Invocation{}, fmt.Errorf("winget id is empty")
		}
		verb := "install"
		args := []string{verb, "--id", spec.WingetID, "--exact", "--silent", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}
		if action.Upgrade {
			args[0] = "upgrade"
		} else {
			args = append(args, "--no-upgrade")
		}
		if spec.Source != "" {
			args = append(args, "--source", spec.Source)
		}
		if spec.Version != "" {
			args = append(args, "--version", spec.Version)
		}
		return Invocation{Command: "winget", Args: args, Timeout: 30 * time.Minute}, nil
	case model.InstallNPMGlobal:
		if spec.Package == "" {
			return Invocation{}, fmt.Errorf("npm package is empty")
		}
		return Invocation{Command: "npm", Args: []string{"install", "--global", spec.Package}, Timeout: 15 * time.Minute}, nil
	case model.InstallUVTool:
		if spec.Package == "" {
			return Invocation{}, fmt.Errorf("uv package is empty")
		}
		args := []string{"tool", "install"}
		if action.Kind == model.ActionRepair || action.Upgrade {
			args = append(args, "--force")
		}
		args = append(args, spec.Package)
		return Invocation{Command: "uv", Args: args, Timeout: 15 * time.Minute}, nil
	default:
		return Invocation{}, fmt.Errorf("unsupported install kind %q", spec.Kind)
	}
}

func newID() string {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return fmt.Sprintf("tx-%d", time.Now().UnixNano())
	}
	return "tx-" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(random)
}
