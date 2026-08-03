//go:build windows

package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/pathenv"
	"github.com/agentstack/agentstack/internal/supervisor"
)

func refreshProcessPath() error {
	machine, err := readWindowsPath("Machine")
	if err != nil {
		return err
	}
	user, err := readWindowsPath("User")
	if err != nil {
		return err
	}
	value := pathenv.MergeWindows(machine, user)
	if value == "" {
		return fmt.Errorf("Windows PATH refresh returned an empty value")
	}
	return os.Setenv("PATH", value)
}

func readWindowsPath(scope string) (string, error) {
	script := `$value=[Environment]::GetEnvironmentVariable('Path',$env:AGENTSTACK_PATH_SCOPE); if($null -eq $value){$value=''}; [Console]::Out.Write([Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($value)))`
	result := (supervisor.Runtime{}).Run(context.Background(), supervisor.Spec{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script},
		Env:     map[string]string{"AGENTSTACK_PATH_SCOPE": scope},
	}, supervisor.RunOptions{Timeout: 30 * time.Second, MaxOutputBytes: pathenv.MaxWindowsStringTransportBytes})
	if result.Err != nil {
		return "", fmt.Errorf("read Windows %s PATH: %w: %s", scope, result.Err, strings.TrimSpace(result.Stderr))
	}
	value, decodeErr := pathenv.DecodeWindowsString(result.Stdout)
	if decodeErr != nil {
		return "", fmt.Errorf("decode Windows %s PATH: %w", scope, decodeErr)
	}
	return value, nil
}
