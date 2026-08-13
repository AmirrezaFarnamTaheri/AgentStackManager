package resourcehub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/artifactgraph"
)

func TestCanonicalSnapshotPreservesResourceIdentityProvenanceAndDigest(t *testing.T) {
	manager := New(t.TempDir())
	source := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(source, []byte("review safely\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resource, err := manager.Import(source, ImportOptions{
		ID:          "code-review",
		Kind:        KindSkill,
		Name:        "Code Review",
		Description: "Review code before mutation",
		Tags:        []string{"review", "safety", "review"},
		Targets:     []Agent{AgentCodex, AgentClaude},
		Scope:       "project",
		Metadata:    map[string]string{"owner": "platform", "maturity": "approved"},
	})
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := manager.CanonicalSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := artifactgraph.VerifySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Artifacts) != 1 {
		t.Fatalf("artifacts=%d", len(snapshot.Artifacts))
	}
	artifact := snapshot.Artifacts[0]
	if artifact.ID != "local/Skill/code-review" || artifact.Kind != artifactgraph.KindSkill {
		t.Fatalf("canonical identity=%#v", artifact)
	}
	if artifact.Content.Digest != resource.Digest || artifact.Content.Ref != "resourcehub://code-review/content" {
		t.Fatalf("content reference=%#v resource=%#v", artifact.Content, resource)
	}
	if artifact.Source.Type != "local-path" || artifact.Source.URI != source || artifact.Source.Revision != resource.Digest {
		t.Fatalf("source reference=%#v", artifact.Source)
	}
	if artifact.Security.ExecutionClass != artifactgraph.ExecutionSandboxed {
		t.Fatalf("execution class=%q", artifact.Security.ExecutionClass)
	}
	if len(artifact.Targets) != 2 || artifact.Targets[0].Target != "claude" || artifact.Targets[1].Target != "codex" {
		t.Fatalf("target bindings=%#v", artifact.Targets)
	}
	if artifact.Metadata.Labels["owner"] != "platform" || len(artifact.Metadata.Tags) != 2 {
		t.Fatalf("metadata=%#v", artifact.Metadata)
	}
	var extension resourceHubExtension
	if err := json.Unmarshal(artifact.Extensions["asm.resourcehub.v1"], &extension); err != nil {
		t.Fatal(err)
	}
	if extension.Entry != "content" || !extension.Enabled || extension.Metadata["maturity"] != "approved" {
		t.Fatalf("resource extension=%#v", extension)
	}

	reloaded, err := manager.CanonicalSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Digest != snapshot.Digest {
		t.Fatalf("canonical graph digest changed across reload: %s %s", snapshot.Digest, reloaded.Digest)
	}
}

func TestCanonicalSnapshotClassifiesCurrentResourceKindsConservatively(t *testing.T) {
	cases := []struct {
		kind      Kind
		canonical artifactgraph.Kind
		execution artifactgraph.ExecutionClass
	}{
		{KindRule, artifactgraph.KindRule, artifactgraph.ExecutionDeclarative},
		{KindPrompt, artifactgraph.KindPrompt, artifactgraph.ExecutionDeclarative},
		{KindCommand, artifactgraph.KindCommand, artifactgraph.ExecutionInterpreted},
		{KindAgent, artifactgraph.KindAgent, artifactgraph.ExecutionDeclarative},
		{KindMCPServer, artifactgraph.KindMCPServer, artifactgraph.ExecutionPrivileged},
		{KindContext, artifactgraph.KindContextResource, artifactgraph.ExecutionDeclarative},
	}
	for _, testCase := range cases {
		kind, execution, _, err := canonicalResourceKind(testCase.kind)
		if err != nil {
			t.Fatal(err)
		}
		if kind != testCase.canonical || execution != testCase.execution {
			t.Fatalf("kind %q mapped to %q/%q", testCase.kind, kind, execution)
		}
	}
}
