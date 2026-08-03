package resourcehub

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	maxAuditFileBytes  int64 = 1 << 20
	maxAuditTotalBytes int64 = 64 << 20
	maxAuditFiles            = 10_000
)

var auditRules = []struct {
	ID       string
	Category string
	Severity Severity
	Pattern  *regexp.Regexp
	Message  string
}{
	{"prompt-injection", "injection", SeverityCritical, regexp.MustCompile(`(?i)\b(ignore|override|disregard)\b.{0,40}\b(previous|prior|system|developer)\b.{0,30}\b(instruction|prompt|message)s?\b`), "instruction attempts to override governing prompts"},
	{"secret-exfiltration", "exfiltration", SeverityCritical, regexp.MustCompile(`(?i)\b(curl|wget|invoke-webrequest|fetch|requests?\.post)\b.{0,120}(api[_-]?key|token|secret|password|credential|\$[A-Z][A-Z0-9_]+)`), "network command appears to transmit a secret or environment value"},
	{"credential-reference", "credential", SeverityHigh, regexp.MustCompile(`(?i)(OPENAI_API_KEY|ANTHROPIC_API_KEY|AWS_SECRET_ACCESS_KEY|PRIVATE_KEY|PASSWORD\s*=|TOKEN\s*=)`), "credential-like material or sensitive environment reference"},
	{"destructive-shell", "privilege", SeverityHigh, regexp.MustCompile(`(?i)(rm\s+-rf\s+[~/]|remove-item\s+.+-recurse.+-force|format\s+[a-z]:|del\s+/[sq])`), "destructive filesystem command"},
	{"shell-pipe-exec", "privilege", SeverityHigh, regexp.MustCompile(`(?i)(curl|wget|irm|invoke-webrequest).{0,120}(\|\s*(sh|bash|zsh|iex)|invoke-expression)`), "remote content is piped directly to a shell"},
	{"policy-bypass", "injection", SeverityHigh, regexp.MustCompile(`(?i)\b(bypass|disable|evade)\b.{0,40}\b(safety|policy|guardrail|approval|permission)s?\b`), "content requests bypass of safeguards"},
	{"hidden-instruction", "obfuscation", SeverityMedium, regexp.MustCompile(`(?i)<!--.{0,200}(ignore|system prompt|developer message).{0,200}-->`), "hidden HTML comment contains instruction-like content"},
}

var textExtensions = map[string]struct{}{
	".md": {}, ".txt": {}, ".json": {}, ".yaml": {}, ".yml": {}, ".toml": {}, ".ini": {}, ".cfg": {},
	".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {}, ".py": {}, ".go": {}, ".rs": {}, ".sh": {}, ".ps1": {}, ".html": {}, ".css": {},
}

func (m Manager) Audit(id string) (AuditResult, error) {
	registry, err := m.LoadRegistry()
	if err != nil {
		return AuditResult{}, err
	}
	resource, ok := registry.Resources[id]
	if !ok {
		return AuditResult{}, fmt.Errorf("unknown resource %q", id)
	}
	result := AuditResult{ResourceID: id, ScannedAt: m.now()}
	root := m.resourceSource(resource)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("resource contains symlink: %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxAuditFileBytes {
			result.FilesSkipped++
			return nil
		}
		if _, ok := textExtensions[strings.ToLower(filepath.Ext(path))]; !ok {
			result.FilesSkipped++
			return nil
		}
		if result.FilesScanned >= maxAuditFiles {
			return fmt.Errorf("resource audit exceeds %d files", maxAuditFiles)
		}
		if result.BytesScanned+info.Size() > maxAuditTotalBytes {
			return fmt.Errorf("resource audit exceeds %d total bytes", maxAuditTotalBytes)
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxAuditFileBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(data)) > maxAuditFileBytes {
			return fmt.Errorf("resource audit file grew beyond %d bytes: %s", maxAuditFileBytes, path)
		}
		result.FilesScanned++
		result.BytesScanned += int64(len(data))
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve audited resource path: %w", err)
		}
		scanAuditText(filepath.ToSlash(rel), string(data), &result)
		return nil
	})
	if err != nil {
		return AuditResult{}, err
	}
	sort.Slice(result.Findings, func(i, j int) bool {
		if severityRank(result.Findings[i].Severity) != severityRank(result.Findings[j].Severity) {
			return severityRank(result.Findings[i].Severity) < severityRank(result.Findings[j].Severity)
		}
		if result.Findings[i].File != result.Findings[j].File {
			return result.Findings[i].File < result.Findings[j].File
		}
		return result.Findings[i].Line < result.Findings[j].Line
	})
	for _, finding := range result.Findings {
		result.RiskScore += severityWeight(finding.Severity)
	}
	if result.RiskScore > 100 {
		result.RiskScore = 100
	}
	result.RiskLabel = riskLabel(result.RiskScore, result.Findings)
	result.Blocked = result.RiskLabel == "critical"
	return result, nil
}

func scanAuditText(file, content string, result *AuditResult) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, int(maxAuditFileBytes))
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		for _, rule := range auditRules {
			if rule.Pattern.MatchString(text) {
				result.Findings = append(result.Findings, Finding{Severity: rule.Severity, Category: rule.Category, RuleID: rule.ID, File: file, Line: line, Message: rule.Message, Snippet: trimSnippet(text)})
			}
		}
		for _, r := range text {
			if unicode.Is(unicode.Cf, r) && r != '\u200d' {
				result.Findings = append(result.Findings, Finding{Severity: SeverityMedium, Category: "obfuscation", RuleID: "invisible-unicode", File: file, Line: line, Message: "invisible Unicode control character", Snippet: trimSnippet(text)})
				break
			}
		}
	}
}

func trimSnippet(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 240 {
		return string(runes[:240]) + "..."
	}
	return value
}

func severityRank(value Severity) int {
	switch value {
	case SeverityCritical:
		return 0
	case SeverityHigh:
		return 1
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 3
	default:
		return 4
	}
}
func severityWeight(value Severity) int {
	switch value {
	case SeverityCritical:
		return 30
	case SeverityHigh:
		return 18
	case SeverityMedium:
		return 8
	case SeverityLow:
		return 3
	default:
		return 1
	}
}
func riskLabel(score int, findings []Finding) string {
	for _, finding := range findings {
		if finding.Severity == SeverityCritical {
			return "critical"
		}
	}
	switch {
	case score == 0:
		return "clean"
	case score <= 20:
		return "low"
	case score <= 45:
		return "medium"
	case score <= 75:
		return "high"
	default:
		return "critical"
	}
}
