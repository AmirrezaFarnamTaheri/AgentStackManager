//go:build windows

package winsecurity

import (
	"bytes"
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
	procGetSecurityDescriptorDACL             = advapi32.NewProc("GetSecurityDescriptorDacl")
	procSetNamedSecurityInfoW                 = advapi32.NewProc("SetNamedSecurityInfoW")
	procConvertSecurityDescriptorToStringSDDL = advapi32.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
	procConvertStringSidToSidW                = advapi32.NewProc("ConvertStringSidToSidW")
	procConvertSidToStringSidW                = advapi32.NewProc("ConvertSidToStringSidW")
	procLocalFree                             = kernel32.NewProc("LocalFree")
)

type aclHeader struct {
	Revision byte
	Sbz1     byte
	Size     uint16
	AceCount uint16
	Sbz2     uint16
}

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

	var present int32
	var defaulted int32
	var aclPointer uintptr
	ok, _, callErr = procGetSecurityDescriptorDACL.Call(
		uintptr(unsafe.Pointer(&descriptor[0])),
		uintptr(unsafe.Pointer(&present)),
		uintptr(unsafe.Pointer(&aclPointer)),
		uintptr(unsafe.Pointer(&defaulted)),
	)
	if ok == 0 {
		return DACL{}, fmt.Errorf("locate file DACL: %w", callErr)
	}
	if present == 0 {
		return DACL{}, fmt.Errorf("file security descriptor has no DACL")
	}
	if aclPointer == 0 {
		return DACL{}, fmt.Errorf("file security descriptor has a NULL DACL")
	}
	header := (*aclHeader)(unsafe.Pointer(aclPointer))
	if header.Size < uint16(unsafe.Sizeof(aclHeader{})) {
		return DACL{}, fmt.Errorf("file DACL has invalid size %d", header.Size)
	}
	acl := append([]byte(nil), unsafe.Slice((*byte)(unsafe.Pointer(aclPointer)), int(header.Size))...)

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
	if len(dacl.ACL) < int(unsafe.Sizeof(aclHeader{})) {
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
	return strings.ToUpper(utf16PointerString(output)), nil
}

func utf16PointerString(value *uint16) string {
	if value == nil {
		return ""
	}
	length := 0
	for *(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(value)) + uintptr(length)*unsafe.Sizeof(*value))) != 0 {
		length++
	}
	return syscall.UTF16ToString(unsafe.Slice(value, length))
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
