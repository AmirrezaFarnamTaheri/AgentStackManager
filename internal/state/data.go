package state

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/safefile"
)

const maxEventLogBytes int64 = 5 << 20

type Event struct {
	Timestamp     time.Time      `json:"timestamp"`
	Level         string         `json:"level"`
	Type          string         `json:"type"`
	CorrelationID string         `json:"correlationId,omitempty"`
	Message       string         `json:"message,omitempty"`
	Fields        map[string]any `json:"fields,omitempty"`
}

type ClearScope string

const (
	ClearOperational ClearScope = "operational"
	ClearMemory      ClearScope = "memory"
	ClearAll         ClearScope = "all"
)

func (s Store) AppendEvent(event Event) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	event.Fields = redactFields(event.Fields)
	path := filepath.Join(s.Root, "logs", "events.jsonl")
	if info, err := os.Stat(path); err == nil && info.Size() >= maxEventLogBytes {
		rotated := filepath.Join(s.Root, "logs", "events.previous.jsonl")
		if err := safefile.Replace(path, rotated); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (s Store) RecentEvents(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	path := filepath.Join(s.Root, "logs", "events.jsonl")
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []Event
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		var event Event
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			events = append(events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func redactFields(fields map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	result := make(map[string]any, len(fields))
	for key, value := range fields {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "credential") || strings.Contains(lower, "authorization") || strings.Contains(lower, "api_key") || strings.Contains(lower, "apikey") {
			result[key] = "[REDACTED]"
			continue
		}
		result[key] = redactValue(value)
	}
	return result
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactFields(typed)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = redactValue(item)
		}
		return result
	case string:
		lower := strings.ToLower(typed)
		for _, marker := range []string{"bearer ", "ghp_", "github_pat_", "sk-", "api_key=", "apikey="} {
			if strings.Contains(lower, marker) {
				return "[REDACTED]"
			}
		}
		return typed
	default:
		return value
	}
}

func (s Store) ExportData(destination string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("export destination is empty")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".agentstack-export-*.zip")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	archive := zip.NewWriter(temp)
	var files []string
	err = filepath.WalkDir(s.Root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(filepath.ToSlash(rel), "locks/") || strings.HasPrefix(filepath.ToSlash(rel), "plans/") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		archive.Close()
		temp.Close()
		return err
	}
	sort.Strings(files)
	for _, path := range files {
		rel, _ := filepath.Rel(s.Root, path)
		info, err := os.Stat(path)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		header.Modified = time.Unix(0, 0).UTC()
		header.SetMode(0o600)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			archive.Close()
			temp.Close()
			return copyErr
		}
		if closeErr != nil {
			archive.Close()
			temp.Close()
			return closeErr
		}
	}
	manifest := map[string]any{"exportedAt": time.Now().UTC(), "dataRoot": "[LOCAL_USER_DATA_ROOT]", "files": len(files)}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	writer, err := archive.Create("EXPORT-MANIFEST.json")
	if err != nil {
		archive.Close()
		temp.Close()
		return err
	}
	if _, err := writer.Write(append(manifestData, '\n')); err != nil {
		archive.Close()
		temp.Close()
		return err
	}
	if err := archive.Close(); err != nil {
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
	return replaceExport(tempName, destination)
}

func replaceExport(source, destination string) error {
	return safefile.Replace(source, destination)
}

func (s Store) ClearData(scope ClearScope) ([]string, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	var targets []string
	switch scope {
	case ClearOperational:
		targets = []string{"transactions", "plans", "logs", "diagnostics", filepath.Join("state", "inventory.json")}
	case ClearMemory:
		targets = []string{"memory"}
	case ClearAll:
		targets = []string{"transactions", "plans", "logs", "diagnostics", "memory", "router", "backups", "backup-index", filepath.Join("state", "inventory.json"), filepath.Join("state", "ownership.json")}
	default:
		return nil, fmt.Errorf("unknown clear scope %q", scope)
	}
	removed := []string{}
	for _, rel := range targets {
		path := filepath.Join(s.Root, rel)
		if err := os.RemoveAll(path); err != nil {
			return removed, err
		}
		removed = append(removed, rel)
	}
	if err := s.ensure(); err != nil {
		return removed, err
	}
	return removed, nil
}
