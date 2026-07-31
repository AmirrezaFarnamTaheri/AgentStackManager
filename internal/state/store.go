package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/planner"
	"github.com/agentstack/agentstack/internal/redact"
	"github.com/agentstack/agentstack/internal/safefile"
)

type ManagedComponent struct {
	ID             string            `json:"id"`
	Source         string            `json:"source"`
	InstalledAt    time.Time         `json:"installedAt,omitempty"`
	LastVerified   time.Time         `json:"lastVerified,omitempty"`
	Active         bool              `json:"active,omitempty"`
	Healthy        bool              `json:"healthy,omitempty"`
	Version        string            `json:"version,omitempty"`
	ManifestDigest string            `json:"manifestDigest,omitempty"`
	InstallKind    model.InstallKind `json:"installKind,omitempty"`
	Package        string            `json:"package,omitempty"`
	WingetID       string            `json:"wingetId,omitempty"`
	PackageSource  string            `json:"packageSource,omitempty"`
	Paths          []string          `json:"paths,omitempty"`
}

type Ownership struct {
	ManagedComponents map[string]ManagedComponent `json:"managedComponents"`
}

type SavedPlan struct {
	Plan      model.Plan      `json:"plan"`
	Request   planner.Request `json:"request"`
	CreatedAt time.Time       `json:"createdAt"`
}

