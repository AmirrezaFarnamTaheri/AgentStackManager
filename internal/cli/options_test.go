package cli

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/selfinstall"
	"github.com/agentstack/agentstack/internal/state"
)

func TestParseSelectionOptions(t *testing.T) {
	request, rest, err := parseSelection([]string{
		"--profile", "recommended",
		"--include", "ollama,aider",
		"--exclude", "vale",
		"--allow-credentials",
		"--provider", "browser=chrome-devtools-mcp",
		"--", "--model", "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.Profile != "recommended" || !request.AllowCredentialed {
		t.Fatalf("unexpected request %#v", request)
	}
	if !reflect.DeepEqual(request.Include, []string{"ollama", "aider"}) {
		t.Fatalf("unexpected include %#v", request.Include)
	}
	if !reflect.DeepEqual(request.Exclude, []string{"vale"}) {
		t.Fatalf("unexpected exclude %#v", request.Exclude)
	}
	if request.ProviderOverrides["browser"] != "chrome-devtools-mcp" {
		t.Fatalf("unexpected overrides %#v", request.ProviderOverrides)
	}
	if !reflect.DeepEqual(rest, []string{"--model", "x"}) {
		t.Fatalf("unexpected rest %#v", rest)
	}
}

func TestSetupNoLaunchInstallsWithoutStartingUI(t *testing.T) {
	var output bytes.Buffer
	called := 0
	command := &CLI{
		Service: &app.Service{},
		Out:     &output,
		Err:     &output,
		InstallSelf: func() (selfinstall.Report, error) {
			called++
			return selfinstall.Report{Destination: `C:\AgentStack\agentstack.exe`, Copied: true}, nil
		},
	}
	if code := command.Run(context.Background(), []string{"setup", "--no-launch"}); code != 0 {
		t.Fatalf("unexpected exit code %d: %s", code, output.String())
	}
	if called != 1 || !bytes.Contains(output.Bytes(), []byte(`"copied": true`)) {
		t.Fatalf("setup did not install exactly once: called=%d output=%s", called, output.String())
	}
	if command.Service.Paths.Executable != `C:\AgentStack\agentstack.exe` {
		t.Fatalf("setup did not switch service to installed executable: %q", command.Service.Paths.Executable)
	}
}

func TestParseSelectionRejectsMalformedProvider(t *testing.T) {
	_, _, err := parseSelection([]string{"--provider", "browser"})
	if err == nil {
		t.Fatal("expected provider error")
	}
}

func TestHelpListsEveryCatalogProfileDynamically(t *testing.T) {
	var output bytes.Buffer
	service := &app.Service{Catalog: model.Catalog{Profiles: []model.Profile{
		{ID: "core", Description: "minimal"},
		{ID: "security", Description: "security tools"},
		{ID: "architecture", Description: "architecture tools"},
	}}}
	command := &CLI{Service: service, Out: &output, Err: &output}
	if code := command.Run(context.Background(), []string{"help"}); code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	for _, profile := range []string{"core", "security", "architecture"} {
		if !bytes.Contains(output.Bytes(), []byte(profile)) {
			t.Fatalf("help omitted profile %q:\n%s", profile, output.String())
		}
	}
}

func TestApplyRejectsSelectionOptionsInsteadOfReviewedPlan(t *testing.T) {
	var output bytes.Buffer
	command := &CLI{Service: &app.Service{}, Out: &output, Err: &output}
	code := command.Run(context.Background(), []string{"apply", "--profile", "core", "--yes"})
	if code != 2 || !bytes.Contains(output.Bytes(), []byte("flag provided but not defined: -profile")) {
		t.Fatalf("apply accepted an unreviewed selection request: code=%d output=%s", code, output.String())
	}
}

func TestMCPInitRequiresExplicitYes(t *testing.T) {
	var output bytes.Buffer
	command := &CLI{Service: &app.Service{}, Out: &output, Err: &output}
	code := command.Run(context.Background(), []string{"mcp", "init", "--profile", "core"})
	if code != 2 || !bytes.Contains(output.Bytes(), []byte("mcp init requires --yes")) {
		t.Fatalf("mcp init did not fail closed: code=%d output=%s", code, output.String())
	}
}

func TestParseSelectionSupportsExplicitUpgradeConsent(t *testing.T) {
	request, _, err := parseSelection([]string{"--allow-upgrades"})
	if err != nil {
		t.Fatal(err)
	}
	if !request.AllowUpgrades || request.Profile != "core" {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestDataPolicyReportsRetentionAndUserControlledState(t *testing.T) {
	var output bytes.Buffer
	command := &CLI{Service: &app.Service{}, Out: &output, Err: &output}
	if code := command.Run(context.Background(), []string{"data", "policy"}); code != 0 {
		t.Fatalf("unexpected exit code %d: %s", code, output.String())
	}
	for _, value := range []string{`"plans": "24h0m0s"`, `"events": "720h0m0s"`, `"backups": "retained until explicit user deletion"`} {
		if !bytes.Contains(output.Bytes(), []byte(value)) {
			t.Fatalf("policy output missing %s: %s", value, output.String())
		}
	}
}

func TestApplyPreconditionFailureDoesNotEmitZeroValueReport(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	service := &app.Service{Store: state.NewStore(t.TempDir())}
	command := &CLI{Service: service, Out: &stdout, Err: &stderr}
	code := command.Run(context.Background(), []string{"apply", "--plan-id", "missing", "--digest", "sha256:missing", "--yes"})
	if code == 0 {
		t.Fatal("expected apply failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("precondition failure emitted a misleading report: %s", stdout.String())
	}
}

func TestVersionIncludesSourceRevisionWhenAvailable(t *testing.T) {
	var output bytes.Buffer
	command := &CLI{
		Service:  &app.Service{},
		Out:      &output,
		Err:      &output,
		Version:  "0.2.0",
		Revision: "git:0123456789abcdef0123456789abcdef01234567",
	}
	if code := command.Run(context.Background(), []string{"version"}); code != 0 {
		t.Fatalf("unexpected exit code %d: %s", code, output.String())
	}
	const expected = "0.2.0 (git:0123456789abcdef0123456789abcdef01234567)\n"
	if output.String() != expected {
		t.Fatalf("unexpected version output: got %q want %q", output.String(), expected)
	}
}
