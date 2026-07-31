//go:build windows

package state

import (
	"syscall"
	"unsafe"
)

var (
	kernel32Process        = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess        = kernel32Process.NewProc("OpenProcess")
	procGetExitCodeProcess = kernel32Process.NewProc("GetExitCodeProcess")
	procCloseHandleProcess = kernel32Process.NewProc("CloseHandle")
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(uint32(pid)))
	if handle == 0 {
		return false
	}
	defer procCloseHandleProcess.Call(handle)
	var code uint32
	ok, _, _ := procGetExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&code)))
	return ok != 0 && code == stillActive
}
