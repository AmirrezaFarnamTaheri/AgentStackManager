package sqlite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/cas"
	"github.com/agentstack/agentstack/internal/migrations/asmv1"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestStageInspectAndIdempotence(t *testing.T) {
	requireSQLite(t)
	receipt, _, _ := stagedReceipt(t, "one")
	path := filepath.Join(t.TempDir(), "fabric", "metadata.db")
	store := New(path)
	summary, err := store.Stage(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ReadOnly || summary.SchemaVersion != SchemaVersion || summary.JournalMode != "wal" || summary.QuickCheck != "ok" {
		t.Fatalf("summary=%#v", summary)
	}
	if summary.ReceiptDigest != receipt.Digest || summary.ArtifactCount != 1 || summary.ResourceCount != 1 || summary.SnapshotCount != 1 {
		t.Fatalf("summary=%#v receipt=%#v", summary, receipt)
	}
	inspection, err := store.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if !inspection.Summary.ReadOnly || inspection.Receipt.Digest != receipt.Digest {
		t.Fatalf("inspection=%#v", inspection)
	}
	repeated, err := store.Stage(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.SnapshotCount != 1 || repeated.ReceiptDigest != receipt.Digest {
		t.Fatalf("repeated=%#v", repeated)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("metadata permissions=%#o", info.Mode().Perm())
	}
}

func TestStageRetainsHistoryAndMovesShadowHead(t *testing.T) {
	requireSQLite(t)
	path := filepath.Join(t.TempDir(), "metadata.db")
	store := New(path)
	first, _, _ := stagedReceipt(t, "one")
	if _, err := store.Stage(first); err != nil {
		t.Fatal(err)
	}
	second, _, _ := stagedReceipt(t, "two")
	summary, err := store.Stage(second)
	if err != nil {
		t.Fatal(err)
	}
	if summary.SnapshotCount != 2 || summary.ReceiptDigest != second.Digest {
		t.Fatalf("summary=%#v", summary)
	}
	inspection, err := store.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Receipt.Digest != second.Digest {
		t.Fatalf("head receipt=%s want=%s", inspection.Receipt.Digest, second.Digest)
	}
}

func TestInspectDoesNotCreateMissingDatabase(t *testing.T) {
	requireSQLite(t)
	path := filepath.Join(t.TempDir(), "missing", "metadata.db")
	_, err := New(path).Inspect()
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("expected missing database error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Dir(path)); !os.IsNotExist(statErr) {
		t.Fatalf("read-only inspect created state: %v", statErr)
	}
}

func TestInspectRejectsTamperedArtifactRow(t *testing.T) {
	requireSQLite(t)
	receipt, _, _ := stagedReceipt(t, "one")
	path := filepath.Join(t.TempDir(), "metadata.db")
	store := New(path)
	if _, err := store.Stage(receipt); err != nil {
		t.Fatal(err)
	}
	db, err := openNative(path, nativeOpenReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	statement, err := db.prepare("UPDATE fabric_artifacts SET artifact_json=? WHERE receipt_digest=?")
	if err != nil {
		t.Fatal(err)
	}
	if err := statement.bindBlob(1, []byte(`{"id":"tampered"}`)); err != nil {
		t.Fatal(err)
	}
	if err := statement.bindText(2, receipt.Digest); err != nil {
		t.Fatal(err)
	}
	if row, err := statement.step(); err != nil || row {
		t.Fatalf("row=%v err=%v", row, err)
	}
	if err := statement.close(); err != nil {
		t.Fatal(err)
	}
	if err := db.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect(); err == nil || !strings.Contains(err.Error(), "does not match sealed receipt") {
		t.Fatalf("expected tamper rejection, got %v", err)
	}
}

func TestBackupIsVerifiedAndNeverOverwrites(t *testing.T) {
	requireSQLite(t)
	receipt, _, _ := stagedReceipt(t, "one")
	path := filepath.Join(t.TempDir(), "metadata.db")
	store := New(path)
	if _, err := store.Stage(receipt); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup.db")
	summary, err := store.Backup(destination)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Path != destination || summary.ReceiptDigest != receipt.Digest || !summary.ReadOnly {
		t.Fatalf("backup summary=%#v", summary)
	}
	inspection, err := New(destination).Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Receipt.Digest != receipt.Digest {
		t.Fatalf("backup receipt=%s", inspection.Receipt.Digest)
	}
	before, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Backup(destination); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected no-overwrite failure, got %v", err)
	}
	after, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("existing backup was changed")
	}
}

func TestDatabasePathRejectsSymlink(t *testing.T) {
	requireSQLite(t)
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	receipt, _, _ := stagedReceipt(t, "one")
	root := t.TempDir()
	target := filepath.Join(root, "target.db")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "metadata.db")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(link).Stage(receipt); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func stagedReceipt(t *testing.T, content string) (asmv1.Receipt, resourcehub.Manager, cas.Store) {
	t.Helper()
	hub := resourcehub.New(filepath.Join(t.TempDir(), "hub"))
	now := time.Date(2026, 8, 4, 20, 0, 0, 0, time.UTC)
	hub.Clock = func() time.Time { return now }
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := hub.Import(source, resourcehub.ImportOptions{ID: "rule", Kind: resourcehub.KindRule}); err != nil {
		t.Fatal(err)
	}
	objectStore := cas.New(filepath.Join(t.TempDir(), "cas"))
	receipt, err := asmv1.Stage(hub, objectStore, func() time.Time { return now.Add(time.Minute) })
	if err != nil {
		t.Fatal(err)
	}
	return receipt, hub, objectStore
}

func TestStageRejectsUnrecognizedExistingSQLiteDatabase(t *testing.T) {
	requireSQLite(t)
	receipt, _, _ := stagedReceipt(t, "one")
	path := filepath.Join(t.TempDir(), "foreign.db")
	state, err := prepareDatabasePath(path, true)
	if err != nil {
		t.Fatal(err)
	}
	db, err := openNative(state.path, nativeOpenReadWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.exec("PRAGMA journal_mode=DELETE"); err != nil {
		t.Fatal(err)
	}
	if err := db.exec("CREATE TABLE foreign_state(value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := db.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := New(path).Stage(receipt); err == nil || !strings.Contains(err.Error(), "unrecognized existing") {
		t.Fatalf("expected foreign database rejection, got %v", err)
	}
	db, err = openNative(path, nativeOpenReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	mode, err := querySingleText(db, "PRAGMA journal_mode")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.close(); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(mode) != "delete" {
		t.Fatalf("foreign database journal mode was changed: %q", mode)
	}
}

func requireSQLite(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("SQLite metadata backend unavailable in this build")
	}
}

func TestInspectRejectsTamperedReceiptAndResourceRows(t *testing.T) {
	requireSQLite(t)
	for _, test := range []struct {
		name  string
		query string
		bind  func(*nativeStmt, asmv1.Receipt) error
		want  string
	}{
		{
			name:  "receipt",
			query: "UPDATE fabric_snapshots SET receipt_json=? WHERE receipt_digest=?",
			bind: func(statement *nativeStmt, receipt asmv1.Receipt) error {
				if err := statement.bindBlob(1, []byte(`{"apiVersion":"tampered"}`)); err != nil {
					return err
				}
				return statement.bindText(2, receipt.Digest)
			},
			want: "invalid digest",
		},
		{
			name:  "resource",
			query: "UPDATE fabric_resources SET record_json=? WHERE receipt_digest=?",
			bind: func(statement *nativeStmt, receipt asmv1.Receipt) error {
				if err := statement.bindBlob(1, []byte(`{"resourceId":"tampered"}`)); err != nil {
					return err
				}
				return statement.bindText(2, receipt.Digest)
			},
			want: "does not match sealed receipt",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt, _, _ := stagedReceipt(t, "one")
			path := filepath.Join(t.TempDir(), "metadata.db")
			store := New(path)
			if _, err := store.Stage(receipt); err != nil {
				t.Fatal(err)
			}
			db, err := openNative(path, nativeOpenReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			statement, err := db.prepare(test.query)
			if err != nil {
				t.Fatal(err)
			}
			if err := test.bind(statement, receipt); err != nil {
				t.Fatal(err)
			}
			if row, err := statement.step(); err != nil || row {
				t.Fatalf("row=%v err=%v", row, err)
			}
			if err := statement.close(); err != nil {
				t.Fatal(err)
			}
			if err := db.close(); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Inspect(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q rejection, got %v", test.want, err)
			}
		})
	}
}

func TestInspectRejectsFutureSchemaAndMissingMigrationLedger(t *testing.T) {
	requireSQLite(t)
	for _, test := range []struct {
		name      string
		statement string
		want      string
	}{
		{name: "future schema", statement: "PRAGMA user_version=2", want: "unsupported SQLite metadata schema version"},
		{name: "missing ledger", statement: "DELETE FROM schema_migrations", want: "migration ledger"},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt, _, _ := stagedReceipt(t, "one")
			path := filepath.Join(t.TempDir(), "metadata.db")
			store := New(path)
			if _, err := store.Stage(receipt); err != nil {
				t.Fatal(err)
			}
			db, err := openNative(path, nativeOpenReadWrite)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.exec(test.statement); err != nil {
				t.Fatal(err)
			}
			if err := db.close(); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Inspect(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q rejection, got %v", test.want, err)
			}
		})
	}
}
