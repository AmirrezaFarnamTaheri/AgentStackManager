//go:build windows

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/agentstack/agentstack/internal/pathenv"
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
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), "AGENTSTACK_PATH_SCOPE="+scope)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read Windows %s PATH: %w: %s", scope, err, strings.TrimSpace(string(output)))
	}
	value, decodeErr := pathenv.DecodeWindowsString(string(output))
	if decodeErr != nil {
		return "", fmt.Errorf("decode Windows %s PATH: %w", scope, decodeErr)
	}
	return value, nil
}
