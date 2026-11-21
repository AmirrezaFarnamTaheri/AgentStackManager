//go:build linux

package processctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnixControllerTerminatesThroughCgroupKill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "process")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	killPath := filepath.Join(dir, "cgroup.kill")
	if err := os.WriteFile(killPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	controller := unixController{pid: 1, pgid: 1, cgroupPath: dir}
	if err := controller.terminate(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(killPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "1" {
		t.Fatalf("cgroup.kill = %q, want 1", data)
	}
}

func TestUnixControllerCloseRemovesEmptyCgroup(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "process")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	controller := unixController{cgroupPath: dir}
	if err := controller.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cgroup directory still exists: %v", err)
	}
	if err := controller.close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
