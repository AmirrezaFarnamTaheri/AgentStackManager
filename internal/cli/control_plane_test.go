package cli_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/app"
	"github.com/agentstack/agentstack/internal/cli"
)

func TestControlPlaneAuditCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cli-audit-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	var stdout, stderr bytes.Buffer
	c := cli.New(&app.Service{}, "1.0.0", "git:test")
	c.Out = &stdout
	c.Err = &stderr

	code := c.Run(context.Background(), []string{"audit", "--root", tmpDir, "--mode", "shadow"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"mode": "shadow"`) && !strings.Contains(stdout.String(), `"report"`) {
		t.Errorf("stdout missing audit report output: %s", stdout.String())
	}
}

func TestControlPlaneReconcileCommand(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cli-reconcile-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	var stdout, stderr bytes.Buffer
	c := cli.New(&app.Service{}, "1.0.0", "git:test")
	c.Out = &stdout
	c.Err = &stderr

	code := c.Run(context.Background(), []string{"reconcile", "--root", tmpDir, "--mode", "shadow"})
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d. Stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"reconciliations"`) {
		t.Errorf("stdout missing reconciliations: %s", stdout.String())
	}
}
