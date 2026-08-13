package mcplink

import (
	"context"
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

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/mcp"
	"github.com/agentstack/agentstack/internal/runner"
	"github.com/agentstack/agentstack/internal/safefile"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const (
	maxMCPClientConfigBytes int64 = 4 << 20
	maxMCPPlanBytes         int64 = 32 << 20
	mcpBackupSchema               = "agentstack.mcplink.backup"
	mcpBackupVersion              = 1
)

type fileClientChange struct {
	entryExists    bool
	owned          bool
	equivalent     bool
	content        []byte
	action         Action
	reason         string
	previous       registration
	previousExists bool
}

type backupRecord struct {
	Schema       string        `json:"schema"`
	Version      int           `json:"version"`
	Client       ClientKind    `json:"client"`
	Path         string        `json:"path"`
	CapturedAt   time.Time     `json:"capturedAt"`
	BeforeDigest string        `json:"beforeDigest"`
	Existed      bool          `json:"existed"`
	Registration *registration `json:"registration,omitempty"`
}

type Options struct {
	ProjectRoot  string
	Home         string
	AgyConfig    string
	Executable   string
	RouterConfig string
	Commands     runner.CommandRunner
}

type Manager struct {
	Root        string
	Options     Options
	Clock       func() time.Time
	Adapters    *adapters.Registry
	beforeApply func(Operation) error
}

func New(root string, options Options) Manager {
	if options.Commands == nil {
		options.Commands = runner.ExecRunner{}
	}
	return Manager{Root: root, Options: options, Clock: func() time.Time { return time.Now().UTC() }}
}
func (m Manager) now() time.Time {
	if m.Clock == nil {
		return time.Now().UTC()
	}
	return m.Clock().UTC()
}
func (m Manager) ensure() error {
	for _, rel := range []string{"plans", "backups"} {
		if err := os.MkdirAll(filepath.Join(m.Root, rel), 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (m Manager) Plan(mode Mode, clients []ClientKind, ttl time.Duration) (Plan, error) {
	if mode != ModeLink && mode != ModeUnlink {
		return Plan{}, fmt.Errorf("unsupported MCP link mode %q", mode)
	}
	if strings.TrimSpace(m.Options.Executable) == "" || strings.TrimSpace(m.Options.RouterConfig) == "" {
		return Plan{}, fmt.Errorf("executable and router config are required")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	clients = uniqueClients(clients)
	if len(clients) == 0 {
		clients = []ClientKind{ClientCodex, ClientClaude, ClientCursor, ClientAgy, ClientOpenCode}
	}
	operations := make([]Operation, 0, len(clients))
	capabilities := make([]adapters.CapabilitySet, 0, len(clients))
	lossReports := make([]adapters.LossReport, 0, len(clients))
	for _, client := range clients {
		adapter, capability, lossReport, err := m.clientAdapter(client)
		if err != nil {
			return Plan{}, err
		}
		var operation Operation
		if client == ClientCodex {
			operation, err = m.planCodex(mode, adapter, capability, lossReport)
		} else {
			operation, err = m.planFileClient(mode, client, adapter, capability, lossReport)
		}
		if err != nil {
			return Plan{}, err
		}
		operations = append(operations, operation)
		capabilities = append(capabilities, capability)
		lossReports = append(lossReports, lossReport)
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].Client < operations[j].Client })
	sortCapabilitySnapshots(capabilities)
	sortLossReports(lossReports)
	now := m.now()
	plan := Plan{
		ID: "mcp-link-" + now.Format("20060102T150405.000000000Z"), Mode: mode,
		GeneratedAt: now, ExpiresAt: now.Add(ttl), Executable: m.Options.Executable, RouterConfig: m.Options.RouterConfig,
		AdapterContract: adapters.ContractVersion, CapabilitySnapshots: capabilities, LossReports: lossReports, Operations: operations,
	}
	digest, err := planDigest(plan)
	if err != nil {
		return Plan{}, err
	}
	plan.Digest = digest
	if err := m.ensure(); err != nil {
		return Plan{}, err
	}
	if err := writeJSON(filepath.Join(m.Root, "plans", plan.ID+".json"), plan); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (m Manager) Apply(ctx context.Context, planID, digest string, confirmed bool) (Report, error) {
	if !confirmed {
		return Report{}, fmt.Errorf("MCP client linking requires explicit confirmation")
	}
	data, err := readBoundedFile(filepath.Join(m.Root, "plans", planID+".json"), maxMCPPlanBytes)
	if err != nil {
		return Report{}, err
	}
	var plan Plan
	if err := strictjson.Decode(data, &plan); err != nil {
		return Report{}, err
	}
	if plan.ID != planID {
		return Report{}, fmt.Errorf("MCP link plan identity mismatch")
	}
	if plan.Digest != digest {
		return Report{}, fmt.Errorf("MCP link plan digest mismatch")
	}
	actual, err := planDigest(plan)
	if err != nil {
		return Report{}, err
	}
	if actual != plan.Digest {
		return Report{}, fmt.Errorf("stored MCP link plan digest is invalid")
	}
	if !m.now().Before(plan.ExpiresAt) {
		return Report{}, fmt.Errorf("MCP link plan expired")
	}
	if plan.Executable != m.Options.Executable || plan.RouterConfig != m.Options.RouterConfig {
		return Report{}, fmt.Errorf("MCP link target changed after review")
	}
	if err := m.verifyAdapterPlan(plan); err != nil {
		return Report{}, err
	}

	type fileBefore struct {
		data   []byte
		exists bool
	}
	fileState := map[string]fileBefore{}
	codexBefore := registration{}
	codexExisted := false
	for _, operation := range plan.Operations {
		if operation.Action == ActionConflict {
			return Report{}, fmt.Errorf("MCP client conflict for %s: %s", operation.Client, operation.Reason)
		}
		if operation.Action == ActionNoop {
			continue
		}
		if operation.Client == ClientCodex {
			current, exists, inspectErr := m.inspectCodex()
			if inspectErr != nil {
				return Report{}, inspectErr
			}
			currentDigest, digestErr := registrationDigest(current, exists)
			if digestErr != nil {
				return Report{}, digestErr
			}
			if currentDigest != operation.BeforeDigest {
				return Report{}, fmt.Errorf("Codex MCP registration changed after review")
			}
			codexBefore, codexExisted = current, exists
			continue
		}
		current, digestErr := fileDigest(operation.Path)
		if digestErr != nil && !errors.Is(digestErr, os.ErrNotExist) {
			return Report{}, digestErr
		}
		before := fileBefore{}
		if errors.Is(digestErr, os.ErrNotExist) {
			current = "absent"
		} else {
			before.exists = true
			before.data, digestErr = readBoundedFile(operation.Path, maxMCPClientConfigBytes)
			if digestErr != nil {
				return Report{}, digestErr
			}
		}
		if current != operation.BeforeDigest {
			return Report{}, fmt.Errorf("MCP client config changed after review: %s", operation.Path)
		}
		fileState[operation.Path] = before
	}

	type rollbackFunc func() error
	rollbacks := make([]rollbackFunc, 0, len(plan.Operations))
	rollbackAll := func() error {
		var rollbackErr error
		for index := len(rollbacks) - 1; index >= 0; index-- {
			rollbackErr = errors.Join(rollbackErr, rollbacks[index]())
		}
		return rollbackErr
	}
	fail := func(report Report, operationErr error) (Report, error) {
		if len(rollbacks) == 0 {
			return report, operationErr
		}
		report.RolledBack = true
		return report, errors.Join(operationErr, rollbackAll())
	}

	report := Report{PlanID: plan.ID, StartedAt: m.now(), LossReports: append([]adapters.LossReport(nil), plan.LossReports...)}
	for _, operation := range plan.Operations {
		if operation.Action == ActionNoop {
			report.Skipped = append(report.Skipped, operation)
			continue
		}
		if m.beforeApply != nil {
			if hookErr := m.beforeApply(operation); hookErr != nil {
				return fail(report, hookErr)
			}
		}
		if operation.Client == ClientCodex {
			backup, err := m.writeMinimalBackup(operation, codexBefore, codexExisted)
			if err != nil {
				return fail(report, err)
			}
			report.Backups = append(report.Backups, backup)
			if err := m.applyCodex(ctx, plan.Mode, operation); err != nil {
				return fail(report, err)
			}
			previous, existed := codexBefore, codexExisted
			rollbacks = append(rollbacks, func() error { return m.restoreCodex(previous, existed) })
			report.Applied = append(report.Applied, operation)
			continue
		}

		before := fileState[operation.Path]
		change, err := m.buildFileClientChange(plan.Mode, operation.Client, before.data, before.exists)
		if err != nil {
			return fail(report, err)
		}
		if change.action != operation.Action || digestBytes(change.content) != operation.AfterDigest {
			return fail(report, fmt.Errorf("MCP client operation no longer matches reviewed plan: %s", operation.Client))
		}
		backup, err := m.writeMinimalBackup(operation, change.previous, change.previousExists)
		if err != nil {
			return fail(report, err)
		}
		report.Backups = append(report.Backups, backup)
		if err := atomicWrite(operation.Path, change.content); err != nil {
			return fail(report, err)
		}
		path := operation.Path
		rollbacks = append(rollbacks, func() error {
			if before.exists {
				return atomicWrite(path, before.data)
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		})
		written, err := fileDigest(operation.Path)
		if err != nil {
			return fail(report, err)
		}
		if operation.AfterDigest != "" && written != operation.AfterDigest {
			return fail(report, fmt.Errorf("MCP client config digest mismatch: %s", operation.Path))
		}
		report.Applied = append(report.Applied, operation)
	}
	report.FinishedAt = m.now()
	_ = os.Remove(filepath.Join(m.Root, "plans", plan.ID+".json"))
	return report, nil
}

func (m Manager) restoreCodex(previous registration, existed bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, currentExists, err := m.inspectCodex()
	if err != nil {
		return err
	}
	if currentExists {
		removed := m.Options.Commands.Run(ctx, runner.Invocation{Command: "codex", Args: []string{"mcp", "remove", "agentstack-router"}, Timeout: 30 * time.Second, MaxOutputBytes: 256 << 10})
		if removed.Err != nil || removed.ExitCode != 0 {
			return fmt.Errorf("rollback Codex MCP removal: %s", resultText(removed))
		}
	}
	if !existed {
		return nil
	}
	args := []string{"mcp", "add", "agentstack-router", "--", previous.Command}
	args = append(args, previous.Args...)
	added := m.Options.Commands.Run(ctx, runner.Invocation{Command: "codex", Args: args, Timeout: 30 * time.Second, MaxOutputBytes: 256 << 10})
	if added.Err != nil || added.ExitCode != 0 {
		return fmt.Errorf("rollback Codex MCP registration: %s", resultText(added))
	}
	return nil
}

func (m Manager) planFileClient(mode Mode, client ClientKind, targetAdapter adapters.Adapter, capability adapters.CapabilitySet, lossReport adapters.LossReport) (Operation, error) {
	path := capability.MCP.Location
	if path == "" || capability.MCP.RegistrationMode != adapters.MCPRegistrationJSONFile {
		return Operation{}, fmt.Errorf("adapter %q does not expose a JSON MCP configuration path", targetAdapter.ID())
	}
	operation := Operation{Client: client, Path: path}
	data, readErr := readBoundedFile(path, maxMCPClientConfigBytes)
	before := "absent"
	exists := readErr == nil
	if readErr == nil {
		before = digestBytes(data)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return Operation{}, readErr
	}
	change, err := m.buildFileClientChange(mode, client, data, exists)
	if err != nil {
		return Operation{}, err
	}
	desiredDigest := digestBytes(change.content)
	presence := adapters.PresencePresent
	if mode == ModeUnlink {
		presence = adapters.PresenceAbsent
	}
	rendered := adapters.RenderedArtifact{
		ArtifactID: "local/MCPServer/agentstack-router", Kind: artifactgraph.KindMCPServer,
		Destination: path, RelativeDestination: filepath.ToSlash(path), DesiredDigest: desiredDigest, Support: adapters.SupportNative,
	}
	observed := adapters.ObservedArtifact{
		ArtifactID: rendered.ArtifactID, Kind: rendered.Kind, Location: path,
		Digest: before, Exists: change.entryExists, Owned: change.owned,
		Equivalent: mode == ModeLink && change.equivalent,
	}
	proposals, err := targetAdapter.Plan(context.Background(), adapters.PlanRequest{
		Environment: m.adapterEnvironment(), Mode: presence, Rendered: rendered,
		Observed: observed, Capability: capability, LossReport: lossReport,
	})
	if err != nil {
		return Operation{}, err
	}
	if len(proposals) != 1 {
		return Operation{}, fmt.Errorf("adapter %q proposed %d MCP operations", targetAdapter.ID(), len(proposals))
	}
	action, err := mcpAction(proposals[0].Action)
	if err != nil {
		return Operation{}, err
	}
	operation.BeforeDigest = before
	operation.Action = action
	operation.Reason = proposals[0].Reason
	operation.AfterDigest = desiredDigest
	applyMCPAdapterMetadata(&operation, proposals[0], lossReport)
	return operation, nil
}

func (m Manager) buildFileClientChange(mode Mode, client ClientKind, data []byte, exists bool) (fileClientChange, error) {
	root := map[string]any{}
	if exists {
		if err := strictjson.Decode(data, &root); err != nil {
			return fileClientChange{}, fmt.Errorf("decode %s MCP config: %w", client, err)
		}
	}
	servers, err := serverMap(root)
	if err != nil {
		return fileClientChange{}, fmt.Errorf("decode %s MCP servers: %w", client, err)
	}
	existing, entryExists := servers["agentstack-router"]
	expected := registrationMap(m.Options.Executable, m.Options.RouterConfig)
	change := fileClientChange{
		entryExists: entryExists,
		owned:       entryExists && ownedRegistration(existing),
		equivalent:  entryExists && equivalentRegistration(existing, expected),
	}
	if entryExists {
		if previousMap := mapValue(existing); previousMap != nil {
			change.previous, change.previousExists = parseRegistration(previousMap)
		}
	}
	if mode == ModeLink {
		switch {
		case !entryExists:
			servers["agentstack-router"] = expected
			change.action = ActionCreate
			change.reason = "AgentStack router entry is absent"
		case equivalentRegistration(existing, expected):
			change.action = ActionNoop
			change.reason = "AgentStack router entry is already equivalent"
		case ownedRegistration(existing):
			servers["agentstack-router"] = expected
			change.action = ActionUpdate
			change.reason = "stale AgentStack-owned router entry"
		default:
			change.action = ActionConflict
			change.reason = "foreign entry named agentstack-router is preserved"
		}
	} else {
		switch {
		case !entryExists:
			change.action = ActionNoop
			change.reason = "AgentStack router entry is absent"
		case ownedRegistration(existing):
			delete(servers, "agentstack-router")
			change.action = ActionRemove
			change.reason = "remove AgentStack-owned router entry"
		default:
			change.action = ActionConflict
			change.reason = "foreign entry named agentstack-router is preserved"
		}
	}
	root["mcpServers"] = servers
	content, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fileClientChange{}, err
	}
	change.content = append(content, '\n')
	if int64(len(change.content)) > maxMCPClientConfigBytes {
		return fileClientChange{}, fmt.Errorf("%s MCP config exceeds %d bytes", client, maxMCPClientConfigBytes)
	}
	return change, nil
}

func (m Manager) writeMinimalBackup(operation Operation, previous registration, existed bool) (string, error) {
	record := backupRecord{
		Schema:       mcpBackupSchema,
		Version:      mcpBackupVersion,
		Client:       operation.Client,
		Path:         operation.Path,
		CapturedAt:   m.now(),
		BeforeDigest: operation.BeforeDigest,
		Existed:      existed,
	}
	if existed {
		if !registrationOwned(previous) {
			return "", fmt.Errorf("refusing to persist unsafe MCP backup for %s", operation.Client)
		}
		copyValue := previous
		copyValue.UnsafeExtras = false
		record.Registration = &copyValue
	}
	path := m.nextBackupPath(operation.Client)
	if err := writeJSON(path, record); err != nil {
		return "", err
	}
	return path, nil
}

func (m Manager) nextBackupPath(client ClientKind) string {
	base := filepath.Join(m.Root, "backups", fmt.Sprintf("%s-%s", m.now().Format("20060102T150405.000000000Z"), client))
	for suffix := 0; ; suffix++ {
		path := base + ".json"
		if suffix > 0 {
			path = fmt.Sprintf("%s-%d.json", base, suffix)
		}
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			return path
		}
	}
}

func (m Manager) planCodex(mode Mode, targetAdapter adapters.Adapter, capability adapters.CapabilitySet, lossReport adapters.LossReport) (Operation, error) {
	if capability.MCP.RegistrationMode != adapters.MCPRegistrationCommand || capability.MCP.Location == "" {
		return Operation{}, fmt.Errorf("adapter %q does not expose command-managed MCP registration", targetAdapter.ID())
	}
	operation := Operation{Client: ClientCodex, Path: capability.MCP.Location}
	current, exists, err := m.inspectCodex()
	if err != nil {
		return Operation{}, err
	}
	beforeDigest, err := registrationDigest(current, exists)
	if err != nil {
		return Operation{}, err
	}
	expected := registration{Command: m.Options.Executable, Args: []string{"mcp-router", "--config", m.Options.RouterConfig}}
	afterDigest, err := registrationDigest(expected, mode == ModeLink)
	if err != nil {
		return Operation{}, err
	}
	presence := adapters.PresencePresent
	if mode == ModeUnlink {
		presence = adapters.PresenceAbsent
	}
	rendered := adapters.RenderedArtifact{
		ArtifactID: "local/MCPServer/agentstack-router", Kind: artifactgraph.KindMCPServer,
		Destination: capability.MCP.Location, RelativeDestination: capability.MCP.Location,
		DesiredDigest: afterDigest, Support: adapters.SupportNative,
	}
	observed := adapters.ObservedArtifact{
		ArtifactID: rendered.ArtifactID, Kind: rendered.Kind, Location: capability.MCP.Location,
		Digest: beforeDigest, Exists: exists, Owned: exists && registrationOwned(current),
		Equivalent: mode == ModeLink && exists && registrationsEquivalent(current, expected),
	}
	proposals, err := targetAdapter.Plan(context.Background(), adapters.PlanRequest{
		Environment: m.adapterEnvironment(), Mode: presence, Rendered: rendered,
		Observed: observed, Capability: capability, LossReport: lossReport,
	})
	if err != nil {
		return Operation{}, err
	}
	if len(proposals) != 1 {
		return Operation{}, fmt.Errorf("adapter %q proposed %d Codex MCP operations", targetAdapter.ID(), len(proposals))
	}
	action, err := mcpAction(proposals[0].Action)
	if err != nil {
		return Operation{}, err
	}
	operation.BeforeDigest = beforeDigest
	operation.AfterDigest = afterDigest
	operation.Action = action
	operation.Reason = proposals[0].Reason
	applyMCPAdapterMetadata(&operation, proposals[0], lossReport)
	return operation, nil
}

func (m Manager) applyCodex(ctx context.Context, mode Mode, operation Operation) error {
	current, exists, err := m.inspectCodex()
	if err != nil {
		return err
	}
	currentDigest, err := registrationDigest(current, exists)
	if err != nil {
		return err
	}
	if currentDigest != operation.BeforeDigest {
		return fmt.Errorf("Codex MCP registration changed after review")
	}
	if mode == ModeLink {
		_, err := mcp.RegisterCodex(ctx, m.Options.Commands, m.Options.Executable, m.Options.RouterConfig)
		return err
	}
	result := m.Options.Commands.Run(ctx, runner.Invocation{Command: "codex", Args: []string{"mcp", "remove", "agentstack-router"}, Timeout: 30 * time.Second, MaxOutputBytes: 256 << 10})
	if result.Err != nil || result.ExitCode != 0 {
		return fmt.Errorf("remove Codex MCP entry: %s", resultText(result))
	}
	return nil
}

func (m Manager) inspectCodex() (registration, bool, error) {
	result := m.Options.Commands.Run(context.Background(), runner.Invocation{Command: "codex", Args: []string{"mcp", "get", "agentstack-router", "--json"}, Timeout: 30 * time.Second, MaxOutputBytes: 256 << 10})
	if result.Err == nil && result.ExitCode == 0 {
		value, err := parseRegistrationJSON([]byte(result.Stdout))
		return value, true, err
	}
	message := strings.ToLower(result.Stderr + "\n" + result.Stdout)
	for _, marker := range []string{"not found", "does not exist", "unknown server", "no server named", "no mcp server"} {
		if strings.Contains(message, marker) {
			return registration{}, false, nil
		}
	}
	return registration{}, false, fmt.Errorf("inspect Codex MCP entry: %s", resultText(result))
}

func (m Manager) clientPath(client ClientKind) (string, error) {
	_, capability, _, err := m.clientAdapter(client)
	if err != nil {
		return "", err
	}
	if capability.MCP.RegistrationMode != adapters.MCPRegistrationJSONFile || capability.MCP.Location == "" {
		return "", fmt.Errorf("unsupported file MCP client %q", client)
	}
	return capability.MCP.Location, nil
}

func planDigest(plan Plan) (string, error) {
	copyValue := plan
	copyValue.Digest = ""
	return integrity.DigestJSON(copyValue)
}
func uniqueClients(input []ClientKind) []ClientKind {
	set := map[ClientKind]struct{}{}
	for _, client := range input {
		switch client {
		case ClientCodex, ClientClaude, ClientCursor, ClientAgy, ClientOpenCode:
			set[client] = struct{}{}
		}
	}
	result := make([]ClientKind, 0, len(set))
	for client := range set {
		result = append(result, client)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
func serverMap(root map[string]any) (map[string]any, error) {
	raw, ok := root["mcpServers"]
	if !ok {
		return map[string]any{}, nil
	}
	servers, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcpServers is not an object")
	}
	return servers, nil
}
func registrationMap(command, config string) map[string]any {
	return map[string]any{"command": command, "args": []any{"mcp-router", "--config", config}}
}

type registration struct {
	Command      string   `json:"command"`
	Args         []string `json:"args,omitempty"`
	UnsafeExtras bool     `json:"-"`
}

func parseRegistrationJSON(data []byte) (registration, error) {
	var root map[string]any
	if err := strictjson.Decode(data, &root); err != nil {
		return registration{}, err
	}
	for _, candidate := range []map[string]any{root, mapValue(root["transport"]), mapValue(root["config"]), mapValue(root["stdio"])} {
		if candidate == nil {
			continue
		}
		if value, ok := parseRegistration(candidate); ok {
			return value, nil
		}
	}
	return registration{}, fmt.Errorf("entry has no stdio command")
}
func parseRegistration(value map[string]any) (registration, bool) {
	command, _ := value["command"].(string)
	if command == "" {
		return registration{}, false
	}
	var args []string
	if raw, ok := value["args"].([]any); ok {
		for _, item := range raw {
			text, ok := item.(string)
			if !ok {
				return registration{}, false
			}
			args = append(args, text)
		}
	} else if raw, ok := value["args"].([]string); ok {
		args = append(args, raw...)
	}
	unsafeExtras := false
	for key, raw := range value {
		switch strings.ToLower(key) {
		case "command", "args", "type", "name", "enabled", "status":
		case "env", "environment":
			if raw != nil {
				unsafeExtras = true
			}
		default:
			unsafeExtras = true
		}
	}
	return registration{Command: command, Args: args, UnsafeExtras: unsafeExtras}, true
}
func mapValue(value any) map[string]any { result, _ := value.(map[string]any); return result }
func equivalentRegistration(existing any, expected map[string]any) bool {
	left, ok := mapValue(existing), true
	if left == nil {
		ok = false
	}
	if !ok {
		return false
	}
	a, aok := parseRegistration(left)
	b, bok := parseRegistration(expected)
	return aok && bok && registrationsEquivalent(a, b)
}
func ownedRegistration(existing any) bool {
	value := mapValue(existing)
	if value == nil {
		return false
	}
	registration, ok := parseRegistration(value)
	return ok && registrationOwned(registration)
}
func registrationsEquivalent(left, right registration) bool {
	if left.UnsafeExtras || right.UnsafeExtras || normalize(left.Command) != normalize(right.Command) || len(left.Args) != len(right.Args) {
		return false
	}
	for i := range left.Args {
		if normalizeArg(left.Args[i]) != normalizeArg(right.Args[i]) {
			return false
		}
	}
	return true
}
func registrationOwned(value registration) bool {
	if value.UnsafeExtras || len(value.Args) != 3 || !strings.EqualFold(value.Args[0], "mcp-router") || value.Args[1] != "--config" || strings.TrimSpace(value.Args[2]) == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(value.Command, "\\", "/")))
	return base == "agentstack" || base == "agentstack.exe"
}
func registrationDigest(value registration, exists bool) (string, error) {
	if !exists {
		return "absent", nil
	}
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return "", fmt.Errorf("digest MCP registration: %w", err)
	}
	return digest, nil
}
func normalize(value string) string {
	return strings.ToLower(filepath.Clean(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")))
}
func normalizeArg(value string) string {
	if strings.ContainsAny(value, "\\/") {
		return normalize(value)
	}
	return strings.ToLower(strings.TrimSpace(value))
}
func fileDigest(path string) (string, error) {
	data, err := readBoundedFile(path, maxMCPClientConfigBytes)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}
func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func resultText(result runner.Result) string {
	var parts []string
	if result.Err != nil {
		parts = append(parts, result.Err.Error())
	}
	if text := strings.TrimSpace(result.Stderr); text != "" {
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		if text := strings.TrimSpace(result.Stdout); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return fmt.Sprintf("exit code %d", result.ExitCode)
	}
	return strings.Join(parts, ": ")
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data)
}
func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".agentstack-mcplink-*.tmp")
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
func readBoundedFile(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes: %s", limit, path)
	}
	return data, nil
}
