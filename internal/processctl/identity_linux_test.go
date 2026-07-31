//go:build linux

package processctl

import (
	"os"
	"testing"
)

func TestPlatformProcessIdentityIsStableForCurrentProcess(t *testing.T) {
	first, err := platformProcessIdentity(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	second, err := platformProcessIdentity(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second {
		t.Fatalf("unstable process identity: %q then %q", first, second)
	}
}
