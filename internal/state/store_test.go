package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
)

func TestStoreWritesAtomicallyAndLoadsOwnership(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	ownership := Ownership{ManagedComponents: map[string]ManagedComponent{"git": {ID: "git", Source: "agentstack", Paths: []string{"C:/Tools/git.exe"}}}}
	if err := store.SaveOwnership(ownership); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadOwnership()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ManagedComponents["git"].Source != "agentstack" {
		t.Fatalf("unexpected ownership: %#v", loaded)
	}
	if len(loaded.ManagedComponents["git"].Paths) != 1 {
		t.Fatalf("owned paths were not preserved: %#v", loaded)
	}
	if _, err := os.Stat(filepath.Join(dir, "state", "ownership.json")); err != nil {
		t.Fatal(err)
	}
}

func TestBackupCreatesDistinctCopyWithoutChangingSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "config.json")
	if err := os.WriteFile(source, []byte(`{"keep":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(filepath.Join(dir, "data"))
	backup, err := store.BackupFile(source, "agy-mcp")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"keep":true}` {
		t.Fatalf("unexpected backup: %s", data)
	}
	original, _ := os.ReadFile(source)
	if string(original) != `{"keep":true}` {
		t.Fatal("source changed")
	}
}

func TestSaveTransactionRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	tx := model.Transaction{ID: "tx-1", Status: model.TransactionSucceeded}
	if err := store.SaveTransaction(tx); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadTransaction("tx-1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != model.TransactionSucceeded {
		t.Fatalf("unexpected status: %s", loaded.Status)
	}
}

func TestSaveTransactionDoesNotPersistRawProcessOutputOrArguments(t *testing.T) {
	store := Store{Root: t.TempDir()}
	tx := model.Transaction{
		ID:     "tx-private",
		Status: model.TransactionFailed,
		Actions: []model.TransactionAction{{
			ComponentID:     "credential-tool",
			Kind:            model.ActionInstall,
			Command:         "tool",
			Args:            []string{"--token", "ghp_not-for-disk"},
			Output:          `installed under C:\\Users\\private\\path with bearer secret`,
			OutputTruncated: false,
			Error:           "request failed with bearer secret",
			Verification:    "api_key=secret",
		}},
	}
	if err := store.SaveTransaction(tx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(store.Root, "transactions", "tx-private.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"ghp_not-for-disk", "private\\\\path", "bearer secret", "api_key=secret", `"args"`, `"output":"installed`} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("persisted transaction contains private process data %q: %s", forbidden, data)
		}
	}
	loaded, err := store.LoadTransaction(tx.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Actions) != 1 || loaded.Actions[0].Output != "" || len(loaded.Actions[0].Args) != 0 {
		t.Fatalf("transaction was not minimized: %+v", loaded.Actions)
	}
	if !loaded.Actions[0].OutputTruncated {
		t.Fatal("expected persisted record to disclose that process output was omitted")
	}
	if loaded.Actions[0].Error != "action failed; detailed process error output was not persisted" || loaded.Actions[0].Verification != "[REDACTED]" {
		t.Fatalf("secret-bearing text was not redacted: %+v", loaded.Actions[0])
	}
}

func TestMutationLeaseRejectsConcurrentOwner(t *testing.T) {
	store := NewStore(t.TempDir())
	first, err := store.AcquireLease("mutation", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := store.AcquireLease("mutation", time.Hour); !errors.Is(err, ErrMutationBusy) {
		t.Fatalf("expected busy lease, got %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := store.AcquireLease("mutation", time.Hour)
	if err != nil {
		t.Fatalf("expected lease after release: %v", err)
	}
	_ = second.Close()
}

func TestSavedPlanRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	value := SavedPlan{Plan: model.Plan{ID: "plan-abc", Digest: "sha256:test"}, Request: planner.Request{Profile: "essential"}}
	if err := store.SavePlan(value); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadPlan("plan-abc")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Plan.Digest != value.Plan.Digest || loaded.Request.Profile != "essential" {
		t.Fatalf("unexpected saved plan: %#v", loaded)
	}
}

func TestRecoverIncompleteTransactionsMarksInterrupted(t *testing.T) {
	store := NewStore(t.TempDir())
	tx := model.Transaction{ID: "tx-running", Status: model.TransactionRunning, StartedAt: time.Now().Add(-time.Minute)}
	if err := store.SaveTransaction(tx); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.RecoverIncompleteTransactions()
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Status != model.TransactionInterrupted {
		t.Fatalf("unexpected recovered transactions: %#v", recovered)
	}
}

func TestRestoreBackupValidatesOwnershipAndDigest(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "config.json")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(filepath.Join(dir, "data"))
	if _, err := store.BackupFile(target, "config"); err != nil {
		t.Fatal(err)
	}
	backups, err := store.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%#v err=%v", backups, err)
	}
	if err := os.WriteFile(target, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RestoreBackup(backups[0].ID, target); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(target)
	if string(data) != "before" {
		t.Fatalf("restore did not replace target: %q", data)
	}
	if _, err := store.RestoreBackup(backups[0].ID, filepath.Join(dir, "other.json")); err == nil {
		t.Fatal("restore must reject a different target")
	}
}

func TestStoreRejectsTraversalIdentifiers(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.SavePlan(SavedPlan{Plan: model.Plan{ID: "../outside"}}); err == nil {
		t.Fatal("plan traversal identifier was accepted")
	}
	if err := store.SaveTransaction(model.Transaction{ID: "../outside"}); err == nil {
		t.Fatal("transaction traversal identifier was accepted")
	}
	if _, err := store.LoadPlan("../outside"); err == nil {
		t.Fatal("invalid plan identifier was accepted for load")
	}
	if err := store.DeletePlan("../outside"); err == nil {
		t.Fatal("invalid plan identifier was accepted for delete")
	}
}
