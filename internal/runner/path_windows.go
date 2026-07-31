//go:build windows

package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func refreshProcessPath() error {
	const script = `$machine=[Environment]::GetEnvironmentVariable('Path','Machine'); $user=[Environment]::GetEnvironmentVariable('Path','User'); Write-Output (($machine,$user)-join ';')`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("read Windows PATH values: %w: %s", err, strings.TrimSpace(string(output)))
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return fmt.Errorf("Windows PATH refresh returned an empty value")
	}
	return os.Setenv("PATH", value)
}
