package state

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/safefile"
)

// RetentionPolicy bounds operational data while deliberately retaining backups
// and ownership state until the user explicitly removes them.
type RetentionPolicy struct {
	Plans        time.Duration
	Transactions time.Duration
	Diagnostics  time.Duration
	Events       time.Duration
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		Plans:        24 * time.Hour,
		Transactions: 30 * 24 * time.Hour,
		Diagnostics:  14 * 24 * time.Hour,
		Events:       30 * 24 * time.Hour,
	}
}

type PruneReport struct {
	Plans        int `json:"plans"`
	Transactions int `json:"transactions"`
	Diagnostics  int `json:"diagnostics"`
	Events       int `json:"events"`
}

func (s Store) Prune(now time.Time, policy RetentionPolicy) (PruneReport, error) {
	if err := s.ensure(); err != nil {
		return PruneReport{}, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var report PruneReport
	plansDir := filepath.Join(s.Root, "plans")
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		return report, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(plansDir, entry.Name())
		var saved SavedPlan
		if err := s.readJSON(path, &saved); err != nil {
			continue // preserve unreadable evidence for manual recovery
		}
		expired := !saved.Plan.ExpiresAt.IsZero() && now.After(saved.Plan.ExpiresAt)
		aged := policy.Plans > 0 && !saved.CreatedAt.IsZero() && now.Sub(saved.CreatedAt) > policy.Plans
		if expired || aged {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return report, err
			}
			report.Plans++
		}
	}

	txDir := filepath.Join(s.Root, "transactions")
	entries, err = os.ReadDir(txDir)
	if err != nil {
		return report, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(txDir, entry.Name())
		var tx model.Transaction
		if err := s.readJSON(path, &tx); err != nil {
			continue
		}
		when := tx.FinishedAt
		if when.IsZero() {
			when = tx.StartedAt
		}
		if policy.Transactions > 0 && !when.IsZero() && now.Sub(when) > policy.Transactions {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return report, err
			}
			report.Transactions++
		}
	}

	diagnosticsDir := filepath.Join(s.Root, "diagnostics")
	entries, err = os.ReadDir(diagnosticsDir)
	if err != nil {
		return report, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return report, err
		}
		if policy.Diagnostics > 0 && now.Sub(info.ModTime()) > policy.Diagnostics {
			if err := os.Remove(filepath.Join(diagnosticsDir, entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				return report, err
			}
			report.Diagnostics++
		}
	}

	if policy.Events > 0 {
		for _, name := range []string{"events.jsonl", "events.previous.jsonl"} {
			removed, err := pruneEventFile(filepath.Join(s.Root, "logs", name), now.Add(-policy.Events))
			if err != nil {
				return report, err
			}
			report.Events += removed
		}
	}
	return report, nil
}

func pruneEventFile(path string, cutoff time.Time) (int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var output bytes.Buffer
	removed := 0
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var event Event
		if json.Unmarshal(line, &event) == nil && !event.Timestamp.IsZero() && event.Timestamp.Before(cutoff) {
			removed++
			continue
		}
		output.Write(line)
		output.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil {
		return removed, err
	}
	if removed == 0 {
		return 0, nil
	}
	if output.Len() == 0 {
		return removed, os.Remove(path)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".events-retention-*.tmp")
	if err != nil {
		return removed, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return removed, err
	}
	if _, err := temp.Write(output.Bytes()); err != nil {
		temp.Close()
		return removed, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return removed, err
	}
	if err := temp.Close(); err != nil {
		return removed, err
	}
	if err := safefile.Replace(tempName, path); err != nil {
		return removed, err
	}
	return removed, nil
}
