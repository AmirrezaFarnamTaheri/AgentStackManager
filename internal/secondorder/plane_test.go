package secondorder_test

import (
	"testing"

	"github.com/agentstack/agentstack/internal/secondorder"
)

func TestSecondOrderConvergencePlanes(t *testing.T) {
	// 1. Contract generation
	contract, err := secondorder.GenerateTargetContract("opencode", "~/.config/opencode", []string{"skills", "prompts"})
	if err != nil {
		t.Fatalf("generate target contract: %v", err)
	}
	if contract.Digest == "" {
		t.Errorf("expected non-empty digest in target contract")
	}

	// 2. Governance profile import
	gov, err := secondorder.ImportGovernanceProfile("gov-profile-1", map[string]string{"max_files": "100"})
	if err != nil {
		t.Fatalf("import governance profile: %v", err)
	}
	if gov.ImportDigest == "" {
		t.Errorf("expected non-empty import digest")
	}

	// 3. Routing recommendation
	rec := secondorder.ComputeRoutingRecommendation("res-1", nil)
	if rec.PreferredTarget != "opencode" {
		t.Errorf("expected opencode preferred target, got %s", rec.PreferredTarget)
	}

	// 4. Evaluation harness
	evalRes := secondorder.RunEvaluationHarness("opencode")
	if !evalRes.Passed {
		t.Errorf("expected eval harness passed=true")
	}

	// 5. Provenance & Evidence
	prov := secondorder.SealProvenanceRecord("res-1", "sha256:abc", []string{"donor", "canonical"})
	if prov.EvidenceDigest == "" {
		t.Errorf("expected non-empty evidence digest")
	}

	// 6. Extension manifest (constrained capabilities, no general executable plugins)
	ext, err := secondorder.RegisterExtensionManifest("ext-1", []string{"read_only_analysis"})
	if err != nil {
		t.Fatalf("register extension manifest: %v", err)
	}
	if ext.ExecutablePlugins {
		t.Errorf("forbidden executablePlugins enabled in extension manifest")
	}
}
