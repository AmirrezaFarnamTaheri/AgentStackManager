package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/reviewedplan"
)

func TestClientFailureForKnownErrors(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		code      string
		message   string
		recovery  string
		retryable bool
	}{
		{
			name:      "unavailable",
			err:       reviewedplan.ErrPlanUnavailable,
			code:      "plan_unavailable",
			message:   "This review is no longer available.",
			recovery:  "Create a fresh plan from the current system state.",
			retryable: true,
		},
		{
			name:      "stale",
			err:       reviewedplan.ErrPlanStale,
			code:      "plan_stale",
			message:   "The system changed after this review was created.",
			recovery:  "Refresh the system state and review a fresh plan.",
			retryable: true,
		},
		{
			name:      "mismatch",
			err:       reviewedplan.ErrPlanMismatch,
			code:      "plan_mismatch",
			message:   "The reviewed changes could not be verified.",
			recovery:  "Create and review a fresh plan before applying changes.",
			retryable: true,
		},
		{
			name:      "confirmation",
			err:       app.ErrConfirmationRequired,
			code:      "confirmation_required",
			message:   "Approval is required before changes can be applied.",
			recovery:  "Review the pending changes and confirm them once.",
			retryable: true,
		},
		{
			name:      "cancelled",
			err:       context.Canceled,
			code:      "operation_cancelled",
			message:   "The operation was cancelled.",
			recovery:  "Refresh the current system state and review a fresh plan before retrying.",
			retryable: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := clientFailureFor(test.err)
			if failure.Code != test.code || failure.Message != test.message || failure.Recovery != test.recovery || failure.Retryable != test.retryable {
				t.Fatalf("failure = %#v", failure)
			}
		})
	}
}

func TestClientFailureForInstallationFailureAndUnknownError(t *testing.T) {
	install := clientFailureFor(errors.New("one or more selected installations failed; see transaction tx-1"))
	if install.Code != "installation_failed" || install.Message != "Some changes could not be completed." {
		t.Fatalf("install failure = %#v", install)
	}
	if !strings.Contains(install.Recovery, "Existing verified items were left unchanged") {
		t.Fatalf("install recovery = %q", install.Recovery)
	}

	unknown := clientFailureFor(errors.New(`open C:\\Users\\ACER\\AppData\\Local\\AgentStack\\plans\\missing.json: The system cannot find the file specified.`))
	if unknown.Code != "operation_failed" {
		t.Fatalf("unknown code = %q", unknown.Code)
	}
	combined := unknown.Message + unknown.Recovery
	if strings.Contains(combined, `C:\\`) || strings.Contains(strings.ToLower(combined), "appdata") {
		t.Fatalf("client copy leaked a path: %#v", unknown)
	}
}
