//go:build windows

package selfinstall

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ensureUserPath(target string) (bool, string, error) {
	script := `$target=$env:AGENTSTACK_BIN; $current=[Environment]::GetEnvironmentVariable('Path','User'); ` +
		`$parts=@($current -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }); ` +
		`$normalized=$target.TrimEnd('\\','/'); ` +
		`$exists=$false; foreach($part in $parts){ if($part.TrimEnd('\\','/').Equals($normalized,[StringComparison]::OrdinalIgnoreCase)){ $exists=$true } }; ` +
		`if($exists){ Write-Output 'unchanged'; exit 0 }; ` +
		`$next=(@($parts)+$target)-join ';'; [Environment]::SetEnvironmentVariable('Path',$next,'User'); Write-Output 'changed'`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), "AGENTSTACK_BIN="+target)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return false, "", fmt.Errorf("update user PATH: %w: %s", err, stderr.String())
	}
	changed := strings.Contains(strings.ToLower(stdout.String()), "changed") && !strings.Contains(strings.ToLower(stdout.String()), "unchanged")
	return changed, strings.TrimSpace(stdout.String()), nil
}
