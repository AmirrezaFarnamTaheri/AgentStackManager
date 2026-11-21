package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
)

func TestBuildApplyOutcomeReportsRequestedFailuresSeparatelyFromUnchangedActions(t *testing.T) {
	plan := model.Plan{}
	transaction := model.Transaction{Status: model.TransactionFailed}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("tool-%02d", i)
		plan.Actions = append(plan.Actions, model.PlanAction{
			ComponentID: id,
			Name:        "Tool " + id,
			Kind:        model.ActionInstall,
			Install:     model.InstallSpec{Kind: model.InstallWinget},
		})
		transaction.Actions = append(transaction.Actions, model.TransactionAction{
			ComponentID: id,
			Kind:        model.ActionInstall,
			ExitCode:    -1,
			Error:       `exec: "winget": executable file not found in %PATH%`,
		})
	}
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("kept-%02d", i)
		plan.Actions = append(plan.Actions, model.PlanAction{ComponentID: id, Name: "Kept " + id, Kind: model.ActionKeep})
		transaction.Actions = append(transaction.Actions, model.TransactionAction{ComponentID: id, Kind: model.ActionKeep, Verified: true})
	}

	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: transaction}, errors.New("one or more selected installations failed"))
	if outcome.Requested != 20 || outcome.Processed != 20 || outcome.Succeeded != 0 || outcome.Failed != 20 || outcome.Skipped != 0 {
		t.Fatalf("outcome counts = %#v", outcome)
	}
	if outcome.Unchanged != 12 || outcome.Outcome != "failed" || outcome.Phase != "finished" {
		t.Fatalf("outcome state = %#v", outcome)
	}
	if outcome.Summary != "No requested changes were applied." {
		t.Fatalf("summary = %q", outcome.Summary)
	}
	if !strings.Contains(outcome.Detail, "12 existing verified items were left unchanged") {
		t.Fatalf("detail = %q", outcome.Detail)
	}
	if len(outcome.Diagnostics) != 20 {
		t.Fatalf("diagnostics = %#v", outcome.Diagnostics)
	}
	first := outcome.Diagnostics[0]
	if first.Category != "installer_unavailable" || first.Method != "WinGet" || first.ExitCode == nil || *first.ExitCode != -1 || first.ErrorCode != "" || !first.Retryable {
		t.Fatalf("first diagnostic = %#v", first)
	}
	if first.Summary != "WinGet is not available to AgentStack." || first.RecommendedAction == "" {
		t.Fatalf("first diagnostic copy = %#v", first)
	}
	if len(outcome.Causes) != 1 || outcome.Causes[0].Count != 20 || outcome.Causes[0].Category != "installer_unavailable" {
		t.Fatalf("causes = %#v", outcome.Causes)
	}

	serialized, err := json.Marshal(outcome)
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(serialized))
	for _, forbidden := range []string{"%path%", "executable file not found", `c:\\users`, "/home/", "command", "stderr", "stdout"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("outcome leaked %q: %s", forbidden, serialized)
		}
	}
}

func TestBuildApplyOutcomeReportsPartialResultsAndGroupsDistinctCauses(t *testing.T) {
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "ok", Name: "Completed Tool", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallNPMGlobal}},
		{ComponentID: "permission", Name: "Protected Tool", Kind: model.ActionRepair, Install: model.InstallSpec{Kind: model.InstallWinget}},
		{ComponentID: "dependency", Name: "Dependent Tool", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallUVTool}},
		{ComponentID: "kept", Name: "Existing Tool", Kind: model.ActionKeep},
	}}
	transaction := model.Transaction{Status: model.TransactionFailed, Actions: []model.TransactionAction{
		{ComponentID: "ok", Kind: model.ActionInstall, Verified: true},
		{ComponentID: "permission", Kind: model.ActionRepair, ExitCode: 5, Error: "Access is denied."},
		{ComponentID: "dependency", Kind: model.ActionInstall, ExitCode: -1, Error: "dependency permission failed: Access is denied."},
		{ComponentID: "kept", Kind: model.ActionKeep, Verified: true},
	}}

	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: transaction}, errors.New("one or more selected installations failed"))
	if outcome.Outcome != "partially_failed" || outcome.Succeeded != 1 || outcome.Failed != 2 || outcome.Unchanged != 1 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if outcome.Summary != "Some requested changes were applied." {
		t.Fatalf("summary = %q", outcome.Summary)
	}
	if len(outcome.Causes) != 2 {
		t.Fatalf("causes = %#v", outcome.Causes)
	}
}

