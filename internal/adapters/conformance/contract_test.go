package conformance_test

import (
	"testing"

	"github.com/agentstack/agentstack/internal/adapters/conformance"
)

func TestComputePlanDigestDeterministic(t *testing.T) {
	plan1 := conformance.AdapterPlan{
		TargetID: "opencode",
		Steps: []conformance.OperationPlanStep{
			{OpType: "create", TargetPath: ".opencode/skills/test-skill/SKILL.md", BaseDigest: "", DesiredDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
		},
	}
	plan2 := conformance.AdapterPlan{
		TargetID: "opencode",
		Steps: []conformance.OperationPlanStep{
			{OpType: "create", TargetPath: ".opencode/skills/test-skill/SKILL.md", BaseDigest: "", DesiredDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111"},
		},
	}

	d1 := conformance.ComputePlanDigest(plan1)
	d2 := conformance.ComputePlanDigest(plan2)

	if d1 != d2 {
		t.Errorf("expected deterministic plan digests: got %s vs %s", d1, d2)
	}
	if len(d1) != 71 { // sha256: + 64 hex chars
		t.Errorf("expected 71-char sha256 digest string, got len %d: %s", len(d1), d1)
	}
}
