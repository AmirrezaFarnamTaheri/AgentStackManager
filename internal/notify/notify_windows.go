//go:build windows

package notify

import (
	"syscall"
	"unsafe"
)

const (
	messageBoxOK        = 0x00000000
	messageBoxIconError = 0x00000010
)

// Error displays a native Windows error dialog. It is used only by the
// GUI-subsystem setup build, where stderr is not visible to the user.
func Error(title, message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBox := user32.NewProc("MessageBoxW")
	titlePtr, titleErr := syscall.UTF16PtrFromString(title)
	messagePtr, messageErr := syscall.UTF16PtrFromString(message)
	if titleErr != nil || messageErr != nil {
		return
	}
	_, _, _ = messageBox.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		messageBoxOK|messageBoxIconError,
	)
}
