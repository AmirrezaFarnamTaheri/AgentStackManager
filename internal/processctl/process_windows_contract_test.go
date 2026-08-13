package processctl

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsChildProcessContractHidesConsoleWindows(t *testing.T) {
	data, err := os.ReadFile("process_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{"createNoWindow", "HideWindow = true", "createNewProcessGroup | createSuspended | createNoWindow"} {
		if !strings.Contains(source, required) {
			t.Fatalf("Windows child process contract missing %q", required)
		}
	}
}
