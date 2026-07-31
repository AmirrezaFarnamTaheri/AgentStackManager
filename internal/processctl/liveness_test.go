package processctl

import (
	"os"
	"testing"
)

func TestIsAliveRecognizesCurrentProcessAndRejectsInvalidPIDs(t *testing.T) {
	if !IsAlive(os.Getpid()) {
		t.Fatal("current process must be reported alive")
	}
	for _, pid := range []int{0, -1} {
		if IsAlive(pid) {
			t.Fatalf("invalid PID %d must not be reported alive", pid)
		}
	}
}
