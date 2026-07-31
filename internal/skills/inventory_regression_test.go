package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillInventoryRejectsUnexpectedAuditedEntries(t *testing.T) {
	source := t.TempDir()
	for _, name := range []string{"expected", "unexpected"} {
		dir := filepath.Join(source, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := ValidateSkillInventory(source, []string{"expected"})
	if err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unexpected skill inventory was accepted: %v", err)
	}
}

func TestCopyMissingSkillsRefusesSourceWithUnexpectedEntries(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	for _, name := range []string{"expected", "unexpected"} {
		dir := filepath.Join(source, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := CopyMissingSkills(source, []string{"expected"}, []string{target}); err == nil {
		t.Fatal("copy accepted a source tree with unexpected skills")
	}
	if _, err := os.Stat(filepath.Join(target, "expected")); !os.IsNotExist(err) {
		t.Fatalf("copy mutated target before exact inventory validation: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "unexpected")); !os.IsNotExist(err) {
		t.Fatalf("unexpected skill was copied: %v", err)
	}
}

func TestSkillInventoryRejectsPortableNameCollisionsAndSeparators(t *testing.T) {
	t.Run("case-collision", func(t *testing.T) {
		source := t.TempDir()
		for _, name := range []string{"Review", "review"} {
			dir := filepath.Join(source, name)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := ValidateSkillInventory(source, []string{"Review", "review"}); err == nil {
			t.Fatal("case-colliding skill names were accepted")
		}
	})

	t.Run("windows-separator", func(t *testing.T) {
		source := t.TempDir()
		name := `review\\nested`
		dir := filepath.Join(source, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# unsafe"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ValidateSkillInventory(source, []string{name}); err == nil {
			t.Fatal("skill name containing a Windows path separator was accepted")
		}
	})

	for _, name := range []string{"review name", "review.", "con"} {
		t.Run("non-portable-"+strings.ReplaceAll(name, " ", "-"), func(t *testing.T) {
			source := t.TempDir()
			dir := filepath.Join(source, name)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# unsafe"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ValidateSkillInventory(source, []string{name}); err == nil {
				t.Fatalf("non-portable skill name %q was accepted", name)
			}
		})
	}
}

func TestCopyMissingSkillsRejectsSymlinkBeforeTargetMutation(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	skill := filepath.Join(source, "review")
	if err := os.MkdirAll(skill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("# review"), 0o600); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "payload.txt")
	if err := os.WriteFile(external, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(skill, "payload-link")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := CopyMissingSkills(source, []string{"review"}, []string{target}); err == nil {
		t.Fatal("skill tree containing a symlink was accepted")
	}
	if _, err := os.Stat(filepath.Join(target, "review")); !os.IsNotExist(err) {
		t.Fatalf("target mutated before symlink rejection: %v", err)
	}
}
