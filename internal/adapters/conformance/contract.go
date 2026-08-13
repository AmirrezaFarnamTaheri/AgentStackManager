package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

type FidelityRecord struct {
	SupportedFields   []string `json:"supportedFields"`
	UnsupportedFields []string `json:"unsupportedFields"`
	Lossy             bool     `json:"lossy"`
	LossDetails       []string `json:"lossDetails"`
}

type AdapterOperation string

const (
	OpDiscover  AdapterOperation = "discover"
	OpNormalize AdapterOperation = "normalize"
	OpRender    AdapterOperation = "render"
	OpPlan      AdapterOperation = "plan"
	OpVerify    AdapterOperation = "verify"
)

type OperationPlanStep struct {
	OpType      string `json:"opType"` // "create", "update", "delete", "noop"
	TargetPath  string `json:"targetPath"`
	BaseDigest  string `json:"baseDigest"`
	DesiredDigest string `json:"desiredDigest"`
}

type AdapterPlan struct {
	TargetID   string              `json:"targetId"`
	Steps      []OperationPlanStep `json:"steps"`
	PlanDigest string              `json:"planDigest"`
}

// PureAdapter Contract Interface
// Rule: Discover, Normalize, Render, Plan, and Verify MUST NEVER mutate filesystem, registry, or client state.
type PureAdapter interface {
	TargetID() string
	Discover(root string) ([]resourcehub.CandidateRevision, error)
	Normalize(raw []byte, kind resourcehub.Kind) (resourcehub.CandidateRevision, error)
	Render(canonical resourcehub.CandidateRevision, targetID string) ([]byte, FidelityRecord, error)
	Plan(current, desired []byte, targetPath string) (AdapterPlan, error)
	Verify(targetPath string, expectedDigest string) error
}

// ComputePlanDigest creates a deterministic SHA-256 digest of an adapter plan.
func ComputePlanDigest(plan AdapterPlan) string {
	var sb strings.Builder
	sb.WriteString(plan.TargetID)
	for _, s := range plan.Steps {
		sb.WriteString(fmt.Sprintf("|%s:%s:%s:%s", s.OpType, s.TargetPath, s.BaseDigest, s.DesiredDigest))
	}
	h := sha256.Sum256([]byte(sb.String()))
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))
}
