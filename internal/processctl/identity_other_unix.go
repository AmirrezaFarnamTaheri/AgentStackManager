//go:build !windows && !linux

package processctl

// Other Unix platforms retain the PID/PGID validation. Linux additionally
// supplies a kernel process-start identity through /proc.
func platformProcessIdentity(int) (string, error) { return "", nil }
