package contextengine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agentstack/agentstack/internal/runner"
)

const (
	maxContextReadBytes   = 1 << 20
	maxContextSearchFiles = 10000
	maxContextSearchBytes = 512 << 10
)

type FileView struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
}

type SearchMatch struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

type SearchResult struct {
	Root         string        `json:"root"`
	Query        string        `json:"query"`
	Matches      []SearchMatch `json:"matches"`
	FilesScanned int           `json:"filesScanned"`
	Truncated    bool          `json:"truncated,omitempty"`
}

type GitContext struct {
	Root       string `json:"root"`
	Branch     string `json:"branch,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Status     string `json:"status,omitempty"`
	DiffStat   string `json:"diffStat,omitempty"`
	Repository bool   `json:"repository"`
}

func (m Manager) ReadFile(root, relative string) (FileView, error) {
	absoluteRoot, target, err := confinedPath(root, relative)
	if err != nil {
		return FileView{}, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return FileView{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return FileView{}, fmt.Errorf("context path must be a regular file")
	}
	file, err := os.Open(target)
	if err != nil {
		return FileView{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxContextReadBytes+1))
	if err != nil {
		return FileView{}, err
	}
	truncated := len(data) > maxContextReadBytes
	if truncated {
		data = utf8Prefix(data, maxContextReadBytes)
	}
	rel, err := filepath.Rel(absoluteRoot, target)
	if err != nil {
		return FileView{}, fmt.Errorf("resolve context path: %w", err)
	}
	return FileView{Path: filepath.ToSlash(rel), Content: string(data), Bytes: len(data), Truncated: truncated}, nil
}

func (m Manager) Search(root, query string, limit int) (SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return SearchResult{}, fmt.Errorf("context search query is required")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	absolute, err := canonicalProjectRoot(root)
	if err != nil {
		return SearchResult{}, err
	}
	needle := strings.ToLower(query)
	result := SearchResult{Root: absolute, Query: query}
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == absolute {
			return nil
		}
		if entry.IsDir() {
			if _, ignored := ignoredDirs[entry.Name()]; ignored {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		result.FilesScanned++
		if result.FilesScanned > maxContextSearchFiles {
			result.Truncated = true
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(absolute, path)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(filepath.ToSlash(rel)), needle) {
			result.Matches = append(result.Matches, SearchMatch{Path: filepath.ToSlash(rel), Line: 0, Snippet: "file name match"})
			if len(result.Matches) >= limit {
				result.Truncated = true
				return filepath.SkipAll
			}
		}
		fileInfo, err := entry.Info()
		if err != nil || fileInfo.Size() > maxContextSearchBytes {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		scanner := bufio.NewScanner(io.LimitReader(file, maxContextSearchBytes))
		scanner.Buffer(make([]byte, 64*1024), maxContextSearchBytes)
		line := 0
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if strings.Contains(strings.ToLower(text), needle) {
				result.Matches = append(result.Matches, SearchMatch{Path: filepath.ToSlash(rel), Line: line, Snippet: boundedSnippet(text, 240)})
				if len(result.Matches) >= limit {
					result.Truncated = true
					break
				}
			}
		}
		_ = file.Close()
		if len(result.Matches) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return SearchResult{}, err
	}
	sort.SliceStable(result.Matches, func(i, j int) bool {
		if result.Matches[i].Path == result.Matches[j].Path {
			return result.Matches[i].Line < result.Matches[j].Line
		}
		return result.Matches[i].Path < result.Matches[j].Path
	})
	return result, nil
}

func (m Manager) Git(ctx context.Context, root string) (GitContext, error) {
	absolute, err := canonicalProjectRoot(root)
	if err != nil {
		return GitContext{}, err
	}
	run := func(args ...string) runner.Result {
		commands := m.Commands
		if commands == nil {
			commands = runner.ExecRunner{}
		}
		return commands.Run(ctx, runner.Invocation{Command: "git", Args: append([]string{"-C", absolute}, args...), Timeout: 10 * time.Second, MaxOutputBytes: 256 << 10})
	}
	inside := run("rev-parse", "--is-inside-work-tree")
	if inside.Err != nil || strings.TrimSpace(inside.Stdout) != "true" {
		return GitContext{Root: absolute, Repository: false}, nil
	}
	branch := run("rev-parse", "--abbrev-ref", "HEAD")
	revision := run("rev-parse", "HEAD")
	status := run("status", "--short")
	diff := run("diff", "--stat", "HEAD")
	for _, result := range []runner.Result{branch, revision, status, diff} {
		if result.Err != nil {
			return GitContext{}, result.Err
		}
	}
	return GitContext{Root: absolute, Repository: true, Branch: strings.TrimSpace(branch.Stdout), Revision: strings.TrimSpace(revision.Stdout), Status: strings.TrimSpace(status.Stdout), DiffStat: strings.TrimSpace(diff.Stdout)}, nil
}

func confinedPath(root, relative string) (string, string, error) {
	absoluteRoot, err := canonicalProjectRoot(root)
	if err != nil {
		return "", "", err
	}
	if filepath.IsAbs(relative) || strings.Contains(relative, "\\") {
		return "", "", fmt.Errorf("context path must be a forward-slash relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("context path escapes project root")
	}
	target := filepath.Join(absoluteRoot, clean)
	rel, err := filepath.Rel(absoluteRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("context path escapes project root")
	}
	parent := filepath.Dir(target)
	for parent != filepath.Dir(absoluteRoot) && parent != absoluteRoot {
		info, statErr := os.Lstat(parent)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("context path traverses a symlink")
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", "", statErr
		}
		parent = filepath.Dir(parent)
	}
	return absoluteRoot, target, nil
}

func canonicalProjectRoot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root is not a directory")
	}
	return resolved, nil
}

func boundedSnippet(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 3 {
		return string(runes[:max(limit, 0)])
	}
	return string(runes[:limit-3]) + "..."
}

func utf8Prefix(data []byte, limit int) []byte {
	if limit < 0 {
		limit = 0
	}
	if len(data) <= limit {
		return data
	}
	cut := limit
	for cut > 0 && cut < len(data) && !utf8.RuneStart(data[cut]) {
		cut--
	}
	return data[:cut]
}
