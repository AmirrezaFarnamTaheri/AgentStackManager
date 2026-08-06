package ui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
)

// ApplyOutcome is the canonical path-free result of one reviewed apply. Phase
// describes execution lifecycle; Outcome describes the terminal result.
type ApplyOutcome struct {
	Phase       string            `json:"phase"`
	Outcome     string            `json:"outcome"`
	Requested   int               `json:"requested"`
	Processed   int               `json:"processed"`
	Succeeded   int               `json:"succeeded"`
	Failed      int               `json:"failed"`
	Skipped     int               `json:"skipped"`
	Unchanged   int               `json:"unchanged"`
	Retryable   bool              `json:"retryable"`
	Summary     string            `json:"summary"`
	Detail      string            `json:"detail"`
	Diagnostics []ApplyDiagnostic `json:"diagnostics,omitempty"`
	Causes      []ApplyCauseGroup `json:"causes,omitempty"`
}

type ApplyDiagnostic struct {
	ComponentID       string `json:"componentId"`
	Label             string `json:"label"`
	Action            string `json:"action"`
	Result            string `json:"result"`
	Category          string `json:"category,omitempty"`
	Method            string `json:"method,omitempty"`
	ExitCode          *int   `json:"exitCode,omitempty"`
	ErrorCode         string `json:"errorCode,omitempty"`
	Summary           string `json:"summary,omitempty"`
	Cause             string `json:"cause,omitempty"`
	Evidence          string `json:"evidence,omitempty"`
	Severity          string `json:"severity,omitempty"`
	Retryable         bool   `json:"retryable"`
	RecommendedAction string `json:"recommendedAction,omitempty"`
	RepairHint        string `json:"repairHint,omitempty"`
}

type ApplyCauseGroup struct {
	Category          string   `json:"category"`
	Method            string   `json:"method,omitempty"`
	ErrorCode         string   `json:"errorCode,omitempty"`
	Title             string   `json:"title,omitempty"`
	Summary           string   `json:"summary"`
	Evidence          string   `json:"evidence,omitempty"`
	RecommendedAction string   `json:"recommendedAction"`
	Count             int      `json:"count"`
	ComponentIDs      []string `json:"componentIds"`
	AffectedLabels    []string `json:"affectedLabels,omitempty"`
}

type failureClassification struct {
	Category   string
	ErrorCode  string
	Title      string
	Cause      string
	Evidence   string
	RepairHint string
	Severity   string
	Retryable  bool
}

type applyOperationResult struct {
	Report  app.ApplyReport `json:"report"`
	Outcome ApplyOutcome    `json:"outcome"`
}

func (r applyOperationResult) ClientFailure(err error) ClientFailure {
	base := clientFailureFor(err)
	if base.Code != "installation_failed" && r.Outcome.Failed == 0 {
		return base
	}
	base.Code = "installation_failed"
	base.Retryable = r.Outcome.Retryable
	base.Message = strings.TrimSpace(r.Outcome.Summary)
	if base.Message == "" {
		switch r.Outcome.Outcome {
		case "failed":
			base.Message = "No requested changes were applied."
		case "partially_failed":
			base.Message = "Some requested changes were applied."
		default:
			base.Message = strings.TrimSpace(base.Message)
		}
	}
	unchanged := unchangedSentence(r.Outcome.Unchanged)
	switch r.Outcome.Outcome {
	case "partially_failed":
		base.Recovery = fmt.Sprintf("%d requested change%s succeeded and %d failed. %s Retry failed items or review a fresh plan.", r.Outcome.Succeeded, pluralSuffix(r.Outcome.Succeeded), r.Outcome.Failed, unchanged)
	case "failed":
		base.Recovery = fmt.Sprintf("%s Retry failed items or review a fresh plan.", unchanged)
	default:
		if strings.TrimSpace(base.Recovery) == "" {
			base.Recovery = "Refresh the current system state and review a fresh plan."
		}
	}
	return base
}

