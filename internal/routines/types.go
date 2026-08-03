package routines

import "time"

type ScheduleKind string

const (
	ScheduleManual   ScheduleKind = "manual"
	ScheduleDaily    ScheduleKind = "daily"
	ScheduleWeekdays ScheduleKind = "weekdays"
	ScheduleInterval ScheduleKind = "interval"
)

type Schedule struct {
	Kind         ScheduleKind `json:"kind"`
	At           string       `json:"at,omitempty"`
	Timezone     string       `json:"timezone,omitempty"`
	EveryMinutes int          `json:"everyMinutes,omitempty"`
}

type StepKind string

const (
	StepInventory           StepKind = "inventory"
	StepMCPDoctor           StepKind = "mcp-doctor"
	StepContextScan         StepKind = "context-scan"
	StepContextScore        StepKind = "context-score"
	StepMemorySearch        StepKind = "memory-search"
	StepPromptRender        StepKind = "prompt-render"
	StepArtifactVerify      StepKind = "artifact-verify"
	StepResourceAudit       StepKind = "resource-audit"
	StepResourceRefreshPlan StepKind = "resource-refresh-plan"
	StepCommand             StepKind = "command"
)

type Step struct {
	ID             string            `json:"id"`
	Kind           StepKind          `json:"kind"`
	Params         map[string]string `json:"params,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
}

type Routine struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId,omitempty"`
	Name        string    `json:"name"`
	Enabled     bool      `json:"enabled"`
	Schedule    Schedule  `json:"schedule"`
	Steps       []Step    `json:"steps"`
	LastRun     time.Time `json:"lastRun,omitempty"`
	NextRun     time.Time `json:"nextRun,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
)

type StepReport struct {
	StepID     string    `json:"stepId"`
	Kind       StepKind  `json:"kind"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Output     any       `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
}

type RunReport struct {
	Schema      string       `json:"schema,omitempty"`
	Version     int          `json:"version,omitempty"`
	ID          string       `json:"id"`
	RoutineID   string       `json:"routineId"`
	WorkspaceID string       `json:"workspaceId,omitempty"`
	Status      RunStatus    `json:"status"`
	StartedAt   time.Time    `json:"startedAt"`
	FinishedAt  time.Time    `json:"finishedAt"`
	Steps       []StepReport `json:"steps"`
	Error       string       `json:"error,omitempty"`
}
