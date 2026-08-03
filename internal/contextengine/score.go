package contextengine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

var inlineCode = regexp.MustCompile("`([^`]+)`")

func (m Manager) Score(root string, targets []resourcehub.Agent) (ScoreResult, error) {
	snapshot, err := m.Scan(root)
	if err != nil {
		return ScoreResult{}, err
	}
	if len(targets) == 0 {
		targets = detectTargets(snapshot.Root)
	}
	targets = uniqueTargetAgents(targets)
	checks := []Check{}
	for _, target := range targets {
		path := contextPath(snapshot.Root, target)
		_, err := os.Stat(path)
		passed := err == nil
		checks = append(checks, Check{ID: "setup." + string(target), Category: "setup", Name: string(target) + " context exists", Earned: boolPoints(passed, 12), Maximum: 12, Passed: passed, Detail: relative(snapshot.Root, path), Suggestion: suggestion(passed, "run agentstack context plan/apply")})
	}
	configs := existingContextFiles(snapshot.Root, targets)
	combined := ""
	for _, path := range configs {
		if data, err := os.ReadFile(path); err == nil {
			combined += "\n" + string(data)
		}
	}
	hasTest := snapshot.Commands["test"] == "" || strings.Contains(combined, snapshot.Commands["test"])
	hasBuild := snapshot.Commands["build"] == "" || strings.Contains(combined, snapshot.Commands["build"])
	checks = append(checks, Check{ID: "quality.commands", Category: "quality", Name: "build and test commands are grounded", Earned: boolPoints(hasTest && hasBuild, 18), Maximum: 18, Passed: hasTest && hasBuild, Detail: fmt.Sprintf("test=%t build=%t", hasTest, hasBuild), Suggestion: suggestion(hasTest && hasBuild, "refresh generated context commands")})
	hasArchitecture := strings.Contains(combined, "## Architecture") || strings.Contains(combined, "## Project structure")
	checks = append(checks, Check{ID: "quality.architecture", Category: "quality", Name: "architecture summary present", Earned: boolPoints(hasArchitecture, 10), Maximum: 10, Passed: hasArchitecture, Detail: fmt.Sprintf("present=%t", hasArchitecture), Suggestion: suggestion(hasArchitecture, "add a factual architecture section")})
	concise := true
	for _, path := range configs {
		if info, err := os.Stat(path); err == nil && info.Size() > 20000 {
			concise = false
		}
	}
	checks = append(checks, Check{ID: "quality.budget", Category: "quality", Name: "context files stay within token budget", Earned: boolPoints(concise, 8), Maximum: 8, Passed: concise, Detail: "20 KiB per context file", Suggestion: suggestion(concise, "move detailed workflows into on-demand skills")})
	broken := brokenReferences(snapshot.Root, combined)
	grounded := len(broken) == 0
	checks = append(checks, Check{ID: "grounding.paths", Category: "grounding", Name: "referenced project paths exist", Earned: boolPoints(grounded, 20), Maximum: 20, Passed: grounded, Detail: strings.Join(broken, ", "), Suggestion: suggestion(grounded, "remove or correct stale path references")})
	fingerprintFresh := true
	if strings.Contains(combined, managedStart) && !strings.Contains(combined, snapshot.Fingerprint) {
		fingerprintFresh = false
	}
	checks = append(checks, Check{ID: "freshness.fingerprint", Category: "freshness", Name: "managed context matches project fingerprint", Earned: boolPoints(fingerprintFresh, 12), Maximum: 12, Passed: fingerprintFresh, Detail: snapshot.Fingerprint, Suggestion: suggestion(fingerprintFresh, "refresh managed context after project changes")})
	parity := len(targets) <= 1 || len(configs) == len(targets)
	checks = append(checks, Check{ID: "parity.targets", Category: "parity", Name: "selected agents have context parity", Earned: boolPoints(parity, 12), Maximum: 12, Passed: parity, Detail: fmt.Sprintf("%d/%d targets", len(configs), len(targets)), Suggestion: suggestion(parity, "refresh all selected agent targets")})
	max, earned := 0, 0
	for _, check := range checks {
		max += check.Maximum
		earned += check.Earned
	}
	score := 0
	if max > 0 {
		score = (earned*100 + max/2) / max
	}
	return ScoreResult{Score: score, Grade: grade(score), GeneratedAt: m.now(), Targets: targets, Checks: checks, Snapshot: snapshot}, nil
}

func detectTargets(root string) []resourcehub.Agent {
	var targets []resourcehub.Agent
	for _, agent := range []resourcehub.Agent{resourcehub.AgentCodex, resourcehub.AgentClaude, resourcehub.AgentCursor, resourcehub.AgentOpenCode, resourcehub.AgentCopilot} {
		if _, err := os.Stat(contextPath(root, agent)); err == nil {
			targets = append(targets, agent)
		}
	}
	if len(targets) == 0 {
		targets = []resourcehub.Agent{resourcehub.AgentCodex}
	}
	return targets
}
func uniqueTargetAgents(input []resourcehub.Agent) []resourcehub.Agent {
	set := map[resourcehub.Agent]struct{}{}
	for _, v := range input {
		set[v] = struct{}{}
	}
	result := make([]resourcehub.Agent, 0, len(set))
	for v := range set {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
func contextPath(root string, agent resourcehub.Agent) string {
	switch agent {
	case resourcehub.AgentCodex, resourcehub.AgentOpenCode:
		return filepath.Join(root, "AGENTS.md")
	case resourcehub.AgentClaude:
		return filepath.Join(root, "CLAUDE.md")
	case resourcehub.AgentCursor:
		return filepath.Join(root, ".cursor", "rules", "agentstack-context.mdc")
	case resourcehub.AgentCopilot:
		return filepath.Join(root, ".github", "copilot-instructions.md")
	default:
		return filepath.Join(root, ".agentstack", "context.md")
	}
}
func existingContextFiles(root string, targets []resourcehub.Agent) []string {
	seen := map[string]struct{}{}
	var paths []string
	for _, target := range targets {
		path := contextPath(root, target)
		if _, ok := seen[path]; ok {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}
func brokenReferences(root, content string) []string {
	set := map[string]struct{}{}
	for _, match := range inlineCode.FindAllStringSubmatch(content, -1) {
		value := strings.TrimSpace(match[1])
		if value == "" || strings.ContainsAny(value, " \t\n") || (!strings.Contains(value, "/") && !strings.Contains(value, "\\")) {
			continue
		}
		if strings.Contains(value, "...") || strings.HasPrefix(value, "http") || filepath.IsAbs(value) {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(value))
		if strings.HasPrefix(clean, "..") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, clean)); errorsIsNotExist(err) {
			set[value] = struct{}{}
		}
	}
	result := sortedKeys(set)
	return result
}
func errorsIsNotExist(err error) bool { return err != nil && os.IsNotExist(err) }
func boolPoints(value bool, points int) int {
	if value {
		return points
	}
	return 0
}
func suggestion(passed bool, value string) string {
	if passed {
		return ""
	}
	return value
}
func grade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 80:
		return "B"
	case score >= 70:
		return "C"
	case score >= 60:
		return "D"
	default:
		return "F"
	}
}
func relative(root, path string) string {
	value, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(value)
}
