//go:build windows

package selfinstall

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/agentstack/agentstack/internal/pathenv"
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
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), "AGENTSTACK_USER_PATH_B64="+encoded)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, "", fmt.Errorf("update user PATH: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return true, "changed", nil
}

func readUserPath() (string, error) {
	script := `$value=[Environment]::GetEnvironmentVariable('Path','User'); if($null -eq $value){$value=''}; [Console]::Out.Write([Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($value)))`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read user PATH: %w: %s", err, strings.TrimSpace(string(output)))
	}
	value, decodeErr := pathenv.DecodeWindowsString(string(output))
	if decodeErr != nil {
		return "", fmt.Errorf("decode user PATH: %w", decodeErr)
	}
	return value, nil
}
