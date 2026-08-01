//go:build windows

package winsecurity

import "testing"

func TestDACLFromSDDLBuildsProtectedNativeACL(t *testing.T) {
	dacl, err := DACLFromSDDL("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		t.Fatalf("build protected DACL from SDDL: %v", err)
	}
	if !dacl.Protected {
		t.Fatal("converted DACL is not protected")
	}
	if len(dacl.ACL) < aclHeaderSize {
		t.Fatalf("converted DACL is too short: %d", len(dacl.ACL))
	}
	if dacl.SDDL == "" {
		t.Fatal("converted DACL has no diagnostic SDDL")
	}
}
