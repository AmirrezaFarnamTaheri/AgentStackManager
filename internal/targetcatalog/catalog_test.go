package targetcatalog_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/targetcatalog"
)

func TestCatalogInitializationAndSealing(t *testing.T) {
	cat, err := targetcatalog.New()
	if err != nil {
		t.Fatalf("failed to create default catalog: %v", err)
	}

	schema := cat.Schema()
	if err := targetcatalog.VerifyCatalogSchema(schema); err != nil {
		t.Errorf("verify catalog schema digest: %v", err)
	}

	if len(schema.Targets) < 20 {
		t.Errorf("expected at least 20 target definitions, got %d", len(schema.Targets))
	}
}

func TestCatalogLookupAndAliases(t *testing.T) {
	cat, err := targetcatalog.New()
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	// Canonical ID lookup
	target, found := cat.Lookup("antigravity-cli")
	if !found {
		t.Fatalf("expected target 'antigravity-cli' to be found")
	}
	if target.Vendor != "Google DeepMind" {
		t.Errorf("expected vendor 'Google DeepMind', got %s", target.Vendor)
	}

	// Alias lookup (.agents -> generic)
	genericTarget, found := cat.Lookup(".agents")
	if !found {
		t.Fatalf("expected alias '.agents' to resolve to generic target")
	}
	if genericTarget.ID != "generic" {
		t.Errorf("expected generic target ID, got %s", genericTarget.ID)
	}

	// Alias lookup (.claude -> claude)
	claudeTarget, found := cat.Lookup(".claude")
	if !found {
		t.Fatalf("expected alias '.claude' to resolve to claude target")
	}
	if claudeTarget.ID != "claude" {
		t.Errorf("expected claude target ID, got %s", claudeTarget.ID)
	}
}

func TestNestedGeminiChildren(t *testing.T) {
	cat, err := targetcatalog.New()
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	children := []string{"antigravity-cli", "antigravity-ide", "gemini-cli"}
	for _, childID := range children {
		target, found := cat.Lookup(childID)
		if !found {
			t.Errorf("expected nested .gemini child %q to be in catalog", childID)
		} else if target.Vendor != "Google DeepMind" {
			t.Errorf("expected vendor 'Google DeepMind' for %s, got %s", childID, target.Vendor)
		}
	}
}

func TestUnverifiedTargetsReadOnlyStatus(t *testing.T) {
	cat, err := targetcatalog.New()
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	unverified := []string{"nara", "openhuman", "openclaw", "qwen", "kimi"}
	for _, id := range unverified {
		target, found := cat.Lookup(id)
		if !found {
			t.Errorf("expected unverified target %q to be in catalog", id)
		} else if !target.ReadOnly {
			t.Errorf("expected unverified target %q to be read-only", id)
		} else if target.Confidence != targetcatalog.ConfidenceUnverified {
			t.Errorf("expected unverified confidence for %q, got %s", id, target.Confidence)
		}
	}
}

func TestDetectTargetFromPath(t *testing.T) {
	cat, err := targetcatalog.New()
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "asm-target-detect-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a .kiro project directory with steering marker
	kiroDir := filepath.Join(tmpDir, ".kiro")
	if err := os.MkdirAll(filepath.Join(kiroDir, "steering"), 0o755); err != nil {
		t.Fatalf("mkdir .kiro/steering: %v", err)
	}

	detected, found := cat.DetectTarget(kiroDir)
	if !found {
		t.Fatalf("expected .kiro path to be detected as target")
	}

	if detected.Definition.ID != "kiro" {
		t.Errorf("expected target ID 'kiro', got %s", detected.Definition.ID)
	}
	if !detected.Managed {
		t.Errorf("expected kiro target to be marked managed due to steering marker")
	}
}

func TestSchemaJSONUnmarshaling(t *testing.T) {
	cat, err := targetcatalog.New()
	if err != nil {
		t.Fatalf("create default catalog: %v", err)
	}

	data, err := json.MarshalIndent(cat.Schema(), "", "  ")
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}

	if err := os.WriteFile("default.json", data, 0o644); err != nil {
		t.Fatalf("write default.json: %v", err)
	}

	unmarshaled, err := targetcatalog.UnmarshalCatalogSchemaJSON(data)
	if err != nil {
		t.Fatalf("unmarshal sealed schema JSON: %v", err)
	}

	if len(unmarshaled.Targets) == 0 {
		t.Errorf("expected non-empty targets list from JSON")
	}
}
