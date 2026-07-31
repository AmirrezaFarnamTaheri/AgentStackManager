//go:build windows

package processctl

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	createSuspended                    = 0x00000004
	createNewProcessGroup              = 0x00000200
	jobObjectExtendedLimitInformation  = 9
	jobObjectCpuRateControlInformation = 15
	jobObjectLimitKillOnJobClose       = 0x00002000
	jobObjectLimitActiveProcess        = 0x00000008
	jobObjectLimitJobMemory            = 0x00000200
	jobObjectCPURateControlEnable      = 0x00000001
	jobObjectCPURateControlHardCap     = 0x00000004
	processSetQuota                    = 0x0100
	processTerminate                   = 0x0001
	processSuspendResume               = 0x0800
	processQueryLimitedInformation     = 0x1000
	stillActive                        = 259
)

var (
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	ntdll                        = syscall.NewLazyDLL("ntdll.dll")
	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = kernel32.NewProc("TerminateJobObject")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procOpenProcess              = kernel32.NewProc("OpenProcess")
	procGetExitCodeProcess       = kernel32.NewProc("GetExitCodeProcess")
	procNtResumeProcess          = ntdll.NewProc("NtResumeProcess")
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

type cpuRateControlInformation struct {
	ControlFlags uint32
	CPURate      uint32
}

func validatePlatformLimits(Limits) error { return nil }

func buildExtendedLimitInformation(limits Limits) extendedLimitInformation {
	info := extendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	if limits.ActiveProcesses > 0 {
		info.BasicLimitInformation.LimitFlags |= jobObjectLimitActiveProcess
		info.BasicLimitInformation.ActiveProcessLimit = limits.ActiveProcesses
	}
	if limits.MemoryBytes > 0 {
		info.BasicLimitInformation.LimitFlags |= jobObjectLimitJobMemory
		info.JobMemoryLimit = uintptr(limits.MemoryBytes)
	}
	return info
}

func buildCPURateInformation(limits Limits) cpuRateControlInformation {
	if limits.CPUPercent == 0 {
		return cpuRateControlInformation{}
	}
	return cpuRateControlInformation{
		ControlFlags: jobObjectCPURateControlEnable | jobObjectCPURateControlHardCap,
		CPURate:      limits.CPUPercent * 100,
	}
}

type windowsController struct{ job syscall.Handle }

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(uint32(pid)))
	if handle == 0 {
		return false
	}
	defer procCloseHandle.Call(handle)
	var code uint32
	ok, _, _ := procGetExitCodeProcess.Call(handle, uintptr(unsafe.Pointer(&code)))
	return ok != 0 && code == stillActive
}

func prepareCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNewProcessGroup | createSuspended
}

func attachCommand(cmd *exec.Cmd, limits Limits) (platformController, error) {
	if cmd.Process == nil {
		return nil, errors.New("process was not started")
	}
	jobRaw, _, createErr := procCreateJobObjectW.Call(0, 0)
	if jobRaw == 0 {
		return nil, createErr
	}
	job := syscall.Handle(jobRaw)
	info := buildExtendedLimitInformation(limits)
	ok, _, setErr := procSetInformationJobObject.Call(uintptr(job), jobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if ok == 0 {
		procCloseHandle.Call(uintptr(job))
		return nil, setErr
	}
	if cpu := buildCPURateInformation(limits); cpu.ControlFlags != 0 {
		ok, _, cpuErr := procSetInformationJobObject.Call(uintptr(job), jobObjectCpuRateControlInformation, uintptr(unsafe.Pointer(&cpu)), unsafe.Sizeof(cpu))
		if ok == 0 {
			procCloseHandle.Call(uintptr(job))
			return nil, cpuErr
		}
	}
	access := uintptr(processSetQuota | processTerminate | processSuspendResume | processQueryLimitedInformation)
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
	status, _, _ := procNtResumeProcess.Call(processRaw)
	if status != 0 {
		procTerminateJobObject.Call(uintptr(job), 1)
		procCloseHandle.Call(uintptr(job))
		return nil, fmt.Errorf("resume child after Job Object assignment: NTSTATUS 0x%08x", uint32(status))
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
