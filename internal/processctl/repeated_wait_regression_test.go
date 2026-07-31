package processctl

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWaitReturnsOriginalExitErrorToEveryCaller(t *testing.T) {
	process, err := Start(exec.Command("sh", "-c", "exit 7"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	first := process.Wait(ctx)
	second := process.Wait(ctx)
	if first == nil || second == nil {
		t.Fatalf("every wait must report the terminal failure: first=%v second=%v", first, second)
	}
	if first.Error() != second.Error() || !strings.Contains(first.Error(), "exit status 7") {
		t.Fatalf("wait results differ: first=%v second=%v", first, second)
	}
}
