package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/runner"
)

type Report struct {
	Added          []string `json:"added"`
	Preserved      []string `json:"preserved"`
	Skipped        []string `json:"skipped,omitempty"`
	Revision       string   `json:"revision,omitempty"`
	ManifestDigest string   `json:"manifestDigest,omitempty"`
}

type Installer struct {
	Commands runner.CommandRunner
	Targets  []string
}

func DefaultTargets() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".gemini", "skills"),
		filepath.Join(home, ".gemini", "antigravity-cli", "skills"),
	}
}

func (i Installer) Install(ctx context.Context, component model.Component) runner.Result {
	if component.Install.Kind != model.InstallSkillPack {
		err := fmt.Errorf("component %q is not a skill pack", component.ID)
		return runner.Result{ExitCode: -1, Err: err, Stderr: err.Error()}
	}
	if component.Install.Repository == "" || component.Install.RepositoryRevision == "" || component.Install.ManifestDigest == "" {
		err := fmt.Errorf("skill pack %q must pin repository, revision, and manifest digest", component.ID)
		return runner.Result{ExitCode: -1, Err: err, Stderr: err.Error()}
	}
	expectedCommit, ok := strings.CutPrefix(component.Install.ManifestDigest, "git-commit:")
	if !ok || len(expectedCommit) != 40 {
		err := fmt.Errorf("skill pack %q has invalid git commit manifest digest", component.ID)
		return runner.Result{ExitCode: -1, Err: err, Stderr: err.Error()}
	}
	commands := i.Commands
	if commands == nil {
		commands = runner.ExecRunner{}
	}
	targets := i.Targets
	if len(targets) == 0 {
		targets = DefaultTargets()
	}
	temp, err := os.MkdirTemp("", "agentstack-skills-*")
	if err != nil {
		return runner.Result{ExitCode: -1, Err: err, Stderr: err.Error()}
	}
	defer os.RemoveAll(temp)
	cloneDir := filepath.Join(temp, "repo")
	steps := [][]string{
		{"init", "--quiet", cloneDir},
		{"-C", cloneDir, "remote", "add", "origin", component.Install.Repository},
		{"-C", cloneDir, "fetch", "--depth", "1", "origin", component.Install.RepositoryRevision},
		{"-C", cloneDir, "checkout", "--detach", "--quiet", "FETCH_HEAD"},
	}
	for _, args := range steps {
		outcome := commands.Run(ctx, runner.Invocation{Command: "git", Args: args, Timeout: 2 * time.Minute})
		if outcome.Err != nil || outcome.ExitCode != 0 {
			if outcome.Err == nil {
				outcome.Err = fmt.Errorf("git exited with code %d", outcome.ExitCode)
			}
			return outcome
		}
	}
	resolved := commands.Run(ctx, runner.Invocation{Command: "git", Args: []string{"-C", cloneDir, "rev-parse", "HEAD"}, Timeout: 30 * time.Second})
	if resolved.Err != nil || resolved.ExitCode != 0 {
		return resolved
	}
	actualCommit := strings.TrimSpace(resolved.Stdout)
	if !strings.EqualFold(actualCommit, expectedCommit) {
		err := fmt.Errorf("skill pack revision mismatch: expected %s got %s", expectedCommit, actualCommit)
		return runner.Result{ExitCode: -1, Err: err, Stderr: err.Error()}
	}
	source := filepath.Join(cloneDir, "skills")
	if err := ValidateSkillInventory(source, component.Install.ExpectedEntries); err != nil {
		return runner.Result{ExitCode: -1, Err: err, Stderr: err.Error()}
	}
	report, err := CopyMissingSkills(source, targets)
	if err != nil {
		return runner.Result{ExitCode: -1, Err: err, Stderr: err.Error()}
	}
	report.Revision = actualCommit
	report.ManifestDigest = component.Install.ManifestDigest
	payload, _ := json.Marshal(report)
	return runner.Result{ExitCode: 0, Stdout: string(payload)}
}

func ValidateSkillInventory(source string, expected []string) error {
	if len(expected) == 0 {
		return fmt.Errorf("expected skill inventory is empty")
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return fmt.Errorf("read skill inventory: %w", err)
	}
	actual := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(source, entry.Name(), "SKILL.md")); err == nil {
			actual[entry.Name()] = true
		}
	}
	missing := []string{}
	for _, name := range expected {
		if !actual[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("skill inventory mismatch; missing audited entries: %s", strings.Join(missing, ", "))
	}
	return nil
}

func CopyMissingSkills(source string, targets []string) (Report, error) {
	entries, err := os.ReadDir(source)
	if err != nil {
		return Report{}, fmt.Errorf("read skill source: %w", err)
	}
	report := Report{}
	for _, target := range targets {
		if err := os.MkdirAll(target, 0o700); err != nil {
			return report, err
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillSource := filepath.Join(source, entry.Name())
		if _, err := os.Stat(filepath.Join(skillSource, "SKILL.md")); err != nil {
			report.Skipped = append(report.Skipped, skillSource)
			continue
		}
		for _, target := range targets {
			destination := filepath.Join(target, entry.Name())
			if _, err := os.Stat(destination); err == nil {
				report.Preserved = append(report.Preserved, destination)
				continue
			} else if !os.IsNotExist(err) {
				return report, err
			}
			if err := copyTreeAtomic(skillSource, destination); err != nil {
				return report, err
			}
			report.Added = append(report.Added, destination)
		}
	}
	return report, nil
}

func copyTreeAtomic(source, destination string) error {
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".agentstack-skill-*.tmp")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := copyTree(source, staging); err != nil {
		return err
	}
	info, err := os.Lstat(filepath.Join(staging, "SKILL.md"))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("staged skill is missing a regular SKILL.md")
	}
	if err := os.Rename(staging, destination); err != nil {
		if _, statErr := os.Stat(destination); statErr == nil {
			return fmt.Errorf("skill destination appeared during atomic install and was preserved: %s", destination)
		}
		return fmt.Errorf("atomically publish skill %s: %w", destination, err)
	}
	return nil
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return outputCloseErr
	})
}