func TestApplyOperationResultProvidesCountAwareFailure(t *testing.T) {
	result := applyOperationResult{Outcome: ApplyOutcome{Outcome: "failed", Requested: 20, Failed: 20, Unchanged: 12, Retryable: true}}
	failure := result.ClientFailure(errors.New("one or more selected installations failed"))
	if failure.Code != "installation_failed" || failure.Message != "No requested changes were applied." {
		t.Fatalf("failure = %#v", failure)
	}
	if !strings.Contains(failure.Recovery, "12 existing verified items were left unchanged") || !strings.Contains(failure.Recovery, "Retry failed items") {
		t.Fatalf("recovery = %q", failure.Recovery)
	}
}

func TestBuildApplyOutcomeClassifiesCollapsedInstallerPrerequisite(t *testing.T) {
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "one", Name: "One", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}},
		{ComponentID: "two", Name: "Two", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}},
	}}
	tx := model.Transaction{Status: model.TransactionFailed, Actions: []model.TransactionAction{
		{ComponentID: "one", Kind: model.ActionInstall, ExitCode: -1, Error: "installer prerequisite winget unavailable"},
		{ComponentID: "two", Kind: model.ActionInstall, ExitCode: -1, Error: "installer prerequisite winget unavailable"},
	}}
	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: tx}, errors.New("one or more selected installations failed"))
	if len(outcome.Causes) != 1 || outcome.Causes[0].Category != "installer_unavailable" || outcome.Causes[0].Count != 2 {
		t.Fatalf("causes = %#v", outcome.Causes)
	}
}

func TestBuildApplyOutcomeReportsCancellationSeparatelyFromFailure(t *testing.T) {
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "completed", Name: "Completed Tool", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallNPMGlobal}},
		{ComponentID: "pending", Name: "Pending Tool", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}},
		{ComponentID: "kept", Name: "Existing Tool", Kind: model.ActionKeep},
	}}
	transaction := model.Transaction{Status: model.TransactionInterrupted, Actions: []model.TransactionAction{
		{ComponentID: "completed", Kind: model.ActionInstall, Verified: true},
		{ComponentID: "kept", Kind: model.ActionKeep, Verified: true},
	}}

	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: transaction}, context.Canceled)
	if outcome.Outcome != "cancelled" || outcome.Succeeded != 1 || outcome.Skipped != 1 || outcome.Unchanged != 1 {
		t.Fatalf("outcome = %#v", outcome)
	}
	if !outcome.Retryable || outcome.Summary != "The run was cancelled after some changes were applied." {
		t.Fatalf("cancelled outcome copy = %#v", outcome)
	}
}

func TestBuildApplyOutcomeDecodesWinGetMissingManifestCode(t *testing.T) {
	const noManifest = 2316632087 // 0x8A150017
	plan := model.Plan{Actions: []model.PlanAction{
		{ComponentID: "trivy", Name: "Trivy", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}},
		{ComponentID: "yq", Name: "yq", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}},
	}}
	transaction := model.Transaction{Status: model.TransactionFailed, Actions: []model.TransactionAction{
		{ComponentID: "trivy", Kind: model.ActionInstall, ExitCode: noManifest, Error: "winget exited with code 2316632087"},
		{ComponentID: "yq", Kind: model.ActionInstall, ExitCode: noManifest, Error: "winget exited with code 2316632087"},
	}}

	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: transaction}, errors.New("one or more selected installations failed"))
	if len(outcome.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v", outcome.Diagnostics)
	}
	for _, diagnostic := range outcome.Diagnostics {
		if diagnostic.Category != "package_version_unavailable" {
			t.Fatalf("category = %q", diagnostic.Category)
		}
		if diagnostic.ErrorCode != "0x8A150017" {
			t.Fatalf("error code = %q", diagnostic.ErrorCode)
		}
		if diagnostic.Summary != "WinGet could not find the pinned package version." {
			t.Fatalf("summary = %q", diagnostic.Summary)
		}
		if !strings.Contains(diagnostic.RecommendedAction, "fresh plan") {
			t.Fatalf("recovery = %q", diagnostic.RecommendedAction)
		}
	}
	if len(outcome.Causes) != 1 || outcome.Causes[0].Count != 2 || outcome.Causes[0].ErrorCode != "0x8A150017" {
		t.Fatalf("causes = %#v", outcome.Causes)
	}
}

