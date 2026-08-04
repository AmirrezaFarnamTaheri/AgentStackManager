//go:build windows

package notify

import (
	"syscall"
	"unsafe"
)

const (
	messageBoxOK        = 0x00000000
	messageBoxIconError = 0x00000010
	messageBoxIconInfo  = 0x00000040
)

// Error displays a native Windows error dialog.
func Error(title, message string) {
	showBox(title, message, messageBoxOK|messageBoxIconError)
}

// Info displays a native Windows info/notification dialog.
func Info(title, message string) {
	showBox(title, message, messageBoxOK|messageBoxIconInfo)
}

func showBox(title, message string, flags uintptr) {
	go func() {
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
			flags,
		)
	}()
}
