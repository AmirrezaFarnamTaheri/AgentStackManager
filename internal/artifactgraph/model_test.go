package artifactgraph

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSealIsDeterministicAcrossOrderingAndExtensionFormatting(t *testing.T) {
	left := testArtifact()
	left.Metadata.Tags = []string{"review", "safety", "review"}
	left.Metadata.Labels = map[string]string{"z": "last", "a": "first"}
	left.Security.Capabilities = []string{"process.spawn", "filesystem.read", "process.spawn"}
	left.Targets = []TargetBinding{{Target: "codex", Scope: "project"}, {Target: "claude", Scope: "project"}}
	left.Extensions = map[string]json.RawMessage{"resourcehub": json.RawMessage(` {"z":2,"a":1} `)}

	right := testArtifact()
	right.Metadata.Tags = []string{"safety", "review"}
	right.Metadata.Labels = map[string]string{"a": "first", "z": "last"}
	right.Security.Capabilities = []string{"filesystem.read", "process.spawn"}
	right.Targets = []TargetBinding{{Target: "claude", Scope: "project"}, {Target: "codex", Scope: "project"}}
	right.Extensions = map[string]json.RawMessage{"resourcehub": json.RawMessage(`{"a":1,"z":2}`)}

	sealedLeft, err := Seal(left)
	if err != nil {
		t.Fatal(err)
	}
	sealedRight, err := Seal(right)
	if err != nil {
		t.Fatal(err)
	}
	if sealedLeft.Digest != sealedRight.Digest {
		t.Fatalf("equivalent artifacts have different digests: %s %s", sealedLeft.Digest, sealedRight.Digest)
	}
	if got := strings.Join(sealedLeft.Metadata.Tags, ","); got != "review,safety" {
		t.Fatalf("tags were not normalized: %q", got)
	}
	if got := strings.Join(sealedLeft.Security.Capabilities, ","); got != "filesystem.read,process.spawn" {
		t.Fatalf("capabilities were not normalized: %q", got)
	}
	if sealedLeft.Targets[0].Target != "claude" || sealedLeft.Targets[1].Target != "codex" {
		t.Fatalf("targets were not normalized: %#v", sealedLeft.Targets)
	}
}

func TestVerifyRejectsTamperedArtifact(t *testing.T) {
	sealed, err := Seal(testArtifact())
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(sealed); err != nil {
		t.Fatal(err)
	}
	sealed.Metadata.Description = "tampered"
	if err := Verify(sealed); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("tampered artifact error=%v", err)
	}
}

func TestCanonicalJSONRoundTripsUnknownExtensions(t *testing.T) {
	value := testArtifact()
	value.Extensions = map[string]json.RawMessage{
		"vendor.alpha": json.RawMessage(`{"unknown":{"enabled":true},"large":9007199254740993}`),
	}
	data, err := CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Artifact
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := Verify(decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(decoded.Extensions["vendor.alpha"]), `9007199254740993`) {
		t.Fatalf("extension was not preserved: %s", decoded.Extensions["vendor.alpha"])
	}
}

func TestNewSnapshotSortsArtifactsAndRejectsDuplicateIDs(t *testing.T) {
	second := testArtifact()
	second.ID = "local/Skill/z-review"
	second.Metadata.Name = "z-review"
	first := testArtifact()
	first.ID = "local/Rule/a-safety"
	first.Kind = KindRule
	first.Metadata.Name = "a-safety"

	snapshot, err := NewSnapshot([]Artifact{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Artifacts[0].ID != first.ID || snapshot.Artifacts[1].ID != second.ID {
		t.Fatalf("snapshot order=%#v", snapshot.Artifacts)
	}
	if err := VerifySnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSnapshot([]Artifact{first, first}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error=%v", err)
	}
}

func TestSealRejectsInvalidSecurityIdentityAndExtensionJSON(t *testing.T) {
	cases := []Artifact{testArtifact(), testArtifact(), testArtifact(), testArtifact()}
	cases[0].ID = "invalid"
	cases[1].Security.ExecutionClass = ExecutionClass("ambient-root")
	cases[2].Extensions = map[string]json.RawMessage{"vendor": json.RawMessage(`{"a":1,"a":2}`)}
	cases[3].Metadata.Name = "different-name"
	for index, value := range cases {
		if _, err := Seal(value); err == nil {
			t.Fatalf("case %d was accepted", index)
		}
	}
}

func testArtifact() Artifact {
	instant := time.Date(2026, 8, 4, 12, 0, 0, 0, time.FixedZone("test", 3*60*60+30*60))
	return Artifact{
		ID:   "local/Skill/code-review",
		Kind: KindSkill,
		Metadata: Metadata{
			Namespace:   "local",
			Name:        "code-review",
			DisplayName: "Code Review",
			Description: "Review code safely",
			Scope:       "project",
		},
		Content: ContentReference{
			Ref:       "resourcehub://code-review/content",
			Digest:    "sha256:" + strings.Repeat("a", 64),
			MediaType: "application/vnd.agentstack.resource.v1",
		},
		Source: SourceReference{Type: "managed", URI: "resourcehub://code-review"},
		Security: SecurityClassification{
			ExecutionClass: ExecutionSandboxed,
		},
		Provenance: Provenance{
			Origin:     "asm.resourcehub/v1",
			ImportedBy: "resourcehub",
			ImportedAt: instant,
			UpdatedAt:  instant,
			Fields: map[string]FieldProvenance{
				"/content": {Source: "resourcehub", Path: "content"},
			},
		},
	}
}
