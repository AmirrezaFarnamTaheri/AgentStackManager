//go:build windows

package security

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	args := []string{path, "/inheritance:r", "/grant:r", "*" + sid + ":(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F", "/remove:g", "*S-1-1-0", "*S-1-5-11", "/c", "/q"}
	if output, err := exec.Command("icacls.exe", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("secure AgentStack data ACL: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return AuditPrivateDir(path)
}

func AuditPrivateDir(path string) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	output, err := exec.Command("icacls.exe", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("audit AgentStack data ACL: %w: %s", err, strings.TrimSpace(string(output)))
	}
	text := strings.ToUpper(string(output))
	if !strings.Contains(text, strings.ToUpper(sid)) || strings.Contains(text, "EVERYONE") || strings.Contains(text, "AUTHENTICATED USERS") {
		return fmt.Errorf("AgentStack data ACL is not private to the current user and system principals")
	}
	return nil
}

func currentUserSID() (string, error) {
	output, err := exec.Command("whoami.exe", "/user", "/fo", "csv", "/nh").Output()
	if err != nil {
		return "", fmt.Errorf("resolve current Windows SID: %w", err)
	}
	records, err := csv.NewReader(bytes.NewReader(output)).ReadAll()
	if err != nil || len(records) != 1 || len(records[0]) < 2 {
		return "", fmt.Errorf("parse current Windows SID")
	}
	return strings.TrimSpace(records[0][1]), nil
}
