//go:build windows

package winsecurity

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	daclSecurityInformation                         = 0x00000004
	protectedDACLInformation                        = 0x80000000
	unprotectedDACLInformation                      = 0x20000000
	securityDescriptorDACLProtected                 = 0x1000
	securityDescriptorStringRevision1               = 1
	errorInsufficientBuffer           syscall.Errno = 122
)

var (
	advapi32                                  = syscall.NewLazyDLL("advapi32.dll")
	kernel32                                  = syscall.NewLazyDLL("kernel32.dll")
	procGetFileSecurityW                      = advapi32.NewProc("GetFileSecurityW")
	procSetFileSecurityW                      = advapi32.NewProc("SetFileSecurityW")
	procGetSecurityDescriptorControl          = advapi32.NewProc("GetSecurityDescriptorControl")
	procConvertSecurityDescriptorToStringSDDL = advapi32.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
	procLocalFree                             = kernel32.NewProc("LocalFree")
)

// DACL is a captured, self-relative file DACL plus the control flags required
// to restore its inheritance protection exactly.
type DACL struct {
	Descriptor          []byte
	SecurityInformation uint32
	SDDL                string
}

// CaptureFileDACL returns a restorable and canonically comparable snapshot of
// the file's discretionary access-control list.
func CaptureFileDACL(path string) (DACL, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return DACL{}, err
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
		return DACL{}, fmt.Errorf("measure file DACL: %w", callErr)
	}
	if needed == 0 {
		return DACL{}, fmt.Errorf("measure file DACL: empty security descriptor")
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
		return DACL{}, fmt.Errorf("read file DACL: %w", callErr)
	}

	var control uint16
	var revision uint32
	ok, _, callErr = procGetSecurityDescriptorControl.Call(
		uintptr(unsafe.Pointer(&descriptor[0])),
		uintptr(unsafe.Pointer(&control)),
		uintptr(unsafe.Pointer(&revision)),
	)
	if ok == 0 {
		return DACL{}, fmt.Errorf("read file DACL control: %w", callErr)
	}
	securityInformation := uint32(daclSecurityInformation | unprotectedDACLInformation)
	if control&securityDescriptorDACLProtected != 0 {
		securityInformation = daclSecurityInformation | protectedDACLInformation
	}
	sddl, err := descriptorDACLString(descriptor)
	if err != nil {
		return DACL{}, err
	}
	return DACL{
		Descriptor:          descriptor,
		SecurityInformation: securityInformation,
		SDDL:                sddl,
	}, nil
}

// ApplyFileDACL restores a previously captured DACL to path.
func ApplyFileDACL(path string, dacl DACL) error {
	if len(dacl.Descriptor) == 0 {
		return fmt.Errorf("file DACL snapshot is empty")
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	ok, _, callErr := procSetFileSecurityW.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(dacl.SecurityInformation),
		uintptr(unsafe.Pointer(&dacl.Descriptor[0])),
	)
	if ok == 0 {
		return fmt.Errorf("apply file DACL: %w", callErr)
	}
	return nil
}

// FileDACLSDDL returns the canonical SDDL representation used for semantic
// comparisons and allowlist auditing.
func FileDACLSDDL(path string) (string, error) {
	dacl, err := CaptureFileDACL(path)
	if err != nil {
		return "", err
	}
	return dacl.SDDL, nil
}

func descriptorDACLString(descriptor []byte) (string, error) {
	if len(descriptor) == 0 {
		return "", fmt.Errorf("convert file DACL: empty security descriptor")
	}
	var output *uint16
	var length uint32
	ok, _, callErr := procConvertSecurityDescriptorToStringSDDL.Call(
		uintptr(unsafe.Pointer(&descriptor[0])),
		securityDescriptorStringRevision1,
		daclSecurityInformation,
		uintptr(unsafe.Pointer(&output)),
		uintptr(unsafe.Pointer(&length)),
	)
	if ok == 0 {
		return "", fmt.Errorf("convert file DACL to SDDL: %w", callErr)
	}
	if output == nil {
		return "", fmt.Errorf("convert file DACL to SDDL: empty result")
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(output)))
	return syscall.UTF16ToString(unsafe.Slice(output, int(length))), nil
}
