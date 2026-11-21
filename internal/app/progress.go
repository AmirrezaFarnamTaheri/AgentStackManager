package app

import (
	"strings"

	"github.com/agentstack/agentstack/internal/model"
)

// ApplyProgress is the sanitized, user-facing lifecycle of one reviewed apply.
// It deliberately omits commands, arguments, paths, stdout, and stderr.
type ApplyProgress struct {
	Phase string `json:"phase"`
	// Completed is retained for older clients and is identical to Processed.
	Completed    int                 `json:"completed"`
	Processed    int                 `json:"processed"`
	Succeeded    int                 `json:"succeeded"`
	Failed       int                 `json:"failed"`
	Skipped      int                 `json:"skipped"`
	Total        int                 `json:"total"`
	CurrentID    string              `json:"currentId,omitempty"`
	CurrentLabel string              `json:"currentLabel,omitempty"`
	Items        []ApplyProgressItem `json:"items,omitempty"`
}

type ApplyProgressItem struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type applyProgressTracker struct {
	items      []ApplyProgressItem
	index      map[string]int
	routerItem map[string]bool
}

func newApplyProgressTracker(plan model.Plan) *applyProgressTracker {
	tracker := &applyProgressTracker{index: map[string]int{}, routerItem: map[string]bool{}}
	for _, action := range plan.Actions {
		if !trackedApplyAction(action.Kind) {
			continue
		}
		label := strings.TrimSpace(action.Name)
		if label == "" {
			label = action.ComponentID
		}
		tracker.index[action.ComponentID] = len(tracker.items)
		tracker.items = append(tracker.items, ApplyProgressItem{
			ID: action.ComponentID, Label: label, Action: string(action.Kind), Status: "waiting",
		})
		if action.Kind == model.ActionConfigure && action.Install.Kind == model.InstallRouter {
			tracker.routerItem[action.ComponentID] = true
		}
	}
	return tracker
}

func trackedApplyAction(kind model.ActionKind) bool {
	switch kind {
	case model.ActionInstall, model.ActionRepair, model.ActionConfigure:
		return true
	default:
		return false
	}
}

func (t *applyProgressTracker) start(action model.PlanAction) ApplyProgress {
	index, ok := t.index[action.ComponentID]
	if !ok || t.routerItem[action.ComponentID] {
		return t.snapshot("installing", "")
	}
	t.items[index].Status = "running"
	t.items[index].Message = actionVerb(action.Kind) + " in progress"
	phase := "installing"
	if action.Kind == model.ActionConfigure {
		phase = "configuring"
	}
	return t.snapshot(phase, action.ComponentID)
}

func (t *applyProgressTracker) updateTransaction(tx model.Transaction) ApplyProgress {
	for _, action := range tx.Actions {
		index, ok := t.index[action.ComponentID]
		if !ok || t.routerItem[action.ComponentID] {
			continue
		}
		if action.Error != "" {
			t.items[index].Status = "failed"
			t.items[index].Message = "Needs attention"
		} else {
			t.items[index].Status = "succeeded"
			t.items[index].Message = "Completed"
		}
	}
	return t.snapshot("installing", "")
}

func (t *applyProgressTracker) startRouter() ApplyProgress {
	current := ""
	for id := range t.routerItem {
		index := t.index[id]
		t.items[index].Status = "running"
		t.items[index].Message = "Configuring connection"
		if current == "" {
			current = id
		}
	}
	return t.snapshot("configuring", current)
}

func (t *applyProgressTracker) finishRouter(err error) ApplyProgress {
	for id := range t.routerItem {
		index := t.index[id]
		if err != nil {
			t.items[index].Status = "failed"
			t.items[index].Message = "Connection needs attention"
		} else {
			t.items[index].Status = "succeeded"
			t.items[index].Message = "Connection configured"
		}
	}
	return t.snapshot("configuring", "")
}

func (t *applyProgressTracker) verifying() ApplyProgress {
	return t.snapshot("verifying", "")
}

func (t *applyProgressTracker) complete() ApplyProgress {
	for index := range t.items {
		if t.items[index].Status == "waiting" || t.items[index].Status == "running" {
			t.items[index].Status = "skipped"
			t.items[index].Message = "Not completed"
		}
	}
	return t.snapshot("complete", "")
}

func (t *applyProgressTracker) snapshot(phase, currentID string) ApplyProgress {
	progress := ApplyProgress{Phase: phase, Total: len(t.items), CurrentID: currentID, Items: append([]ApplyProgressItem(nil), t.items...)}
	for _, item := range progress.Items {
		switch item.Status {
		case "succeeded":
			progress.Succeeded++
			progress.Processed++
		case "failed":
			progress.Failed++
			progress.Processed++
		case "skipped", "not-needed":
			progress.Skipped++
			progress.Processed++
		}
		if item.ID == currentID {
			progress.CurrentLabel = item.Label
		}
	}
	progress.Completed = progress.Processed
	return progress
}

func actionVerb(kind model.ActionKind) string {
	switch kind {
	case model.ActionRepair:
		return "Repair"
	case model.ActionConfigure:
		return "Configuration"
	default:
		return "Installation"
	}
}
