//go:build !windows

package selfinstall

func ensureUserPath(target string) (bool, string, error) {
	return false, "PATH mutation is only performed by the Windows build; add " + target + " manually on this platform", nil
}