func buildApplyOutcome(report app.ApplyReport, runErr error) ApplyOutcome {
	outcome := ApplyOutcome{Phase: "finished"}
	transactionActions := make(map[string]model.TransactionAction, len(report.Transaction.Actions))
	for _, action := range report.Plan.Actions {
		if trackedOutcomeAction(action.Kind) {
			outcome.Requested++
		}
	}
	for _, action := range report.Transaction.Actions {
		transactionActions[action.ComponentID] = action
		if unchangedOutcomeAction(action.Kind) && action.Verified && strings.TrimSpace(action.Error) == "" {
			outcome.Unchanged++
		}
	}

	causeMap := map[string]*ApplyCauseGroup{}
	for _, planAction := range report.Plan.Actions {
		if !trackedOutcomeAction(planAction.Kind) {
			continue
		}
		txAction, exists := transactionActions[planAction.ComponentID]
		label := strings.TrimSpace(planAction.Name)
		if label == "" {
			label = planAction.ComponentID
		}
		diagnostic := ApplyDiagnostic{
			ComponentID: planAction.ComponentID,
			Label:       label,
			Action:      string(planAction.Kind),
			Method:      applyMethod(planAction),
		}
		switch {
		case !exists:
			diagnostic.Result = "skipped"
			diagnostic.Summary = "This item was not attempted."
			diagnostic.RecommendedAction = "Refresh the system state and review this item in a fresh plan."
			outcome.Skipped++
		case strings.TrimSpace(txAction.Error) != "" || !txAction.Verified:
			diagnostic.Result = "failed"
			exitCode := txAction.ExitCode
			diagnostic.ExitCode = &exitCode
			classification := classifyApplyFailure(txAction.Error, diagnostic.Method, exitCode)
			diagnostic.Category = classification.Category
			diagnostic.ErrorCode = classification.ErrorCode
			diagnostic.Summary = classification.Cause
			diagnostic.Cause = classification.Cause
			diagnostic.Evidence = classification.Evidence
			diagnostic.Severity = classification.Severity
			diagnostic.Retryable = classification.Retryable
			diagnostic.RecommendedAction = classification.RepairHint
			diagnostic.RepairHint = classification.RepairHint
			outcome.Failed++
			key := strings.Join([]string{diagnostic.Category, diagnostic.Method, diagnostic.ErrorCode, diagnostic.Cause, diagnostic.RepairHint}, "\x00")
			group := causeMap[key]
			if group == nil {
				group = &ApplyCauseGroup{Category: diagnostic.Category, Method: diagnostic.Method, ErrorCode: diagnostic.ErrorCode, Title: classification.Title, Summary: diagnostic.Cause, Evidence: diagnostic.Evidence, RecommendedAction: diagnostic.RepairHint}
				causeMap[key] = group
			}
			group.Count++
			group.ComponentIDs = append(group.ComponentIDs, diagnostic.ComponentID)
			group.AffectedLabels = append(group.AffectedLabels, diagnostic.Label)
		default:
			diagnostic.Result = "succeeded"
			diagnostic.Summary = "Completed and verified."
			outcome.Succeeded++
		}
		outcome.Diagnostics = append(outcome.Diagnostics, diagnostic)
	}
	outcome.Processed = outcome.Succeeded + outcome.Failed + outcome.Skipped
	outcome.Retryable = outcome.Failed > 0

	for _, group := range causeMap {
		sort.Strings(group.ComponentIDs)
		sort.Strings(group.AffectedLabels)
		outcome.Causes = append(outcome.Causes, *group)
	}
	sort.Slice(outcome.Causes, func(i, j int) bool {
		if outcome.Causes[i].Count != outcome.Causes[j].Count {
			return outcome.Causes[i].Count > outcome.Causes[j].Count
		}
		if outcome.Causes[i].Summary != outcome.Causes[j].Summary {
			return outcome.Causes[i].Summary < outcome.Causes[j].Summary
		}
		return outcome.Causes[i].Method < outcome.Causes[j].Method
	})

	switch {
	case errors.Is(runErr, context.Canceled):
		outcome.Outcome = "cancelled"
		outcome.Retryable = outcome.Succeeded < outcome.Requested
		if outcome.Succeeded > 0 {
			outcome.Summary = "The run was cancelled after some changes were applied."
		} else {
			outcome.Summary = "The run was cancelled."
		}
		outcome.Detail = fmt.Sprintf("%d requested change%s succeeded and %d were not completed. %s Refresh the current state before retrying.", outcome.Succeeded, pluralSuffix(outcome.Succeeded), outcome.Requested-outcome.Succeeded, unchangedSentence(outcome.Unchanged))
	case outcome.Failed > 0 && outcome.Succeeded == 0:
		outcome.Outcome = "failed"
		outcome.Summary = "No requested changes were applied."
		outcome.Detail = fmt.Sprintf("%s Resolve the cause, then retry failed items in a fresh reviewed plan.", unchangedSentence(outcome.Unchanged))
	case outcome.Failed > 0:
		outcome.Outcome = "partially_failed"
		outcome.Summary = "Some requested changes were applied."
		outcome.Detail = fmt.Sprintf("%d requested change%s succeeded and %d failed. %s", outcome.Succeeded, pluralSuffix(outcome.Succeeded), outcome.Failed, unchangedSentence(outcome.Unchanged))
	case runErr != nil:
		outcome.Outcome = "failed"
		outcome.Summary = "The operation could not be completed."
		outcome.Detail = "No requested changes were confirmed as successful. Refresh the system state before creating a fresh plan."
	case outcome.Skipped > 0:
		outcome.Outcome = "partially_failed"
		outcome.Summary = "The run finished with skipped items."
		outcome.Detail = fmt.Sprintf("%d requested change%s succeeded and %d were skipped. %s", outcome.Succeeded, pluralSuffix(outcome.Succeeded), outcome.Skipped, unchangedSentence(outcome.Unchanged))
	default:
		outcome.Outcome = "succeeded"
		outcome.Summary = "All requested changes were applied."
		outcome.Detail = fmt.Sprintf("%d requested change%s completed and verified. %s", outcome.Succeeded, pluralSuffix(outcome.Succeeded), unchangedSentence(outcome.Unchanged))
	}
	return outcome
}

