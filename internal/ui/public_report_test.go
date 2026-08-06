package ui

import (
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
)

func TestPublicApplyReportRemovesCommandsOutputAndPaths(t *testing.T) {
	input := app.ApplyReport{
		Plan: model.Plan{ID: "plan-1", Actions: []model.PlanAction{{
			ComponentID: "tool-a", Name: "Tool A", Kind: model.ActionInstall,
			Install: model.InstallSpec{Kind: model.InstallWinget, Source: `C:\Users\example\private`},
		}}},
		Transaction: model.Transaction{ID: "tx-1", Actions: []model.TransactionAction{{
			ComponentID: "tool-a", Kind: model.ActionInstall, Command: "secret-command",
			Args: []string{"--token", "secret"}, Output: "secret output", Error: `open C:\Users\example\private: failed`,
			Verification: `/home/example/private/tool`,
		}}},
		Router: &app.MCPInitReport{RouterConfigPath: `C:\Users\example\router.json`, BackupPath: `/home/example/backup`, Warm: []app.WarmResult{{Command: "secret warm", Message: `/private/path`}}},
	}

	got := publicApplyReport(input)
	if got.Plan.Actions[0].Install.Source != "" || got.Transaction.Actions[0].Command != "" || len(got.Transaction.Actions[0].Args) != 0 || got.Transaction.Actions[0].Output != "" {
		t.Fatalf("public report retained sensitive process details: %#v", got)
	}
	if got.Router.RouterConfigPath != "" || got.Router.BackupPath != "" || got.Router.Warm[0].Command != "" {
		t.Fatalf("public report retained private router details: %#v", got.Router)
	}
	serialized := strings.ToLower(got.Transaction.Actions[0].Error + got.Transaction.Actions[0].Verification + got.Router.Warm[0].Message)
	for _, forbidden := range []string{"users", "home/example", "private/path", "secret"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("public report retained %q: %s", forbidden, serialized)
		}
	}
}

func TestPublicTransactionsRemoveCommandsOutputAndPrivateEvidence(t *testing.T) {
	input := []model.Transaction{{
		ID: "tx-1",
		Actions: []model.TransactionAction{{
			ComponentID:  "tool-a",
			Kind:         model.ActionInstall,
			Command:      "npm",
			Args:         []string{"install", "--token", "secret"},
			Output:       "npm progress and secret output",
			Error:        `open C:\Users\example\private: failed`,
			Verification: `/home/example/private/tool`,
		}},
	}}
	got := publicTransactions(input)
	if len(got) != 1 || len(got[0].Actions) != 1 {
		t.Fatalf("public transactions shape = %#v", got)
	}
	action := got[0].Actions[0]
	if action.Command != "" || len(action.Args) != 0 || action.Output != "" {
		t.Fatalf("public transactions retained process details: %#v", action)
	}
	serialized := strings.ToLower(action.Error + action.Verification)
	for _, forbidden := range []string{"npm", "secret", "users\\example", "home/example", "c:\\users"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("public transactions retained %q: %s", forbidden, serialized)
		}
	}
	if input[0].Actions[0].Command != "npm" || input[0].Actions[0].Output == "" {
		t.Fatal("sanitization mutated backend transaction evidence")
	}
}
