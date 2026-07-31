package pathenv

import (
	"path"
	"strings"
)

// MergeWindows combines semicolon-separated Windows PATH values while
// preserving first-seen spelling and order and removing equivalent segments.
func MergeWindows(values ...string) string {
	seen := map[string]struct{}{}
	parts := make([]string, 0)
	for _, value := range values {
		for _, raw := range strings.Split(value, ";") {
			segment := strings.TrimSpace(raw)
			if segment == "" {
				continue
			}
			key := normalizeWindows(segment)
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			parts = append(parts, segment)
		}
	}
	return strings.Join(parts, ";")
}

// AppendWindows appends target only when no equivalent Windows PATH segment
// already exists.
func AppendWindows(current, target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return current, false
	}
	targetKey := normalizeWindows(target)
	for _, raw := range strings.Split(current, ";") {
		if normalizeWindows(raw) == targetKey {
			return current, false
		}
	}
	if strings.TrimSpace(current) == "" {
		return target, true
	}
	// Preserve unrelated PATH spelling, ordering, and duplicates. The
	// self-installer owns only the one segment it appends.
	if strings.HasSuffix(current, ";") {
		return current + target, true
	}
	return current + ";" + target, true
}

func normalizeWindows(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"`))
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `\`, "/")
	lowerClean := func(prefix, candidate string) string {
		return prefix + strings.ToLower(path.Clean(candidate))
	}

	// Keep namespace categories in the comparison key. Windows treats UNC,
	// rooted local, drive-absolute, and drive-relative paths differently even
	// when a generic slash cleaner would collapse them to the same text.
	if strings.HasPrefix(value, "//") {
		rest := strings.TrimLeft(value, "/")
		return lowerClean("unc:", "/"+rest)
	}
	if len(value) >= 2 && value[1] == ':' {
		drive := strings.ToLower(value[:2])
		rest := value[2:]
		if strings.HasPrefix(rest, "/") {
			rest = "/" + strings.TrimLeft(rest, "/")
			return lowerClean("drive-absolute:"+drive, rest)
		}
		return lowerClean("drive-relative:"+drive+":", rest)
	}
	if strings.HasPrefix(value, "/") {
		return lowerClean("rooted:", "/"+strings.TrimLeft(value, "/"))
	}
	return lowerClean("relative:", value)
}
