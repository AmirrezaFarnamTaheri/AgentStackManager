package selfinstall

import (
	"path/filepath"
	"strings"
)

func AppendPathSegment(current, target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return current, false
	}
	normalizedTarget := normalizePath(target)
	parts := strings.Split(current, ";")
	for _, part := range parts {
		if normalizePath(part) == normalizedTarget {
			return current, false
		}
	}
	if strings.TrimSpace(current) == "" {
		return target, true
	}
	return strings.TrimRight(current, ";") + ";" + target, true
}

func normalizePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, `\/`)
	value = filepath.Clean(value)
	return strings.ToLower(value)
}
