package processctl

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"time"
)

var ErrProcessIdentityChanged = errors.New("process identity changed before termination")

type platformController interface {
	terminate() error
	close() error
}

// IsAlive reports whether pid currently identifies a live process.
func IsAlive(pid int) bool {
	return processAlive(pid)
}

type Process struct {
	Cmd        *exec.Cmd
	controller platformController
	waitDone   chan struct{}
	waitMu     sync.RWMutex
	waitErr    error
}

func Start(cmd *exec.Cmd) (*Process, error) {
	return StartWithLimits(cmd, Limits{})
}

func StartWithLimits(cmd *exec.Cmd, limits Limits) (*Process, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if err := validatePlatformLimits(limits); err != nil {
		return nil, err
	}
	prepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	controller, err := attachCommand(cmd, limits)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, err
	}
	process := &Process{Cmd: cmd, controller: controller, waitDone: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		_ = controller.close()
		process.waitMu.Lock()
		process.waitErr = err
		process.waitMu.Unlock()
		close(process.waitDone)
	}()
	return process, nil
}

func (p *Process) terminalError() error {
	p.waitMu.RLock()
	defer p.waitMu.RUnlock()
	return p.waitErr
}

func (p *Process) Wait(ctx context.Context) error {
	if p == nil {
		return errors.New("process is nil")
	}
	select {
	case <-p.waitDone:
		return p.terminalError()
	case <-ctx.Done():
		_ = p.Terminate()
		select {
		case <-p.waitDone:
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return ctx.Err()
		}
	}
}

func (p *Process) GracefulClose(closeInput func() error, grace time.Duration) error {
	if p == nil {
		return nil
	}
	if closeInput != nil {
		_ = closeInput()
	}
	if grace <= 0 {
		grace = 2 * time.Second
	}
	select {
	case <-p.waitDone:
		return p.terminalError()
	case <-time.After(grace):
		_ = p.Terminate()
		select {
		case <-p.waitDone:
			return p.terminalError()
		case <-time.After(5 * time.Second):
			return errors.New("process tree did not terminate")
		}
	}
}

func (p *Process) Terminate() error {
	if p == nil || p.controller == nil {
		return nil
	}
	return p.controller.terminate()
}
