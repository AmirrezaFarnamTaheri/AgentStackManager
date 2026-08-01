package security

import (
	"fmt"
	"regexp"
	"strings"
)

var sddlACEPattern = regexp.MustCompile(`\(([^()]*)\)`)

type sidResolver func(string) (string, error)

func auditPrivateSDDL(sddl, currentUserSID string) error {
	return auditPrivateSDDLWithResolver(sddl, currentUserSID, staticSIDResolver)
}

func auditPrivateSDDLWithResolver(sddl, currentUserSID string, resolve sidResolver) error {
	sddl = strings.TrimSpace(sddl)
	currentUserSID = strings.TrimSpace(currentUserSID)
	if currentUserSID == "" {
		return fmt.Errorf("current user SID is empty")
	}
	currentUserSID, err := resolve(currentUserSID)
	if err != nil {
		return fmt.Errorf("resolve current user SID: %w", err)
	}
	systemSID, err := resolve("SY")
	if err != nil {
		return fmt.Errorf("resolve system SID: %w", err)
	}
	administratorsSID, err := resolve("BA")
	if err != nil {
		return fmt.Errorf("resolve administrators SID: %w", err)
	}
	allowed := map[string]string{
		currentUserSID:    "user",
		systemSID:         "system",
		administratorsSID: "administrators",
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
		trustee := strings.TrimSpace(fields[len(fields)-1])
		resolvedTrustee, err := resolve(trustee)
		if err != nil {
			return fmt.Errorf("Windows DACL contains unexpected principal %q", trustee)
		}
		principal, ok := allowed[resolvedTrustee]
		if !ok {
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

func staticSIDResolver(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "SY":
		return "S-1-5-18", nil
	case "BA":
		return "S-1-5-32-544", nil
	}
	if strings.HasPrefix(value, "S-1-") {
		return value, nil
	}
	return "", fmt.Errorf("unsupported SID alias %q", value)
}
