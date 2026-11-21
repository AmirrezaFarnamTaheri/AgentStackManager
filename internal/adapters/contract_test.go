package adapters

import (
	"context"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/artifactgraph"
)

func TestCapabilityAndLossDigestsAreDeterministic(t *testing.T) {
	capability, err := SealCapabilitySet(CapabilitySet{
		AdapterID: "asm.builtin.codex", AdapterVersion: "1.0.0", Target: "codex", TargetVersionRange: "*",
		Aliases: []string{"codex", "codex"}, DeploymentModes: []string{"link", "copy"},
		Artifacts: map[artifactgraph.Kind]ArtifactCapability{
			artifactgraph.KindSkill: {Support: SupportNative, Scopes: []string{"project", "project"}, Directory: ".agents/skills"},
		},
		MCP: MCPClientCapability{Support: SupportNative, RegistrationMode: MCPRegistrationCommand, Location: "codex:mcp:agentstack-router", EntryName: "agentstack-router", Transports: []string{"stdio"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCapabilitySet(capability); err != nil {
		t.Fatal(err)
	}
	second, err := SealCapabilitySet(capability)
	if err != nil {
		t.Fatal(err)
	}
	if second.Digest != capability.Digest {
		t.Fatalf("digest changed: %s != %s", second.Digest, capability.Digest)
	}

	report, err := SealLossReport(LossReport{Target: "codex", AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest, Losses: []Loss{
		{ArtifactID: "local/Agent/reviewer", Field: "/content", Kind: LossTransformation, Code: "content-passthrough", Reason: "target-specific encoding is not applied"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Fidelity != FidelityPartial || !report.HasLosses() {
		t.Fatalf("report=%#v", report)
	}
	if err := VerifyLossReport(report); err != nil {
		t.Fatal(err)
	}
}

func TestPlanStateMachinePreservesForeignAndDivergedState(t *testing.T) {
	adapter := &fakeAdapter{id: "codex"}
	capability, _ := SealCapabilitySet(CapabilitySet{AdapterID: "fake", AdapterVersion: "1", Target: "codex", TargetVersionRange: "*", MCP: MCPClientCapability{Support: SupportUnsupported, RegistrationMode: MCPRegistrationNone}})
	loss, _ := SealLossReport(LossReport{Target: "codex", AdapterID: "fake", AdapterVersion: "1", CapabilityDigest: capability.Digest})
	cases := []struct {
		name string
		req  PlanRequest
		want Action
	}{
		{"create", PlanRequest{Mode: PresencePresent, Rendered: RenderedArtifact{ArtifactID: "a", Destination: "x", DesiredDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Observed: ObservedArtifact{}, Capability: capability, LossReport: loss}, ActionCreate},
		{"noop", PlanRequest{Mode: PresencePresent, Rendered: RenderedArtifact{ArtifactID: "a", Destination: "x", DesiredDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Observed: ObservedArtifact{Exists: true, Equivalent: true, Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, Capability: capability, LossReport: loss}, ActionNoop},
		{"update-owned", PlanRequest{Mode: PresencePresent, Rendered: RenderedArtifact{ArtifactID: "a", Destination: "x", DesiredDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Observed: ObservedArtifact{Exists: true, Owned: true, Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", BaseDigest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, Capability: capability, LossReport: loss}, ActionUpdate},
		{"diverged", PlanRequest{Mode: PresencePresent, Rendered: RenderedArtifact{ArtifactID: "a", Destination: "x", DesiredDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Observed: ObservedArtifact{Exists: true, Owned: true, Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", BaseDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}, Capability: capability, LossReport: loss}, ActionConflict},
		{"foreign", PlanRequest{Mode: PresencePresent, Rendered: RenderedArtifact{ArtifactID: "a", Destination: "x", DesiredDigest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Observed: ObservedArtifact{Exists: true, Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, Capability: capability, LossReport: loss}, ActionConflict},
		{"remove-owned", PlanRequest{Mode: PresenceAbsent, Rendered: RenderedArtifact{ArtifactID: "a", Destination: "x"}, Observed: ObservedArtifact{Exists: true, Owned: true, Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, Capability: capability, LossReport: loss}, ActionRemove},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			operations, err := adapter.Plan(context.Background(), tc.req)
			if err != nil {
				t.Fatal(err)
			}
			if len(operations) != 1 || operations[0].Action != tc.want {
				t.Fatalf("operations=%#v", operations)
			}
		})
	}
}

type fakeAdapter struct{ id string }

func (f *fakeAdapter) ID() string            { return f.id }
func (f *fakeAdapter) SchemaVersion() string { return ContractVersion }
func (f *fakeAdapter) Capabilities(context.Context, Environment) (CapabilitySet, error) {
	return CapabilitySet{}, nil
}
func (f *fakeAdapter) Discover(context.Context, DiscoverRequest) ([]ObservedArtifact, error) {
	return nil, nil
}
func (f *fakeAdapter) Import(context.Context, ImportRequest) (artifactgraph.Artifact, LossReport, error) {
	return artifactgraph.Artifact{}, LossReport{}, nil
}
func (f *fakeAdapter) Render(context.Context, RenderRequest) (RenderedSet, LossReport, error) {
	return RenderedSet{}, LossReport{}, nil
}
func (f *fakeAdapter) Plan(_ context.Context, req PlanRequest) ([]ProposedOperation, error) {
	return Plan(req)
}
func (f *fakeAdapter) Verify(_ context.Context, req VerifyRequest) (VerificationResult, error) {
	return Verify(req), nil
}

func TestPlanRejectsMismatchedLossIdentityAndInvalidDigests(t *testing.T) {
	capability, err := SealCapabilitySet(CapabilitySet{
		AdapterID: "asm.builtin.codex", AdapterVersion: "1.0.0", Target: "codex", TargetVersionRange: "*",
		MCP: MCPClientCapability{Support: SupportUnsupported, RegistrationMode: MCPRegistrationNone},
	})
	if err != nil {
		t.Fatal(err)
	}
	mismatched, err := SealLossReport(LossReport{Target: "claude", AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Plan(PlanRequest{
		Mode:       PresencePresent,
		Rendered:   RenderedArtifact{ArtifactID: "local/Rule/review", Destination: "rules/review.md", DesiredDigest: "sha256:" + strings.Repeat("a", 64)},
		Capability: capability, LossReport: mismatched,
	})
	if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("mismatched loss identity accepted: %v", err)
	}

	validLoss, err := SealLossReport(LossReport{Target: capability.Target, AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Plan(PlanRequest{
		Mode:       PresencePresent,
		Rendered:   RenderedArtifact{ArtifactID: "local/Rule/review", Destination: "rules/review.md", DesiredDigest: "not-a-digest"},
		Capability: capability, LossReport: validLoss,
	})
	if err == nil || !strings.Contains(err.Error(), "desired digest") {
		t.Fatalf("invalid desired digest accepted: %v", err)
	}
}

func TestRenderedSetRejectsTraversalAndInvalidDigest(t *testing.T) {
	base := RenderedSet{
		AdapterID: "asm.builtin.codex", AdapterVersion: "1.0.0", Target: "codex",
		Outputs: []RenderedArtifact{{
			ArtifactID: "local/Rule/review", Kind: artifactgraph.KindRule,
			Destination: "/project/.agents/rules/review.md", RelativeDestination: "../escape.md",
			DesiredDigest: "sha256:" + strings.Repeat("a", 64), Support: SupportNative,
		}},
	}
	if _, err := SealRenderedSet(base); err == nil || !strings.Contains(err.Error(), "relative destination") {
		t.Fatalf("traversal was accepted: %v", err)
	}
	base.Outputs[0].RelativeDestination = ".agents/rules/review.md"
	base.Outputs[0].DesiredDigest = "invalid"
	if _, err := SealRenderedSet(base); err == nil || !strings.Contains(err.Error(), "rendered output") {
		t.Fatalf("invalid digest was accepted: %v", err)
	}
}

func TestVerifyAcceptsEquivalentNoopWithoutRewritingDigest(t *testing.T) {
	result := Verify(VerifyRequest{
		Operation: ProposedOperation{Action: ActionNoop, AfterDigest: "sha256:" + strings.Repeat("a", 64)},
		Observed:  ObservedArtifact{Exists: true, Equivalent: true, Digest: "sha256:" + strings.Repeat("b", 64)},
	})
	if !result.Verified {
		t.Fatalf("result=%#v", result)
	}
}

func TestVerifyAcceptsAbsentNoopPostcondition(t *testing.T) {
	result := Verify(VerifyRequest{Operation: ProposedOperation{Action: ActionNoop, AfterDigest: "absent"}, Observed: ObservedArtifact{Exists: false}})
	if !result.Verified {
		t.Fatalf("result=%#v", result)
	}
	result = Verify(VerifyRequest{Operation: ProposedOperation{Action: ActionNoop, AfterDigest: "absent"}, Observed: ObservedArtifact{Exists: true}})
	if result.Verified {
		t.Fatalf("existing target verified against absent noop: %#v", result)
	}
}

func TestInactiveMCPCapabilityRejectsRegistrationFields(t *testing.T) {
	_, err := SealCapabilitySet(CapabilitySet{
		AdapterID: "asm.builtin.generic", AdapterVersion: "1.0.0", Target: "generic", TargetVersionRange: "*",
		MCP: MCPClientCapability{Support: SupportUnsupported, RegistrationMode: MCPRegistrationNone, EntryName: "agentstack-router"},
	})
	if err == nil || !strings.Contains(err.Error(), "must not advertise") {
		t.Fatalf("inactive registration fields accepted: %v", err)
	}
}
