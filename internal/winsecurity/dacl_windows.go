//go:build windows

package winsecurity

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	daclSecurityInformation                         = 0x00000004
	protectedDACLInformation                        = 0x80000000
	unprotectedDACLInformation                      = 0x20000000
	securityDescriptorDACLProtected                 = 0x1000
	securityDescriptorStringRevision1               = 1
	seFileObject                                    = 1
	errorInsufficientBuffer           syscall.Errno = 122
)

var (
	advapi32                                  = syscall.NewLazyDLL("advapi32.dll")
	kernel32                                  = syscall.NewLazyDLL("kernel32.dll")
	procGetFileSecurityW                      = advapi32.NewProc("GetFileSecurityW")
	procGetSecurityDescriptorControl          = advapi32.NewProc("GetSecurityDescriptorControl")
	procSetNamedSecurityInfoW                 = advapi32.NewProc("SetNamedSecurityInfoW")
	procConvertSecurityDescriptorToStringSDDL = advapi32.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
	procConvertStringSidToSidW                = advapi32.NewProc("ConvertStringSidToSidW")
	procConvertSidToStringSidW                = advapi32.NewProc("ConvertSidToStringSidW")
	procLocalFree                             = kernel32.NewProc("LocalFree")
	procLocalSize                             = kernel32.NewProc("LocalSize")
)

const (
	securityDescriptorRelativeSize = 20
	daclOffsetPosition             = 16
	aclHeaderSize                  = 8
)

// DACL is a captured native discretionary access-control list plus the
// inheritance-protection state required to restore it exactly.
type DACL struct {
	ACL       []byte
	Protected bool
	SDDL      string
}

// EqualDACL compares the native ACL and its inheritance-protection state. SDDL
// is deliberately excluded because Windows may render equivalent trustees or
// descriptor control flags with different aliases after an ACL is installed.
func EqualDACL(left, right DACL) bool {
	return left.Protected == right.Protected && bytes.Equal(left.ACL, right.ACL)
}

// CaptureFileDACL returns a restorable, representation-independent snapshot of
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

	if len(descriptor) < securityDescriptorRelativeSize {
		return DACL{}, fmt.Errorf("file security descriptor is too short: %d bytes", len(descriptor))
	}
	daclOffset := int(binary.LittleEndian.Uint32(descriptor[daclOffsetPosition : daclOffsetPosition+4]))
	if daclOffset == 0 {
		return DACL{}, fmt.Errorf("file security descriptor has a NULL DACL")
	}
	if daclOffset < securityDescriptorRelativeSize || daclOffset > len(descriptor)-aclHeaderSize {
		return DACL{}, fmt.Errorf("file DACL offset %d is outside the security descriptor", daclOffset)
	}
	aclSize := int(binary.LittleEndian.Uint16(descriptor[daclOffset+2 : daclOffset+4]))
	if aclSize < aclHeaderSize || aclSize > len(descriptor)-daclOffset {
		return DACL{}, fmt.Errorf("file DACL has invalid size %d", aclSize)
	}
	acl := append([]byte(nil), descriptor[daclOffset:daclOffset+aclSize]...)

	sddl, err := descriptorDACLString(descriptor)
	if err != nil {
		return DACL{}, err
	}
	return DACL{
		ACL:       acl,
		Protected: control&securityDescriptorDACLProtected != 0,
		SDDL:      sddl,
	}, nil
}

// ApplyFileDACL restores a previously captured native DACL to path.
func ApplyFileDACL(path string, dacl DACL) error {
	if len(dacl.ACL) < aclHeaderSize {
		return fmt.Errorf("file DACL snapshot is empty or malformed")
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	securityInformation := uint32(daclSecurityInformation | unprotectedDACLInformation)
	if dacl.Protected {
		securityInformation = daclSecurityInformation | protectedDACLInformation
	}
	result, _, _ := procSetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(name)),
		seFileObject,
		uintptr(securityInformation),
		0,
		0,
		uintptr(unsafe.Pointer(&dacl.ACL[0])),
		0,
	)
	if result != 0 {
		return fmt.Errorf("apply file DACL: %w", syscall.Errno(result))
	}
	return nil
}

// FileDACLSDDL returns the SDDL representation used for allowlist auditing and
// diagnostics. Callers must resolve trustee aliases before comparing identity.
func FileDACLSDDL(path string) (string, error) {
	dacl, err := CaptureFileDACL(path)
	if err != nil {
		return "", err
	}
	return dacl.SDDL, nil
}

// CanonicalSIDString resolves a numeric SID or SDDL trustee alias, such as SY,
// BA, or LA, to the canonical numeric SID string for the current machine.
func CanonicalSIDString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("SID is empty")
	}
	input, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return "", err
	}
	var sid uintptr
	ok, _, callErr := procConvertStringSidToSidW.Call(
		uintptr(unsafe.Pointer(input)),
		uintptr(unsafe.Pointer(&sid)),
	)
	if ok == 0 {
		return "", fmt.Errorf("resolve SID %q: %w", value, callErr)
	}
	if sid == 0 {
		return "", fmt.Errorf("resolve SID %q: empty result", value)
	}
	defer procLocalFree.Call(sid)

	var output *uint16
	ok, _, callErr = procConvertSidToStringSidW.Call(
		sid,
		uintptr(unsafe.Pointer(&output)),
	)
	if ok == 0 {
		return "", fmt.Errorf("format SID %q: %w", value, callErr)
	}
	if output == nil {
		return "", fmt.Errorf("format SID %q: empty result", value)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(output)))
	canonical, err := localUTF16String(output)
	if err != nil {
		return "", fmt.Errorf("format SID %q: %w", value, err)
	}
	return strings.ToUpper(canonical), nil
}

func localUTF16String(value *uint16) (string, error) {
	if value == nil {
		return "", fmt.Errorf("empty LocalAlloc string")
	}
	size, _, callErr := procLocalSize.Call(uintptr(unsafe.Pointer(value)))
	if size == 0 {
		return "", fmt.Errorf("measure LocalAlloc string: %w", callErr)
	}
	return syscall.UTF16ToString(unsafe.Slice(value, int(size)/2)), nil
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
