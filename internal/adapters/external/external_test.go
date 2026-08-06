package external

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/builtin"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/processctl"
	"github.com/agentstack/agentstack/internal/strictjson"
)

func TestExternalProtocolHelperProcess(t *testing.T) {
	marker := -1
	for i, value := range os.Args {
		if value == "asm-external-helper" {
			marker = i
			break
		}
	}
	if marker < 0 {
		t.Skip("helper process")
	}
	mode := "normal"
	target := builtin.TargetCodex
	if marker+1 < len(os.Args) {
		mode = os.Args[marker+1]
	}
	if marker+2 < len(os.Args) {
		target = os.Args[marker+2]
	}
	switch mode {
	case "timeout":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "overflow":
		_, _ = io.CopyN(os.Stdout, zeroReader{}, defaultMaxResponseBytes+1024)
		os.Exit(0)
	case "crash":
		_, _ = fmt.Fprintln(os.Stderr, "intentional crash")
		os.Exit(23)
	case "malformed":
		_, _ = io.WriteString(os.Stdout, "{not-json")
		os.Exit(0)
	}
	adapter, err := builtin.MustRegistry().Get(target)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if mode == "env-check" && os.Getenv("ASM_EXTERNAL_SECRET") != "" {
		_, _ = fmt.Fprintln(os.Stderr, "ambient environment leaked")
		os.Exit(2)
	}
	if mode == "capability-drift" {
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			os.Exit(2)
		}
		var request Request
		if err := strictjson.Decode(data, &request); err != nil {
			os.Exit(2)
		}
		selected := adapter
		if request.Operation == OperationCapabilities {
			markerPath := filepath.Join(".capability-seen")
			if _, err := os.Stat(markerPath); err == nil {
				selected = extraCapabilityAdapter{Adapter: adapter}
			} else if err := os.WriteFile(markerPath, []byte("seen"), 0o600); err != nil {
				os.Exit(2)
			}
		}
		if err := ServeOne(context.Background(), selected, bytes.NewReader(data), os.Stdout); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	switch mode {
	case "extra-capability":
		adapter = extraCapabilityAdapter{Adapter: adapter}
	case "escape-render":
		adapter = escapeRenderAdapter{Adapter: adapter}
	case "divergent-plan":
		adapter = divergentPlanAdapter{Adapter: adapter}
	case "response-mismatch", "stderr-success":
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			os.Exit(2)
		}
		var output bytes.Buffer
		if err := ServeOne(context.Background(), adapter, bytes.NewReader(data), &output); err != nil {
			os.Exit(2)
		}
		if mode == "response-mismatch" {
			var response Response
			if err := strictjson.Decode(output.Bytes(), &response); err != nil {
				os.Exit(2)
			}
			response.RequestID += "-wrong"
			encoded, _ := strictjson.MarshalCanonical(response)
			_, _ = os.Stdout.Write(encoded)
		} else {
			_, _ = os.Stdout.Write(output.Bytes())
			_, _ = fmt.Fprintln(os.Stderr, "unexpected successful diagnostic")
		}
		os.Exit(0)
	}
	if err := ServeOne(context.Background(), adapter, os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

type zeroReader struct{}

func (zeroReader) Read(data []byte) (int, error) {
	for i := range data {
		data[i] = 'x'
	}
	return len(data), nil
}

type extraCapabilityAdapter struct{ adapters.Adapter }

func (a extraCapabilityAdapter) Capabilities(ctx context.Context, environment adapters.Environment) (adapters.CapabilitySet, error) {
	value, err := a.Adapter.Capabilities(ctx, environment)
	if err != nil {
		return adapters.CapabilitySet{}, err
	}
	value.Aliases = append(value.Aliases, "codex-extra")
	value.DeploymentModes = append(value.DeploymentModes, "teleport")
	value.Artifacts[artifactgraph.KindAdapter] = adapters.ArtifactCapability{
		Support: adapters.SupportNative, Directory: ".external/adapters", Format: "opaque",
		Scopes: []string{"global"}, Fields: map[string]adapters.FieldSupport{"content": adapters.FieldNative},
	}
	return adapters.SealCapabilitySet(value)
}

type escapeRenderAdapter struct{ adapters.Adapter }

func (a escapeRenderAdapter) Render(ctx context.Context, request adapters.RenderRequest) (adapters.RenderedSet, adapters.LossReport, error) {
	rendered, report, err := a.Adapter.Render(ctx, request)
	if err != nil || len(rendered.Outputs) == 0 {
		return rendered, report, err
	}
	rendered.Outputs[0].Destination = filepath.Join(filepath.Dir(request.Environment.TargetRoot), "escape", filepath.Base(rendered.Outputs[0].Destination))
	rendered.Outputs[0].RelativeDestination = "escape/" + filepath.Base(rendered.Outputs[0].Destination)
	rendered, err = adapters.SealRenderedSet(rendered)
	return rendered, report, err
}

type divergentPlanAdapter struct{ adapters.Adapter }

func (a divergentPlanAdapter) Plan(ctx context.Context, request adapters.PlanRequest) ([]adapters.ProposedOperation, error) {
	operations, err := a.Adapter.Plan(ctx, request)
	if err == nil && len(operations) > 0 {
		operations[0].Action = adapters.ActionConflict
		operations[0].Reason = "external divergence"
	}
	return operations, err
}

func TestOpenPinsExecutableNegotiatesAndMatchesReference(t *testing.T) {
	admission, reference := helperAdmission(t, "normal", builtin.TargetCodex)
	candidate, err := Open(context.Background(), Config{Admission: admission, Reference: reference})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := candidate.Descriptor()
	if err := VerifyDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	if descriptor.Target != builtin.TargetCodex || descriptor.ExecutableDigest != admission.ExecutableDigest || descriptor.ProtocolVersion != ProtocolVersion {
		t.Fatalf("unexpected descriptor: %+v", descriptor)
	}
	got, err := candidate.Capabilities(context.Background(), admission.Environment)
	if err != nil {
		t.Fatal(err)
	}
	want, err := reference.Capabilities(context.Background(), admission.Environment)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effective capability differs from reviewed reference\n got=%+v\nwant=%+v", got, want)
	}
	sandbox := candidate.staged.sandbox
	if err := candidate.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sandbox); !os.IsNotExist(err) {
		t.Fatalf("sandbox remains after close: %v", err)
	}
}

