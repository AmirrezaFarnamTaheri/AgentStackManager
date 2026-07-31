package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/processctl"
	"github.com/agentstack/agentstack/internal/runner"
	versionutil "github.com/agentstack/agentstack/internal/version"
)

const (
	defaultProbeTimeout = 20 * time.Second
	defaultProbeLimit   = 1 << 20
)

type Locator interface {
	LookPath(name string) (string, error)
}

type osLocator struct{}

func (osLocator) LookPath(name string) (string, error) { return exec.LookPath(name) }

type CommandResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Err       error
	Truncated bool
}

type Probe interface {
	Run(ctx context.Context, command string, args ...string) CommandResult
}

type OSProbe struct {
	Timeout        time.Duration
	MaxOutputBytes int
}

func (p OSProbe) Run(parent context.Context, command string, args ...string) CommandResult {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = defaultProbeTimeout
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	limit := p.MaxOutputBytes
	if limit <= 0 {
		limit = defaultProbeLimit
	}
	cmd := exec.Command(command, args...)
	var stdout, stderr cappedBuffer
	stdout.limit, stderr.limit = limit, limit
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	process, startErr := processctl.Start(cmd)
	var err error
	if startErr != nil {
		err = startErr
	} else {
		err = process.Wait(ctx)
	}
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String(), Err: err, Truncated: stdout.truncated || stderr.truncated}
	if err == nil {
		return result
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
	} else {
		result.ExitCode = -1
	}
	return result
}

type cappedBuffer struct {
	data      bytes.Buffer
	limit     int
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.limit - b.data.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.data.Write(p[:remaining])
			b.truncated = true
		} else {
			_, _ = b.data.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return original, nil
}

func (b *cappedBuffer) String() string { return b.data.String() }

type Scanner struct {
	Locator    Locator
	Probe      Probe
	IncludeRaw bool
	Platform   string
}

func NewScanner() Scanner {
	return Scanner{Locator: osLocator{}, Probe: OSProbe{}, Platform: runtime.GOOS}
}

func (s Scanner) Scan(ctx context.Context, c model.Catalog, managed map[string]bool) model.Inventory {
	if s.Locator == nil {
		s.Locator = osLocator{}
	}
	if s.Probe == nil {
		s.Probe = OSProbe{}
	}
	result := model.Inventory{
		GeneratedAt: time.Now().UTC(),
		Items:       make(map[string]model.InventoryItem, len(c.Components)),
		External:    map[string][]model.ExternalPackage{},
	}
	if s.IncludeRaw {
		result.RawSources = map[string]string{}
	}
	for _, component := range c.Components {
		item := model.InventoryItem{ComponentID: component.ID, Managed: managed[component.ID], Compatible: true}
		if !componentSupportsPlatform(component, s.Platform) {
			item.HealthMessage = "component is not supported on this platform"
			result.Items[component.ID] = item
			continue
		}
		for _, command := range component.DetectCommands {
			path, err := s.Locator.LookPath(command)
			if err == nil {
				item.Installed = true
				item.DetectedCommand = command
				item.ExecutablePath = path
				break
			}
		}
		result.Items[component.ID] = item
	}
	s.scanNPM(ctx, &result)
	s.scanWinget(ctx, c, &result)
	s.scanUV(ctx, &result)
	reconcilePackageInventory(c, &result)
	s.applyVersionPolicies(ctx, c, &result)
	finalizeManagedInventory(c, &result)
	result.Revision = inventoryRevision(result)
	return result
}

func componentSupportsPlatform(component model.Component, platform string) bool {
	if len(component.Platforms) == 0 || platform == "" {
		return true
	}
	for _, candidate := range component.Platforms {
		if candidate == platform {
			return true
		}
	}
	return false
}

func inventoryRevision(value model.Inventory) string {
	value.GeneratedAt = time.Time{}
	value.Revision = ""
	value.RawSources = nil
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return ""
	}
	return digest
}

func finalizeManagedInventory(c model.Catalog, result *model.Inventory) {
	for _, component := range c.Components {
		item := result.Items[component.ID]
		if !item.Managed || item.Installed {
			continue
		}
		switch component.Install.Kind {
		case model.InstallRouter, model.InstallSkillPack:
			item.Broken = true
			item.HealthMessage = "AgentStack owns this component, but its configured files or router state require reconciliation"
		default:
			item.Broken = true
			item.HealthMessage = "AgentStack previously installed this component, but its command or package is no longer detectable"
		}
		result.Items[component.ID] = item
	}
}

