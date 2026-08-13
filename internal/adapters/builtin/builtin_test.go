package builtin

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
)

func TestBuiltinsExposeVersionedCapabilitySnapshots(t *testing.T) {
	registry := MustRegistry()
	root := t.TempDir()
	capabilities, err := registry.Capabilities(context.Background(), RuntimeEnvironment(root, root, root, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(capabilities) != 7 {
		t.Fatalf("capabilities=%d", len(capabilities))
	}
	for _, capability := range capabilities {
		if err := adapters.VerifyCapabilitySet(capability); err != nil {
			t.Fatalf("%s: %v", capability.Target, err)
		}
	}
	codex, err := registry.Get(TargetCodex)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := codex.Capabilities(context.Background(), RuntimeEnvironment(root, root, root, ""))
	if err != nil {
		t.Fatal(err)
	}
	if capability.MCP.RegistrationMode != adapters.MCPRegistrationCommand || capability.MCP.Location != "codex:mcp:agentstack-router" {
		t.Fatalf("codex MCP=%#v", capability.MCP)
	}
	agy, _ := registry.Get(TargetAgy)
	agyCapability, err := agy.Capabilities(context.Background(), RuntimeEnvironment(root, root, root, ""))
	if err != nil {
		t.Fatal(err)
	}
	wantAgy := filepath.Join(root, ".gemini", "config", "mcp_config.json")
	if agyCapability.MCP.Location != wantAgy {
		t.Fatalf("agy path=%q want %q", agyCapability.MCP.Location, wantAgy)
	}
}

func TestRenderProducesVisibleFidelityAndConfinedDestination(t *testing.T) {
	registry := MustRegistry()
	adapter, _ := registry.Get(TargetCodex)
	root := t.TempDir()
	artifact := mustArtifact(t, artifactgraph.KindAgent, "reviewer")
	rendered, report, err := adapter.Render(context.Background(), adapters.RenderRequest{Environment: RuntimeEnvironment(root, root, root, ""), Artifact: artifact, SourcePath: filepath.Join(root, "source")})
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered.Outputs) != 1 || rendered.Outputs[0].Destination != filepath.Join(root, ".codex", "agents", "reviewer.md") {
		t.Fatalf("rendered=%#v", rendered)
	}
	if report.Fidelity != adapters.FidelityPartial || len(report.Losses) != 1 || report.Losses[0].Code != "content-passthrough" {
		t.Fatalf("report=%#v", report)
	}

	skill := mustArtifact(t, artifactgraph.KindSkill, "lint")
	_, skillReport, err := adapter.Render(context.Background(), adapters.RenderRequest{Environment: RuntimeEnvironment(root, root, root, ""), Artifact: skill, SourcePath: filepath.Join(root, "skill")})
	if err != nil {
		t.Fatal(err)
	}
	if skillReport.Fidelity != adapters.FidelityFull || skillReport.HasLosses() {
		t.Fatalf("skill report=%#v", skillReport)
	}
}

func TestFallbackIsMachineReadable(t *testing.T) {
	adapter, _ := MustRegistry().Get(TargetCopilot)
	root := t.TempDir()
	artifact := mustArtifact(t, artifactgraph.KindAgent, "reviewer")
	rendered, report, err := adapter.Render(context.Background(), adapters.RenderRequest{Environment: RuntimeEnvironment(root, root, root, ""), Artifact: artifact, SourcePath: filepath.Join(root, "source")})
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Outputs[0].Support != adapters.SupportFallback || report.Fidelity != adapters.FidelityLossy {
		t.Fatalf("rendered=%#v report=%#v", rendered, report)
	}
	if report.Losses[0].Kind != adapters.LossFallback {
		t.Fatalf("loss=%#v", report.Losses[0])
	}
}

func mustArtifact(t *testing.T, kind artifactgraph.Kind, name string) artifactgraph.Artifact {
	t.Helper()
	value, err := artifactgraph.Seal(artifactgraph.Artifact{
		ID: "local/" + string(kind) + "/" + name, Kind: kind,
		Metadata:   artifactgraph.Metadata{Namespace: "local", Name: name},
		Content:    artifactgraph.ContentReference{Ref: "resourcehub://" + name, Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", MediaType: "text/markdown"},
		Source:     artifactgraph.SourceReference{Type: "managed", URI: "resourcehub://" + name},
		Security:   artifactgraph.SecurityClassification{ExecutionClass: artifactgraph.ExecutionDeclarative},
		Provenance: artifactgraph.Provenance{Origin: "test", ImportedBy: "test", ImportedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRegistryDeduplicatesCanonicalAndAliasRequests(t *testing.T) {
	registry := MustRegistry()
	root := t.TempDir()
	capabilities, err := registry.Capabilities(context.Background(), RuntimeEnvironment(root, root, root, ""), []string{TargetAgy, "gemini", "antigravity"})
	if err != nil {
		t.Fatal(err)
	}
	if len(capabilities) != 1 || capabilities[0].Target != TargetAgy {
		t.Fatalf("capabilities=%#v", capabilities)
	}
}

func TestPassthroughReportsPopulatedMetadataOmissions(t *testing.T) {
	adapter, err := MustRegistry().Get(TargetCodex)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	artifact := mustArtifact(t, artifactgraph.KindRule, "review")
	artifact.Metadata.Description = "Review changes"
	artifact.Metadata.Labels = map[string]string{"team": "platform"}
	artifact, err = artifactgraph.Seal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := adapter.Render(context.Background(), adapters.RenderRequest{
		Environment: RuntimeEnvironment(root, root, root, ""),
		Artifact:    artifact,
		SourcePath:  filepath.Join(root, "source"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Fidelity != adapters.FidelityLossy || len(report.Losses) != 3 {
		t.Fatalf("report=%#v", report)
	}
	want := []string{"content-passthrough", "metadata-description-omitted", "metadata-labels-omitted"}
	for index, code := range want {
		if report.Losses[index].Code != code {
			t.Fatalf("loss[%d]=%#v want code %q", index, report.Losses[index], code)
		}
	}
}
