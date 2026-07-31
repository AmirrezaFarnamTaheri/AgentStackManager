package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentstack/agentstack/internal/runner"
	"testing"

	"github.com/agentstack/agentstack/internal/model"
)

func TestCopyMissingSkillsPreservesExistingDirectories(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(filepath.Join(source, "review"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "debug"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "review", "SKILL.md"), []byte("new review"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "debug", "SKILL.md"), []byte("debug"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "review"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "review", "SKILL.md"), []byte("existing review"), 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := CopyMissingSkills(source, []string{target})
	if err != nil {
		t.Fatal(err)
	}
	existing, _ := os.ReadFile(filepath.Join(target, "review", "SKILL.md"))
	if string(existing) != "existing review" {
		t.Fatal("existing skill was overwritten")
	}
	if _, err := os.Stat(filepath.Join(target, "debug", "SKILL.md")); err != nil {
		t.Fatal("missing skill was not copied")
	}
	if len(report.Added) != 1 || len(report.Preserved) != 1 {
		t.Fatalf("unexpected report %#v", report)
	}
}

func TestCopyMissingSkillsPublishesCompleteSkillWithoutStagingResidue(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source")
	target := filepath.Join(t.TempDir(), "target")
	skill := filepath.Join(source, "atomic-skill")
	if err := os.MkdirAll(filepath.Join(skill, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# Atomic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "nested", "data.txt"), []byte("complete"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := CopyMissingSkills(source, []string{target})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(target, "atomic-skill")
	if len(report.Added) != 1 || report.Added[0] != destination {
		t.Fatalf("unexpected report: %+v", report)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "nested", "data.txt")); err != nil || string(data) != "complete" {
		t.Fatalf("atomic skill destination incomplete: data=%q err=%v", data, err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".agentstack-skill-") {
			t.Fatalf("staging residue remained: %s", entry.Name())
		}
	}
}

func TestInstallerRejectsNonSkillPackComponent(t *testing.T) {
	installer := Installer{}
	result := installer.Install(context.Background(), model.Component{ID: "git", Install: model.InstallSpec{Kind: model.InstallWinget}})
	if result.Err == nil {
		t.Fatal("expected error")
	}
}

type skillCommandRunner struct {
	calls []runner.Invocation
	sha   string
}

func (r *skillCommandRunner) Run(_ context.Context, inv runner.Invocation) runner.Result {
	r.calls = append(r.calls, inv)
	if len(inv.Args) >= 4 && inv.Args[len(inv.Args)-2] == "rev-parse" {
		return runner.Result{ExitCode: 0, Stdout: r.sha + "\n"}
	}
	return runner.Result{ExitCode: 0}
}

func TestInstallerRequiresPinnedRevisionAndDigest(t *testing.T) {
	installer := Installer{}
	component := model.Component{ID: "skills", Install: model.InstallSpec{Kind: model.InstallSkillPack, Repository: "https://example.test/repo.git"}}
	result := installer.Install(context.Background(), component)
	if result.Err == nil {
		t.Fatal("unpinned skill pack should fail closed")
	}
}

func TestInstallerRejectsResolvedCommitMismatch(t *testing.T) {
	commands := &skillCommandRunner{sha: strings.Repeat("b", 40)}
	installer := Installer{Commands: commands, Targets: []string{t.TempDir()}}
	component := model.Component{ID: "skills", Install: model.InstallSpec{
		Kind: model.InstallSkillPack, Repository: "https://example.test/repo.git", RepositoryRevision: "v1.0.0", ManifestDigest: "git-commit:" + strings.Repeat("a", 40),
	}}
	result := installer.Install(context.Background(), component)
	if result.Err == nil || !strings.Contains(result.Err.Error(), "revision mismatch") {
		t.Fatalf("expected revision mismatch, got %#v", result)
	}
}

func TestValidateSkillInventoryRejectsMissingAuditedSkill(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "brainstorming"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "brainstorming", "SKILL.md"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSkillInventory(root, []string{"brainstorming", "systematic-debugging"}); err == nil {
		t.Fatal("expected missing audited skill to be rejected")
	}
}