func reconcilePackageInventory(c model.Catalog, result *model.Inventory) {
	npm := packageIndex(result.External["npm"])
	uv := packageIndex(result.External["uv"])
	winget := packageIndex(result.External["winget"])
	for _, component := range c.Components {
		item := result.Items[component.ID]
		var packages map[string]model.ExternalPackage
		var packageName string
		switch component.Install.Kind {
		case model.InstallNPMGlobal:
			packages = npm
			packageName = normalizeNPMPackage(component.Install.Package)
		case model.InstallUVTool:
			packages = uv
			packageName = normalizePythonPackage(component.Install.Package)
		case model.InstallWinget:
			packages = winget
			packageName = component.Install.WingetID
		default:
			continue
		}
		if installed, ok := packages[strings.ToLower(packageName)]; ok {
			item.Installed = true
			if item.Version == "" {
				item.Version = installed.Version
			}
			item.PackageSource = installed.Source
			item.Publisher = installed.Publisher
			result.Items[component.ID] = item
		}
	}
}

func packageIndex(packages []model.ExternalPackage) map[string]model.ExternalPackage {
	result := make(map[string]model.ExternalPackage, len(packages))
	for _, item := range packages {
		key := item.ID
		if key == "" {
			key = item.Name
		}
		result[strings.ToLower(key)] = item
	}
	return result
}

func normalizeNPMPackage(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "@") {
		if separator := strings.LastIndex(value, "@"); separator > strings.Index(value, "/") {
			return value[:separator]
		}
		return value
	}
	if separator := strings.LastIndex(value, "@"); separator > 0 {
		return value[:separator]
	}
	return value
}

func normalizePythonPackage(value string) string {
	value = strings.TrimSpace(value)
	for _, separator := range []string{"==", ">=", "<=", "~=", "!=", ">", "<"} {
		if index := strings.Index(value, separator); index >= 0 {
			value = value[:index]
			break
		}
	}
	if extra := strings.Index(value, "["); extra >= 0 {
		value = value[:extra]
	}
	return strings.TrimSpace(value)
}

func (s Scanner) applyVersionPolicies(ctx context.Context, c model.Catalog, result *model.Inventory) {
	for _, component := range c.Components {
		item := result.Items[component.ID]
		if !item.Installed || component.VersionPolicy == nil {
			continue
		}
		policy := component.VersionPolicy
		if policy.Probe.Command != "" {
			probe := s.Probe.Run(ctx, policy.Probe.Command, policy.Probe.Args...)
			if probe.Err != nil || probe.ExitCode != 0 {
				item.Compatible = false
				item.Incompatible = true
				item.HealthMessage = "version probe failed: " + compactError(probe)
				result.Items[component.ID] = item
				continue
			}
			value, err := versionutil.Extract(probe.Stdout+"\n"+probe.Stderr, policy.Pattern)
			if err != nil {
				item.Compatible = false
				item.Incompatible = true
				item.HealthMessage = err.Error()
				result.Items[component.ID] = item
				continue
			}
			item.Version = value
		}
		if item.Version == "" {
			item.Compatible = false
			item.Incompatible = true
			item.HealthMessage = "installed version could not be determined"
			result.Items[component.ID] = item
			continue
		}
		compatible, err := versionutil.Compatible(item.Version, policy.Minimum, policy.Maximum)
		if err != nil || !compatible {
			item.Compatible = false
			item.Incompatible = true
			if err != nil {
				item.HealthMessage = err.Error()
			} else {
				item.HealthMessage = fmt.Sprintf("version %s is outside supported range [%s, %s]", item.Version, policy.Minimum, policy.Maximum)
			}
		} else {
			item.Compatible = true
			item.Incompatible = false
		}
		result.Items[component.ID] = item
	}
}

func compactError(result CommandResult) string {
	message := strings.TrimSpace(result.Stderr)
	if message == "" && result.Err != nil {
		message = result.Err.Error()
	}
	if result.Truncated {
		message += " (output truncated)"
	}
	return message
}

