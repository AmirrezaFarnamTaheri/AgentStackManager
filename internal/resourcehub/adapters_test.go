package resourcehub

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/builtin"
)

func TestPlanSyncBindsCapabilitiesAndEnforcesDenyLoss(t *testing.T) {
	manager := New(t.TempDir())
	rulePath := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(rulePath, []byte("review changes before apply\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rule, err := manager.Import(rulePath, ImportOptions{ID: "review", Kind: KindRule})
	if err != nil {
		t.Fatal(err)
	}
	targetRoot := t.TempDir()
	if err := manager.RegisterTarget(Target{ID: "codex", Agent: AgentCodex, Root: targetRoot, Mode: ModeCopy, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	plan, err := manager.PlanSync("codex", []string{rule.ID}, PlanOptions{TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := adapters.VerifyLossReport(plan.LossReport); err != nil {
		t.Fatal(err)
	}
	if plan.AdapterID == "" || plan.AdapterVersion == "" || plan.CapabilityDigest == "" {
		t.Fatalf("adapter snapshot missing: %#v", plan)
	}
	if plan.LossReport.Fidelity != adapters.FidelityPartial || len(plan.LossReport.Losses) != 1 {
		t.Fatalf("unexpected loss report: %#v", plan.LossReport)
	}
	if len(plan.Operations) != 1 {
		t.Fatalf("operations=%#v", plan.Operations)
	}
	operation := plan.Operations[0]
	if operation.AdapterID != plan.AdapterID || operation.CapabilityDigest != plan.CapabilityDigest || operation.Fidelity != adapters.FidelityPartial {
		t.Fatalf("operation is not capability-bound: %#v", operation)
	}
	registryState, err := manager.LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	_, capability, err := manager.targetCapability(registryState.Targets["codex"])
	if err != nil {
		t.Fatal(err)
	}
	if err := verifySyncOperationLoss(capability, operation); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.PlanSync("codex", []string{rule.ID}, PlanOptions{TTL: time.Hour, DenyLoss: true}); err == nil || !strings.Contains(err.Error(), "reported losses") {
		t.Fatalf("deny-loss did not reject partial fidelity: %v", err)
	}

	skillRoot := filepath.Join(t.TempDir(), "lint")
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("# Lint\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skill, err := manager.Import(skillRoot, ImportOptions{ID: "lint", Kind: KindSkill})
	if err != nil {
		t.Fatal(err)
	}
	full, err := manager.PlanSync("codex", []string{skill.ID}, PlanOptions{TTL: time.Hour, DenyLoss: true})
	if err != nil {
		t.Fatal(err)
	}
	if full.LossReport.Fidelity != adapters.FidelityFull || full.LossReport.HasLosses() {
		t.Fatalf("native skill should be full fidelity: %#v", full.LossReport)
	}
}

func TestApplySyncRejectsTamperedPerOperationLossBinding(t *testing.T) {
	manager := New(t.TempDir())
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("rule\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resource, err := manager.Import(source, ImportOptions{ID: "rule", Kind: KindRule})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RegisterTarget(Target{ID: "codex", Agent: AgentCodex, Root: t.TempDir(), Mode: ModeCopy, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	plan, err := manager.PlanSync("codex", []string{resource.ID}, PlanOptions{TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	plan.Operations[0].Losses = nil
	plan.Operations[0].Fidelity = adapters.FidelityFull
	plan.Digest, err = syncPlanDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(manager.Root, "plans", plan.ID+".json"), plan); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ApplySync(plan.ID, plan.Digest, true); err == nil || !strings.Contains(err.Error(), "loss report mismatch") {
		t.Fatalf("tampered operation loss binding was accepted: %v", err)
	}
}

func TestApplySyncRejectsCapabilityDrift(t *testing.T) {
	base, err := builtin.MustRegistry().Get(builtin.TargetCodex)
	if err != nil {
		t.Fatal(err)
	}
	drifting := &versionedAdapter{Adapter: base}
	registry, err := adapters.NewRegistry(drifting)
	if err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir())
	manager.Adapters = registry
	skillRoot := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resource, err := manager.Import(skillRoot, ImportOptions{ID: "safe", Kind: KindSkill})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RegisterTarget(Target{ID: "codex", Agent: AgentCodex, Root: t.TempDir(), Mode: ModeCopy, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	plan, err := manager.PlanSync("codex", []string{resource.ID}, PlanOptions{TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	drifting.version = "2.0.0"
	if _, err := manager.ApplySync(plan.ID, plan.Digest, true); err == nil || !strings.Contains(err.Error(), "capability changed") {
		t.Fatalf("capability drift was accepted: %v", err)
	}
}

type versionedAdapter struct {
	adapters.Adapter
	version string
}

func (a *versionedAdapter) Capabilities(ctx context.Context, environment adapters.Environment) (adapters.CapabilitySet, error) {
	capability, err := a.Adapter.Capabilities(ctx, environment)
	if err != nil {
		return adapters.CapabilitySet{}, err
	}
	if a.version != "" {
		capability.AdapterVersion = a.version
	}
	return adapters.SealCapabilitySet(capability)
}
