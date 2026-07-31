//go:build windows

package processctl

func processAliveForTest(pid int) bool { return IsAlive(pid) }
