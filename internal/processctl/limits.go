package processctl

import (
	"errors"
	"fmt"
)

const (
	minimumMemoryLimit = 64 << 20
	maximumProcesses   = 1024
)

var ErrResourceLimitsUnsupported = errors.New("hard process resource limits are unsupported on this platform")

// Limits defines optional hard ceilings for a managed process tree.
// CPUPercent is a hard cap expressed from 1 through 100. Linux maps it to cgroup v2 cpu.max; Windows maps it to a Job Object CPU rate cap.
type Limits struct {
	MemoryBytes     uint64 `json:"memoryBytes,omitempty"`
	CPUPercent      uint32 `json:"cpuPercent,omitempty"`
	ActiveProcesses uint32 `json:"activeProcesses,omitempty"`
}

func (l Limits) Disabled() bool {
	return l.MemoryBytes == 0 && l.CPUPercent == 0 && l.ActiveProcesses == 0
}

func (l Limits) Validate() error {
	if l.MemoryBytes > 0 && l.MemoryBytes < minimumMemoryLimit {
		return fmt.Errorf("memory limit must be at least %d bytes", minimumMemoryLimit)
	}
	if l.CPUPercent > 100 {
		return fmt.Errorf("CPU percentage must be between 1 and 100")
	}
	if l.ActiveProcesses > maximumProcesses {
		return fmt.Errorf("active process limit must not exceed %d", maximumProcesses)
	}
	return nil
}
