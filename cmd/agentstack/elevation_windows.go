//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var (
	shell32DLL        = syscall.NewLazyDLL("shell32.dll")
	isUserAnAdminProc = shell32DLL.NewProc("IsUserAnAdmin")
	shellExecuteProc  = shell32DLL.NewProc("ShellExecuteW")
)

func ensureElevated(args []string, markerPresent bool) (bool, error) {
	admin, err := currentProcessIsAdministrator()
	if err != nil {
		return false, err
	}
	if admin {
		return false, nil
	}
	if markerPresent {
		return false, fmt.Errorf("the elevated process did not receive an administrator token")
	}
	executable, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("locate executable: %w", err)
	}
	params := make([]string, 0, len(args)+1)
	params = append(params, elevationMarker)
	for _, arg := range args {
		params = append(params, syscall.EscapeArg(arg))
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(executable)
	arguments, _ := syscall.UTF16PtrFromString(strings.Join(params, " "))
	result, _, callErr := shellExecuteProc.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(arguments)),
		0,
		1,
	)
	if result <= 32 {
		if callErr != syscall.Errno(0) {
			return false, fmt.Errorf("request UAC elevation: %w", callErr)
		}
		return false, fmt.Errorf("request UAC elevation failed with ShellExecute code %d", result)
	}
	return true, nil
}

func currentProcessIsAdministrator() (bool, error) {
	result, _, callErr := isUserAnAdminProc.Call()
	if result != 0 {
		return true, nil
	}
	if callErr != syscall.Errno(0) {
		return false, fmt.Errorf("check administrator token: %w", callErr)
	}
	return false, nil
}
