package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/safefile"
)

var ErrMutationBusy = errors.New("another AgentStack mutation is already running")

type LeaseRecord struct {
	Name        string    `json:"name"`
	PID         int       `json:"pid"`
	Nonce       string    `json:"nonce"`
	AcquiredAt  time.Time `json:"acquiredAt"`
	HeartbeatAt time.Time `json:"heartbeatAt"`
}

type Lease struct {
	path  string
	nonce string
}

func (s Store) AcquireLease(name string, staleAfter time.Duration) (*Lease, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("lease name is empty")
	}
	if staleAfter <= 0 {
		staleAfter = 6 * time.Hour
	}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	path := filepath.Join(s.Root, "locks", safeName(name)+".lock")
	for attempt := 0; attempt < 2; attempt++ {
		nonceBytes := make([]byte, 16)
		if _, err := rand.Read(nonceBytes); err != nil {
			return nil, fmt.Errorf("generate lease nonce: %w", err)
		}
		now := time.Now().UTC()
		record := LeaseRecord{Name: name, PID: os.Getpid(), Nonce: hex.EncodeToString(nonceBytes), AcquiredAt: now, HeartbeatAt: now}
		data, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if _, err = file.Write(append(data, '\n')); err == nil {
				err = file.Sync()
			}
			closeErr := file.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				_ = os.Remove(path)
				return nil, err
			}
			return &Lease{path: path, nonce: record.Nonce}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		existing, readErr := readLeaseRecord(path)
		if readErr != nil || time.Since(existing.HeartbeatAt) > staleAfter || !processAlive(existing.PID) {
			stalePath := path + ".stale-" + time.Now().UTC().Format("20060102T150405.000000000Z")
			if renameErr := os.Rename(path, stalePath); renameErr != nil {
				return nil, fmt.Errorf("recover stale mutation lease: %w", renameErr)
			}
			continue
		}
		return nil, fmt.Errorf("%w: pid=%d since=%s", ErrMutationBusy, existing.PID, existing.AcquiredAt.Format(time.RFC3339))
	}
	return nil, ErrMutationBusy
}

func (l *Lease) Touch() error {
	if l == nil {
		return nil
	}
	record, err := readLeaseRecord(l.path)
	if err != nil {
		return err
	}
	if record.Nonce != l.nonce {
		return fmt.Errorf("mutation lease ownership changed")
	}
	record.HeartbeatAt = time.Now().UTC()
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(l.path), ".agentstack-lease-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
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
	current, err := readLeaseRecord(l.path)
	if err != nil {
		return err
	}
	if current.Nonce != l.nonce {
		return fmt.Errorf("mutation lease ownership changed before heartbeat commit")
	}
	return safefile.Replace(tempName, l.path)
}

func (l *Lease) Close() error {
	if l == nil {
		return nil
	}
	record, err := readLeaseRecord(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if record.Nonce != l.nonce {
		return fmt.Errorf("refusing to release mutation lease owned by another process")
	}
	return os.Remove(l.path)
}

func readLeaseRecord(path string) (LeaseRecord, error) {
	var record LeaseRecord
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	return record, nil
}

func safeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
