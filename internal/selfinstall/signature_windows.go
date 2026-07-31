//go:build windows

package selfinstall

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/runner"
)

func VerifyAuthenticode(path, expectedThumbprint string) error {
	expectedThumbprint = normalizeThumbprint(expectedThumbprint)
	if expectedThumbprint == "" {
		return fmt.Errorf("expected Authenticode publisher thumbprint is empty")
	}
	script := `$s=Get-AuthenticodeSignature -LiteralPath $args[0]; if ($s.Status -ne 'Valid') { Write-Error ("invalid Authenticode status: " + $s.Status); exit 11 }; $actual=($s.SignerCertificate.Thumbprint -replace '\s','').ToUpperInvariant(); $expected=($args[1] -replace '\s','').ToUpperInvariant(); if ($actual -ne $expected) { Write-Error ("publisher thumbprint mismatch: " + $actual); exit 12 }; Write-Output $actual`
	command := "powershell.exe"
	if _, err := exec.LookPath(command); err != nil {
		command = "pwsh.exe"
	}
	result := runner.ExecRunner{}.Run(context.Background(), runner.Invocation{Command: command, Args: []string{"-NoProfile", "-NonInteractive", "-Command", script, path, expectedThumbprint}, Timeout: 30 * time.Second, MaxOutputBytes: 64 << 10})
	if result.Err != nil || result.ExitCode != 0 {
		return fmt.Errorf("verify Authenticode signature for %s: %s", path, strings.TrimSpace(result.Stderr+" "+result.Stdout))
	}
	return nil
}
