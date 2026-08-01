//go:build windows

package security

import (
	"os/exec"
	"strings"
	"testing"
)

func TestAuditPrivateDirRejectsUnexpectedEveryoneACE(t *testing.T) {
	path := t.TempDir()
	if err := EnsurePrivateDir(path); err != nil {
		t.Fatalf("secure test directory: %v", err)
	}

	output, err := exec.Command(
		"icacls.exe",
		path,
		"/grant",
		"*S-1-1-0:(OI)(CI)RX",
		"/c",
		"/q",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("inject unexpected Everyone ACE: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if err := AuditPrivateDir(path); err == nil {
		t.Fatal("audit accepted an unexpected Everyone ACE")
	}

	if err := EnsurePrivateDir(path); err != nil {
		t.Fatalf("repair test directory ACL: %v", err)
	}
	if err := AuditPrivateDir(path); err != nil {
		t.Fatalf("audit rejected repaired private ACL: %v", err)
	}
}

func TestEnsurePrivateDirRemovesBuiltinUsersACE(t *testing.T) {
	path := t.TempDir()
	output, err := exec.Command(
		"icacls.exe",
		path,
		"/grant",
		"*S-1-5-32-545:(OI)(CI)RX",
		"/c",
		"/q",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("inject BUILTIN Users ACE: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if err := EnsurePrivateDir(path); err != nil {
		t.Fatalf("remove BUILTIN Users ACE: %v", err)
	}
	if err := AuditPrivateDir(path); err != nil {
		t.Fatalf("audit rejected exact repaired ACL: %v", err)
	}
}
