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
var defaultMode = "ui"
var consoleSHA256 = ""
var publisherThumbprint = ""

func main() {
	setupBinary := strings.EqualFold(defaultMode, "setup")
	originalArgs := append([]string(nil), os.Args[1:]...)
	setupDefault := setupBinary && (len(originalArgs) == 0 || (len(originalArgs) == 1 && originalArgs[0] == "--no-launch"))
	var releaseConsolePath string
	if setupBinary {
		setupPath, pathErr := os.Executable()
		if pathErr != nil {
			notify.Error("AgentStack Setup", "Cannot locate the setup executable: "+pathErr.Error())
			os.Exit(1)
		}
		consolePath, siblingErr := selfinstall.FindReleaseConsoleSibling(setupPath, runtime.GOARCH)
		if siblingErr == nil {
			siblingErr = selfinstall.VerifyReleasePair(setupPath, consolePath, consoleSHA256, publisherThumbprint)
		}
		if siblingErr != nil {
			notify.Error("AgentStack Setup", siblingErr.Error()+"\n\nExtract the complete architecture-specific release ZIP, then run AgentStack-Setup.exe again.")
			os.Exit(1)
		}
		releaseConsolePath = consolePath
		if len(originalArgs) == 1 && originalArgs[0] == "--verify-only" {
			fmt.Fprintln(os.Stdout, "verified AgentStack setup and console release pair")
			return
		}
	}
	service, err := app.NewDefault()
	if err != nil {
		message := fmt.Sprintf("AgentStack setup could not initialize:\n\n%v\n\nRun AgentStack-Setup.ps1 from PowerShell for detailed diagnostics.", err)
		if setupDefault {
			notify.Error("AgentStack Setup", message)
		}
		fmt.Fprintln(os.Stderr, "error: initialize AgentStack:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	args := originalArgs
	if setupDefault {
		args = []string{"setup"}
		if len(originalArgs) == 1 && originalArgs[0] == "--no-launch" {
			args = append(args, "--no-launch")
		}
	}
	command := cli.New(service, version)
	if setupDefault {
		command.InstallSelf = func() (selfinstall.Report, error) {
			return selfinstall.InstallFrom(releaseConsolePath)
		}
		var captured bytes.Buffer
		command.Err = io.MultiWriter(os.Stderr, &captured)
		code := command.Run(ctx, args)
		if code != 0 {
			message := strings.TrimSpace(captured.String())
			if message == "" {
				message = "AgentStack setup failed. Run AgentStack-Setup.ps1 from PowerShell for detailed diagnostics."
			}
			notify.Error("AgentStack Setup", message)
		}
		os.Exit(code)
	}
	os.Exit(command.Run(ctx, args))
}
