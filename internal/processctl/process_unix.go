//go:build !windows

package processctl

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

type unixController struct {
	pid             int
	pgid            int
	identity        string
	identityMissing bool
	readIdentity    func(int) (string, error)
	getpgid         func(int) (int, error)
	kill            func(int, syscall.Signal) error
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func validatePlatformLimits(limits Limits) error {
	if limits.Disabled() {
		return nil
	}
	return ErrResourceLimitsUnsupported
}

func attachCommand(cmd *exec.Cmd, _ Limits) (platformController, error) {
	if cmd.Process == nil {
		return nil, errors.New("process was not started")
	}
	controller, err := newUnixController(cmd.Process.Pid, platformProcessIdentity, syscall.Getpgid, syscall.Kill)
	if err != nil {
		return nil, err
	}
	return controller, nil
}

func newUnixController(
	pid int,
	readIdentity func(int) (string, error),
	getpgid func(int) (int, error),
	kill func(int, syscall.Signal) error,
) (unixController, error) {
	if pid <= 0 {
		return unixController{}, fmt.Errorf("invalid child process id %d", pid)
	}
	identity := ""
	identityMissing := false
	if readIdentity != nil {
		value, err := readIdentity(pid)
		if err == nil {
			identity = value
		} else if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ESRCH) {
			identityMissing = true
		} else {
			return unixController{}, fmt.Errorf("capture child process identity: %w", err)
		}
	}
	// prepareCommand requests a new process group whose ID is the child PID.
	// Recording that kernel contract avoids failing Start merely because a
	// short-lived child exits before userspace can inspect /proc or Getpgid.
	return unixController{
		pid:             pid,
		pgid:            pid,
		identity:        identity,
		identityMissing: identityMissing,
		readIdentity:    readIdentity,
		getpgid:         getpgid,
		kill:            kill,
	}, nil
}

func (c unixController) terminate() error {
	if c.pid <= 0 || c.pgid <= 0 {
		return nil
	}
	getpgid := c.getpgid
	if getpgid == nil {
		getpgid = syscall.Getpgid
	}
	kill := c.kill
	if kill == nil {
		kill = syscall.Kill
	}
	readIdentity := c.readIdentity
	if readIdentity == nil {
		readIdentity = platformProcessIdentity
	}
	leaderMissing := c.identityMissing
	if c.identityMissing {
		currentIdentity, identityErr := readIdentity(c.pid)
		if identityErr == nil && currentIdentity != "" {
			return fmt.Errorf("%w: pid=%d appeared after initial identity capture missed the original leader", ErrProcessIdentityChanged, c.pid)
		}
		if identityErr != nil && !errors.Is(identityErr, syscall.ENOENT) && !errors.Is(identityErr, syscall.ESRCH) {
			return fmt.Errorf("verify missing child process identity: %w", identityErr)
		}
	}
	if c.identity != "" {
		currentIdentity, identityErr := readIdentity(c.pid)
		if identityErr == nil {
			if currentIdentity != c.identity {
				return fmt.Errorf("%w: pid=%d process start identity changed", ErrProcessIdentityChanged, c.pid)
			}
		} else if errors.Is(identityErr, syscall.ENOENT) || errors.Is(identityErr, syscall.ESRCH) {
			leaderMissing = true
		} else {
			return fmt.Errorf("verify child process start identity: %w", identityErr)
		}
	}

	currentPGID, err := getpgid(c.pid)
	if err == nil {
		if leaderMissing {
			return fmt.Errorf("%w: pid=%d reappeared after the original group leader exited", ErrProcessIdentityChanged, c.pid)
		}
		if currentPGID != c.pgid || c.pgid != c.pid {
			return fmt.Errorf("%w: pid=%d recorded_pgid=%d current_pgid=%d", ErrProcessIdentityChanged, c.pid, c.pgid, currentPGID)
		}
	} else if errors.Is(err, syscall.ESRCH) {
		// The group leader may exit before descendants. Probe the recorded
		// process group directly so containment still reaches those children.
		probeErr := kill(-c.pgid, 0)
		if errors.Is(probeErr, syscall.ESRCH) {
			return nil
		}
		if probeErr != nil && !errors.Is(probeErr, syscall.EPERM) {
			return fmt.Errorf("verify recorded child process group: %w", probeErr)
		}
	} else {
		return fmt.Errorf("verify child process group before termination: %w", err)
	}
	if err := kill(-c.pgid, syscall.SIGKILL); errors.Is(err, syscall.ESRCH) {
		return nil
	} else {
		return err
	}
}

func (unixController) close() error { return nil }
