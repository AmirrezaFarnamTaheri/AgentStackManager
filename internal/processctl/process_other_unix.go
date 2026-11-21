//go:build !windows && !linux

package processctl

import (
	"os/exec"
	"syscall"
)

func startPlatformCommand(cmd *exec.Cmd, limits Limits) (platformController, error) {
	if !limits.Disabled() {
		return nil, ErrResourceLimitsUnsupported
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return attachUnixCommand(cmd.Process.Pid, "")
}
