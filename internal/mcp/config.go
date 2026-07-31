package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/model"
	"github.com/agentstack/agentstack/internal/safefile"
)

type ServerConfig struct {
	Command        string             `json:"command"`
	Args           []string           `json:"args,omitempty"`
	Env            map[string]string  `json:"env,omitempty"`
	Warm           *model.CommandSpec `json:"warm,omitempty"`
	Persistent     bool               `json:"persistent,omitempty"`
	IdleTTLSeconds int                `json:"idleTTLSeconds,omitempty"`
}

type RouterConfig struct {
	Version   int                     `json:"version"`
	Profile   string                  `json:"profile,omitempty"`
	UpdatedAt time.Time               `json:"updatedAt,omitempty"`
	Servers   map[string]ServerConfig `json:"servers"`
}

func BuildRouterConfig(c model.Catalog, plan model.Plan, dataDir string) (RouterConfig, error) {
	result := RouterConfig{Version: 1, Profile: plan.Profile, UpdatedAt: time.Now().UTC(), Servers: map[string]ServerConfig{}}
	for _, action := range plan.Actions {
		if action.Kind != model.ActionConfigure && action.Kind != model.ActionKeep {
			continue
		}
		component, ok := c.ComponentByID(action.ComponentID)
		if !ok {
			return RouterConfig{}, fmt.Errorf("plan references unknown component %q", action.ComponentID)
		}
		if component.Install.Kind != model.InstallRouter || component.Router == nil {
			continue
		}
		server := ServerConfig{
			Command:        component.Router.Command,
			Args:           append([]string(nil), component.Router.Args...),
			Warm:           component.Router.Warm,
			Persistent:     component.Router.Persistent,
			IdleTTLSeconds: component.Router.IdleTTLSeconds,
		}
		if len(component.Router.Env) > 0 {
			server.Env = make(map[string]string, len(component.Router.Env))
			for key, value := range component.Router.Env {
				server.Env[key] = expandDataPath(value, dataDir)
			}
		}
		if server.Warm != nil {
			warmCopy := *server.Warm
			warmCopy.Args = append([]string(nil), server.Warm.Args...)
			if len(server.Warm.Env) > 0 {
				warmCopy.Env = map[string]string{}
				for key, value := range server.Warm.Env {
					warmCopy.Env[key] = expandDataPath(value, dataDir)
				}
			}
			server.Warm = &warmCopy
		}
		result.Servers[component.ID] = server
	}
	return result, nil
}

func expandDataPath(value, dataDir string) string {
	const prefix = "${AGENTSTACK_DATA}"
	if !strings.Contains(value, prefix) {
		return value
	}
	if strings.HasPrefix(value, prefix+"/") {
		return filepath.Join(dataDir, filepath.FromSlash(strings.TrimPrefix(value, prefix+"/")))
	}
	return strings.ReplaceAll(value, prefix, dataDir)
}

func WriteRouterConfig(path string, config RouterConfig) error {
	if config.Version < 1 {
		return fmt.Errorf("router config version must be positive")
	}
	if config.Servers == nil {
		config.Servers = map[string]ServerConfig{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(path, data, 0o600)
}

func LoadRouterConfig(path string) (RouterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return RouterConfig{}, err
	}
	var config RouterConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return RouterConfig{}, fmt.Errorf("decode router config: %w", err)
	}
	if config.Version < 1 {
		return RouterConfig{}, fmt.Errorf("router config version must be positive")
	}
	if config.Servers == nil {
		config.Servers = map[string]ServerConfig{}
	}
	return config, nil
}

func RouterConfigEquivalent(left, right RouterConfig) bool {
	return left.Version == right.Version &&
		left.Profile == right.Profile &&
		reflect.DeepEqual(left.Servers, right.Servers)
}

type MergeResult struct {
	Changed    bool               `json:"changed"`
	Repaired   bool               `json:"repaired,omitempty"`
	Conflict   bool               `json:"conflict"`
	Status     RegistrationStatus `json:"status"`
	BackupPath string             `json:"backupPath,omitempty"`
	Path       string             `json:"path"`
}

func MergeAgyConfig(path, executable string, args []string, backupDir string) (MergeResult, error) {
	result := MergeResult{Path: path}
	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return result, fmt.Errorf("decode existing AGY config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return result, err
	}

	var servers map[string]any
	if raw, ok := root["mcpServers"]; ok {
		var valid bool
		servers, valid = raw.(map[string]any)
		if !valid {
			return result, fmt.Errorf("existing mcpServers is not an object")
		}
	} else {
		servers = map[string]any{}
		root["mcpServers"] = servers
	}
	expected := clientRegistration{Command: executable, Args: append([]string(nil), args...)}
	if raw, exists := servers["agentstack-router"]; exists {
		existingMap, ok := raw.(map[string]any)
		if !ok {
			result.Conflict = true
			result.Status = RegistrationForeignConflict
			return result, fmt.Errorf("AGY MCP registration conflict: existing agentstack-router is not an object")
		}
		existing, err := parseRegistrationMap(existingMap)
		if err != nil {
			result.Conflict = true
			result.Status = RegistrationForeignConflict
			return result, fmt.Errorf("AGY MCP registration conflict: %w", err)
		}
		if registrationsEquivalent(existing, expected) {
			result.Status = RegistrationEquivalent
			return result, nil
		}
		if !registrationOwnedByAgentStack(existing) {
			result.Conflict = true
			result.Status = RegistrationForeignConflict
			return result, fmt.Errorf("AGY MCP registration conflict: foreign agentstack-router entry was preserved")
		}
		result.Repaired = true
		result.Status = RegistrationRepaired
	} else {
		result.Status = RegistrationAdded
	}

	if _, err := os.Stat(path); err == nil {
		if err := os.MkdirAll(backupDir, 0o700); err != nil {
			return result, err
		}
		backupPath := filepath.Join(backupDir, time.Now().UTC().Format("20060102T150405.000000000Z")+"-agy-mcp_config.json")
		if err := copyFile(path, backupPath); err != nil {
			return result, err
		}
		result.BackupPath = backupPath
	}
	servers["agentstack-router"] = map[string]any{"command": executable, "args": args}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return result, err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return result, err
	}
	if err := atomicWrite(path, data, 0o600); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}

func parseRegistrationMap(value map[string]any) (clientRegistration, error) {
	command, _ := value["command"].(string)
	if command == "" {
		return clientRegistration{}, fmt.Errorf("entry has no command")
	}
	args, err := stringSlice(value["args"])
	if err != nil {
		return clientRegistration{}, err
	}
	return clientRegistration{Command: command, Args: args}, nil
}

func SortedServerNames(config RouterConfig) []string {
	names := make([]string, 0, len(config.Servers))
	for name := range config.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".agentstack-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(mode); err != nil {
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

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
