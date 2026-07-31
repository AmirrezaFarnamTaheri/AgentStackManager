//go:build !windows

package security

import (
	"fmt"
	"os"
)

func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("directory %s is accessible to group or others", path)
	}
	return nil
}

func AuditPrivateDir(path string) error { return EnsurePrivateDir(path) }
