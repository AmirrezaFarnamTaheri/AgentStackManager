package contextengine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/agentstack/agentstack/internal/runner"
)

type fakeGitRunner struct{}

func (fakeGitRunner) Run(_ context.Context, invocation runner.Invocation) runner.Result {
	joined := strings.Join(invocation.Args, " ")
	switch {
	case strings.Contains(joined, "--is-inside-work-tree"):
		return runner.Result{Stdout: "true\n"}
	case strings.Contains(joined, "--abbrev-ref"):
		return runner.Result{Stdout: "main\n"}
	case strings.HasSuffix(joined, "rev-parse HEAD"):
		return runner.Result{Stdout: strings.Repeat("a", 40) + "\n"}
	case strings.Contains(joined, "status --short"):
		return runner.Result{Stdout: " M README.md\n"}
	case strings.Contains(joined, "diff --stat"):
		return runner.Result{Stdout: " README.md | 1 +\n"}
	default:
		return runner.Result{}
	}
}

type recordingGitRunner struct{ calls []runner.Invocation }

func (r *recordingGitRunner) Run(ctx context.Context, invocation runner.Invocation) runner.Result {
	r.calls = append(r.calls, invocation)
	return fakeGitRunner{}.Run(ctx, invocation)
}

func TestReadFileAndSearchStayInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n// agent memory bridge\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New(t.TempDir())
	view, err := manager.ReadFile(root, "src/main.go")
	if err != nil || !strings.Contains(view.Content, "memory bridge") {
		t.Fatalf("view=%#v err=%v", view, err)
	}
	if _, err := manager.ReadFile(root, "../secret"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	result, err := manager.Search(root, "memory bridge", 10)
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Line != 2 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestGitContextUsesBoundedRunner(t *testing.T) {
	manager := New(t.TempDir())
	manager.Commands = fakeGitRunner{}
	result, err := manager.Git(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Repository || result.Branch != "main" || len(result.Revision) != 40 {
		t.Fatalf("unexpected git context: %#v", result)
	}
}

func TestGitInspectionUsesOnlyReadOnlyCommands(t *testing.T) {
	runnerRecorder := &recordingGitRunner{}
	manager := New(t.TempDir())
	manager.Commands = runnerRecorder
	if _, err := manager.Git(context.Background(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(runnerRecorder.calls) == 0 {
		t.Fatal("expected Git inspection commands")
	}
	for _, call := range runnerRecorder.calls {
		if call.Command != "git" {
			t.Fatalf("unexpected executable: %#v", call)
		}
		joined := strings.Join(call.Args, " ")
		for _, forbidden := range []string{" add ", " commit ", " checkout ", " reset ", " clean ", " update-index ", " apply "} {
			if strings.Contains(" "+joined+" ", forbidden) {
				t.Fatalf("Git inspection attempted mutation: %s", joined)
			}
		}
		if call.Timeout <= 0 || call.MaxOutputBytes <= 0 {
			t.Fatalf("Git inspection was not bounded: %#v", call)
		}
	}
}

func TestReadFileResolvesSymlinkedWorkspaceRootWithoutEscaping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	realRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(realRoot, "context.txt"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkParent := t.TempDir()
	linkedRoot := filepath.Join(linkParent, "project")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatal(err)
	}
	view, err := New(t.TempDir()).ReadFile(linkedRoot, "context.txt")
	if err != nil || view.Content != "safe" {
		t.Fatalf("view=%#v err=%v", view, err)
	}
}

func TestBoundedSnippetDoesNotSplitUTF8(t *testing.T) {
	value := strings.Repeat("界", 100)
	snippet := boundedSnippet(value, 11)
	if !utf8.ValidString(snippet) {
		t.Fatalf("snippet is not valid UTF-8: %q", snippet)
	}
	if !strings.HasSuffix(snippet, "...") {
		t.Fatalf("snippet should indicate truncation: %q", snippet)
	}
}

func TestReadFileTruncationDoesNotSplitUTF8(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("a", maxContextReadBytes-1) + "界"
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	view, err := New(t.TempDir()).ReadFile(root, "large.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !view.Truncated {
		t.Fatal("expected oversized context file to be truncated")
	}
	if !utf8.ValidString(view.Content) {
		t.Fatalf("truncated content is not valid UTF-8")
	}
	if view.Bytes != len(view.Content) || view.Bytes > maxContextReadBytes {
		t.Fatalf("invalid byte count after truncation: view=%d content=%d", view.Bytes, len(view.Content))
	}
}
