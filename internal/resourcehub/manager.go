package resourcehub

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"

	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

type Manager struct {
	Root                   string
	Clock                  func() time.Time
	Adapters               *adapters.Registry
	beforeSyncOperation    func(SyncOperation) error
	beforeRefreshOperation func(RefreshOperation) error
}

const (
	maxResourceRegistryBytes = 16 << 20
	maxResourcePlanBytes     = 32 << 20
	maxResourceMetadataBytes = 1 << 20
	maxManagedStateBytes     = 32 << 20
)

func New(root string) Manager {
	return Manager{Root: root, Clock: func() time.Time { return time.Now().UTC() }}
}

func (m Manager) now() time.Time {
	if m.Clock == nil {
		return time.Now().UTC()
	}
	return m.Clock().UTC()
}

func (m Manager) ensure() error {
	for _, rel := range []string{"resources", "plans", "sync-state", "backups", "locks"} {
		if err := os.MkdirAll(filepath.Join(m.Root, rel), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (m Manager) registryPath() string { return filepath.Join(m.Root, "registry.json") }

func (m Manager) LoadRegistry() (Registry, error) {
	if err := m.ensure(); err != nil {
		return Registry{}, err
	}
	data, err := safefile.ReadBoundedRegular(m.registryPath(), maxResourceRegistryBytes)
	if errors.Is(err, os.ErrNotExist) {
		return Registry{Version: 1, Resources: map[string]Resource{}, Targets: map[string]Target{}}, nil
	}
	if err != nil {
		return Registry{}, err
	}
	var registry Registry
	if err := strictjson.Decode(data, &registry); err != nil {
		return Registry{}, fmt.Errorf("decode resource registry: %w", err)
	}
	if registry.Version != 1 {
		return Registry{}, fmt.Errorf("unsupported resource registry version %d", registry.Version)
	}
	if registry.Resources == nil {
		registry.Resources = map[string]Resource{}
	}
	if registry.Targets == nil {
		registry.Targets = map[string]Target{}
	}
	if err := validateRegistry(registry); err != nil {
		return Registry{}, err
	}
	return registry, nil
}

func (m Manager) saveRegistry(registry Registry) error {
	registry.Version = 1
	registry.UpdatedAt = m.now()
	if registry.Resources == nil {
		registry.Resources = map[string]Resource{}
	}
	if registry.Targets == nil {
		registry.Targets = map[string]Target{}
	}
	if err := validateRegistry(registry); err != nil {
		return err
	}
	return writeJSON(m.registryPath(), registry)
}

func validateRegistry(registry Registry) error {
	if len(registry.Resources) > maxResourceFiles || len(registry.Targets) > maxResourceFiles {
		return fmt.Errorf("resource registry exceeds %d entries", maxResourceFiles)
	}
	for key, resource := range registry.Resources {
		if key != resource.ID || !validID(resource.ID) {
			return fmt.Errorf("resource registry key %q does not match a valid resource id", key)
		}
		if err := validateKind(resource.Kind); err != nil {
			return fmt.Errorf("resource %q: %w", key, err)
		}
		if resource.Entry != "content" {
			return fmt.Errorf("resource %q has invalid entry path %q", key, resource.Entry)
		}
		if !validSHA256Digest(resource.Digest) {
			return fmt.Errorf("resource %q has invalid digest", key)
		}
		for _, agent := range resource.Targets {
			if err := validateAgent(agent); err != nil {
				return fmt.Errorf("resource %q: %w", key, err)
			}
		}
	}
	for key, target := range registry.Targets {
		if key != target.ID || !validID(target.ID) {
			return fmt.Errorf("resource target key %q does not match a valid target id", key)
		}
		if err := validateAgent(target.Agent); err != nil {
			return fmt.Errorf("target %q: %w", key, err)
		}
		if err := validateMode(target.Mode); err != nil {
			return fmt.Errorf("target %q: %w", key, err)
		}
		if strings.ContainsRune(target.Root, '\x00') || !filepath.IsAbs(target.Root) {
			return fmt.Errorf("target %q root must be an absolute path", key)
		}
	}
	return nil
}

func validSHA256Digest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	for _, r := range strings.TrimPrefix(value, "sha256:") {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (m Manager) Import(source string, options ImportOptions) (Resource, error) {
	if !validID(options.ID) {
		return Resource{}, fmt.Errorf("resource id is empty or invalid")
	}
	if err := validateKind(options.Kind); err != nil {
		return Resource{}, err
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return Resource{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return Resource{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Resource{}, fmt.Errorf("refusing symlink resource source")
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return Resource{}, fmt.Errorf("resource source must be a regular file or directory")
	}
	registry, err := m.LoadRegistry()
	if err != nil {
		return Resource{}, err
	}
	previous, exists := registry.Resources[options.ID]
	if exists && !options.Replace {
		return Resource{}, fmt.Errorf("resource %q already exists", options.ID)
	}
	resourceDir := filepath.Join(m.Root, "resources", options.ID)
	stage, err := os.MkdirTemp(filepath.Join(m.Root, "resources"), ".import-*")
	if err != nil {
		return Resource{}, err
	}
	defer os.RemoveAll(stage)
	content := filepath.Join(stage, "content")
	if info.IsDir() {
		if err := copyTree(absolute, content); err != nil {
			return Resource{}, err
		}
	} else {
		if err := os.MkdirAll(content, 0o700); err != nil {
			return Resource{}, err
		}
		if err := copyFile(absolute, filepath.Join(content, filepath.Base(absolute)), info.Mode().Perm()); err != nil {
			return Resource{}, err
		}
	}
	digest, err := treeDigest(content)
	if err != nil {
		return Resource{}, err
	}
	now := m.now()
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = options.ID
	}
	importedAt := now
	if exists {
		importedAt = previous.ImportedAt
	}
	trackedSource := absolute
	if strings.TrimSpace(options.trackedSource) != "" {
		trackedSource = filepath.Clean(options.trackedSource)
	}
	resource := Resource{
		ID: options.ID, Kind: options.Kind, Name: name, Description: strings.TrimSpace(options.Description),
		Digest: digest, Entry: "content", Source: trackedSource, Tags: uniqueStrings(options.Tags), Targets: uniqueAgents(options.Targets),
		Scope: strings.TrimSpace(options.Scope), Enabled: true, ImportedAt: importedAt, UpdatedAt: now, Metadata: cloneMap(options.Metadata),
	}
	if err := writeJSON(filepath.Join(stage, "resource.json"), resource); err != nil {
		return Resource{}, err
	}
	backup := ""
	if exists {
		backup = m.nextBackupPath(options.ID, "replace", now)
		if err := os.Rename(resourceDir, backup); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return Resource{}, err
			}
			backup = ""
		}
	}
	if err := os.Rename(stage, resourceDir); err != nil {
		if backup != "" {
			_ = os.Rename(backup, resourceDir)
		}
		return Resource{}, err
	}
	registry.Resources[resource.ID] = resource
	if err := m.saveRegistry(registry); err != nil {
		rollbackErr := os.RemoveAll(resourceDir)
		if backup != "" {
			rollbackErr = errors.Join(rollbackErr, os.Rename(backup, resourceDir))
		}
		return Resource{}, errors.Join(err, rollbackErr)
	}
	return resource, nil
}

func (m Manager) RegisterTarget(target Target) error {
	if !validID(target.ID) {
		return fmt.Errorf("target id is empty or invalid")
	}
	if err := validateAgent(target.Agent); err != nil {
		return err
	}
	if err := validateMode(target.Mode); err != nil {
		return err
	}
	root, err := filepath.Abs(target.Root)
	if err != nil {
		return err
	}
	if target.Mode == "" {
		target.Mode = ModeAuto
	}
	target.Root = root
	registry, err := m.LoadRegistry()
	if err != nil {
		return err
	}
	registry.Targets[target.ID] = target
	return m.saveRegistry(registry)
}

func (m Manager) RemoveResource(id string) error {
	if !validID(id) {
		return fmt.Errorf("resource id is empty or invalid")
	}
	registry, err := m.LoadRegistry()
	if err != nil {
		return err
	}
	if _, ok := registry.Resources[id]; !ok {
		return nil
	}
	resourceDir := filepath.Join(m.Root, "resources", id)
	backup := m.nextBackupPath(id, "remove", m.now())
	if err := os.Rename(resourceDir, backup); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		backup = ""
	}
	delete(registry.Resources, id)
	if err := m.saveRegistry(registry); err != nil {
		if backup != "" {
			return errors.Join(err, os.Rename(backup, resourceDir))
		}
		return err
	}
	return nil
}

func (m Manager) nextBackupPath(id, operation string, now time.Time) string {
	base := filepath.Join(m.Root, "backups", fmt.Sprintf("%s-%s-%s", now.UTC().Format("20060102T150405.000000000Z"), operation, id))
	for suffix := 0; ; suffix++ {
		candidate := base
		if suffix > 0 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func (m Manager) ListResources() ([]Resource, error) {
	registry, err := m.LoadRegistry()
	if err != nil {
		return nil, err
	}
	items := make([]Resource, 0, len(registry.Resources))
	for _, item := range registry.Resources {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind == items[j].Kind {
			return items[i].ID < items[j].ID
		}
		return items[i].Kind < items[j].Kind
	})
	return items, nil
}

func (m Manager) ListTargets() ([]Target, error) {
	registry, err := m.LoadRegistry()
	if err != nil {
		return nil, err
	}
	items := make([]Target, 0, len(registry.Targets))
	for _, item := range registry.Targets {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func (m Manager) resourceSource(resource Resource) string {
	return filepath.Join(m.Root, "resources", resource.ID, resource.Entry)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".agentstack-resourcehub-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
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
	return safefile.Replace(name, path)
}

func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func uniqueStrings(input []string) []string {
	seen := map[string]struct{}{}
	var result []string
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func uniqueAgents(input []Agent) []Agent {
	seen := map[Agent]struct{}{}
	var result []Agent
	for _, item := range input {
		if validateAgent(item) != nil {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func (m Manager) ListBackups() ([]BackupInfo, error) {
	if err := m.ensure(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(m.Root, "backups"))
	if err != nil {
		return nil, err
	}
	backups := make([]BackupInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		root := filepath.Join(m.Root, "backups", entry.Name())
		data, err := safefile.ReadBoundedRegular(filepath.Join(root, "resource.json"), maxResourceMetadataBytes)
		if err != nil {
			return nil, fmt.Errorf("read resource backup %s: %w", entry.Name(), err)
		}
		var resource Resource
		if err := strictjson.Decode(data, &resource); err != nil {
			return nil, fmt.Errorf("decode resource backup %s: %w", entry.Name(), err)
		}
		digest, err := treeDigest(filepath.Join(root, resource.Entry))
		if err != nil {
			return nil, fmt.Errorf("digest resource backup %s: %w", entry.Name(), err)
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		backups = append(backups, BackupInfo{ID: entry.Name(), ResourceID: resource.ID, Digest: digest, CreatedAt: info.ModTime().UTC(), Path: root})
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].CreatedAt.Equal(backups[j].CreatedAt) {
			return backups[i].ID > backups[j].ID
		}
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})
	return backups, nil
}

func (m Manager) RestoreBackup(backupID string, confirmed bool) (Resource, error) {
	if !confirmed {
		return Resource{}, fmt.Errorf("resource backup restore requires explicit confirmation")
	}
	if !validID(backupID) || filepath.Base(backupID) != backupID {
		return Resource{}, fmt.Errorf("resource backup id is empty or invalid")
	}
	root := filepath.Join(m.Root, "backups", backupID)
	info, err := os.Lstat(root)
	if err != nil {
		return Resource{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Resource{}, fmt.Errorf("resource backup is not a directory")
	}
	data, err := safefile.ReadBoundedRegular(filepath.Join(root, "resource.json"), maxResourceMetadataBytes)
	if err != nil {
		return Resource{}, err
	}
	var resource Resource
	if err := strictjson.Decode(data, &resource); err != nil {
		return Resource{}, err
	}
	source := filepath.Join(root, resource.Entry)
	digest, err := treeDigest(source)
	if err != nil {
		return Resource{}, err
	}
	if digest != resource.Digest {
		return Resource{}, fmt.Errorf("resource backup digest mismatch")
	}
	return m.Import(source, ImportOptions{
		ID: resource.ID, Kind: resource.Kind, Name: resource.Name, Description: resource.Description,
		Tags: resource.Tags, Targets: resource.Targets, Scope: resource.Scope, Metadata: resource.Metadata, Replace: true,
		trackedSource: resource.Source,
	})
}

// ResourceContentPath returns the current authoritative content path for one
// Resource Hub record. It is a read-only migration aid; callers must not
// mutate the returned path outside Resource Hub operations.
func (m Manager) ResourceContentPath(id string) (string, error) {
	if !validID(id) {
		return "", fmt.Errorf("resource id is empty or invalid")
	}
	registry, err := m.LoadRegistry()
	if err != nil {
		return "", err
	}
	resource, ok := registry.Resources[id]
	if !ok {
		return "", fmt.Errorf("resource %q not found", id)
	}
	return m.resourceSource(resource), nil
}
