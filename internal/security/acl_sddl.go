package security

import (
	"fmt"
	"regexp"
	"strings"
)

var sddlACEPattern = regexp.MustCompile(`\(([^()]*)\)`)

func auditPrivateSDDL(sddl, currentUserSID string) error {
	sddl = strings.TrimSpace(sddl)
	currentUserSID = strings.ToUpper(strings.TrimSpace(currentUserSID))
	if currentUserSID == "" {
		return fmt.Errorf("current user SID is empty")
	}
	daclIndex := strings.Index(strings.ToUpper(sddl), "D:")
	if daclIndex < 0 {
		return fmt.Errorf("Windows security descriptor has no DACL")
	}
	dacl := sddl[daclIndex+2:]
	if saclIndex := strings.Index(strings.ToUpper(dacl), "S:"); saclIndex >= 0 {
		dacl = dacl[:saclIndex]
	}
	firstACE := strings.Index(dacl, "(")
	if firstACE < 0 {
		return fmt.Errorf("Windows DACL has no access-control entries")
	}
	if !strings.Contains(strings.ToUpper(dacl[:firstACE]), "P") {
		return fmt.Errorf("Windows DACL inheritance is not protected")
	}
	required := map[string]bool{"user": false, "system": false, "administrators": false}
	matches := sddlACEPattern.FindAllStringSubmatch(dacl, -1)
	for _, match := range matches {
		fields := strings.Split(match[1], ";")
		if len(fields) < 6 {
			return fmt.Errorf("malformed Windows DACL entry %q", match[0])
		}
		aceType := strings.ToUpper(strings.TrimSpace(fields[0]))
		flags := strings.ToUpper(strings.TrimSpace(fields[1]))
		rights := strings.ToUpper(strings.TrimSpace(fields[2]))
		trustee := strings.ToUpper(strings.TrimSpace(fields[len(fields)-1]))
		principal := ""
		switch trustee {
		case currentUserSID:
			principal = "user"
		case "SY", "S-1-5-18":
			principal = "system"
		case "BA", "S-1-5-32-544":
			principal = "administrators"
		default:
			return fmt.Errorf("Windows DACL contains unexpected principal %q", trustee)
		}
		if aceType != "A" {
			return fmt.Errorf("Windows DACL contains non-allow ACE for %s", principal)
		}
		if !strings.Contains(flags, "OI") || !strings.Contains(flags, "CI") {
			return fmt.Errorf("Windows DACL entry for %s does not inherit to children", principal)
		}
		if !strings.Contains(rights, "FA") {
			return fmt.Errorf("Windows DACL entry for %s does not grant full control", principal)
		}
		required[principal] = true
	}
	for principal, present := range required {
		if !present {
			return fmt.Errorf("Windows DACL is missing required %s principal", principal)
		}
	}
	return nil
}
