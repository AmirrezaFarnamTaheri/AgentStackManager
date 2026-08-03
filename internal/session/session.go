package session

import (
	"context"
	"fmt"
	"os"

	"github.com/agentstack/agentstack/internal/supervisor"
)

type StartRequest struct {
	Command string
	Args    []string
	Env     map[string]string
}

type Starter interface {
	Run(context.Context, StartRequest) error
}

type ExecStarter struct{}

func (ExecStarter) Run(ctx context.Context, request StartRequest) error {
	process, err := (supervisor.Runtime{}).Start(ctx, supervisor.Spec{
		Command: request.Command,
		Args:    append([]string(nil), request.Args...),
		Env:     request.Env,
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	if err != nil {
		return fmt.Errorf("start %s: %w", request.Command, err)
	}
	if err := process.Wait(ctx); err != nil {
		return fmt.Errorf("run %s: %w", request.Command, err)
	}
	return nil
}

func Launch(ctx context.Context, command string, args []string, initialize func(context.Context) error, starter Starter) error {
	if command == "" {
		return fmt.Errorf("session command is empty")
	}
	if initialize != nil {
		if err := initialize(ctx); err != nil {
			return err
		}
	}
	if starter == nil {
		starter = ExecStarter{}
	}
	return starter.Run(ctx, StartRequest{Command: command, Args: append([]string(nil), args...)})
}
