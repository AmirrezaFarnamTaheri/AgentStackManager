//go:build !windows

package processctl

import (
	"errors"
	"os/exec"
	"syscall"
)

type unixController struct{ pid int }

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachCommand(cmd *exec.Cmd) (platformController, error) {
	if cmd.Process == nil {
		return nil, errors.New("process was not started")
	}
	return unixController{pid: cmd.Process.Pid}, nil
}

func (c unixController) terminate() error {
	if c.pid <= 0 {
		return nil
	}
	err := syscall.Kill(-c.pid, syscall.SIGKILL)
	if err == syscall.ESRCH {
		return nil
	}
	return err
}
func (unixController) close() error { return nil }
