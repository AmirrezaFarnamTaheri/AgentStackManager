package ui

import (
	"context"
	"errors"
	"strings"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/reviewedplan"
)

// ClientFailure is the stable, path-free failure contract exposed to the
// browser. Internal errors remain available to server logs and transaction
// evidence, but never become user-facing copy.
type ClientFailure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Recovery  string `json:"recovery,omitempty"`
	Retryable bool   `json:"retryable"`
}

func clientFailureFor(err error) ClientFailure {
	switch {
	case errors.Is(err, context.Canceled):
		return ClientFailure{
			Code:      "operation_cancelled",
			Message:   "The operation was cancelled.",
			Recovery:  "Refresh the current system state and review a fresh plan before retrying.",
			Retryable: true,
		}
	case errors.Is(err, reviewedplan.ErrPlanUnavailable):
		return ClientFailure{
			Code:      "plan_unavailable",
			Message:   "This review is no longer available.",
			Recovery:  "Create a fresh plan from the current system state.",
			Retryable: true,
		}
	case errors.Is(err, reviewedplan.ErrPlanStale):
		return ClientFailure{
			Code:      "plan_stale",
			Message:   "The system changed after this review was created.",
			Recovery:  "Refresh the system state and review a fresh plan.",
			Retryable: true,
		}
	case errors.Is(err, reviewedplan.ErrPlanMismatch):
		return ClientFailure{
			Code:      "plan_mismatch",
			Message:   "The reviewed changes could not be verified.",
			Recovery:  "Create and review a fresh plan before applying changes.",
			Retryable: true,
		}
	case errors.Is(err, app.ErrConfirmationRequired):
		return ClientFailure{
			Code:      "confirmation_required",
			Message:   "Approval is required before changes can be applied.",
			Recovery:  "Review the pending changes and confirm them once.",
			Retryable: true,
		}
	case strings.Contains(strings.ToLower(err.Error()), "one or more selected installations failed"):
		return ClientFailure{
			Code:      "installation_failed",
			Message:   "Some changes could not be completed.",
			Recovery:  "Existing verified items were left unchanged. Retry failed items or review a fresh plan.",
			Retryable: true,
		}
	default:
		return ClientFailure{
			Code:      "operation_failed",
			Message:   "AgentStack could not complete this operation.",
			Recovery:  "Review the technical details, refresh the system state, and try again.",
			Retryable: true,
		}
	}
}
