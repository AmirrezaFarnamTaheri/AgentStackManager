package processctl

import (
	"errors"
	"testing"
)

func TestLimitsValidation(t *testing.T) {
	if err := (Limits{MemoryBytes: 2 << 30, CPUPercent: 80, ActiveProcesses: 32}).Validate(); err != nil {
		t.Fatalf("valid limits rejected: %v", err)
	}
	for _, limits := range []Limits{
		{CPUPercent: 101},
		{CPUPercent: 0, MemoryBytes: 1},
		{ActiveProcesses: 10001},
	} {
		if err := limits.Validate(); err == nil {
			t.Fatalf("invalid limits accepted: %#v", limits)
		}
	}
}

func TestZeroLimitsAreDisabled(t *testing.T) {
	if !(Limits{}).Disabled() {
		t.Fatal("zero limits must be disabled")
	}
	if (Limits{MemoryBytes: 1}).Disabled() {
		t.Fatal("nonzero limits must be enabled")
	}
}

func TestUnsupportedLimitsErrorIsClassifiable(t *testing.T) {
	if !errors.Is(ErrResourceLimitsUnsupported, ErrResourceLimitsUnsupported) {
		t.Fatal("unsupported error must be classifiable")
	}
}
