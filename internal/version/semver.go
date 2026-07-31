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
	core := raw
	if index := strings.IndexAny(core, "-+"); index >= 0 {
		core = core[:index]
	}
	parts := strings.Split(core, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return Value{}, fmt.Errorf("invalid semantic version %q", input)
	}
	values := []int{0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		value, err := strconv.Atoi(parts[i])
		if err != nil {
			return Value{}, fmt.Errorf("invalid semantic version %q", input)
		}
		values[i] = value
	}
	return Value{Major: values[0], Minor: values[1], Patch: values[2], Raw: raw}, nil
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
	return 0, nil
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
