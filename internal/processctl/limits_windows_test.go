//go:build windows

package processctl

import (
	"context"
	"os/exec"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var procQueryInformationJobObjectForTest = syscall.NewLazyDLL("kernel32.dll").NewProc("QueryInformationJobObject")

func TestPrepareWindowsCommandStartsSuspendedUntilJobAssignment(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/d", "/s", "/c", "exit 0")
	prepareWindowsCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("Windows process attributes were not configured")
	}
	want := uint32(createNewProcessGroup | createSuspended)
	if cmd.SysProcAttr.CreationFlags&want != want {
		t.Fatalf("creation flags %#x do not include suspended job assignment flags %#x", cmd.SysProcAttr.CreationFlags, want)
	}
}

func TestWindowsLimitInformationIncludesConfiguredCeilings(t *testing.T) {
	limits := Limits{MemoryBytes: 2 << 30, CPUPercent: 80, ActiveProcesses: 32}
	extended := buildExtendedLimitInformation(limits)
	if extended.BasicLimitInformation.LimitFlags&jobObjectLimitKillOnJobClose == 0 {
		t.Fatal("kill-on-close containment flag is missing")
	}
	if extended.JobMemoryLimit != uintptr(limits.MemoryBytes) {
		t.Fatalf("memory limit mismatch: %d", extended.JobMemoryLimit)
	}
	if extended.BasicLimitInformation.ActiveProcessLimit != limits.ActiveProcesses {
		t.Fatalf("active process limit mismatch: %d", extended.BasicLimitInformation.ActiveProcessLimit)
	}
	cpu := buildCPURateInformation(limits)
	if cpu.CPURate != 8000 || cpu.ControlFlags != jobObjectCPURateControlEnable|jobObjectCPURateControlHardCap {
		t.Fatalf("CPU hard cap mismatch: %#v", cpu)
	}
}

func TestStartWithLimitsCreatesRunnableWindowsJob(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/d", "/s", "/c", "exit 0")
	managed, err := StartWithLimits(cmd, Limits{MemoryBytes: 128 << 20, CPUPercent: 80, ActiveProcesses: 4})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := managed.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithLimitsInstallsKernelJobCeilings(t *testing.T) {
	limits := Limits{MemoryBytes: 256 << 20, CPUPercent: 65, ActiveProcesses: 3}
	cmd := exec.Command("cmd.exe", "/d", "/s", "/c", "ping -n 30 127.0.0.1 >nul")
	managed, err := StartWithLimits(cmd, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = managed.Terminate()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = managed.Wait(ctx)
	}()

	controller, ok := managed.controller.(windowsController)
	if !ok || controller.job == 0 {
		t.Fatalf("managed process does not expose a live Windows Job Object: %#v", managed.controller)
	}

	var extended extendedLimitInformation
	var returned uint32
	okRaw, _, queryErr := procQueryInformationJobObjectForTest.Call(
		uintptr(controller.job),
		jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&extended)),
		unsafe.Sizeof(extended),
		uintptr(unsafe.Pointer(&returned)),
	)
	if okRaw == 0 {
		t.Fatalf("query extended Job Object limits: %v", queryErr)
	}
	wantFlags := uint32(jobObjectLimitKillOnJobClose | jobObjectLimitActiveProcess | jobObjectLimitJobMemory)
	if extended.BasicLimitInformation.LimitFlags&wantFlags != wantFlags {
		t.Fatalf("kernel Job Object flags %#x do not contain %#x", extended.BasicLimitInformation.LimitFlags, wantFlags)
	}
	if extended.JobMemoryLimit != uintptr(limits.MemoryBytes) {
		t.Fatalf("kernel job memory limit = %d, want %d", extended.JobMemoryLimit, limits.MemoryBytes)
	}
	if extended.BasicLimitInformation.ActiveProcessLimit != limits.ActiveProcesses {
		t.Fatalf("kernel active process limit = %d, want %d", extended.BasicLimitInformation.ActiveProcessLimit, limits.ActiveProcesses)
	}

	var cpu cpuRateControlInformation
	returned = 0
	okRaw, _, queryErr = procQueryInformationJobObjectForTest.Call(
		uintptr(controller.job),
		jobObjectCpuRateControlInformation,
		uintptr(unsafe.Pointer(&cpu)),
		unsafe.Sizeof(cpu),
		uintptr(unsafe.Pointer(&returned)),
	)
	if okRaw == 0 {
		t.Fatalf("query CPU Job Object limit: %v", queryErr)
	}
	wantCPUFlags := uint32(jobObjectCPURateControlEnable | jobObjectCPURateControlHardCap)
	if cpu.ControlFlags&wantCPUFlags != wantCPUFlags || cpu.CPURate != limits.CPUPercent*100 {
		t.Fatalf("kernel CPU limit = %#v, want flags %#x rate %d", cpu, wantCPUFlags, limits.CPUPercent*100)
	}
}
