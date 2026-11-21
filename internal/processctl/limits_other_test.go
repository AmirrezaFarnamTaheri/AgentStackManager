//go:build !windows && !linux

package processctl

import (
	"errors"
	"os/exec"
	"testing"
)

func TestStartWithLimitsFailsClosedOnUnsupportedPlatform(t *testing.T) {
	_, err := StartWithLimits(exec.Command("this-command-must-not-run"), Limits{MemoryBytes: 128 << 20})
	if !errors.Is(err, ErrResourceLimitsUnsupported) {
		t.Fatalf("expected unsupported-limits error, got %v", err)
	}
}
