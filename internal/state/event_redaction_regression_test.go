package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendEventRedactsSecretsFromMessageBeforePersistence(t *testing.T) {
	store := NewStore(t.TempDir())
	secret := "audit-bearer-secret"
	if err := store.AppendEvent(Event{Type: "audit", Message: `Authorization: Bearer ` + secret + ` {"token":"json-secret"}`}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(store.Root, "logs", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{secret, "json-secret"} {
		if strings.Contains(string(raw), leaked) {
			t.Fatalf("event log leaked %q: %s", leaked, raw)
		}
	}
}

func TestRecentEventsRedactsLegacyUnredactedMessages(t *testing.T) {
	store := NewStore(t.TempDir())
	logPath := filepath.Join(store.Root, "logs", "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("{\"type\":\"legacy\",\"message\":\"Authorization: Bearer legacy-secret\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	events, err := store.RecentEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || strings.Contains(events[0].Message, "legacy-secret") {
		t.Fatalf("legacy event was not sanitized: %#v", events)
	}
}
