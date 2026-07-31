// Package redact provides a single privacy boundary for text and structured
// values that may contain credentials or authorization material.
package redact

import (
	"regexp"
	"strings"
)

const Replacement = "[REDACTED]"

var (
	authorizationPattern = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer\s+)?[^\s,;"}]+`)
	bearerPattern        = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
	jsonSecretPattern    = regexp.MustCompile(`(?i)("?(?:access[_-]?token|refresh[_-]?token|id[_-]?token|token|api[_-]?key|apikey|secret|password|credential|authorization)"?\s*:\s*")([^"]*)(")`)
	assignmentPattern    = regexp.MustCompile(`(?i)(\b(?:access[_-]?token|refresh[_-]?token|id[_-]?token|token|api[_-]?key|apikey|secret|password|credential|authorization)\s*=\s*)[^\s,;]+`)
	jwtPattern           = regexp.MustCompile(`\b[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{8,}\b`)
	providerTokenPattern = regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{16,})\b`)
)

// Text removes common credential representations while preserving surrounding
// diagnostic context.
func Text(value string) string {
	if value == "" {
		return ""
	}
	value = jsonSecretPattern.ReplaceAllString(value, `${1}`+Replacement+`${3}`)
	value = authorizationPattern.ReplaceAllString(value, `${1}`+Replacement)
	value = bearerPattern.ReplaceAllString(value, Replacement)
	value = assignmentPattern.ReplaceAllString(value, `${1}`+Replacement)
	value = jwtPattern.ReplaceAllString(value, Replacement)
	value = providerTokenPattern.ReplaceAllString(value, Replacement)
	return value
}

// Fields recursively redacts secret-bearing keys and string values.
func Fields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	result := make(map[string]any, len(fields))
	for key, value := range fields {
		if SensitiveKey(key) {
			result[key] = Replacement
			continue
		}
		result[key] = Value(value)
	}
	return result
}

// Value recursively redacts maps, slices, and text values.
func Value(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return Fields(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = Value(item)
		}
		return result
	case string:
		if Text(typed) != typed {
			return Replacement
		}
		return typed
	default:
		return value
	}
}

// SensitiveKey reports whether a structured field name denotes credential
// material.
func SensitiveKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
	for _, marker := range []string{"token", "secret", "password", "credential", "authorization", "apikey"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
