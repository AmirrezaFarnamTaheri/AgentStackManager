package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestDiscoverTargetCandidatesFindsKnownUserLevelApps(t *testing.T) {
	home := t.TempDir()
	for _, marker := range []string{".codex", ".claude", ".gemini", ".opencode", ".cursor", ".github"} {
		if err := os.MkdirAll(filepath.Join(home, marker), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	targets := []resourcehub.Target{
		{ID: "codex-user", Agent: resourcehub.AgentCodex, Root: home, Mode: resourcehub.ModeCopy, Enabled: true},
		{ID: "claude-user", Agent: resourcehub.AgentClaude, Root: home, Mode: resourcehub.ModeCopy, Enabled: false},
	}
	candidates := discoverTargetCandidates(home, targets)
	if len(candidates) < 6 {
		t.Fatalf("candidates=%#v", candidates)
	}
	byAgent := map[resourcehub.Agent]TargetCandidate{}
	original := map[resourcehub.Agent]bool{
		resourcehub.AgentCodex: true, resourcehub.AgentClaude: true, resourcehub.AgentAGY: true,
		resourcehub.AgentOpenCode: true, resourcehub.AgentCursor: true, resourcehub.AgentCopilot: true,
	}
	for _, candidate := range candidates {
		if !original[candidate.Agent] {
			continue
		}
		byAgent[candidate.Agent] = candidate
		if !candidate.Detected {
			t.Fatalf("%s was not detected: %#v", candidate.Agent, candidate)
		}
		if candidate.Root != home {
			t.Fatalf("%s root=%q", candidate.Agent, candidate.Root)
		}
	}
	if !byAgent[resourcehub.AgentCodex].Registered || !byAgent[resourcehub.AgentCodex].Enabled {
		t.Fatalf("codex candidate=%#v", byAgent[resourcehub.AgentCodex])
	}
	if !byAgent[resourcehub.AgentClaude].Registered || byAgent[resourcehub.AgentClaude].Enabled {
		t.Fatalf("claude candidate=%#v", byAgent[resourcehub.AgentClaude])
	}
	if _, ok := byAgent[resourcehub.AgentAGY]; !ok {
		t.Fatalf("AGY candidate missing: %#v", candidates)
	}
}

func TestDiscoverTargetCandidatesPreservesMultipleProfilesAndEvidence(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o700); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	codex := filepath.Join(bin, "codex")
	if err := os.WriteFile(codex, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	codexCmd := filepath.Join(bin, "codex.cmd")
	if err := os.WriteFile(codexCmd, []byte("@echo off\nexit /b 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	targets := []resourcehub.Target{
		{ID: "codex-personal", Agent: resourcehub.AgentCodex, Root: home, Mode: resourcehub.ModeCopy, Enabled: true, Scope: "global", Label: "Personal"},
		{ID: "codex-project", Agent: resourcehub.AgentCodex, Root: filepath.Join(home, "project"), Mode: resourcehub.ModeCopy, Enabled: true, Scope: "project", Label: "Project"},
	}
	candidates := discoverTargetCandidates(home, targets)
	var codexCandidates []TargetCandidate
	for _, candidate := range candidates {
		if candidate.Agent == resourcehub.AgentCodex {
			codexCandidates = append(codexCandidates, candidate)
		}
	}
	if len(codexCandidates) != 2 {
		t.Fatalf("codex candidates=%#v", codexCandidates)
	}
	for _, candidate := range codexCandidates {
		if candidate.SupportLevel != "verified" || !candidate.Writable {
			t.Fatalf("support=%#v", candidate)
		}
		if candidate.DetectionState != "confirmed" || candidate.Confidence < 80 {
			t.Fatalf("detection=%#v", candidate)
		}
		if len(candidate.Evidence) == 0 {
			t.Fatalf("evidence missing: %#v", candidate)
		}
	}
}

func TestDiscoverTargetCandidatesSeparatesKnownFromWritable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	if err := os.MkdirAll(filepath.Join(home, ".vscode"), 0o700); err != nil {
		t.Fatal(err)
	}
	candidates := discoverTargetCandidates(home, nil)
	for _, candidate := range candidates {
		if candidate.Agent != resourcehub.AgentVSCode {
			continue
		}
		if candidate.SupportLevel != "known" || candidate.Writable {
			t.Fatalf("vscode candidate=%#v", candidate)
		}
		if candidate.DetectionState != "configuration-only" {
			t.Fatalf("vscode detection=%#v", candidate)
		}
		return
	}
	t.Fatal("VS Code candidate missing")
}
