package similarity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// NormalizeBody strips YAML frontmatter delimiters, normalizes line endings (\r\n -> \n),
// trims trailing whitespace from lines, and removes leading/trailing empty lines.
// Crucially, it preserves exact code blocks, URLs, command-line flags, and logical negations.
func NormalizeBody(raw string) string {
	content := raw

	// Strip YAML frontmatter if present at the start of the file
	if strings.HasPrefix(content, "---") {
		// Find second "---" delimiter
		rest := content[3:]
		if idx := strings.Index(rest, "\n---"); idx != -1 {
			content = rest[idx+4:]
		} else if idx := strings.Index(rest, "\r\n---"); idx != -1 {
			content = rest[idx+6:]
		}
	}

	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	lines := strings.Split(content, "\n")
	cleanedLines := make([]string, 0, len(lines))

	for _, line := range lines {
		// Trim right whitespace per line
		trimmed := strings.TrimRight(line, " \t")
		cleanedLines = append(cleanedLines, trimmed)
	}

	// Trim leading empty lines
	for len(cleanedLines) > 0 && cleanedLines[0] == "" {
		cleanedLines = cleanedLines[1:]
	}
	// Trim trailing empty lines
	for len(cleanedLines) > 0 && cleanedLines[len(cleanedLines)-1] == "" {
		cleanedLines = cleanedLines[:len(cleanedLines)-1]
	}

	return strings.Join(cleanedLines, "\n")
}

// ComputeNormalizedDigest computes the SHA-256 digest of the normalized body with sha256: prefix.
func ComputeNormalizedDigest(raw string) string {
	norm := NormalizeBody(raw)
	h := sha256.Sum256([]byte(norm))
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))
}
