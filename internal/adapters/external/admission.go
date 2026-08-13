package external

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/processctl"
)

const (
	defaultTimeout            = 5 * time.Second
	maximumTimeout            = 30 * time.Second
	defaultMaxRequestBytes    = int64(1 << 20)
	defaultMaxResponseBytes   = int64(1 << 20)
	defaultMaxStderrBytes     = int64(64 << 10)
	defaultMaxExecutableBytes = int64(128 << 20)
	maximumArguments          = 16
	maximumArgumentBytes      = 4096
	maximumArgumentsBytes     = 16 << 10
)

type Limits struct {
	Timeout            time.Duration     `json:"timeout"`
	MaxRequestBytes    int64             `json:"maxRequestBytes"`
	MaxResponseBytes   int64             `json:"maxResponseBytes"`
	MaxStderrBytes     int64             `json:"maxStderrBytes"`
	MaxExecutableBytes int64             `json:"maxExecutableBytes"`
	Process            processctl.Limits `json:"process,omitempty"`
}

func DefaultLimits() Limits {
	return Limits{
		Timeout: defaultTimeout, MaxRequestBytes: defaultMaxRequestBytes,
		MaxResponseBytes: defaultMaxResponseBytes, MaxStderrBytes: defaultMaxStderrBytes,
		MaxExecutableBytes: defaultMaxExecutableBytes,
	}
}

type Admission struct {
	Executable       string               `json:"executable"`
	ExecutableDigest string               `json:"executableDigest"`
	Arguments        []string             `json:"arguments,omitempty"`
	Target           string               `json:"target"`
	SandboxRoot      string               `json:"sandboxRoot,omitempty"`
	Environment      adapters.Environment `json:"environment"`
	Limits           Limits               `json:"limits"`
}

type stagedExecutable struct {
	path            string
	sandbox         string
	digest          string
	size            int64
	arguments       []string
	argumentsDigest string
	limits          Limits
}

func normalizeAdmission(value Admission) (Admission, error) {
	value.Executable = strings.TrimSpace(value.Executable)
	value.ExecutableDigest = strings.TrimSpace(value.ExecutableDigest)
	value.Target = strings.TrimSpace(value.Target)
	value.SandboxRoot = strings.TrimSpace(value.SandboxRoot)
	if value.Executable == "" || !filepath.IsAbs(value.Executable) {
		return Admission{}, fmt.Errorf("external adapter executable must be an absolute path")
	}
	if value.Target == "" {
		return Admission{}, fmt.Errorf("external adapter target is required")
	}
	if !validDigest(value.ExecutableDigest) {
		return Admission{}, fmt.Errorf("external adapter executable requires a pinned sha256 digest")
	}
	arguments, err := normalizeArguments(value.Arguments)
	if err != nil {
		return Admission{}, err
	}
	value.Arguments = arguments
	limits, err := normalizeLimits(value.Limits)
	if err != nil {
		return Admission{}, err
	}
	value.Limits = limits
	if value.SandboxRoot != "" {
		absolute, err := filepath.Abs(value.SandboxRoot)
		if err != nil {
			return Admission{}, fmt.Errorf("resolve external adapter sandbox root: %w", err)
		}
		value.SandboxRoot = absolute
	}
	return value, nil
}

func normalizeLimits(value Limits) (Limits, error) {
	defaults := DefaultLimits()
	if value.Timeout == 0 {
		value.Timeout = defaults.Timeout
	}
	if value.MaxRequestBytes == 0 {
		value.MaxRequestBytes = defaults.MaxRequestBytes
	}
	if value.MaxResponseBytes == 0 {
		value.MaxResponseBytes = defaults.MaxResponseBytes
	}
	if value.MaxStderrBytes == 0 {
		value.MaxStderrBytes = defaults.MaxStderrBytes
	}
	if value.MaxExecutableBytes == 0 {
		value.MaxExecutableBytes = defaults.MaxExecutableBytes
	}
	if value.Timeout <= 0 || value.Timeout > maximumTimeout {
		return Limits{}, fmt.Errorf("external adapter timeout must be between 1ns and %s", maximumTimeout)
	}
	if value.MaxRequestBytes <= 0 || value.MaxRequestBytes > defaultMaxRequestBytes {
		return Limits{}, fmt.Errorf("external adapter request ceiling must be between 1 and %d bytes", defaultMaxRequestBytes)
	}
	if value.MaxResponseBytes <= 0 || value.MaxResponseBytes > defaultMaxResponseBytes {
		return Limits{}, fmt.Errorf("external adapter response ceiling must be between 1 and %d bytes", defaultMaxResponseBytes)
	}
	if value.MaxStderrBytes <= 0 || value.MaxStderrBytes > defaultMaxStderrBytes {
		return Limits{}, fmt.Errorf("external adapter stderr ceiling must be between 1 and %d bytes", defaultMaxStderrBytes)
	}
	if value.MaxExecutableBytes <= 0 || value.MaxExecutableBytes > defaultMaxExecutableBytes {
		return Limits{}, fmt.Errorf("external adapter executable ceiling must be between 1 and %d bytes", defaultMaxExecutableBytes)
	}
	if err := value.Process.Validate(); err != nil {
		return Limits{}, fmt.Errorf("external adapter process ceilings: %w", err)
	}
	return value, nil
}

