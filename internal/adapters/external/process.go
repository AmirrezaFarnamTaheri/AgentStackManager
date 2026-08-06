package external

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/processctl"
)

type boundedBuffer struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int64
	overflow bool
	cancel   context.CancelFunc
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.overflow {
		return 0, fmt.Errorf("output limit exceeded")
	}
	remaining := b.limit - int64(b.buffer.Len())
	if int64(len(data)) > remaining {
		if remaining > 0 {
			_, _ = b.buffer.Write(data[:remaining])
		}
		b.overflow = true
		if b.cancel != nil {
			b.cancel()
		}
		return int(max64(remaining, 0)), fmt.Errorf("output limit exceeded")
	}
	return b.buffer.Write(data)
}

func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buffer.Bytes()...)
}

func (b *boundedBuffer) Overflowed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overflow
}

func (value stagedExecutable) execute(ctx context.Context, request []byte) ([]byte, []byte, error) {
	if int64(len(request)) > value.limits.MaxRequestBytes {
		return nil, nil, fmt.Errorf("external adapter request exceeds %d bytes", value.limits.MaxRequestBytes)
	}
	invokeCtx, cancel := withInvocationDeadline(ctx, value.limits.Timeout)
	defer cancel()
	cmd := exec.Command(value.path, value.arguments...)
	cmd.Dir = filepath.Join(value.sandbox, "work")
	environment, err := sandboxEnvironment(value.sandbox)
	if err != nil {
		return nil, nil, err
	}
	cmd.Env = environment
	cmd.Stdin = bytes.NewReader(request)
	stdout := &boundedBuffer{limit: value.limits.MaxResponseBytes, cancel: cancel}
	stderr := &boundedBuffer{limit: value.limits.MaxStderrBytes, cancel: cancel}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	managed, startErr := processctl.StartWithLimits(cmd, value.limits.Process)
	if startErr != nil {
		return nil, nil, fmt.Errorf("start external adapter process: %w", startErr)
	}
	err = managed.Wait(invokeCtx)
	out := stdout.Bytes()
	errOut := stderr.Bytes()
	if stdout.Overflowed() {
		return nil, errOut, fmt.Errorf("external adapter response exceeds %d bytes", value.limits.MaxResponseBytes)
	}
	if stderr.Overflowed() {
		return nil, nil, fmt.Errorf("external adapter stderr exceeds %d bytes", value.limits.MaxStderrBytes)
	}
	if invokeCtx.Err() != nil {
		if errors.Is(invokeCtx.Err(), context.DeadlineExceeded) {
			return nil, errOut, fmt.Errorf("external adapter exceeded deadline %s", value.limits.Timeout)
		}
		return nil, errOut, fmt.Errorf("external adapter invocation canceled: %w", invokeCtx.Err())
	}
	if err != nil {
		message := sanitizeDiagnostic(errOut)
		if message == "" {
			return nil, nil, fmt.Errorf("external adapter process failed: %w", err)
		}
		return nil, nil, fmt.Errorf("external adapter process failed: %w: %s", err, message)
	}
	if len(bytes.TrimSpace(errOut)) != 0 {
		return nil, nil, fmt.Errorf("external adapter wrote to stderr on success: %s", sanitizeDiagnostic(errOut))
	}
	return out, nil, nil
}

func withInvocationDeadline(parent context.Context, limit time.Duration) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(limit)
	if parentDeadline, ok := parent.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return context.WithDeadline(parent, deadline)
}

func sandboxEnvironment(sandbox string) ([]string, error) {
	environment := []string{
		"ASM_EXTERNAL_ADAPTER=1",
		"ASM_EXTERNAL_ADAPTER_PROTOCOL=" + ProtocolVersion,
		"HOME=" + sandbox,
		"TMPDIR=" + filepath.Join(sandbox, "work"),
		"LANG=C",
		"LC_ALL=C",
	}
	if runtime.GOOS == "windows" {
		environment = []string{
			"ASM_EXTERNAL_ADAPTER=1",
			"ASM_EXTERNAL_ADAPTER_PROTOCOL=" + ProtocolVersion,
			"HOME=" + sandbox,
			"USERPROFILE=" + sandbox,
			"TEMP=" + filepath.Join(sandbox, "work"),
			"TMP=" + filepath.Join(sandbox, "work"),
		}
		for _, key := range []string{"SystemRoot", "WINDIR"} {
			if value := os.Getenv(key); value != "" {
				environment = append(environment, key+"="+value)
			}
		}
	}
	if os.Getenv("GOCOVERDIR") != "" {
		coverageDir := filepath.Join(sandbox, "coverage")
		if err := os.MkdirAll(coverageDir, 0o700); err != nil {
			return nil, fmt.Errorf("prepare external adapter coverage directory: %w", err)
		}
		if err := os.Chmod(coverageDir, 0o700); err != nil {
			return nil, fmt.Errorf("harden external adapter coverage directory: %w", err)
		}
		environment = append(environment, "GOCOVERDIR="+coverageDir)
	}
	return environment, nil
}

func sanitizeDiagnostic(data []byte) string {
	value := strings.TrimSpace(string(data))
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || (r >= 0x20 && r != 0x7f) {
			return r
		}
		return ' '
	}, value)
	if len(value) > 512 {
		value = value[:512] + "..."
	}
	return value
}

func max64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
