package selfinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentstack/agentstack/internal/safefile"
)

type Report struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Copied      bool   `json:"copied"`
	PathChanged bool   `json:"pathChanged"`
	Message     string `json:"message,omitempty"`
}

func DefaultInstallDir() (string, error) {
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "Programs", "AgentStack", "bin"), nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(config, "AgentStack", "bin"), nil
}

func InstallSelf() (Report, error) {
	source, err := os.Executable()
	if err != nil {
		return Report{}, err
	}
	return InstallFrom(source)
}

// InstallFrom installs an explicit AgentStack console binary. The graphical
// setup executable uses this entry point so the PATH command retains console
// stdout/stderr behavior.
func InstallFrom(source string) (Report, error) {
	var err error
	source, err = filepath.Abs(source)
	if err != nil {
		return Report{}, err
	}
	dir, err := DefaultInstallDir()
	if err != nil {
		return Report{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Report{}, err
	}
	name := "agentstack"
	if filepath.Ext(source) == ".exe" {
		name += ".exe"
	}
	destination := filepath.Join(dir, name)
	report := Report{Source: source, Destination: destination}
	same, err := sameFileContent(source, destination)
	if err != nil {
		return report, err
	}
	if !same {
		if err := copyExecutable(source, destination); err != nil {
			return report, err
		}
		report.Copied = true
	}
	changed, message, err := ensureUserPath(dir)
	if err != nil {
		return report, err
	}
	report.PathChanged, report.Message = changed, message
	return report, nil
}

// FindReleaseConsoleSibling locates the architecture-specific console binary
// shipped beside AgentStack-Setup.exe.
func FindReleaseConsoleSibling(setupPath, architecture string) (string, error) {
	setupPath, err := filepath.Abs(setupPath)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(setupPath)
	candidates := []string{
		filepath.Join(dir, "agentstack-windows-"+architecture+".exe"),
		filepath.Join(dir, "agentstack.exe"),
	}
	for _, candidate := range candidates {
		same, pathErr := samePath(candidate, setupPath)
		if pathErr != nil {
			return "", pathErr
		}
		if same {
			continue
		}
		info, statErr := os.Stat(candidate)
		if statErr == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", statErr
		}
	}
	return "", fmt.Errorf("console binary for %s was not found beside %s", architecture, setupPath)
}

func VerifyFileSHA256(path, expected string) error {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if len(expected) != 64 {
		return fmt.Errorf("expected SHA-256 must contain 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(expected); err != nil {
		return fmt.Errorf("expected SHA-256 is invalid: %w", err)
	}
	actual, err := fileHash(path)
	if err != nil {
		return err
	}
	actualHex := hex.EncodeToString(actual[:])
	if actualHex != expected {
		return fmt.Errorf("SHA-256 mismatch for %s: expected %s got %s", path, expected, actualHex)
	}
	return nil
}

func VerifyReleasePair(setupPath, consolePath, expectedConsoleSHA256, expectedPublisherThumbprint string) error {
	if err := VerifyFileSHA256(consolePath, expectedConsoleSHA256); err != nil {
		return err
	}
	if _, err := os.Stat(setupPath); err != nil {
		return fmt.Errorf("setup executable is unavailable: %w", err)
	}
	return nil
}

func normalizeThumbprint(value string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
}

func samePath(left, right string) (bool, error) {
	left, err := filepath.Abs(left)
	if err != nil {
		return false, err
	}
	right, err = filepath.Abs(right)
	if err != nil {
		return false, err
	}
	return filepath.Clean(left) == filepath.Clean(right), nil
}

func sameFileContent(left, right string) (bool, error) {
	leftHash, err := fileHash(left)
	if err != nil {
		return false, err
	}
	rightHash, err := fileHash(right)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return leftHash == rightHash, nil
}

func fileHash(path string) ([32]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [32]byte{}, err
	}
	var result [32]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}

func copyExecutable(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	temp, err := os.CreateTemp(filepath.Dir(destination), ".agentstack-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o755); err != nil {
		temp.Close()
		return err
	}
	if _, err := io.Copy(temp, input); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := safefile.Replace(tempName, destination); err != nil {
		return fmt.Errorf("replace AgentStack-owned binary: %w", err)
	}
	return nil
}
