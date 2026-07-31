//go:build windows

package safefile

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	daclSecurityInformation                       = 0x00000004
	protectedDACLInformation                      = 0x80000000
	unprotectedDACLInformation                    = 0x20000000
	securityDescriptorDACLProtected               = 0x1000
	errorInsufficientBuffer         syscall.Errno = 122
)

var (
	advapi32Metadata                 = syscall.NewLazyDLL("advapi32.dll")
	procGetFileSecurityW             = advapi32Metadata.NewProc("GetFileSecurityW")
	procSetFileSecurityW             = advapi32Metadata.NewProc("SetFileSecurityW")
	procGetSecurityDescriptorControl = advapi32Metadata.NewProc("GetSecurityDescriptorControl")
)

type fileMetadata struct {
	descriptor          []byte
	securityInformation uint32
}

func captureFileMetadata(path string) (fileMetadata, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fileMetadata{}, err
	}
	var needed uint32
	ok, _, callErr := procGetFileSecurityW.Call(
		uintptr(unsafe.Pointer(name)),
		daclSecurityInformation,
		0,
		0,
		uintptr(unsafe.Pointer(&needed)),
	)
	if ok == 0 && callErr != errorInsufficientBuffer {
		return fileMetadata{}, fmt.Errorf("measure destination DACL: %w", callErr)
	}
	if needed == 0 {
		return fileMetadata{}, fmt.Errorf("measure destination DACL: empty security descriptor")
	}
	descriptor := make([]byte, needed)
	ok, _, callErr = procGetFileSecurityW.Call(
		uintptr(unsafe.Pointer(name)),
		daclSecurityInformation,
		uintptr(unsafe.Pointer(&descriptor[0])),
		uintptr(len(descriptor)),
		uintptr(unsafe.Pointer(&needed)),
	)
	if ok == 0 {
		return fileMetadata{}, fmt.Errorf("read destination DACL: %w", callErr)
	}
	var control uint16
	var revision uint32
	ok, _, callErr = procGetSecurityDescriptorControl.Call(
		uintptr(unsafe.Pointer(&descriptor[0])),
		uintptr(unsafe.Pointer(&control)),
		uintptr(unsafe.Pointer(&revision)),
	)
	if ok == 0 {
		return fileMetadata{}, fmt.Errorf("read destination DACL control: %w", callErr)
	}
	securityInformation := uint32(daclSecurityInformation | unprotectedDACLInformation)
	if control&securityDescriptorDACLProtected != 0 {
		securityInformation = daclSecurityInformation | protectedDACLInformation
	}
	return fileMetadata{descriptor: descriptor, securityInformation: securityInformation}, nil
}

func applyFileMetadata(path string, metadata fileMetadata) error {
	if len(metadata.descriptor) == 0 {
		return fmt.Errorf("destination DACL snapshot is empty")
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	ok, _, callErr := procSetFileSecurityW.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(metadata.securityInformation),
		uintptr(unsafe.Pointer(&metadata.descriptor[0])),
	)
	if ok == 0 {
		return fmt.Errorf("apply destination DACL to replacement: %w", callErr)
	}
	return nil
}
