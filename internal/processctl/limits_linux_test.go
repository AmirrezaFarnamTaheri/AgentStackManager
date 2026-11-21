//go:build linux

package processctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareLinuxCgroupWritesAllLimits(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory pids"), 0o600); err != nil {
		t.Fatal(err)
	}
	limits := Limits{MemoryBytes: 128 << 20, CPUPercent: 80, ActiveProcesses: 7}
	file, path, err := prepareLinuxCgroup(root, limits)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	defer os.RemoveAll(filepath.Join(root, "agentstack"))
	for name, want := range map[string]string{
		"memory.max": "134217728",
		"cpu.max":    "80000 100000",
		"pids.max":   "7",
	} {
		data, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(data)); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestPrepareLinuxCgroupFailsClosedWithoutV2(t *testing.T) {
	_, _, err := prepareLinuxCgroup(t.TempDir(), Limits{MemoryBytes: 128 << 20})
	if err == nil || !strings.Contains(err.Error(), "cgroup v2 is unavailable") {
		t.Fatalf("expected cgroup v2 failure, got %v", err)
	}
}
