package security

import (
	"fmt"
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

func TestAuditPrivateSDDLResolvesLocalAdministratorAliasToCurrentUser(t *testing.T) {
	resolver := func(value string) (string, error) {
		switch strings.ToUpper(strings.TrimSpace(value)) {
		case "LA", testUserSID:
			return testUserSID, nil
		case "SY":
			return "S-1-5-18", nil
		case "BA":
			return "S-1-5-32-544", nil
		default:
			return "", fmt.Errorf("unknown SID %q", value)
		}
	}
	sddl := "D:PAI(A;OICI;FA;;;LA)(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	if err := auditPrivateSDDLWithResolver(sddl, testUserSID, resolver); err != nil {
		t.Fatalf("current user's canonical LA alias was rejected: %v", err)
	}
}

func TestAuditPrivateSDDLRejectsLocalAdministratorAliasForDifferentUser(t *testing.T) {
	resolver := func(value string) (string, error) {
		switch strings.ToUpper(strings.TrimSpace(value)) {
		case "LA":
			return "S-1-5-21-111-222-333-500", nil
		case testUserSID:
			return testUserSID, nil
		case "SY":
			return "S-1-5-18", nil
		case "BA":
			return "S-1-5-32-544", nil
		default:
			return "", fmt.Errorf("unknown SID %q", value)
		}
	}
	sddl := "D:PAI(A;OICI;FA;;;LA)(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	if err := auditPrivateSDDLWithResolver(sddl, testUserSID, resolver); err == nil {
		t.Fatal("LA alias for a different account was accepted as the current user")
	}
}
