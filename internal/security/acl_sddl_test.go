package security

import (
	"strings"
	"testing"
)

const testUserSID = "S-1-5-21-111-222-333-1001"

func TestAuditPrivateSDDLAcceptsOnlyRequiredFullControlPrincipals(t *testing.T) {
	sddl := "O:" + testUserSID + "G:SYD:PAI(A;OICI;FA;;;" + testUserSID + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	if err := auditPrivateSDDL(sddl, testUserSID); err != nil {
		t.Fatalf("valid private DACL rejected: %v", err)
	}
}

func TestAuditPrivateSDDLRejectsUnexpectedPrincipal(t *testing.T) {
	sddl := "D:PAI(A;OICI;FA;;;" + testUserSID + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FR;;;BU)"
	err := auditPrivateSDDL(sddl, testUserSID)
	if err == nil || !strings.Contains(err.Error(), "unexpected principal") {
		t.Fatalf("unexpected principal was accepted: %v", err)
	}
}

func TestAuditPrivateSDDLRejectsMissingOrWeakRequiredACE(t *testing.T) {
	cases := []string{
		"D:PAI(A;OICI;FA;;;" + testUserSID + ")(A;OICI;FA;;;SY)",
		"D:PAI(A;OICI;FR;;;" + testUserSID + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
		"D:PAI(D;OICI;FA;;;" + testUserSID + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)",
	}
	for _, sddl := range cases {
		if err := auditPrivateSDDL(sddl, testUserSID); err == nil {
			t.Fatalf("unsafe DACL accepted: %s", sddl)
		}
	}
}
