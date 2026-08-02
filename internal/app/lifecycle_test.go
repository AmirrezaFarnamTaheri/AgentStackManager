package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/mcp"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/state"
)

func TestOwnedPreviewRefusesUnverifiedOwnership(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"adopted": {ID: "adopted", Source: "external", InstallKind: model.InstallNPMGlobal, Package: "example@1.0.0"},
	}}); err != nil {
		t.Fatal(err)
	}
	report, err := service.OwnedPreview([]string{"adopted"}, "remove")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actions) != 1 || report.Actions[0].Supported || report.Actions[0].Status != "planned" {
		t.Fatalf("unverified ownership must be refused: %+v", report)
	}
}

func TestRemoveOwnedSkillMovesToQuarantine(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	skillRoot := filepath.Join(service.Paths.DataRoot, "recognized-skills")
	service.SkillRoots = []string{skillRoot}
	skill := filepath.Join(skillRoot, "owned")
	if err := os.MkdirAll(skill, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"skills": {ID: "skills", Source: "agentstack", InstallKind: model.InstallSkillPack, Paths: []string{skill}},
	}}); err != nil {
		t.Fatal(err)
	}
	report, err := service.RemoveOwned(context.Background(), []string{"skills"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actions) != 1 || report.Actions[0].Status != "succeeded" || len(report.Actions[0].Quarantine) != 1 {
		t.Fatalf("unexpected removal report: %+v", report)
	}
	if _, err := os.Stat(skill); !os.IsNotExist(err) {
		t.Fatalf("owned source should be moved, stat=%v", err)
	}
	if data, err := os.ReadFile(filepath.Join(report.Actions[0].Quarantine[0], "SKILL.md")); err != nil || string(data) != "safe" {
		t.Fatalf("quarantine copy invalid: data=%q err=%v", data, err)
	}
	ownership, err := service.Store.LoadOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := ownership.ManagedComponents["skills"]; exists {
		t.Fatal("successfully quarantined component remained owned")
	}
}

func TestRemoveOwnedSkillQuarantinesSameNameFromMultipleRoots(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	firstRoot := filepath.Join(service.Paths.DataRoot, "agents-skills")
	secondRoot := filepath.Join(service.Paths.DataRoot, "gemini-skills")
	service.SkillRoots = []string{firstRoot, secondRoot}
	firstSkill := filepath.Join(firstRoot, "owned")
	secondSkill := filepath.Join(secondRoot, "owned")
	for path, content := range map[string]string{firstSkill: "agents", secondSkill: "gemini"} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"skills": {ID: "skills", Source: "agentstack", InstallKind: model.InstallSkillPack, Paths: []string{firstSkill, secondSkill}},
	}}); err != nil {
		t.Fatal(err)
	}

	report, err := service.RemoveOwned(context.Background(), []string{"skills"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Actions) != 1 || report.Actions[0].Status != "succeeded" || len(report.Actions[0].Quarantine) != 2 {
		t.Fatalf("unexpected multi-root quarantine report: %+v", report)
	}
	if report.Actions[0].Quarantine[0] == report.Actions[0].Quarantine[1] {
		t.Fatalf("multi-root quarantine destinations collided: %+v", report.Actions[0].Quarantine)
	}
	contents := map[string]bool{}
	for _, path := range report.Actions[0].Quarantine {
		data, readErr := os.ReadFile(filepath.Join(path, "SKILL.md"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		contents[string(data)] = true
	}
	if !contents["agents"] || !contents["gemini"] {
		t.Fatalf("quarantine did not preserve both skill trees: %#v", contents)
	}
}

func TestRemoveOwnedSkillRefusesPathOutsideRecognizedRoots(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	service.SkillRoots = []string{filepath.Join(service.Paths.DataRoot, "recognized-skills")}
	outside := filepath.Join(service.Paths.DataRoot, "unrelated", "do-not-delete")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outside, "marker.txt")
	if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"skills": {ID: "skills", Source: "agentstack", InstallKind: model.InstallSkillPack, Paths: []string{outside}},
	}}); err != nil {
		t.Fatal(err)
	}
	report, err := service.RemoveOwned(context.Background(), []string{"skills"}, true)
	if err == nil {
		t.Fatal("expected unsafe ownership path to be refused")
	}
	if len(report.Actions) != 1 || report.Actions[0].Status != "failed" {
		t.Fatalf("unexpected report: %+v", report)
	}
	if data, readErr := os.ReadFile(marker); readErr != nil || string(data) != "preserve" {
		t.Fatalf("unrelated path was changed: data=%q err=%v", data, readErr)
	}
	ownership, loadErr := service.Store.LoadOwnership()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if _, exists := ownership.ManagedComponents["skills"]; !exists {
		t.Fatal("failed removal erased ownership evidence")
	}
}