func TestRunConformanceMatchesPhase5Corpus(t *testing.T) {
	if raceBuild {
		t.Skip("full subprocess conformance is covered by normal execution; focused host paths run under the race detector")
	}
	admission, reference := helperAdmission(t, "normal", builtin.TargetCodex)
	report, err := RunConformance(context.Background(), admission, reference)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyConformanceReport(report); err != nil {
		t.Fatal(err)
	}
	if !report.Passed || report.Summary.Cases == 0 || report.Summary.Matched != report.Summary.Cases || len(report.Mismatches) != 0 {
		t.Fatalf("unexpected differential report: %+v", report.Summary)
	}
}

func TestCapabilityIntersectionRemovesUnreviewedClaims(t *testing.T) {
	admission, reference := helperAdmission(t, "extra-capability", builtin.TargetCodex)
	candidate, err := Open(context.Background(), Config{Admission: admission, Reference: reference})
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	raw, effective, intersection, err := candidate.CapabilityEvidence(context.Background(), admission.Environment)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.Artifacts[artifactgraph.KindAdapter]; !ok {
		t.Fatal("helper did not expose the unreviewed capability")
	}
	if _, ok := effective.Artifacts[artifactgraph.KindAdapter]; ok {
		t.Fatal("unreviewed artifact capability survived intersection")
	}
	if !contains(intersection.Changes, "/artifacts/Adapter", CapabilityRestricted) || !contains(intersection.Changes, "/deploymentModes/teleport", CapabilityRestricted) {
		t.Fatalf("restriction evidence missing: %+v", intersection.Changes)
	}
	if err := VerifyIntersectionReport(intersection); err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionRejectsDigestMismatchAndSymlink(t *testing.T) {
	admission, reference := helperAdmission(t, "normal", builtin.TargetCodex)
	admission.ExecutableDigest = "sha256:" + strings.Repeat("0", 64)
	if _, err := Open(context.Background(), Config{Admission: admission, Reference: reference}); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("digest mismatch error=%v", err)
	}
	admission, reference = helperAdmission(t, "normal", builtin.TargetCodex)
	link := filepath.Join(t.TempDir(), "adapter-link")
	if err := os.Symlink(admission.Executable, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	admission.Executable = link
	if _, err := Open(context.Background(), Config{Admission: admission, Reference: reference}); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("symlink admission error=%v", err)
	}
}

