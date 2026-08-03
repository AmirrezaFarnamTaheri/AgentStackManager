package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/runner"
)

type fabricCommandRunner struct{}

func (fabricCommandRunner) Run(context.Context, runner.Invocation) runner.Result {
	return runner.Result{Stdout: "ok\n", ExitCode: 0}
}

func testFabricCLI(t *testing.T) (*CLI, *bytes.Buffer) {
	t.Helper()
	root := t.TempDir()
	output := &bytes.Buffer{}
	service := &app.Service{Paths: app.Paths{DataRoot: root, Executable: filepath.Join(root, "agentstack"), RouterConfig: filepath.Join(root, "router.json"), AgyConfig: filepath.Join(root, "agy.json")}, Commands: fabricCommandRunner{}}
	return &CLI{Service: service, Out: output, Err: output}, output
}

func runFabricCLI(t *testing.T, command *CLI, output *bytes.Buffer, args ...string) []byte {
	t.Helper()
	output.Reset()
	if code := command.Run(context.Background(), args); code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, output.String())
	}
	return append([]byte(nil), output.Bytes()...)
}

func TestUnifiedWorkspaceMemoryArtifactAndRoutineCLI(t *testing.T) {
	command, output := testFabricCLI(t)
	project := t.TempDir()
	runFabricCLI(t, command, output, "workspace", "create", "--id", "asm", "--name", "ASM", "--root", project, "--prompt", "Review ${workspace.name}: ${goal}", "--var", "goal=convergence")
	rendered := runFabricCLI(t, command, output, "workspace", "render", "--id", "asm")
	if !bytes.Contains(rendered, []byte("Review ASM: convergence")) {
		t.Fatalf("render=%s", rendered)
	}
	runFabricCLI(t, command, output, "memory", "remember", "--layer", "workspace", "--scope", "asm", "--key", "architecture", "--value", "five planes")
	recalled := runFabricCLI(t, command, output, "memory", "recall", "--workspace", "asm", "--key", "architecture")
	if !bytes.Contains(recalled, []byte("five planes")) {
		t.Fatalf("recall=%s", recalled)
	}
	artifactSource := filepath.Join(t.TempDir(), "evidence.txt")
	if err := os.WriteFile(artifactSource, []byte("verified evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "artifact", "add", "--workspace", "asm", "--id", "proof", "--path", artifactSource)
	verified := runFabricCLI(t, command, output, "artifact", "verify", "--id", "proof")
	if !bytes.Contains(verified, []byte(`"verified": true`)) {
		t.Fatalf("verify=%s", verified)
	}
	routineFile := filepath.Join(t.TempDir(), "routine.json")
	if err := os.WriteFile(routineFile, []byte(`{"id":"daily","workspaceId":"asm","name":"Daily","enabled":true,"schedule":{"kind":"manual"},"steps":[{"id":"echo","kind":"command","command":"echo","args":["ok"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "routine", "put", "--file", routineFile)
	report := runFabricCLI(t, command, output, "routine", "run", "--id", "daily", "--yes")
	if !bytes.Contains(report, []byte(`"status": "succeeded"`)) {
		t.Fatalf("routine=%s", report)
	}
}

func TestResourceHubContextAndMCPClientCLI(t *testing.T) {
	command, output := testFabricCLI(t)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/demo\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n// context bridge\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	search := runFabricCLI(t, command, output, "context", "search", "--root", project, "--query", "context bridge")
	if !bytes.Contains(search, []byte(`"line": 2`)) {
		t.Fatalf("search=%s", search)
	}
	resourceSource := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(resourceSource, []byte("safe skill\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "hub", "import", "--id", "safe-skill", "--kind", "skill", "--path", resourceSource)
	target := filepath.Join(t.TempDir(), "codex")
	runFabricCLI(t, command, output, "hub", "target-add", "--id", "codex-local", "--agent", "codex", "--root", target, "--mode", "copy")
	planJSON := runFabricCLI(t, command, output, "hub", "plan-sync", "--target", "codex-local")
	var plan struct{ ID, Digest string }
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "hub", "apply-sync", "--plan-id", plan.ID, "--digest", plan.Digest, "--yes")
	if _, err := os.Stat(filepath.Join(target, ".agents", "skills", "safe-skill")); err != nil {
		t.Fatal(err)
	}
	mcpPlanJSON := runFabricCLI(t, command, output, "mcp", "clients", "plan", "--root", project, "--client", "cursor")
	var mcpPlan struct{ ID, Digest string }
	if err := json.Unmarshal(mcpPlanJSON, &mcpPlan); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "mcp", "clients", "apply", "--root", project, "--plan-id", mcpPlan.ID, "--digest", mcpPlan.Digest, "--yes")
	if _, err := os.Stat(filepath.Join(project, ".cursor", "mcp.json")); err != nil {
		t.Fatal(err)
	}
}

func TestConvergedMutationCommandsFailClosedWithoutYes(t *testing.T) {
	command, output := testFabricCLI(t)
	for _, args := range [][]string{{"hub", "apply-sync", "--plan-id", "p", "--digest", "d"}, {"context", "apply", "--plan-id", "p", "--digest", "d"}, {"routine", "run", "--id", "r"}, {"mcp", "clients", "apply", "--plan-id", "p", "--digest", "d"}} {
		output.Reset()
		if code := command.Run(context.Background(), args); code != 2 {
			t.Fatalf("%v code=%d output=%s", args, code, output.String())
		}
	}
}

func TestReadStrictJSONRejectsOversizedInputBeforeDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte(" "), maxStrictJSONInputBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}

	var value map[string]any
	err := readStrictJSON(path, &value)
	if err == nil || !strings.Contains(err.Error(), "strict JSON input exceeds") {
		t.Fatalf("error=%v", err)
	}
}

func TestReadStrictJSONRejectsNonRegularInput(t *testing.T) {
	var value map[string]any
	err := readStrictJSON(t.TempDir(), &value)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("error=%v", err)
	}
}
