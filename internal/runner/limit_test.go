package runner

import (
	"context"
	"os"
	"testing"
)

func TestLimitedBufferZeroLimitUsesDefaultCapacity(t *testing.T) {
	buffer := newLimitedBuffer(0)
	payload := make([]byte, 1<<20)
	if n, err := buffer.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("write default-capacity payload: n=%d err=%v", n, err)
	}
	if buffer.Truncated() || len(buffer.String()) != len(payload) {
		t.Fatalf("zero limit did not use default capacity: truncated=%v len=%d", buffer.Truncated(), len(buffer.String()))
	}
}

func TestLimitedBufferExactCapacityIsNotTruncated(t *testing.T) {
	buffer := newLimitedBuffer(4)
	if n, err := buffer.Write([]byte("abcd")); err != nil || n != 4 {
		t.Fatalf("write exact capacity: n=%d err=%v", n, err)
	}
	if got := buffer.String(); got != "abcd" {
		t.Fatalf("buffer=%q", got)
	}
	if buffer.Truncated() {
		t.Fatal("exact-capacity write was marked truncated")
	}
}

func TestLimitedBufferAccountsForPreviouslyWrittenBytes(t *testing.T) {
	buffer := newLimitedBuffer(5)
	_, _ = buffer.Write([]byte("ab"))
	if n, err := buffer.Write([]byte("cdef")); err != nil || n != 4 {
		t.Fatalf("write overflow payload: n=%d err=%v", n, err)
	}
	if got := buffer.String(); got != "abcde" {
		t.Fatalf("buffer exceeded capacity: %q", got)
	}
	if !buffer.Truncated() {
		t.Fatal("overflow write was not marked truncated")
	}
}

func TestLimitedBufferEmptyWriteAtCapacityDoesNotTruncate(t *testing.T) {
	buffer := newLimitedBuffer(1)
	_, _ = buffer.Write([]byte("x"))
	if n, err := buffer.Write(nil); err != nil || n != 0 {
		t.Fatalf("empty write: n=%d err=%v", n, err)
	}
	if buffer.Truncated() {
		t.Fatal("empty write at capacity was marked truncated")
	}
}

func TestExecRunnerLongRunningZeroTimeoutRemainsUnbounded(t *testing.T) {
	result := (ExecRunner{DefaultTimeout: 1}).Run(context.Background(), Invocation{
		Command:     os.Args[0],
		Args:        []string{"-test.run=^TestExecRunnerHelperProcess$"},
		Env:         map[string]string{"AGENTSTACK_RUNNER_HELPER": "output"},
		LongRunning: true,
	})
	if result.Err != nil || result.ExitCode != 0 {
		t.Fatalf("long-running command inherited a zero/default timeout: %+v", result)
	}
}
