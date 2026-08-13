//go:build linux

package processctl

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

var linuxCgroupRoot = "/sys/fs/cgroup"

func startPlatformCommand(cmd *exec.Cmd, limits Limits) (platformController, error) {
	if limits.Disabled() {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return attachUnixCommand(cmd.Process.Pid, "")
	}

	cgroupFile, cgroupPath, err := prepareLinuxCgroup(linuxCgroupRoot, limits)
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = cgroupFile.Close()
		_ = os.Remove(cgroupPath)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:     true,
		UseCgroupFD: true,
		CgroupFD:    int(cgroupFile.Fd()),
	}
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, err
	}
	if err := cgroupFile.Close(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = os.Remove(cgroupPath)
		return nil, fmt.Errorf("close Linux cgroup directory descriptor: %w", err)
	}
	controller, err := attachUnixCommand(cmd.Process.Pid, cgroupPath)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = os.Remove(cgroupPath)
		return nil, err
	}
	return controller, nil
}

func prepareLinuxCgroup(root string, limits Limits) (*os.File, string, error) {
	if limits.Disabled() {
		return nil, "", errors.New("Linux cgroup preparation requires enabled limits")
	}
	if _, err := os.Stat(filepath.Join(root, "cgroup.controllers")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("%w: cgroup v2 is unavailable", ErrResourceLimitsUnsupported)
		}
		return nil, "", fmt.Errorf("inspect cgroup v2 root: %w", err)
	}
	parent := filepath.Join(root, "agentstack")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, "", fmt.Errorf("create AgentStack cgroup parent: %w", err)
	}
	path, err := os.MkdirTemp(parent, "process-")
	if err != nil {
		return nil, "", fmt.Errorf("create process cgroup: %w", err)
	}
	cleanup := func() { _ = os.Remove(path) }
	if limits.MemoryBytes > 0 {
		if err := writeCgroupLimit(path, "memory.max", strconv.FormatUint(limits.MemoryBytes, 10)); err != nil {
			cleanup()
			return nil, "", err
		}
	}
	if limits.CPUPercent > 0 {
		const period = 100000
		quota := int64(limits.CPUPercent) * period / 100
		if err := writeCgroupLimit(path, "cpu.max", fmt.Sprintf("%d %d", quota, period)); err != nil {
			cleanup()
			return nil, "", err
		}
	}
	if limits.ActiveProcesses > 0 {
		if err := writeCgroupLimit(path, "pids.max", strconv.FormatUint(uint64(limits.ActiveProcesses), 10)); err != nil {
			cleanup()
			return nil, "", err
		}
	}
	file, err := os.Open(path)
	if err != nil {
		cleanup()
		return nil, "", fmt.Errorf("open process cgroup: %w", err)
	}
	return file, path, nil
}

func writeCgroupLimit(dir, name, value string) error {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		return fmt.Errorf("write Linux cgroup %s: %w", name, err)
	}
	return nil
}
