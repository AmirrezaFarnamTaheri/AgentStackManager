//go:build !windows

package processctl

import "syscall"

func processAliveForTest(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
