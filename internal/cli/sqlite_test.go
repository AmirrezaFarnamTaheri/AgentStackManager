package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sqlitestore "github.com/agentstack/agentstack/internal/store/sqlite"
)

func TestResourceHubSQLiteStageInspectVerifyAndBackupCLI(t *testing.T) {
	if !sqlitestore.Available() {
		t.Skip("SQLite metadata backend unavailable in this build")
	}
	command, output := testFabricCLI(t)
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("preserve reviewed authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "hub", "import", "--id", "reviewed", "--kind", "rule", "--path", source)
	databasePath := filepath.Join(t.TempDir(), "fabric", "metadata.db")
	casRoot := filepath.Join(t.TempDir(), "cas")
	staged := runFabricCLI(t, command, output, "hub", "db-stage", "--db", databasePath, "--cas-root", casRoot)
	var summary sqlitestore.Summary
	if err := json.Unmarshal(staged, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.ReceiptDigest == "" || summary.ResourceCount != 1 || summary.SnapshotCount != 1 || summary.ReadOnly {
		t.Fatalf("stage summary=%s", staged)
	}
	inspected := runFabricCLI(t, command, output, "hub", "db-inspect", "--db", databasePath)
	var inspection sqlitestore.Summary
	if err := json.Unmarshal(inspected, &inspection); err != nil {
		t.Fatal(err)
	}
	if !inspection.ReadOnly || inspection.ReceiptDigest != summary.ReceiptDigest {
		t.Fatalf("inspect summary=%s", inspected)
	}
	verified := runFabricCLI(t, command, output, "hub", "db-verify", "--db", databasePath, "--cas-root", casRoot)
	if !bytes.Contains(verified, []byte(`"verified": true`)) || !bytes.Contains(verified, []byte(summary.ReceiptDigest)) {
		t.Fatalf("verify=%s", verified)
	}
	backup := filepath.Join(t.TempDir(), "metadata-backup.db")
	backedUp := runFabricCLI(t, command, output, "hub", "db-backup", "--db", databasePath, "--destination", backup, "--yes")
	if !bytes.Contains(backedUp, []byte(summary.ReceiptDigest)) {
		t.Fatalf("backup=%s", backedUp)
	}
	if _, err := sqlitestore.New(backup).Inspect(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceHubSQLiteBackupRequiresConfirmation(t *testing.T) {
	command, output := testFabricCLI(t)
	output.Reset()
	code := command.Run(context.Background(), []string{"hub", "db-backup", "--destination", "backup.db"})
	if code != 2 {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
}

func TestResourceHubSQLiteVerifyRejectsStaleAuthoritativeState(t *testing.T) {
	if !sqlitestore.Available() {
		t.Skip("SQLite metadata backend unavailable in this build")
	}
	command, output := testFabricCLI(t)
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "hub", "import", "--id", "rule", "--kind", "rule", "--path", source)
	databasePath := filepath.Join(t.TempDir(), "metadata.db")
	casRoot := filepath.Join(t.TempDir(), "cas")
	runFabricCLI(t, command, output, "hub", "db-stage", "--db", databasePath, "--cas-root", casRoot)
	if err := os.WriteFile(source, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "hub", "import", "--id", "rule", "--kind", "rule", "--path", source, "--replace")
	output.Reset()
	code := command.Run(context.Background(), []string{"hub", "db-verify", "--db", databasePath, "--cas-root", casRoot})
	if code != 1 || !bytes.Contains(output.Bytes(), []byte("stale")) {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
}
