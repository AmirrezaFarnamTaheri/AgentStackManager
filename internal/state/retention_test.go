package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/model"
)

func TestPruneRemovesExpiredOperationalDataButKeepsBackups(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	if err := store.SavePlan(SavedPlan{Plan: model.Plan{ID: "expired", ExpiresAt: now.Add(-time.Minute)}, CreatedAt: now.Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveTransaction(model.Transaction{ID: "old", Status: model.TransactionSucceeded, StartedAt: now.Add(-48 * time.Hour), FinishedAt: now.Add(-47 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	diagnostic := filepath.Join(root, "diagnostics", "old.zip")
	if err := os.WriteFile(diagnostic, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(diagnostic, old, old); err != nil {
		t.Fatal(err)
	}
	backupSource := filepath.Join(root, "source.json")
	if err := os.WriteFile(backupSource, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BackupFile(backupSource, "test"); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{Timestamp: now.Add(-48 * time.Hour), Type: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendEvent(Event{Timestamp: now.Add(-time.Hour), Type: "recent"}); err != nil {
		t.Fatal(err)
	}
	report, err := store.Prune(now, RetentionPolicy{Plans: time.Hour, Transactions: 24 * time.Hour, Diagnostics: 24 * time.Hour, Events: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if report.Plans != 1 || report.Transactions != 1 || report.Diagnostics != 1 || report.Events != 1 {
		t.Fatalf("unexpected prune report: %+v", report)
	}
	events, err := store.RecentEvents(10)
	if err != nil || len(events) != 1 || events[0].Type != "recent" {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	backups, err := store.ListBackups()
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups should remain: len=%d err=%v", len(backups), err)
	}
}