func TestProcessLimitsAndProtocolIdentityFailClosed(t *testing.T) {
	cases := []struct {
		mode string
		want string
	}{
		{"timeout", "deadline"},
		{"overflow", "response exceeds"},
		{"crash", "process failed"},
		{"malformed", "decode external adapter response"},
		{"response-mismatch", "response identity mismatch"},
		{"stderr-success", "wrote to stderr"},
	}
	for _, item := range cases {
		t.Run(item.mode, func(t *testing.T) {
			admission, reference := helperAdmission(t, item.mode, builtin.TargetCodex)
			if item.mode == "timeout" {
				admission.Limits.Timeout = 50 * time.Millisecond
			}
			_, err := Open(context.Background(), Config{Admission: admission, Reference: reference})
			if err == nil || !strings.Contains(err.Error(), item.want) {
				t.Fatalf("error=%v want substring %q", err, item.want)
			}
		})
	}
}

func TestNormalizeLimitsRejectsInvalidProcessCeilings(t *testing.T) {
	for _, item := range []struct {
		name   string
		limits processctl.Limits
		want   string
	}{
		{name: "memory", limits: processctl.Limits{MemoryBytes: 1}, want: "memory limit"},
		{name: "cpu", limits: processctl.Limits{CPUPercent: 101}, want: "CPU percentage"},
		{name: "processes", limits: processctl.Limits{ActiveProcesses: 1025}, want: "active process limit"},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := normalizeLimits(Limits{Process: item.limits})
			if err == nil || !strings.Contains(err.Error(), item.want) {
				t.Fatalf("normalizeLimits error=%v want substring %q", err, item.want)
			}
		})
	}
}