func normalizeArguments(values []string) ([]string, error) {
	if len(values) > maximumArguments {
		return nil, fmt.Errorf("external adapter accepts at most %d fixed arguments", maximumArguments)
	}
	result := append([]string(nil), values...)
	total := 0
	for i, value := range result {
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("external adapter argument %d contains NUL", i)
		}
		if len(value) > maximumArgumentBytes {
			return nil, fmt.Errorf("external adapter argument %d exceeds %d bytes", i, maximumArgumentBytes)
		}
		total += len(value)
	}
	if total > maximumArgumentsBytes {
		return nil, fmt.Errorf("external adapter fixed arguments exceed %d bytes", maximumArgumentsBytes)
	}
	return result, nil
}

func stage(value Admission) (stagedExecutable, error) {
	admission, err := normalizeAdmission(value)
	if err != nil {
		return stagedExecutable{}, err
	}
	info, err := os.Lstat(admission.Executable)
	if err != nil {
		return stagedExecutable{}, fmt.Errorf("inspect external adapter executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return stagedExecutable{}, fmt.Errorf("external adapter executable must be a regular non-symlink file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return stagedExecutable{}, fmt.Errorf("external adapter executable is not executable")
	}
	if info.Size() <= 0 || info.Size() > admission.Limits.MaxExecutableBytes {
		return stagedExecutable{}, fmt.Errorf("external adapter executable size %d exceeds admission bounds", info.Size())
	}
	input, err := os.Open(admission.Executable)
	if err != nil {
		return stagedExecutable{}, fmt.Errorf("open external adapter executable: %w", err)
	}
	defer input.Close()
	openedInfo, err := input.Stat()
	if err != nil {
		return stagedExecutable{}, fmt.Errorf("stat opened external adapter executable: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return stagedExecutable{}, fmt.Errorf("external adapter executable identity changed during admission")
	}
	parent := admission.SandboxRoot
	if parent != "" {
		parentInfo, statErr := os.Lstat(parent)
		if statErr != nil {
			return stagedExecutable{}, fmt.Errorf("inspect external adapter sandbox root: %w", statErr)
		}
		if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
			return stagedExecutable{}, fmt.Errorf("external adapter sandbox root must be a non-symlink directory")
		}
	}
	sandbox, err := os.MkdirTemp(parent, "agentstack-external-adapter-")
	if err != nil {
		return stagedExecutable{}, fmt.Errorf("create external adapter sandbox: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(sandbox)
		}
	}()
	if err := os.Chmod(sandbox, 0o700); err != nil {
		return stagedExecutable{}, fmt.Errorf("harden external adapter sandbox: %w", err)
	}
	work := filepath.Join(sandbox, "work")
	if err := os.Mkdir(work, 0o700); err != nil {
		return stagedExecutable{}, fmt.Errorf("create external adapter work directory: %w", err)
	}
	extension := ""
	if runtime.GOOS == "windows" {
		extension = filepath.Ext(admission.Executable)
	}
	stagedPath := filepath.Join(sandbox, "adapter"+extension)
	output, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o500)
	if err != nil {
		return stagedExecutable{}, fmt.Errorf("create staged external adapter executable: %w", err)
	}
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hasher), io.LimitReader(input, admission.Limits.MaxExecutableBytes+1))
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil {
		return stagedExecutable{}, fmt.Errorf("stage external adapter executable: %w", copyErr)
	}
	if written != openedInfo.Size() || written > admission.Limits.MaxExecutableBytes {
		return stagedExecutable{}, fmt.Errorf("external adapter executable changed or exceeded bounds during staging")
	}
	if syncErr != nil {
		return stagedExecutable{}, fmt.Errorf("sync staged external adapter executable: %w", syncErr)
	}
	if closeErr != nil {
		return stagedExecutable{}, fmt.Errorf("close staged external adapter executable: %w", closeErr)
	}
	if err := os.Chmod(stagedPath, 0o500); err != nil {
		return stagedExecutable{}, fmt.Errorf("harden staged external adapter executable: %w", err)
	}
	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if digest != admission.ExecutableDigest {
		return stagedExecutable{}, fmt.Errorf("external adapter executable digest mismatch")
	}
	argsDigest, err := integrity.DigestJSON(admission.Arguments)
	if err != nil {
		return stagedExecutable{}, fmt.Errorf("digest external adapter arguments: %w", err)
	}
	cleanup = false
	return stagedExecutable{
		path: stagedPath, sandbox: sandbox, digest: digest, size: written,
		arguments: admission.Arguments, argumentsDigest: argsDigest, limits: admission.Limits,
	}, nil
}

func (value stagedExecutable) cleanup() error {
	if value.sandbox == "" {
		return nil
	}
	return os.RemoveAll(value.sandbox)
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
