//go:build linux

package processctl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// platformProcessIdentity returns the kernel start-time tick for pid. A PID can
// be reused, but the pair (PID, start time) identifies the process that
// AgentStack actually started for the lifetime relevant to termination.
func platformProcessIdentity(pid int) (string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", fmt.Sprintf("%d", pid), "stat"))
	if err != nil {
		return "", err
	}
	text := string(data)
	closing := strings.LastIndex(text, ")")
	if closing < 0 || closing+1 >= len(text) {
		return "", fmt.Errorf("parse /proc/%d/stat: malformed comm field", pid)
	}
	fields := strings.Fields(text[closing+1:])
	// fields[0] is stat field 3 (state), so field 22 (starttime) is index 19.
	if len(fields) <= 19 {
		return "", fmt.Errorf("parse /proc/%d/stat: expected start-time field", pid)
	}
	return fields[19], nil
}
