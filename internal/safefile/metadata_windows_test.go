//go:build windows

package safefile

import (
	"bytes"
	"encoding/csv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplacePreservesExplicitWindowsDACL(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.tmp")
	destination := filepath.Join(dir, "destination.json")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	sid := currentUserSIDForTest(t)
	args := []string{destination, "/inheritance:r", "/grant:r", "*" + sid + ":F", "/remove:g", "*S-1-1-0", "*S-1-5-11", "/c", "/q"}
	if output, err := exec.Command("icacls.exe", args...).CombinedOutput(); err != nil {
		t.Fatalf("set explicit destination DACL: %v: %s", err, strings.TrimSpace(string(output)))
	}

	before, err := captureFileMetadata(destination)
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := captureFileMetadata(source)
	if err != nil {
		t.Fatal(err)
	}
	if before.dacl.SDDL == sourceBefore.dacl.SDDL {
		t.Fatal("test precondition failed: source already has destination DACL")
	}

	if err := Replace(source, destination); err != nil {
		t.Fatal(err)
	}
	after, err := captureFileMetadata(destination)
	if err != nil {
		t.Fatal(err)
	}
	if before.dacl.SecurityInformation != after.dacl.SecurityInformation || before.dacl.SDDL != after.dacl.SDDL {
		t.Fatalf("destination DACL changed during atomic replacement: before=%q after=%q", before.dacl.SDDL, after.dacl.SDDL)
	}
}

func currentUserSIDForTest(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("whoami.exe", "/user", "/fo", "csv", "/nh").Output()
	if err != nil {
		t.Fatalf("resolve current Windows SID: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(output)).ReadAll()
	if err != nil || len(records) != 1 || len(records[0]) < 2 {
		t.Fatalf("parse current Windows SID: %v", err)
	}
	return strings.TrimSpace(records[0][1])
}
