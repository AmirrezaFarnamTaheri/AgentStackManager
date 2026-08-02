package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var semverPattern = regexp.MustCompile(`(?i)(?:^|[^0-9])v?([0-9]+(?:\.[0-9]+){0,3}(?:[-+][0-9A-Za-z.-]+)?)`)

type Value struct {
	Major int
	Minor int
	Patch int
	Raw   string

	prerelease []string
}

func Extract(input, pattern string) (string, error) {
	if pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("compile version pattern: %w", err)
		}
		match := re.FindStringSubmatch(input)
		if len(match) < 2 {
			return "", fmt.Errorf("version output did not match configured pattern")
		}
		return strings.TrimSpace(match[1]), nil
	}
	match := semverPattern.FindStringSubmatch(input)
	if len(match) < 2 {
		return "", fmt.Errorf("no semantic version found in %q", strings.TrimSpace(input))
	}
	return match[1], nil
}

func Parse(input string) (Value, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(input, "v"))
	versionAndBuild := strings.SplitN(raw, "+", 2)
	if !validIdentifiers(versionAndBuild[1:], false) {
		return Value{}, fmt.Errorf("invalid semantic version %q", input)
	}
	coreAndPrerelease := strings.SplitN(versionAndBuild[0], "-", 2)
	var prerelease []string
	if len(coreAndPrerelease) == 2 {
		prerelease = strings.Split(coreAndPrerelease[1], ".")
		if !validIdentifiers(prerelease, true) {
			return Value{}, fmt.Errorf("invalid semantic version %q", input)
		}
	}
	parts := strings.Split(coreAndPrerelease[0], ".")
	if len(parts) != 3 {
		return Value{}, fmt.Errorf("invalid semantic version %q", input)
	}
	values := []int{0, 0, 0}
	for i := 0; i < len(parts); i++ {
		if parts[i] == "" || strings.IndexFunc(parts[i], func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return Value{}, fmt.Errorf("invalid semantic version %q", input)
		}
		value, err := strconv.Atoi(parts[i])
		if err != nil {
			return Value{}, fmt.Errorf("invalid semantic version %q", input)
		}
		values[i] = value
	}
	return Value{Major: values[0], Minor: values[1], Patch: values[2], Raw: raw, prerelease: prerelease}, nil
}

func Compare(left, right string) (int, error) {
	l, err := Parse(left)
	if err != nil {
		return 0, err
	}
	r, err := Parse(right)
	if err != nil {
		return 0, err
	}
	for _, pair := range [][2]int{{l.Major, r.Major}, {l.Minor, r.Minor}, {l.Patch, r.Patch}} {
		if pair[0] < pair[1] {
			return -1, nil
		}
		if pair[0] > pair[1] {
			return 1, nil
		}
	}
	return comparePrerelease(l.prerelease, r.prerelease), nil
}

func validIdentifiers(groups []string, rejectLeadingZeroes bool) bool {
	for _, group := range groups {
		for _, identifier := range strings.Split(group, ".") {
			if identifier == "" || strings.IndexFunc(identifier, func(r rune) bool {
				return (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '-'
			}) >= 0 {
				return false
			}
			if rejectLeadingZeroes && len(identifier) > 1 && identifier[0] == '0' && isNumeric(identifier) {
				return false
			}
		}
	}
	return true
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		if comparison := comparePrereleaseIdentifier(left[index], right[index]); comparison != 0 {
			return comparison
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumeric, rightNumeric := isNumeric(left), isNumeric(right)
	if leftNumeric && !rightNumeric {
		return -1
	}
	if !leftNumeric && rightNumeric {
		return 1
	}
	if leftNumeric {
		if len(left) < len(right) {
			return -1
		}
		if len(left) > len(right) {
			return 1
		}
	}
	return strings.Compare(left, right)
}

func isNumeric(value string) bool {
	return value != "" && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

func Compatible(value, minimum, maximum string) (bool, error) {
	if minimum != "" {
		comparison, err := Compare(value, minimum)
		if err != nil {
			return false, err
		}
		if comparison < 0 {
			return false, nil
		}
	}
	if maximum != "" {
		comparison, err := Compare(value, maximum)
		if err != nil {
			return false, err
		}
		if comparison > 0 {
			return false, nil
		}
	}
	return true, nil
}
