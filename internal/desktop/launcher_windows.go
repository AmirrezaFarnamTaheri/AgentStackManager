//go:build windows

package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

const (
	createNoWindow               = 0x08000000
	processSetQuota              = 0x0100
	processTerminate             = 0x0001
	jobObjectExtendedLimitInfo   = 9
	jobObjectLimitKillOnJobClose = 0x00002000
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	createJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	openProcess              = kernel32.NewProc("OpenProcess")
	closeHandle              = kernel32.NewProc("CloseHandle")
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

func launch(ctx context.Context, target string) error {
	executable, err := findDesktopEngine()
	if err != nil {
		return err
	}
	dataPath, err := desktopDataPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		return fmt.Errorf("create desktop data directory: %w", err)
	}
	command := exec.Command(executable, appArguments(target, dataPath)...)
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start desktop application window: %w", err)
	}
	job, jobErr := createKillJob(command.Process.Pid)
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	select {
	case err := <-wait:
		if job != 0 {
			_, _, _ = closeHandle.Call(job)
		}
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				return fmt.Errorf("desktop application window: %w", err)
			}
		}
		return nil
	case <-ctx.Done():
		if job != 0 {
			_, _, _ = closeHandle.Call(job)
		} else {
			_ = command.Process.Kill()
		}
		<-wait
		if jobErr != nil {
			return fmt.Errorf("%w (desktop process-tree containment unavailable: %v)", ctx.Err(), jobErr)
		}
		return ctx.Err()
	}
}

func desktopDataPath() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		return "", fmt.Errorf("Windows application data directory is unavailable")
	}
	return filepath.Join(base, "AgentStack", "Desktop"), nil
}

func findDesktopEngine() (string, error) {
	seen := map[string]struct{}{}
	candidates := []string{}
	for _, environment := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
		base := os.Getenv(environment)
		if base == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
		)
	}
	for _, name := range []string{"msedge.exe", "chrome.exe", "chromium.exe"} {
		if value, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, value)
		}
	}
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, duplicate := seen[clean]; duplicate {
			continue
		}
		seen[clean] = struct{}{}
		info, err := os.Stat(clean)
		if err == nil && !info.IsDir() {
			return clean, nil
		}
	}
	return "", fmt.Errorf("AgentStack needs Microsoft Edge WebView or Google Chrome to host its desktop window")
}

func createKillJob(pid int) (uintptr, error) {
	job, _, callErr := createJobObjectW.Call(0, 0)
	if job == 0 {
		return 0, fmt.Errorf("create Windows job object: %w", callErr)
	}
	limits := extendedLimitInformation{}
	limits.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	result, _, callErr := setInformationJobObject.Call(job, jobObjectExtendedLimitInfo, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits))
	if result == 0 {
		_, _, _ = closeHandle.Call(job)
		return 0, fmt.Errorf("configure Windows job object: %w", callErr)
	}
	process, _, callErr := openProcess.Call(processSetQuota|processTerminate, 0, uintptr(uint32(pid)))
	if process == 0 {
		_, _, _ = closeHandle.Call(job)
		return 0, fmt.Errorf("open desktop process: %w", callErr)
	}
	defer func() { _, _, _ = closeHandle.Call(process) }()
	result, _, callErr = assignProcessToJobObject.Call(job, process)
	if result == 0 {
		_, _, _ = closeHandle.Call(job)
		return 0, fmt.Errorf("assign desktop process tree: %w", callErr)
	}
	return job, nil
}