func (s Scanner) scanNPM(ctx context.Context, result *model.Inventory) {
	if _, err := s.Locator.LookPath("npm"); err != nil {
		return
	}
	probe := s.Probe.Run(ctx, "npm", "list", "--global", "--depth=0", "--json")
	if probe.Stdout == "" {
		return
	}
	if s.IncludeRaw {
		result.RawSources["npm"] = probe.Stdout
	}
	var payload struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if json.Unmarshal([]byte(probe.Stdout), &payload) != nil {
		return
	}
	packages := make([]model.ExternalPackage, 0, len(payload.Dependencies))
	for name, info := range payload.Dependencies {
		packages = append(packages, model.ExternalPackage{Name: name, ID: name, Version: info.Version, Source: "npmjs"})
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	result.External["npm"] = packages
}

func (s Scanner) scanWinget(ctx context.Context, c model.Catalog, result *model.Inventory) {
	if _, err := s.Locator.LookPath("winget"); err != nil {
		return
	}
	tempDir, err := os.MkdirTemp("", "agentstack-winget-export-*")
	if err != nil {
		return
	}
	defer os.RemoveAll(tempDir)
	outputPath := filepath.Join(tempDir, "packages.json")
	probe := s.Probe.Run(ctx, "winget", "export", "--output", outputPath, "--include-versions", "--accept-source-agreements", "--disable-interactivity")
	data, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		data = []byte(probe.Stdout)
	}
	if len(data) == 0 {
		return
	}
	if s.IncludeRaw {
		result.RawSources["winget"] = string(data)
	}
	var payload struct {
		Sources []struct {
			SourceDetails struct {
				Name       string `json:"Name"`
				Identifier string `json:"Identifier"`
				Argument   string `json:"Argument"`
			} `json:"SourceDetails"`
			Packages []struct {
				PackageIdentifier string `json:"PackageIdentifier"`
				Version           string `json:"Version"`
			} `json:"Packages"`
		} `json:"Sources"`
	}
	if json.Unmarshal(data, &payload) != nil {
		return
	}
	catalogByID := map[string]model.Component{}
	for _, component := range c.Components {
		if component.Install.WingetID != "" {
			catalogByID[strings.ToLower(component.Install.WingetID)] = component
		}
	}
	var packages []model.ExternalPackage
	for _, source := range payload.Sources {
		sourceName := source.SourceDetails.Name
		if sourceName == "" {
			sourceName = source.SourceDetails.Identifier
		}
		for _, item := range source.Packages {
			component := catalogByID[strings.ToLower(item.PackageIdentifier)]
			packages = append(packages, model.ExternalPackage{Name: item.PackageIdentifier, ID: item.PackageIdentifier, Version: item.Version, Source: sourceName, Publisher: component.Install.Publisher})
		}
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].ID < packages[j].ID })
	result.External["winget"] = packages
}

func (s Scanner) scanUV(ctx context.Context, result *model.Inventory) {
	if _, err := s.Locator.LookPath("uv"); err != nil {
		return
	}
	probe := s.Probe.Run(ctx, "uv", "tool", "list")
	if probe.Stdout == "" {
		return
	}
	if s.IncludeRaw {
		result.RawSources["uv"] = probe.Stdout
	}
	var packages []model.ExternalPackage
	for _, line := range strings.Split(probe.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		item := model.ExternalPackage{Name: fields[0], ID: fields[0], Source: "pypi"}
		if len(fields) > 1 {
			item.Version = strings.TrimPrefix(fields[1], "v")
		}
		packages = append(packages, item)
	}
	result.External["uv"] = packages
}

// Verifier converts component-specific postconditions into the runner contract.
type Verifier struct {
	Scanner Scanner
	Catalog model.Catalog
}

func (v Verifier) Verify(ctx context.Context, component model.Component, action model.PlanAction, result runner.Result) runner.VerificationResult {
	if component.Install.Kind == model.InstallSkillPack {
		var report struct {
			Added     []string `json:"added"`
			Preserved []string `json:"preserved"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &report); err != nil {
			return runner.VerificationResult{Message: "decode skill installation report: " + err.Error()}
		}
		paths := append(append([]string(nil), report.Added...), report.Preserved...)
		if len(paths) == 0 {
			return runner.VerificationResult{Message: "skill pack produced no verified paths"}
		}
		for _, path := range paths {
			if info, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil || !info.Mode().IsRegular() {
				return runner.VerificationResult{Message: "missing verified SKILL.md at " + path}
			}
		}
		return runner.VerificationResult{OK: true, Message: fmt.Sprintf("verified %d skill directories", len(paths))}
	}
	catalog := model.Catalog{Version: v.Catalog.Version, Components: []model.Component{component}}
	scanner := v.Scanner
	if scanner.Locator == nil {
		scanner = NewScanner()
	}
	inventory := scanner.Scan(ctx, catalog, nil)
	item := inventory.Items[component.ID]
	if !item.Installed {
		return runner.VerificationResult{Message: "component remains undetected after installer success"}
	}
	if item.Incompatible {
		return runner.VerificationResult{Message: item.HealthMessage}
	}
	message := "detected installed component"
	if item.Version != "" {
		message += " version " + item.Version
	}
	return runner.VerificationResult{OK: true, Message: message}
}

func Minimized(value model.Inventory) model.Inventory {
	value.RawSources = nil
	for id, item := range value.Items {
		// Persist only the fact and version of discovery. Absolute executable
		// paths disclose user names and directory layout and are not required to
		// rebuild a plan; a fresh scan supplies them when needed.
		item.ExecutablePath = ""
		value.Items[id] = item
	}
	return value
}

func ValidatePostcondition(result runner.VerificationResult) error {
	if result.OK {
		return nil
	}
	if result.Message == "" {
		return errors.New("postcondition failed")
	}
	return errors.New(result.Message)
}
