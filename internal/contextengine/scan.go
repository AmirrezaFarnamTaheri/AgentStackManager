package contextengine

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/runner"
)

const maxScanFiles = 20000

var ignoredDirs = map[string]struct{}{
	".git": {}, "node_modules": {}, "vendor": {}, "dist": {}, "build": {}, "target": {}, ".next": {}, "coverage": {},
	"__pycache__": {}, ".pytest_cache": {}, ".mypy_cache": {}, ".ruff_cache": {}, ".agentstack": {},
}

var languageByExtension = map[string]string{
	".go": "go", ".rs": "rust", ".ts": "typescript", ".tsx": "typescript", ".js": "javascript", ".jsx": "javascript",
	".py": "python", ".java": "java", ".kt": "kotlin", ".swift": "swift", ".rb": "ruby", ".php": "php", ".cs": "csharp",
	".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".vue": "vue", ".svelte": "svelte", ".sh": "shell", ".ps1": "powershell",
}

var contextPaths = map[string]struct{}{
	"AGENTS.md": {}, "CLAUDE.md": {}, ".cursorrules": {}, ".github/copilot-instructions.md": {},
	".cursor/rules/agentstack-context.mdc": {}, ".opencode/rules/agentstack-context.md": {}, ".agentstack/context.md": {},
}

type Manager struct {
	Root        string
	Clock       func() time.Time
	Commands    runner.CommandRunner
	beforeApply func(RefreshOperation) error
}

func New(root string) Manager {
	return Manager{Root: root, Clock: func() time.Time { return time.Now().UTC() }}
}
func (m Manager) now() time.Time {
	if m.Clock == nil {
		return time.Now().UTC()
	}
	return m.Clock().UTC()
}

func (m Manager) Scan(root string) (Snapshot, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return Snapshot{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Snapshot{}, err
	}
	if !info.IsDir() {
		return Snapshot{}, fmt.Errorf("project root is not a directory")
	}
	snapshot := Snapshot{Root: absolute, GeneratedAt: m.now(), Languages: map[string]int{}, Commands: map[string]string{}}
	hash := sha256.New()
	topSet := map[string]struct{}{}
	manifestSet := map[string]struct{}{}
	configSet := map[string]struct{}{}
	mcpSet := map[string]struct{}{}
	count := 0
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == absolute {
			return nil
		}
		rel, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		parts := strings.Split(relSlash, "/")
		if entry.IsDir() {
			if _, ignored := ignoredDirs[entry.Name()]; ignored {
				return filepath.SkipDir
			}
			if len(parts) == 1 {
				topSet[parts[0]] = struct{}{}
			}
			return nil
		}
		if _, excluded := contextPaths[relSlash]; excluded {
			return nil
		}
		count++
		if count > maxScanFiles {
			snapshot.Truncated = true
			return filepath.SkipAll
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%d\n", relSlash, fileInfo.Size(), fileInfo.ModTime().UnixNano())
		if len(parts) == 1 {
			topSet[parts[0]] = struct{}{}
		}
		if language := languageByExtension[strings.ToLower(filepath.Ext(relSlash))]; language != "" {
			snapshot.Languages[language]++
			snapshot.SourceFiles++
			snapshot.SourceBytes += fileInfo.Size()
		}
		switch relSlash {
		case "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "requirements.txt", "pom.xml", "build.gradle", "build.gradle.kts", "Makefile":
			manifestSet[relSlash] = struct{}{}
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				_, _ = hash.Write(data)
				absorbManifest(relSlash, data, &snapshot)
			}
		case ".mcp.json", ".cursor/mcp.json", ".claude/settings.json", ".claude/settings.local.json", "opencode.json":
			mcpSet[relSlash] = struct{}{}
		}
		if isAgentConfig(relSlash) {
			configSet[relSlash] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	snapshot.Fingerprint = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	snapshot.TopLevel = sortedKeys(topSet)
	snapshot.Manifests = sortedKeys(manifestSet)
	snapshot.AgentConfigs = sortedKeys(configSet)
	snapshot.MCPConfigs = sortedKeys(mcpSet)
	snapshot.Frameworks = uniqueSorted(snapshot.Frameworks)
	return snapshot, nil
}

func absorbManifest(name string, data []byte, snapshot *Snapshot) {
	switch name {
	case "go.mod":
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			if fields := strings.Fields(scanner.Text()); len(fields) == 2 && fields[0] == "module" {
				snapshot.Module = fields[1]
				break
			}
		}
		snapshot.Commands["test"] = "go test ./..."
		snapshot.Commands["build"] = "go build ./..."
		snapshot.Commands["format"] = "gofmt -w ."
	case "package.json":
		var pkg struct {
			Name            string            `json:"name"`
			Scripts         map[string]string `json:"scripts"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			if snapshot.Module == "" {
				snapshot.Module = pkg.Name
			}
			manager := "npm"
			for _, pair := range []struct{ key, value string }{{"test", "test"}, {"build", "build"}, {"lint", "lint"}, {"format", "format"}, {"dev", "dev"}} {
				if _, ok := pkg.Scripts[pair.key]; ok {
					snapshot.Commands[pair.value] = manager + " run " + pair.key
				}
			}
			deps := map[string]string{}
			for key, value := range pkg.Dependencies {
				deps[key] = value
			}
			for key, value := range pkg.DevDependencies {
				deps[key] = value
			}
			for dep, framework := range map[string]string{"react": "React", "next": "Next.js", "vue": "Vue", "svelte": "Svelte", "@angular/core": "Angular", "vite": "Vite", "electron": "Electron", "@tauri-apps/api": "Tauri"} {
				if _, ok := deps[dep]; ok {
					snapshot.Frameworks = append(snapshot.Frameworks, framework)
				}
			}
		}
	case "Cargo.toml":
		if snapshot.Commands["test"] == "" {
			snapshot.Commands["test"] = "cargo test"
		}
		if snapshot.Commands["build"] == "" {
			snapshot.Commands["build"] = "cargo build"
		}
		snapshot.Commands["lint"] = "cargo clippy --all-targets --all-features -- -D warnings"
		snapshot.Commands["format"] = "cargo fmt --all -- --check"
	case "pyproject.toml", "requirements.txt":
		if snapshot.Commands["test"] == "" {
			snapshot.Commands["test"] = "pytest"
		}
	}
}

func isAgentConfig(rel string) bool {
	if _, ok := contextPaths[rel]; ok {
		return true
	}
	return strings.HasPrefix(rel, ".claude/rules/") || strings.HasPrefix(rel, ".cursor/rules/") || strings.HasPrefix(rel, ".agents/skills/") || strings.HasPrefix(rel, ".claude/skills/")
}

func sortedKeys(input map[string]struct{}) []string {
	result := make([]string, 0, len(input))
	for key := range input {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
func uniqueSorted(input []string) []string {
	set := map[string]struct{}{}
	for _, v := range input {
		if strings.TrimSpace(v) != "" {
			set[v] = struct{}{}
		}
	}
	return sortedKeys(set)
}