func TestExternalRenderEscapeAndPlanDivergenceAreRejected(t *testing.T) {
	if raceBuild {
		t.Skip("full subprocess conformance is covered by normal execution; focused host paths run under the race detector")
	}
	for _, item := range []struct {
		mode string
		want string
	}{
		{"escape-render", "escapes the target root"},
		{"divergent-plan", "plan diverges"},
	} {
		t.Run(item.mode, func(t *testing.T) {
			admission, reference := helperAdmission(t, item.mode, builtin.TargetCodex)
			report, err := RunConformance(context.Background(), admission, reference)
			if err != nil {
				t.Fatal(err)
			}
			if report.Passed || len(report.Mismatches) == 0 {
				t.Fatalf("unsafe external adapter passed: %+v", report.Summary)
			}
			found := false
			for _, mismatch := range report.Mismatches {
				if strings.Contains(mismatch.Reason, item.want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing rejection %q: %+v", item.want, report.Mismatches)
			}
		})
	}
}

func TestConformanceReportTamperingIsRejected(t *testing.T) {
	if raceBuild {
		t.Skip("full subprocess conformance is covered by normal execution; focused host paths run under the race detector")
	}
	admission, reference := helperAdmission(t, "normal", builtin.TargetCodex)
	report, err := RunConformance(context.Background(), admission, reference)
	if err != nil {
		t.Fatal(err)
	}
	report.Summary.Matched--
	if err := VerifyConformanceReport(report); err == nil {
		t.Fatal("tampered external conformance report verified")
	}
}

func helperAdmission(t *testing.T, mode, target string) (Admission, adapters.Adapter) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	digest := digestFile(t, executable)
	reference, err := builtin.MustRegistry().Get(target)
	if err != nil {
		t.Fatal(err)
	}
	sandboxRoot := t.TempDir()
	environmentRoot := filepath.Join(sandboxRoot, "environment")
	for _, path := range []string{
		filepath.Join(environmentRoot, "project"), filepath.Join(environmentRoot, "target"),
		filepath.Join(environmentRoot, "home"), filepath.Join(environmentRoot, "home", ".gemini", "config"),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	environment := builtin.RuntimeEnvironment(
		filepath.Join(environmentRoot, "project"), filepath.Join(environmentRoot, "target"),
		filepath.Join(environmentRoot, "home"), filepath.Join(environmentRoot, "home", ".gemini", "config", "mcp_config.json"),
	)
	return Admission{
		Executable: executable, ExecutableDigest: digest,
		Arguments: []string{"-test.run=^TestExternalProtocolHelperProcess$", "--", "asm-external-helper", mode, target},
		Target:    target, SandboxRoot: sandboxRoot, Environment: environment, Limits: DefaultLimits(),
	}, reference
}

func digestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func contains(values []CapabilityChange, path string, kind CapabilityChangeKind) bool {
	for _, value := range values {
		if value.Path == path && value.Kind == kind {
			return true
		}
	}
	return false
}

func TestAdmittedExecutableIsPrivateAndEnvironmentIsSanitized(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	copyRoot := t.TempDir()
	copyName := "adapter-copy"
	if filepath.Ext(executable) != "" {
		copyName += filepath.Ext(executable)
	}
	copyPath := filepath.Join(copyRoot, copyName)
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, data, 0o700); err != nil {
		t.Fatal(err)
	}
	admission, reference := helperAdmission(t, "env-check", builtin.TargetCodex)
	admission.Executable = copyPath
	admission.ExecutableDigest = digestFile(t, copyPath)
	t.Setenv("ASM_EXTERNAL_SECRET", "must-not-leak")
	candidate, err := Open(context.Background(), Config{Admission: admission, Reference: reference})
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	if err := os.WriteFile(copyPath, []byte("tampered after admission"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := candidate.Capabilities(context.Background(), admission.Environment); err != nil {
		t.Fatalf("staged executable depended on mutated source or inherited environment: %v", err)
	}
	if !strings.HasPrefix(candidate.staged.path, candidate.staged.sandbox+string(filepath.Separator)) {
		t.Fatalf("staged executable is outside its private sandbox: %s", candidate.staged.path)
	}
}

func TestCapabilityDriftWithinSessionIsRejected(t *testing.T) {
	admission, reference := helperAdmission(t, "capability-drift", builtin.TargetCodex)
	candidate, err := Open(context.Background(), Config{Admission: admission, Reference: reference})
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	if _, err := candidate.Capabilities(context.Background(), admission.Environment); err == nil || !strings.Contains(err.Error(), "changed during the admitted session") {
		t.Fatalf("capability drift error=%v", err)
	}
}

func TestSandboxEnvironmentRedirectsCoverageIntoPrivateSandbox(t *testing.T) {
	sandbox := t.TempDir()
	parentCoverage := filepath.Join(t.TempDir(), "parent-coverage")
	t.Setenv("GOCOVERDIR", parentCoverage)

	environment, err := sandboxEnvironment(sandbox)
	if err != nil {
		t.Fatal(err)
	}
	coverage := ""
	for _, value := range environment {
		if strings.HasPrefix(value, "GOCOVERDIR=") {
			coverage = strings.TrimPrefix(value, "GOCOVERDIR=")
		}
		if strings.Contains(value, parentCoverage) {
			t.Fatalf("parent coverage path leaked into sandbox environment: %q", value)
		}
	}
	want := filepath.Join(sandbox, "coverage")
	if coverage != want {
		t.Fatalf("GOCOVERDIR=%q want %q", coverage, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("coverage directory mode=%v", info.Mode())
	}
}
