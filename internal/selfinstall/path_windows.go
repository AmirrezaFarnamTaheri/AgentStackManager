//go:build windows

package selfinstall

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/pathenv"
	"github.com/agentstack/agentstack/internal/supervisor"
)

func ensureUserPath(target string) (bool, string, error) {
	current, err := readUserPath()
	if err != nil {
		return false, "", err
	}
	next, changed := AppendPathSegment(current, target)
	if !changed {
		return false, "unchanged", nil
	}
	encoded := pathenv.EncodeWindowsString(next)
	script := `$value=[Text.Encoding]::Unicode.GetString([Convert]::FromBase64String($env:AGENTSTACK_USER_PATH_B64)); [Environment]::SetEnvironmentVariable('Path',$value,'User')`
	result := (supervisor.Runtime{}).Run(context.Background(), supervisor.Spec{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script},
		Env:     map[string]string{"AGENTSTACK_USER_PATH_B64": encoded},
	}, supervisor.RunOptions{Timeout: 30 * time.Second, MaxOutputBytes: 64 << 10})
	if result.Err != nil {
		return false, "", fmt.Errorf("update user PATH: %w: %s", result.Err, strings.TrimSpace(result.Stderr))
	}
	return true, "changed", nil
}

func readUserPath() (string, error) {
	script := `$value=[Environment]::GetEnvironmentVariable('Path','User'); if($null -eq $value){$value=''}; [Console]::Out.Write([Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($value)))`
	result := (supervisor.Runtime{}).Run(context.Background(), supervisor.Spec{
		Command: "powershell.exe",
		Args:    []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script},
	}, supervisor.RunOptions{Timeout: 30 * time.Second, MaxOutputBytes: pathenv.MaxWindowsStringTransportBytes})
	if result.Err != nil {
		return "", fmt.Errorf("read user PATH: %w: %s", result.Err, strings.TrimSpace(result.Stderr))
	}
	value, decodeErr := pathenv.DecodeWindowsString(result.Stdout)
	if decodeErr != nil {
		return "", fmt.Errorf("decode user PATH: %w", decodeErr)
	}
	return value, nil
}
