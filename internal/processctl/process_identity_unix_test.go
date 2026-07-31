//go:build !windows

package processctl

import (
	"errors"
	"syscall"
	"testing"
)

func TestUnixTerminateRefusesChangedProcessGroupIdentity(t *testing.T) {
	killed := false
	controller := unixController{
		pid:     101,
		pgid:    101,
		getpgid: func(int) (int, error) { return 202, nil },
		kill: func(int, syscall.Signal) error {
			killed = true
			return nil
		},
	}
	err := controller.terminate()
	if !errors.Is(err, ErrProcessIdentityChanged) {
		t.Fatalf("expected identity error, got %v", err)
	}
	if killed {
		t.Fatal("changed process group must not be signaled")
	}
}

func TestUnixTerminateSignalsOnlyRecordedProcessGroup(t *testing.T) {
	var target int
	controller := unixController{
		pid:     101,
		pgid:    101,
		getpgid: func(int) (int, error) { return 101, nil },
		kill: func(pid int, signal syscall.Signal) error {
			target = pid
			if signal != syscall.SIGKILL {
				t.Fatalf("unexpected signal %v", signal)
			}
			return nil
		},
	}
	if err := controller.terminate(); err != nil {
		t.Fatal(err)
	}
	if target != -101 {
		t.Fatalf("expected process group -101, got %d", target)
	}
}

func TestUnixTerminateReachesDescendantsAfterLeaderExit(t *testing.T) {
	calls := []syscall.Signal{}
	controller := unixController{
		pid:      101,
		pgid:     101,
		identity: "original-start",
		readIdentity: func(int) (string, error) {
			return "", syscall.ENOENT
		},
		getpgid: func(int) (int, error) {
			return 0, syscall.ESRCH
		},
		kill: func(pid int, signal syscall.Signal) error {
			if pid != -101 {
				t.Fatalf("unexpected process group target %d", pid)
			}
			calls = append(calls, signal)
			return nil
		},
	}
	if err := controller.terminate(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != 0 || calls[1] != syscall.SIGKILL {
		t.Fatalf("expected group probe then termination, got %v", calls)
	}
}

func TestUnixTerminateRefusesReusedPIDWithSamePGID(t *testing.T) {
	killed := false
	controller := unixController{
		pid:      101,
		pgid:     101,
		identity: "original-start",
		readIdentity: func(int) (string, error) {
			return "reused-start", nil
		},
		getpgid: func(int) (int, error) { return 101, nil },
		kill: func(int, syscall.Signal) error {
			killed = true
			return nil
		},
	}
	if err := controller.terminate(); !errors.Is(err, ErrProcessIdentityChanged) {
		t.Fatalf("expected process identity change, got %v", err)
	}
	if killed {
		t.Fatal("reused PID/process group was signaled")
	}
}

func TestUnixTerminateRefusesPIDThatReappearsAfterLeaderExit(t *testing.T) {
	killed := false
	controller := unixController{
		pid:      101,
		pgid:     101,
		identity: "original-start",
		readIdentity: func(int) (string, error) {
			return "", syscall.ENOENT
		},
		getpgid: func(int) (int, error) { return 101, nil },
		kill: func(int, syscall.Signal) error {
			killed = true
			return nil
		},
	}
	if err := controller.terminate(); !errors.Is(err, ErrProcessIdentityChanged) {
		t.Fatalf("expected reappeared PID identity error, got %v", err)
	}
	if killed {
		t.Fatal("reappeared PID was signaled")
	}
}

func TestUnixTerminateRefusesPIDThatAppearsAfterInitialIdentityMiss(t *testing.T) {
	killed := false
	controller := unixController{
		pid:             101,
		pgid:            101,
		identityMissing: true,
		readIdentity: func(int) (string, error) {
			return "reused-start", nil
		},
		getpgid: func(int) (int, error) { return 101, nil },
		kill: func(int, syscall.Signal) error {
			killed = true
			return nil
		},
	}
	if err := controller.terminate(); !errors.Is(err, ErrProcessIdentityChanged) {
		t.Fatalf("expected initial identity-miss reuse error, got %v", err)
	}
	if killed {
		t.Fatal("PID that appeared after identity capture miss was signaled")
	}
}

func TestNewUnixControllerToleratesLeaderExitBeforeIdentityCapture(t *testing.T) {
	controller, err := newUnixController(
		101,
		func(int) (string, error) { return "", syscall.ENOENT },
		func(int) (int, error) { return 0, syscall.ESRCH },
		func(int, syscall.Signal) error { return nil },
	)
	if err != nil {
		t.Fatalf("short-lived child must remain startable: %v", err)
	}
	if controller.pid != 101 || controller.pgid != 101 || controller.identity != "" || !controller.identityMissing {
		t.Fatalf("unexpected short-lived controller: %+v", controller)
	}
}
