package supervisor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/processctl"
)

const DefaultCaptureLimit = 1 << 20

type Limits = processctl.Limits

type Spec struct {
	Command string
	Args    []string
	Env     map[string]string
	Dir     string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Limits  processctl.Limits
}

type RunOptions struct {
	Timeout        time.Duration
	MaxOutputBytes int
}

type Result struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Err       error
	Truncated bool
}

type Runtime struct{}

type Process struct {
	managed *processctl.Process
}

func (p *Process) Wait(ctx context.Context) error {
	if p == nil || p.managed == nil {
		return errors.New("supervisor: process is not started")
	}
	return p.managed.Wait(ctx)
}

func (p *Process) GracefulClose(closeInput func() error, grace time.Duration) error {
	if p == nil || p.managed == nil {
		return nil
	}
	return p.managed.GracefulClose(closeInput, grace)
}

func (p *Process) Terminate() error {
	if p == nil || p.managed == nil {
		return nil
	}
	return p.managed.Terminate()
}

type PipedProcess struct {
	*Process
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

func (p *PipedProcess) Close() error {
	if p == nil {
		return nil
	}
	var joined error
	if p.Stdin != nil {
		joined = errors.Join(joined, p.Stdin.Close())
	}
	if p.Stdout != nil {
		joined = errors.Join(joined, p.Stdout.Close())
	}
	if p.Stderr != nil {
		joined = errors.Join(joined, p.Stderr.Close())
	}
	joined = errors.Join(joined, p.Terminate())
	return joined
}

func (Runtime) Run(parent context.Context, spec Spec, options RunOptions) Result {
	ctx := parent
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, options.Timeout)
		defer cancel()
	}
	limit := options.MaxOutputBytes
	if limit <= 0 {
		limit = DefaultCaptureLimit
	}
	stdout := newCappedBuffer(limit)
	stderr := newCappedBuffer(limit)
	if spec.Stdout != nil {
		spec.Stdout = io.MultiWriter(stdout, spec.Stdout)
	} else {
		spec.Stdout = stdout
	}
	if spec.Stderr != nil {
		spec.Stderr = io.MultiWriter(stderr, spec.Stderr)
	} else {
		spec.Stderr = stderr
	}
	process, err := (Runtime{}).Start(ctx, spec)
	if err == nil {
		err = process.Wait(ctx)
	}
	result := Result{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Err:       err,
		Truncated: stdout.Truncated() || stderr.Truncated(),
	}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.ExitCode = -1
	}
	return result
}

func (Runtime) Start(ctx context.Context, spec Spec) (*Process, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd, err := command(spec)
	if err != nil {
		return nil, err
	}
	managed, err := startCommand(cmd, spec.Limits)
	if err != nil {
		return nil, err
	}
	return &Process{managed: managed}, nil
}

func (Runtime) StartPiped(ctx context.Context, spec Spec) (*PipedProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if spec.Stdin != nil || spec.Stdout != nil {
		return nil, errors.New("supervisor: StartPiped owns stdin and stdout")
	}
	cmd, err := command(spec)
	if err != nil {
		return nil, err
	}

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		return nil, err
	}
	var stderrReader, stderrWriter *os.File
	if spec.Stderr == nil {
		stderrReader, stderrWriter, err = os.Pipe()
		if err != nil {
			_ = stdinReader.Close()
			_ = stdinWriter.Close()
			_ = stdoutReader.Close()
			_ = stdoutWriter.Close()
			return nil, err
		}
	}
	cmd.Stdin = stdinReader
	cmd.Stdout = stdoutWriter
	if stderrWriter != nil {
		cmd.Stderr = stderrWriter
	}

	closeParentChildEnds := func() {
		_ = stdinReader.Close()
		_ = stdoutWriter.Close()
		if stderrWriter != nil {
			_ = stderrWriter.Close()
		}
	}
	closeAll := func() {
		closeParentChildEnds()
		_ = stdinWriter.Close()
		_ = stdoutReader.Close()
		if stderrReader != nil {
			_ = stderrReader.Close()
		}
	}

	type startResult struct {
		process *processctl.Process
		err     error
	}
	started := make(chan startResult, 1)
	go func() {
		managed, startErr := startCommand(cmd, spec.Limits)
		started <- startResult{process: managed, err: startErr}
	}()
	select {
	case result := <-started:
		closeParentChildEnds()
		if result.err != nil {
			_ = stdinWriter.Close()
			_ = stdoutReader.Close()
			if stderrReader != nil {
				_ = stderrReader.Close()
			}
			return nil, result.err
		}
		return &PipedProcess{
			Process: &Process{managed: result.process},
			Stdin:   stdinWriter,
			Stdout:  stdoutReader,
			Stderr:  stderrReader,
		}, nil
	case <-ctx.Done():
		go func() {
			result := <-started
			closeAll()
			if result.process != nil {
				_ = result.process.Terminate()
			}
		}()
		return nil, ctx.Err()
	}
}

func command(spec Spec) (*exec.Cmd, error) {
	if spec.Command == "" {
		return nil, errors.New("supervisor: command is empty")
	}
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Stdin = spec.Stdin
	cmd.Stdout = spec.Stdout
	cmd.Stderr = spec.Stderr
	if len(spec.Env) > 0 {
		env := cmd.Environ()
		keys := make([]string, 0, len(spec.Env))
		for key := range spec.Env {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			env = append(env, key+"="+spec.Env[key])
		}
		cmd.Env = env
	}
	return cmd, nil
}

func startCommand(cmd *exec.Cmd, limits processctl.Limits) (*processctl.Process, error) {
	if limits.Disabled() {
		return processctl.Start(cmd)
	}
	return processctl.StartWithLimits(cmd, limits)
}

type cappedBuffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	limit     int
	truncated bool
}

func newCappedBuffer(limit int) *cappedBuffer {
	return &cappedBuffer{limit: limit}
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	remaining := b.limit - b.data.Len()
	if remaining > 0 {
		chunk := p
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
			b.truncated = true
		}
		if _, err := b.data.Write(chunk); err != nil {
			return 0, fmt.Errorf("capture process output: %w", err)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return original, nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

func (b *cappedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
