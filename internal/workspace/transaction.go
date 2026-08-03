package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const (
	workspaceTransactionSchema     = "agentstack.workspace.transaction"
	workspaceTransactionVersion    = 1
	workspaceTransactionStaleAfter = 30 * time.Minute
)

var ErrWorkspaceTransactionInProgress = errors.New("workspace transaction is in progress")

type collectionUpdate struct {
	path string
	data []byte
}

type fileSnapshot struct {
	path   string
	data   []byte
	exists bool
}

type transactionPointer struct {
	Schema    string    `json:"schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

type transactionSnapshot struct {
	Path       string `json:"path"`
	BackupFile string `json:"backupFile,omitempty"`
	Exists     bool   `json:"exists"`
}

type transactionJournal struct {
	Schema    string                `json:"schema"`
	Version   int                   `json:"version"`
	ID        string                `json:"id"`
	CreatedAt time.Time             `json:"createdAt"`
	Snapshots []transactionSnapshot `json:"snapshots"`
}

func encodeCollection[T any](schema string, items map[string]T) ([]byte, error) {
	if items == nil {
		items = map[string]T{}
	}
	data, err := json.MarshalIndent(collectionEnvelope[T]{
		Schema:  schema,
		Version: workspaceStoreVersion,
		Items:   items,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (m Manager) commitCollections(updates ...collectionUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	if err := m.recoverPendingTransaction(false); err != nil {
		return err
	}
	journal, txDir, err := m.beginTransaction(updates)
	if err != nil {
		return err
	}
	rollback := func(operationErr error) error {
		return errors.Join(operationErr, m.restoreTransaction(journal, txDir, true))
	}
	for _, update := range updates {
		if m.beforeSave != nil {
			if err := m.beforeSave(update.path); err != nil {
				return rollback(err)
			}
		}
		if err := writeBytesAtomic(update.path, update.data); err != nil {
			return rollback(err)
		}
	}
	if err := os.Remove(m.transactionPointerPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("workspace state committed but transaction finalization failed: %w", err)
	}
	// The active pointer is the authority. Once it is removed, stale journal
	// cleanup cannot change committed state and is safe to retry later.
	_ = os.RemoveAll(txDir)
	m.cleanupOrphanTransactions()
	return nil
}

func (m Manager) beginTransaction(updates []collectionUpdate) (transactionJournal, string, error) {
	if err := os.MkdirAll(filepath.Join(m.Root, "transactions"), 0o700); err != nil {
		return transactionJournal{}, "", err
	}
	txDir, err := os.MkdirTemp(filepath.Join(m.Root, "transactions"), ".txn-")
	if err != nil {
		return transactionJournal{}, "", err
	}
	id := filepath.Base(txDir)
	journal := transactionJournal{
		Schema:    workspaceTransactionSchema,
		Version:   workspaceTransactionVersion,
		ID:        id,
		CreatedAt: time.Now().UTC(),
		Snapshots: make([]transactionSnapshot, 0, len(updates)),
	}
	seen := map[string]struct{}{}
	for index, update := range updates {
		rel, err := m.transactionRelativePath(update.path)
		if err != nil {
			_ = os.RemoveAll(txDir)
			return transactionJournal{}, "", err
		}
		if _, duplicate := seen[rel]; duplicate {
			_ = os.RemoveAll(txDir)
			return transactionJournal{}, "", fmt.Errorf("duplicate workspace transaction path %q", rel)
		}
		seen[rel] = struct{}{}
		snapshot := transactionSnapshot{Path: rel}
		data, readErr := os.ReadFile(update.path)
		if readErr == nil {
			snapshot.Exists = true
			snapshot.BackupFile = fmt.Sprintf("snapshot-%03d.bin", index)
			if err := writeBytesAtomic(filepath.Join(txDir, snapshot.BackupFile), data); err != nil {
				_ = os.RemoveAll(txDir)
				return transactionJournal{}, "", err
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			_ = os.RemoveAll(txDir)
			return transactionJournal{}, "", readErr
		}
		journal.Snapshots = append(journal.Snapshots, snapshot)
	}
	if err := writeJSONAtomic(filepath.Join(txDir, "journal.json"), journal); err != nil {
		_ = os.RemoveAll(txDir)
		return transactionJournal{}, "", err
	}
	pointer := transactionPointer{Schema: workspaceTransactionSchema, Version: workspaceTransactionVersion, ID: id, CreatedAt: journal.CreatedAt}
	if err := writeJSONAtomic(m.transactionPointerPath(), pointer); err != nil {
		_ = os.RemoveAll(txDir)
		return transactionJournal{}, "", err
	}
	return journal, txDir, nil
}

func (m Manager) recoverPendingTransaction(force bool) error {
	data, err := os.ReadFile(m.transactionPointerPath())
	if errors.Is(err, os.ErrNotExist) {
		m.cleanupOrphanTransactions()
		return nil
	}
	if err != nil {
		return err
	}
	var pointer transactionPointer
	if err := strictjson.Decode(data, &pointer); err != nil {
		return fmt.Errorf("decode workspace transaction pointer: %w", err)
	}
	if pointer.Schema != workspaceTransactionSchema || pointer.Version != workspaceTransactionVersion || !validTransactionID(pointer.ID) {
		return fmt.Errorf("invalid workspace transaction pointer")
	}
	if !force && time.Since(pointer.CreatedAt) < workspaceTransactionStaleAfter {
		return ErrWorkspaceTransactionInProgress
	}
	txDir := filepath.Join(m.Root, "transactions", pointer.ID)
	journalData, err := os.ReadFile(filepath.Join(txDir, "journal.json"))
	if err != nil {
		return fmt.Errorf("read workspace transaction journal: %w", err)
	}
	var journal transactionJournal
	if err := strictjson.Decode(journalData, &journal); err != nil {
		return fmt.Errorf("decode workspace transaction journal: %w", err)
	}
	if journal.Schema != workspaceTransactionSchema || journal.Version != workspaceTransactionVersion || journal.ID != pointer.ID {
		return fmt.Errorf("workspace transaction journal identity mismatch")
	}
	return m.restoreTransaction(journal, txDir, true)
}

func (m Manager) restoreTransaction(journal transactionJournal, txDir string, finalize bool) error {
	var restoreErr error
	for _, snapshot := range journal.Snapshots {
		target, err := m.transactionTargetPath(snapshot.Path)
		if err != nil {
			restoreErr = errors.Join(restoreErr, err)
			continue
		}
		if snapshot.Exists {
			if snapshot.BackupFile == "" || filepath.Base(snapshot.BackupFile) != snapshot.BackupFile {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("invalid workspace transaction backup for %s", snapshot.Path))
				continue
			}
			backup, err := os.ReadFile(filepath.Join(txDir, snapshot.BackupFile))
			if err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			restoreErr = errors.Join(restoreErr, writeBytesAtomic(target, backup))
			continue
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			restoreErr = errors.Join(restoreErr, err)
		}
	}
	if restoreErr != nil || !finalize {
		return restoreErr
	}
	if err := os.Remove(m.transactionPointerPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.RemoveAll(txDir)
}

func (m Manager) transactionRelativePath(path string) (string, error) {
	root, err := filepath.Abs(m.Root)
	if err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, absolute)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("workspace transaction path escapes root: %s", path)
	}
	return filepath.Clean(rel), nil
}

func (m Manager) transactionTargetPath(rel string) (string, error) {
	if rel == "" || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(filepath.Clean(rel), ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid workspace transaction target %q", rel)
	}
	return filepath.Join(m.Root, filepath.Clean(rel)), nil
}

func (m Manager) transactionPointerPath() string {
	return filepath.Join(m.Root, "active-transaction.json")
}

func (m Manager) cleanupOrphanTransactions() {
	root := filepath.Join(m.Root, "transactions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	active := ""
	if data, err := os.ReadFile(m.transactionPointerPath()); err == nil {
		var pointer transactionPointer
		if strictjson.Decode(data, &pointer) == nil {
			active = pointer.ID
		}
	}
	cutoff := time.Now().UTC().Add(-workspaceTransactionStaleAfter)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == active {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func validTransactionID(id string) bool {
	return id != "" && filepath.Base(id) == id && strings.HasPrefix(id, ".txn-")
}

func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeBytesAtomic(path, append(data, '\n'))
}

func writeBytesAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".agentstack-workspace-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := safefile.Replace(name, path); err != nil {
		return fmt.Errorf("commit workspace state %s: %w", filepath.Base(path), err)
	}
	return nil
}