type BackupRecord struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Source    string    `json:"source"`
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"createdAt"`
}

type Store struct{ Root string }

func NewStore(root string) Store { return Store{Root: root} }

func (s Store) ensure() error {
	for _, dir := range []string{"state", "transactions", "plans", "backups", "backup-index", "memory", "router", "locks", "logs", "diagnostics"} {
		if err := os.MkdirAll(filepath.Join(s.Root, dir), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) SavePlan(value SavedPlan) error {
	if !validRecordID(value.Plan.ID) {
		return fmt.Errorf("plan id is empty or invalid")
	}
	if value.CreatedAt.IsZero() {
		value.CreatedAt = time.Now().UTC()
	}
	return s.writeJSON(filepath.Join(s.Root, "plans", safeName(value.Plan.ID)+".json"), value)
}

func (s Store) LoadPlan(id string) (SavedPlan, error) {
	if !validRecordID(id) {
		return SavedPlan{}, fmt.Errorf("plan id is empty or invalid")
	}
	var value SavedPlan
	err := s.readJSON(filepath.Join(s.Root, "plans", safeName(id)+".json"), &value)
	return value, err
}

func (s Store) DeletePlan(id string) error {
	if !validRecordID(id) {
		return fmt.Errorf("plan id is empty or invalid")
	}
	err := os.Remove(filepath.Join(s.Root, "plans", safeName(id)+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s Store) SaveOwnership(value Ownership) error {
	if value.ManagedComponents == nil {
		value.ManagedComponents = map[string]ManagedComponent{}
	}
	return s.writeJSON(filepath.Join(s.Root, "state", "ownership.json"), value)
}

func (s Store) LoadOwnership() (Ownership, error) {
	var value Ownership
	err := s.readJSON(filepath.Join(s.Root, "state", "ownership.json"), &value)
	if errors.Is(err, os.ErrNotExist) {
		return Ownership{ManagedComponents: map[string]ManagedComponent{}}, nil
	}
	if value.ManagedComponents == nil {
		value.ManagedComponents = map[string]ManagedComponent{}
	}
	return value, err
}

func (s Store) SaveInventory(value model.Inventory) error {
	return s.writeJSON(filepath.Join(s.Root, "state", "inventory.json"), value)
}

func (s Store) SaveTransaction(value model.Transaction) error {
	if !validRecordID(value.ID) {
		return fmt.Errorf("transaction id is empty or invalid")
	}
	return s.writeJSON(filepath.Join(s.Root, "transactions", safeName(value.ID)+".json"), minimizedTransaction(value))
}

// minimizedTransaction deliberately persists only the evidence needed to
// understand and recover an AgentStack transaction. Package-manager stdout,
// command arguments, and child-process stderr can contain access tokens,
// private paths, proxy credentials, or other user data, so they remain in the
// in-memory apply report but are never written to disk.
func minimizedTransaction(value model.Transaction) model.Transaction {
	copyValue := value
	copyValue.Actions = make([]model.TransactionAction, len(value.Actions))
	for index, action := range value.Actions {
		copyAction := action
		copyAction.Args = nil
		copyAction.Output = ""
		copyAction.OutputTruncated = action.OutputTruncated || action.Output != ""
		if action.Error != "" {
			copyAction.Error = "action failed; detailed process error output was not persisted"
		}
		copyAction.Verification = redactPersistedText(action.Verification)
		copyValue.Actions[index] = copyAction
	}
	return copyValue
}

func redactPersistedText(value string) string {
	redacted := redact.Text(value)
	if redacted != value {
		return redact.Replacement
	}
	return value
}

func validRecordID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func (s Store) LoadTransaction(id string) (model.Transaction, error) {
	var value model.Transaction
	err := s.readJSON(filepath.Join(s.Root, "transactions", safeName(id)+".json"), &value)
	return value, err
}

func (s Store) BackupFile(source, label string) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer input.Close()
	safeLabel := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(label)
	created := time.Now().UTC()
	id := safeName(created.Format("20060102T150405.000000000Z") + "-" + safeLabel)
	name := fmt.Sprintf("%s-%s", id, filepath.Base(source))
	destination := filepath.Join(s.Root, "backups", name)
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	digest, err := fileSHA256(destination)
	if err != nil {
		return "", err
	}
	record := BackupRecord{ID: id, Label: label, Source: source, Path: destination, SHA256: digest, CreatedAt: created}
	if err := s.writeJSON(filepath.Join(s.Root, "backup-index", id+".json"), record); err != nil {
		return "", err
	}
	return destination, nil
}

func (s Store) ListBackups() ([]BackupRecord, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "backup-index"))
	if err != nil {
		return nil, err
	}
	result := make([]BackupRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var record BackupRecord
		if err := s.readJSON(filepath.Join(s.Root, "backup-index", entry.Name()), &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

// ResolveBackup verifies a backup index record, ownership of the requested
// target, and the content digest without mutating the target. It is used by
// preview and restore so the reviewed restore intent is identical to the
// operation that is later applied.
func (s Store) ResolveBackup(id, target string) (BackupRecord, string, error) {
	var record BackupRecord
	if err := s.readJSON(filepath.Join(s.Root, "backup-index", safeName(id)+".json"), &record); err != nil {
		return record, "", err
	}
	if target == "" {
		target = record.Source
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return record, "", err
	}
	cleanSource, err := filepath.Abs(record.Source)
	if err != nil {
		return record, "", err
	}
	if cleanTarget != cleanSource {
		return record, "", fmt.Errorf("backup %s belongs to %s, not %s", id, cleanSource, cleanTarget)
	}
	digest, err := fileSHA256(record.Path)
	if err != nil {
		return record, "", err
	}
	if digest != record.SHA256 {
		return record, "", fmt.Errorf("backup digest mismatch: expected %s got %s", record.SHA256, digest)
	}
	return record, cleanTarget, nil
}

func (s Store) RestoreBackup(id, target string) (BackupRecord, error) {
	record, target, err := s.ResolveBackup(id, target)
	if err != nil {
		return record, err
	}
	if _, err := os.Stat(target); err == nil {
		if _, err := s.BackupFile(target, "pre-restore"); err != nil {
			return record, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return record, err
	}
	data, err := os.ReadFile(record.Path)
	if err != nil {
		return record, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return record, err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".agentstack-restore-*.tmp")
	if err != nil {
		return record, err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return record, err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return record, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return record, err
	}
	if err := temp.Close(); err != nil {
		return record, err
	}
	if err := safefile.Replace(name, target); err != nil {
		return record, err
	}
	return record, nil
}

func (s Store) RecoverIncompleteTransactions() ([]model.Transaction, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "transactions"))
	if err != nil {
		return nil, err
	}
	var recovered []model.Transaction
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var tx model.Transaction
		path := filepath.Join(s.Root, "transactions", entry.Name())
		if err := s.readJSON(path, &tx); err != nil {
			return nil, err
		}
		if tx.Status != model.TransactionRunning {
			continue
		}
		tx.Status = model.TransactionInterrupted
		tx.FinishedAt = time.Now().UTC()
		if err := s.SaveTransaction(tx); err != nil {
			return nil, err
		}
		recovered = append(recovered, tx)
	}
	return recovered, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s Store) writeJSON(path string, value any) error {
	if err := s.ensure(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".agentstack-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return safefile.Replace(tempName, path)
}

func (s Store) readJSON(path string, destination any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}
