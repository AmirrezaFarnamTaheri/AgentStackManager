//go:build !windows

package selfinstall

import "fmt"

func VerifyAuthenticode(path, expectedThumbprint string) error {
	return fmt.Errorf("Authenticode verification is supported only on Windows")
}