func TestBuildApplyOutcomeClassifiesIncompleteSkillDestination(t *testing.T) {
	plan := model.Plan{Actions: []model.PlanAction{{
		ComponentID: "superpowers", Name: "Superpowers skills", Kind: model.ActionInstall,
		Install: model.InstallSpec{Kind: model.InstallSkillPack},
	}}}
	transaction := model.Transaction{Status: model.TransactionFailed, Actions: []model.TransactionAction{{
		ComponentID: "superpowers", Kind: model.ActionInstall, ExitCode: -1,
		Error: "skill destination is missing a regular SKILL.md and contains user files",
	}}}

	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: transaction}, errors.New("one or more selected installations failed"))
	if len(outcome.Diagnostics) != 1 || outcome.Diagnostics[0].Category != "skill_destination_conflict" {
		t.Fatalf("diagnostics = %#v", outcome.Diagnostics)
	}
	if !strings.Contains(outcome.Diagnostics[0].RecommendedAction, "conflicting skill folder") {
		t.Fatalf("recovery = %q", outcome.Diagnostics[0].RecommendedAction)
	}
}

func TestBuildApplyOutcomeExplainsWinGetNoPackagesFound(t *testing.T) {
	const noPackages = 2316632084 // 0x8A150014
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "scc", Name: "scc", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}}}}
	tx := model.Transaction{Status: model.TransactionFailed, Actions: []model.TransactionAction{{ComponentID: "scc", Kind: model.ActionInstall, ExitCode: noPackages, Error: "winget exited with code 2316632084"}}}
	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: tx}, errors.New("installation failed"))
	item := outcome.Diagnostics[0]
	if item.Category != "package_not_found" || item.ErrorCode != "0x8A150014" {
		t.Fatalf("diagnostic=%#v", item)
	}
	if !strings.Contains(item.Cause, "No package matched") || !strings.Contains(item.RepairHint, "refresh") {
		t.Fatalf("cause=%q repair=%q", item.Cause, item.RepairHint)
	}
	if item.Evidence == "" || item.Severity != "error" {
		t.Fatalf("evidence/severity=%#v", item)
	}
}

func TestBuildApplyOutcomeExplainsAdministratorRequirement(t *testing.T) {
	const requiresAdmin = 2316632089 // 0x8A150019
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "tool", Name: "Tool", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallWinget}}}}
	tx := model.Transaction{Status: model.TransactionFailed, Actions: []model.TransactionAction{{ComponentID: "tool", Kind: model.ActionInstall, ExitCode: requiresAdmin, Error: "winget failed"}}}
	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: tx}, errors.New("installation failed"))
	item := outcome.Diagnostics[0]
	if item.Category != "administrator_required" || !strings.Contains(strings.ToLower(item.Cause), "administrator") {
		t.Fatalf("diagnostic=%#v", item)
	}
}

func TestBuildApplyOutcomeUnknownFailureStillExplainsWhatIsKnownWithoutLeakingRawOutput(t *testing.T) {
	plan := model.Plan{Actions: []model.PlanAction{{ComponentID: "tool", Name: "Tool", Kind: model.ActionInstall, Install: model.InstallSpec{Kind: model.InstallManual}}}}
	tx := model.Transaction{Status: model.TransactionFailed, Actions: []model.TransactionAction{{ComponentID: "tool", Kind: model.ActionInstall, ExitCode: 77, Error: `mystery failure at C:\Users\Private\secret.txt token=abc`}}}
	outcome := buildApplyOutcome(app.ApplyReport{Plan: plan, Transaction: tx}, errors.New("installation failed"))
	item := outcome.Diagnostics[0]
	if item.Category != "unclassified_failure" || !strings.Contains(item.Cause, "unrecognized failure") || !strings.Contains(item.Evidence, "77") {
		t.Fatalf("diagnostic=%#v", item)
	}
	serialized, _ := json.Marshal(item)
	lower := strings.ToLower(string(serialized))
	for _, forbidden := range []string{"users", "secret.txt", "token=abc"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("diagnostic leaked %q: %s", forbidden, serialized)
		}
	}
}
