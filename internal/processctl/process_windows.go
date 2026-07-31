//go:build windows

package processctl

import (
	"errors"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	createNewProcessGroup             = 0x00000200
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x00002000
	processSetQuota                   = 0x0100
	processTerminate                  = 0x0001
	processQueryLimitedInformation    = 0x1000
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = kernel32.NewProc("TerminateJobObject")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
)

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type basicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type extendedLimitInformation struct {
	BasicLimitInformation basicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type windowsController struct{ job syscall.Handle }

func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

func attachCommand(cmd *exec.Cmd) (platformController, error) {
	if cmd.Process == nil {
		return nil, errors.New("process was not started")
	}
	jobRaw, _, createErr := procCreateJobObjectW.Call(0, 0)
	if jobRaw == 0 {
		return nil, createErr
	}
	job := syscall.Handle(jobRaw)
	info := extendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	ok, _, setErr := procSetInformationJobObject.Call(uintptr(job), jobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if ok == 0 {
		procCloseHandle.Call(uintptr(job))
		return nil, setErr
	}
	access := uintptr(processSetQuota | processTerminate | processQueryLimitedInformation)
	processRaw, _, openErr := procOpenProcess.Call(access, 0, uintptr(uint32(cmd.Process.Pid)))
	if processRaw == 0 {
		procCloseHandle.Call(uintptr(job))
		return nil, openErr
	}
	defer procCloseHandle.Call(processRaw)
	ok, _, assignErr := procAssignProcessToJobObject.Call(uintptr(job), processRaw)
	if ok == 0 {
		procCloseHandle.Call(uintptr(job))
		return nil, assignErr
	}
	return windowsController{job: job}, nil
}

func (c windowsController) terminate() error {
	if c.job == 0 {
		return nil
	}
	ok, _, err := procTerminateJobObject.Call(uintptr(c.job), 1)
	if ok == 0 {
		return err
	}
	return nil
}
func (c windowsController) close() error {
	if c.job == 0 {
		return nil
	}
	ok, _, err := procCloseHandle.Call(uintptr(c.job))
	if ok == 0 {
		return err
	}
	return nil
}
