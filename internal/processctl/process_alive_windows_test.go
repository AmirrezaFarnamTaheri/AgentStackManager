//go:build windows

package processctl

func processAliveForTest(pid int) bool {
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(uint32(pid)))
	if handle == 0 {
		return false
	}
	procCloseHandle.Call(handle)
	return true
}