func trackedOutcomeAction(kind model.ActionKind) bool {
	return kind == model.ActionInstall || kind == model.ActionRepair || kind == model.ActionConfigure
}

func unchangedOutcomeAction(kind model.ActionKind) bool {
	switch kind {
	case model.ActionKeep, model.ActionPreserveInactive, model.ActionSkip, model.ActionSkipDominated:
		return true
	default:
		return false
	}
}

func applyMethod(action model.PlanAction) string {
	switch action.Install.Kind {
	case model.InstallWinget:
		return "WinGet"
	case model.InstallNPMGlobal:
		return "npm"
	case model.InstallUVTool:
		return "uv"
	case model.InstallSkillPack:
		return "Skill package"
	case model.InstallRouter:
		return "AgentStack configuration"
	case model.InstallManual:
		return "Manual setup"
	default:
		if action.Kind == model.ActionConfigure {
			return "AgentStack configuration"
		}
		return "AgentStack"
	}
}

func classifyApplyFailure(raw, method string, exitCode int) failureClassification {
	lower := strings.ToLower(strings.TrimSpace(raw))
	classification := failureClassification{
		Category:   "unclassified_failure",
		Title:      "Installer failure",
		Cause:      method + " reported an unrecognized failure.",
		Evidence:   processEvidence(method, exitCode),
		RepairHint: "Refresh system state, verify the package source, and retry this item. If it fails again, use the error code in Technical details.",
		Severity:   "error",
		Retryable:  true,
	}
	if method == "WinGet" && exitCode >= 0 {
		classification.ErrorCode = fmt.Sprintf("0x%08X", uint32(exitCode))
		classification.Evidence = "WinGet returned " + classification.ErrorCode + "."
		switch uint32(exitCode) {
		case 0x8A150010:
			classification.Category = "no_applicable_installer"
			classification.Title = "No compatible installer"
			classification.Cause = "The package exists, but none of its installers apply to this Windows version or architecture."
			classification.RepairHint = "Check the tool's system requirements or choose a package version that supports this computer."
			return classification
		case 0x8A150014:
			classification.Category = "package_not_found"
			classification.Title = "Package not found"
			classification.Cause = "No package matched the requested WinGet identifier in the configured sources."
			classification.RepairHint = "Run a WinGet source refresh, refresh AgentStack's catalog, then create a fresh plan."
			return classification
		case 0x8A150015:
			classification.Category = "package_source_missing"
			classification.Title = "No WinGet sources configured"
			classification.Cause = "WinGet has no package sources configured, so it cannot search for this tool."
			classification.RepairHint = "Repair or reset WinGet sources, refresh AgentStack, then retry."
			return classification
		case 0x8A150017:
			classification.Category = "package_version_unavailable"
			classification.Title = "Pinned package version unavailable"
			classification.Cause = "WinGet could not find the pinned package version."
			classification.RepairHint = "Refresh AgentStack's catalog, then create a fresh plan with a currently available version."
			return classification
		case 0x8A150019:
			classification.Category = "administrator_required"
			classification.Title = "Administrator access required"
			classification.Cause = "WinGet requires an administrator token for this installation."
			classification.RepairHint = "Approve the Windows administrator prompt, then retry the failed item."
			return classification
		case 0x8A150008:
			classification.Category = "download_failed"
			classification.Title = "Installer download failed"
			classification.Cause = "WinGet could not download the installer."
			classification.RepairHint = "Check network, proxy, firewall, and package-source access, then retry."
			return classification
		case 0x8A150102:
			classification.Category = "installer_busy"
			classification.Title = "Another installation is running"
			classification.Cause = "Windows Installer is already processing another installation."
			classification.RepairHint = "Wait for the other installation to finish, then retry."
			return classification
		}
	}
	switch {
	case strings.Contains(lower, "dependency ") && strings.Contains(lower, " failed"):
		classification.Category = "dependency_failed"
		classification.Title = "Required dependency failed"
		classification.Cause = "A required tool failed earlier in this run, so this item could not continue."
		classification.RepairHint = "Resolve the failed dependency first, then retry this item."
	case strings.Contains(lower, "missing a regular skill.md") || strings.Contains(lower, "skill destination") && strings.Contains(lower, "incomplete"):
		classification.Category = "skill_destination_conflict"
		classification.Title = "Conflicting skill folder"
		classification.Cause = "An existing skill folder is incomplete or incompatible with the package being installed."
		classification.RepairHint = "Move or repair the conflicting skill folder, refresh the system, then retry."
	case strings.Contains(lower, "access is denied") || strings.Contains(lower, "permission denied") || strings.Contains(lower, "operation not permitted"):
		classification.Category = "permission_denied"
		classification.Title = "Permission denied"
		classification.Cause = "AgentStack did not have permission to write or execute a required installation step."
		classification.RepairHint = "Approve administrator access or choose a writable installation scope, then retry."
	case strings.Contains(lower, "installer prerequisite") && strings.Contains(lower, "unavailable"):
		classification.Category = "installer_unavailable"
		classification.Title = method + " unavailable"
		classification.Cause = method + " is not available to AgentStack."
		classification.RepairHint = "Install or repair " + method + ", refresh the system, then retry."
	case strings.Contains(lower, "executable file not found") || strings.Contains(lower, "not found in %path%") || strings.Contains(lower, "is not recognized as an internal or external command"):
		classification.Category = "installer_unavailable"
		classification.Title = method + " unavailable"
		classification.Cause = method + " is not available to AgentStack."
		classification.RepairHint = "Install or repair " + method + ", refresh AgentStack so PATH is reloaded, then retry."
	case strings.Contains(lower, "postcondition verification failed") || strings.Contains(lower, "could not verify"):
		classification.Category = "verification_failed"
		classification.Title = "Installed result could not be verified"
		classification.Cause = "The installer exited, but AgentStack could not detect the expected command or compatible version."
		classification.Evidence = "The installer exit code was " + fmt.Sprint(exitCode) + ", but the post-install probe failed."
		classification.RepairHint = "Refresh the system to reload PATH, then review the detected command and version before retrying."
	case strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timed out") || strings.Contains(lower, "timeout"):
		classification.Category = "timeout"
		classification.Title = "Installation timed out"
		classification.Cause = "The installation did not finish before the safety timeout."
		classification.RepairHint = "Close installer prompts or locks, then retry."
	case strings.Contains(lower, "network") || strings.Contains(lower, "connection reset") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "unable to resolve") || strings.Contains(lower, "no such host") || strings.Contains(lower, "tls") || strings.Contains(lower, "certificate"):
		classification.Category = "network_unavailable"
		classification.Title = "Package source unreachable"
		classification.Cause = "The installer could not reach its package source."
		classification.RepairHint = "Check network, proxy, TLS inspection, and package-source access, then retry."
	}
	return classification
}

func processEvidence(method string, exitCode int) string {
	if strings.TrimSpace(method) == "" {
		method = "The installer"
	}
	return fmt.Sprintf("%s exited with code %d.", method, exitCode)
}

func unchangedSentence(count int) string {
	if count == 0 {
		return "No existing verified items were changed."
	}
	return fmt.Sprintf("%d existing verified item%s were left unchanged.", count, pluralSuffix(count))
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
