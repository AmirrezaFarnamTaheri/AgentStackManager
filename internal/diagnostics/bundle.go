package diagnostics

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/redact"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/state"
)

type Summary struct {
	GeneratedAt   time.Time `json:"generatedAt"`
	Version       string    `json:"version"`
	GoVersion     string    `json:"goVersion"`
	OS            string    `json:"os"`
	Architecture  string    `json:"architecture"`
	CatalogDigest string    `json:"catalogDigest"`
	Healthy       bool      `json:"healthy"`
}

type Input struct {
	Destination   string
	Version       string
	CatalogDigest string
	Inventory     model.Inventory
	Events        []state.Event
	Healthy       bool
}

func Create(input Input) error {
	if input.Destination == "" {
		return fmt.Errorf("diagnostic destination is empty")
	}
	if err := os.MkdirAll(filepath.Dir(input.Destination), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(input.Destination), ".agentstack-diagnostics-*.zip")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	archive := zip.NewWriter(temp)
	summary := Summary{GeneratedAt: time.Now().UTC(), Version: input.Version, GoVersion: runtime.Version(), OS: runtime.GOOS, Architecture: runtime.GOARCH, CatalogDigest: input.CatalogDigest, Healthy: input.Healthy}
	if err := writeJSON(archive, "summary.json", summary); err != nil {
		archive.Close()
		temp.Close()
		return err
	}
	if err := writeJSON(archive, "inventory.json", sanitizeInventory(input.Inventory)); err != nil {
		archive.Close()
		temp.Close()
		return err
	}
	events := make([]state.Event, len(input.Events))
	for index, event := range input.Events {
		events[index] = state.SanitizeEvent(event)
	}
	if err := writeJSON(archive, "events.json", events); err != nil {
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
	return safefile.Replace(name, input.Destination)
}

func writeJSON(archive *zip.Writer, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetModTime(time.Unix(0, 0).UTC())
	header.SetMode(0o600)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(append(data, '\n'))
	return err
}

func sanitizeInventory(value model.Inventory) model.Inventory {
	value.RawSources = nil
	items := make(map[string]model.InventoryItem, len(value.Items))
	for id, item := range value.Items {
		item.ExecutablePath = sanitizePath(item.ExecutablePath)
		items[id] = item
	}
	value.Items = items
	return value
}

func sanitizePath(value string) string {
	if value == "" {
		return ""
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	base := path.Base(normalized)
	if base == "." || base == "/" {
		return "[PATH]"
	}
	return path.Join("[PATH]", base)
}

func SHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func RedactText(value string) string {
	return redact.Text(value)
}
