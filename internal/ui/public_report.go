package ui

import (
	"strings"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
)

// publicApplyReport removes process output, command arguments, and local paths
// before an apply report crosses the browser boundary. The UI receives enough
// evidence to explain progress and recovery without receiving secrets or host
// implementation details.
func publicApplyReport(value app.ApplyReport) app.ApplyReport {
	result := value
	result.Plan = publicPlan(value.Plan)
	result.Transaction = publicTransaction(value.Transaction)
	if value.Router != nil {
		copyRouter := *value.Router
		copyRouter.RouterConfigPath = ""
		copyRouter.BackupPath = ""
		copyRouter.Warm = append([]app.WarmResult(nil), value.Router.Warm...)
		for index := range copyRouter.Warm {
			copyRouter.Warm[index].Command = ""
			copyRouter.Warm[index].Message = publicMessage(copyRouter.Warm[index].Message)
		}
		result.Router = &copyRouter
	}
	return result
}

func publicPlan(value model.Plan) model.Plan {
	result := value
	result.Actions = make([]model.PlanAction, len(value.Actions))
	for index, action := range value.Actions {
		copyAction := action
		copyAction.Install = model.InstallSpec{}
		result.Actions[index] = copyAction
	}
	return result
}

func publicTransactions(values []model.Transaction) []model.Transaction {
	result := make([]model.Transaction, len(values))
	for index, value := range values {
		result[index] = publicTransaction(value)
	}
	return result
}

func publicTransaction(value model.Transaction) model.Transaction {
	result := value
	result.Actions = make([]model.TransactionAction, len(value.Actions))
	for index, action := range value.Actions {
		copyAction := action
		copyAction.Command = ""
		copyAction.Args = nil
		copyAction.Output = ""
		copyAction.OutputTruncated = action.OutputTruncated || action.Output != ""
		copyAction.Verification = publicMessage(copyAction.Verification)
		if strings.TrimSpace(copyAction.Error) != "" {
			copyAction.Error = "This item could not be completed."
		}
		result.Actions[index] = copyAction
	}
	return result
}

func publicMessage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.Contains(value, `:\`) || strings.Contains(value, "/") || strings.Contains(lower, "appdata") || strings.Contains(lower, "users\\") {
		return "Details were removed because they may contain a private path."
	}
	if len(value) > 240 {
		return value[:240] + "…"
	}
	return value
}
