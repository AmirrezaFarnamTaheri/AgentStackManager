package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/agentstack/agentstack/internal/processctl"
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
	cmd := exec.Command(request.Command, request.Args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(request.Env) > 0 {
		env := os.Environ()
		for key, value := range request.Env {
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}
	process, err := processctl.Start(cmd)
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