func TestRemoveOwnedSkillRefusesSymlinkedPath(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	skillRoot := filepath.Join(service.Paths.DataRoot, "recognized-skills")
	service.SkillRoots = []string{skillRoot}
	outside := filepath.Join(service.Paths.DataRoot, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skillRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(skillRoot, "owned")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"skills": {ID: "skills", Source: "agentstack", InstallKind: model.InstallSkillPack, Paths: []string{link}},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RemoveOwned(context.Background(), []string{"skills"}, true); err == nil {
		t.Fatal("expected symlinked owned path to be refused")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("symlink target was changed: %v", err)
	}
}

func TestDeactivateOwnedRouterPreservesOtherEntriesAndCreatesBackup(t *testing.T) {
	service := minimalService(t, model.Catalog{}, &appRunner{})
	config := mcp.RouterConfig{Version: 1, Servers: map[string]mcp.ServerConfig{
		"owned": {Command: "owned"},
		"other": {Command: "other"},
	}}
	if err := mcp.WriteRouterConfig(service.Paths.RouterConfig, config); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"owned": {ID: "owned", Source: "agentstack-router", InstallKind: model.InstallRouter, Active: true},
	}}); err != nil {
		t.Fatal(err)
	}
	report, err := service.DeactivateOwned(context.Background(), []string{"owned"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Actions[0].Status != "succeeded" {
		t.Fatalf("unexpected report: %+v", report)
	}
	after, err := mcp.LoadRouterConfig(service.Paths.RouterConfig)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := after.Servers["owned"]; exists {
		t.Fatal("owned router entry remained active")
	}
	if _, exists := after.Servers["other"]; !exists {
		t.Fatal("unrelated router entry was removed")
	}
	backups, err := service.Store.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected one backup, len=%d err=%v", len(backups), err)
	}
	ownership, _ := service.Store.LoadOwnership()
	if ownership.ManagedComponents["owned"].Active {
		t.Fatal("ownership did not record inactive state")
	}
}

func TestRemoveOwnedPackageUsesRecordedIdentity(t *testing.T) {
	commands := &appRunner{}
	service := minimalService(t, model.Catalog{}, commands)
	if err := service.Store.SaveOwnership(state.Ownership{ManagedComponents: map[string]state.ManagedComponent{
		"tool": {ID: "tool", Source: "agentstack", InstallKind: model.InstallNPMGlobal, Package: "@scope/tool@1.2.3"},
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RemoveOwned(context.Background(), []string{"tool"}, true); err != nil {
		t.Fatal(err)
	}
	if len(commands.calls) != 1 || commands.calls[0].Command != "npm" {
		t.Fatalf("unexpected command calls: %+v", commands.calls)
	}
	want := []string{"uninstall", "--global", "@scope/tool"}
	if len(commands.calls[0].Args) != len(want) {
		t.Fatalf("unexpected args: %#v", commands.calls[0].Args)
	}
	for index := range want {
		if commands.calls[0].Args[index] != want[index] {
			t.Fatalf("unexpected args: %#v", commands.calls[0].Args)
		}
	}
}
