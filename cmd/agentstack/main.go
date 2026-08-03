package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/cli"
	"github.com/agentstack/agentstack/internal/notify"
	"github.com/agentstack/agentstack/internal/selfinstall"
)

var version = "dev"
var revision = "unavailable"
var defaultMode = "ui"
var consoleSHA256 = ""
var publisherThumbprint = ""

type launchConfig struct {
	Version             string
	Revision            string
	DefaultMode         string
	ConsoleSHA256       string
	PublisherThumbprint string
}

type launchDependencies struct {
	Executable         func() (string, error)
	FindConsoleSibling func(string, string) (string, error)
	VerifyReleasePair  func(string, string, string, string) error
	NewService         func() (*app.Service, error)
	InstallFrom        func(string) (selfinstall.Report, error)
	NotifyError        func(string, string)
	Stdout             io.Writer
	Stderr             io.Writer
	Architecture       string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], launchConfig{
		Version:             version,
		Revision:            revision,
		DefaultMode:         defaultMode,
		ConsoleSHA256:       consoleSHA256,
		PublisherThumbprint: publisherThumbprint,
	}, defaultLaunchDependencies()))
}

func defaultLaunchDependencies() launchDependencies {
	return launchDependencies{
		Executable:         os.Executable,
		FindConsoleSibling: selfinstall.FindReleaseConsoleSibling,
		VerifyReleasePair:  selfinstall.VerifyReleasePair,
		NewService:         app.NewDefault,
		InstallFrom:        selfinstall.InstallFrom,
		NotifyError:        notify.Error,
		Stdout:             os.Stdout,
		Stderr:             os.Stderr,
		Architecture:       runtime.GOARCH,
	}
}

func run(ctx context.Context, args []string, config launchConfig, dependencies launchDependencies) int {
	dependencies = normalizeLaunchDependencies(dependencies)
	setupBinary := strings.EqualFold(config.DefaultMode, "setup")
	originalArgs := append([]string(nil), args...)
	setupDefault := setupBinary && (len(originalArgs) == 0 || (len(originalArgs) == 1 && originalArgs[0] == "--no-launch"))

	var releaseConsolePath string
	if setupBinary {
		setupPath, err := dependencies.Executable()
		if err != nil {
			dependencies.NotifyError("AgentStack Setup", "Cannot locate the setup executable: "+err.Error())
			return 1
		}
		consolePath, err := dependencies.FindConsoleSibling(setupPath, dependencies.Architecture)
		if err == nil {
			err = dependencies.VerifyReleasePair(setupPath, consolePath, config.ConsoleSHA256, config.PublisherThumbprint)
		}
		if err != nil {
			dependencies.NotifyError("AgentStack Setup", err.Error()+"\n\nExtract the complete architecture-specific release ZIP, then run AgentStack-Setup.exe again.")
			return 1
		}
		releaseConsolePath = consolePath
		if len(originalArgs) == 1 && originalArgs[0] == "--verify-only" {
			fmt.Fprintln(dependencies.Stdout, "verified AgentStack setup and console release pair")
			return 0
		}
	}

	service, err := dependencies.NewService()
	if err != nil {
		message := fmt.Sprintf("AgentStack setup could not initialize:\n\n%v\n\nRun AgentStack-Setup.ps1 from PowerShell for detailed diagnostics.", err)
		if setupDefault {
			dependencies.NotifyError("AgentStack Setup", message)
		}
		fmt.Fprintln(dependencies.Stderr, "error: initialize AgentStack:", err)
		return 1
	}

	commandArgs := originalArgs
	if setupDefault {
		commandArgs = []string{"setup"}
		if len(originalArgs) == 1 && originalArgs[0] == "--no-launch" {
			commandArgs = append(commandArgs, "--no-launch")
		}
	}
	command := cli.New(service, config.Version, config.Revision)
	command.Out = dependencies.Stdout
	command.Err = dependencies.Stderr
	if !setupDefault {
		return command.Run(ctx, commandArgs)
	}

	command.InstallSelf = func() (selfinstall.Report, error) {
		return dependencies.InstallFrom(releaseConsolePath)
	}
	var captured bytes.Buffer
	command.Err = io.MultiWriter(dependencies.Stderr, &captured)
	code := command.Run(ctx, commandArgs)
	if code != 0 {
		message := strings.TrimSpace(captured.String())
		if message == "" {
			message = "AgentStack setup failed. Run AgentStack-Setup.ps1 from PowerShell for detailed diagnostics."
		}
		dependencies.NotifyError("AgentStack Setup", message)
	}
	return code
}

func normalizeLaunchDependencies(dependencies launchDependencies) launchDependencies {
	defaults := defaultLaunchDependencies()
	if dependencies.Executable == nil {
		dependencies.Executable = defaults.Executable
	}
	if dependencies.FindConsoleSibling == nil {
		dependencies.FindConsoleSibling = defaults.FindConsoleSibling
	}
	if dependencies.VerifyReleasePair == nil {
		dependencies.VerifyReleasePair = defaults.VerifyReleasePair
	}
	if dependencies.NewService == nil {
		dependencies.NewService = defaults.NewService
	}
	if dependencies.InstallFrom == nil {
		dependencies.InstallFrom = defaults.InstallFrom
	}
	if dependencies.NotifyError == nil {
		dependencies.NotifyError = defaults.NotifyError
	}
	if dependencies.Stdout == nil {
		dependencies.Stdout = io.Discard
	}
	if dependencies.Stderr == nil {
		dependencies.Stderr = io.Discard
	}
	if dependencies.Architecture == "" {
		dependencies.Architecture = defaults.Architecture
	}
	return dependencies
}
