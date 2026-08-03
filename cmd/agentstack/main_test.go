package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/selfinstall"
)

func TestRunRegularVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"version"}, launchConfig{Version: "1.2.3", Revision: "git:abc", DefaultMode: "ui"}, launchDependencies{
		NewService: func() (*app.Service, error) { return &app.Service{}, nil },
		Stdout:     &stdout,
		Stderr:     &stderr,
	})
	if code != 0 || strings.TrimSpace(stdout.String()) != "1.2.3 (git:abc)" {
		t.Fatalf("run() code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}
}

func TestRunSetupVerifyOnlyStopsBeforeServiceInitialization(t *testing.T) {
	var stdout bytes.Buffer
	serviceCalled := false
	code := run(context.Background(), []string{"--verify-only"}, launchConfig{DefaultMode: "setup"}, launchDependencies{
		Executable:         func() (string, error) { return "setup.exe", nil },
		FindConsoleSibling: func(string, string) (string, error) { return "agentstack.exe", nil },
		VerifyReleasePair:  func(string, string, string, string) error { return nil },
		NewService: func() (*app.Service, error) {
			serviceCalled = true
			return nil, errors.New("must not run")
		},
		Stdout: &stdout,
	})
	if code != 0 || serviceCalled || !strings.Contains(stdout.String(), "verified AgentStack setup") {
		t.Fatalf("run() code = %d, serviceCalled = %v, stdout = %q", code, serviceCalled, stdout.String())
	}
}

func TestRunSetupNoLaunchUsesVerifiedConsole(t *testing.T) {
	var stdout, stderr bytes.Buffer
	installedPath := ""
	code := run(context.Background(), []string{"--no-launch"}, launchConfig{DefaultMode: "setup"}, launchDependencies{
		Executable:         func() (string, error) { return "setup.exe", nil },
		FindConsoleSibling: func(string, string) (string, error) { return "console.exe", nil },
		VerifyReleasePair:  func(string, string, string, string) error { return nil },
		NewService:         func() (*app.Service, error) { return &app.Service{}, nil },
		InstallFrom: func(path string) (selfinstall.Report, error) {
			installedPath = path
			return selfinstall.Report{Destination: "installed.exe"}, nil
		},
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if code != 0 || installedPath != "console.exe" {
		t.Fatalf("run() code = %d, installedPath = %q, stderr = %q", code, installedPath, stderr.String())
	}
	if !strings.Contains(stdout.String(), "installed.exe") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunSetupInitializationFailureNotifies(t *testing.T) {
	var stderr bytes.Buffer
	notification := ""
	code := run(context.Background(), []string{"--no-launch"}, launchConfig{DefaultMode: "setup"}, launchDependencies{
		Executable:         func() (string, error) { return "setup.exe", nil },
		FindConsoleSibling: func(string, string) (string, error) { return "console.exe", nil },
		VerifyReleasePair:  func(string, string, string, string) error { return nil },
		NewService:         func() (*app.Service, error) { return nil, errors.New("state unavailable") },
		NotifyError:        func(_, message string) { notification = message },
		Stderr:             &stderr,
	})
	if code != 1 || !strings.Contains(notification, "state unavailable") || !strings.Contains(stderr.String(), "initialize AgentStack") {
		t.Fatalf("run() code = %d, notification = %q, stderr = %q", code, notification, stderr.String())
	}
}

func TestRunSetupVerificationFailureStops(t *testing.T) {
	notification := ""
	code := run(context.Background(), nil, launchConfig{DefaultMode: "setup"}, launchDependencies{
		Executable:         func() (string, error) { return "setup.exe", nil },
		FindConsoleSibling: func(string, string) (string, error) { return "", errors.New("console missing") },
		NotifyError:        func(_, message string) { notification = message },
	})
	if code != 1 || !strings.Contains(notification, "console missing") {
		t.Fatalf("run() code = %d, notification = %q", code, notification)
	}
}
