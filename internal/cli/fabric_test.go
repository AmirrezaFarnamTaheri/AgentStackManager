package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/builtin"
	"github.com/agentstack/agentstack/internal/adapters/conformance"
	"github.com/agentstack/agentstack/internal/adapters/external"
	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/artifactgraph"
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
	graphJSON := runFabricCLI(t, command, output, "hub", "graph")
	var graph artifactgraph.Snapshot
	if err := json.Unmarshal(graphJSON, &graph); err != nil {
		t.Fatal(err)
	}
	if err := artifactgraph.VerifySnapshot(graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Artifacts) != 1 || graph.Artifacts[0].ID != "local/Skill/safe-skill" {
		t.Fatalf("graph=%s", graphJSON)
	}
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

func TestHubAdaptersCLIResolvesAliasesAndEmitsVerifiedCapabilities(t *testing.T) {
	command, output := testFabricCLI(t)
	project := t.TempDir()
	payload := runFabricCLI(t, command, output, "hub", "adapters", "--project-root", project, "--target-root", project, "--target", "agy", "--target", "gemini")
	var response struct {
		Contract string                   `json:"contract"`
		Adapters []adapters.CapabilitySet `json:"adapters"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if response.Contract != adapters.ContractVersion || len(response.Adapters) != 1 || response.Adapters[0].Target != "agy" {
		t.Fatalf("unexpected adapters response: %s", payload)
	}
	if err := adapters.VerifyCapabilitySet(response.Adapters[0]); err != nil {
		t.Fatal(err)
	}
}

func TestHubAdapterConformanceCLIEmitsVerifiedReport(t *testing.T) {
	command, output := testFabricCLI(t)
	project := t.TempDir()
	payload := runFabricCLI(t, command, output, "hub", "adapter-conformance", "--project-root", project, "--target-root", project, "--target", "agy", "--target", "gemini")
	var report conformance.Report
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if err := conformance.VerifyReport(report); err != nil {
		t.Fatal(err)
	}
	if !report.Passed() || len(report.Targets) != 1 || report.Targets[0].Target != "agy" {
		t.Fatalf("unexpected conformance report: %s", payload)
	}
}

func TestExternalAdapterCLIHelperProcess(t *testing.T) {
	marker := -1
	for i, value := range os.Args {
		if value == "asm-cli-external-helper" {
			marker = i
			break
		}
	}
	if marker < 0 {
		t.Skip("helper process")
	}
	target := builtin.TargetCodex
	if marker+1 < len(os.Args) {
		target = os.Args[marker+1]
	}
	adapter, err := builtin.MustRegistry().Get(target)
	if err != nil {
		os.Exit(2)
	}
	if err := external.ServeOne(context.Background(), adapter, os.Stdin, os.Stdout); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestHubExternalAdapterConformanceCLIEmitsVerifiedReport(t *testing.T) {
	if raceBuild {
		t.Skip("full subprocess conformance is covered by normal execution; CLI parsing remains covered by the non-race integration test")
	}
	command, output := testFabricCLI(t)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	payload := runFabricCLI(t, command, output,
		"hub", "adapter-external-conformance",
		"--executable", executable,
		"--sha256", digest,
		"--target", "codex",
		"--arg=-test.run=^TestExternalAdapterCLIHelperProcess$",
		"--arg=--",
		"--arg=asm-cli-external-helper",
		"--arg=codex",
	)
	var report external.ConformanceReport
	if err := json.Unmarshal(payload, &report); err != nil {
		t.Fatal(err)
	}
	if err := external.VerifyConformanceReport(report); err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.Summary.Matched != report.Summary.Cases || report.Descriptor.Target != "codex" {
		t.Fatalf("unexpected external conformance report: %s", payload)
	}
}
