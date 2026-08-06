package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

type TargetEvidence struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	Verified bool   `json:"verified"`
	Message  string `json:"message"`
}

type TargetCandidate struct {
	ID             string            `json:"id"`
	Agent          resourcehub.Agent `json:"agent"`
	Name           string            `json:"name"`
	Kind           string            `json:"kind"`
	Root           string            `json:"-"`
	Scope          string            `json:"scope,omitempty"`
	Label          string            `json:"label,omitempty"`
	Detected       bool              `json:"detected"`
	DetectionState string            `json:"detectionState"`
	SupportLevel   string            `json:"supportLevel"`
	Confidence     int               `json:"confidence"`
	Writable       bool              `json:"writable"`
	Registered     bool              `json:"registered"`
	Enabled        bool              `json:"enabled"`
	Evidence       []TargetEvidence  `json:"evidence,omitempty"`
	Message        string            `json:"message"`
}

func discoverTargetCandidates(home string, targets []resourcehub.Target) []TargetCandidate {
	absolute, _ := filepath.Abs(strings.TrimSpace(home))
	if absolute == "" {
		absolute = strings.TrimSpace(home)
	}
	byAgent := map[resourcehub.Agent][]resourcehub.Target{}
	for _, target := range targets {
		byAgent[target.Agent] = append(byAgent[target.Agent], target)
	}
	result := make([]TargetCandidate, 0, len(knownAgentEnvironments)+len(targets))
	for _, known := range knownAgentEnvironments {
		agentTargets := byAgent[known.Agent]
		sort.Slice(agentTargets, func(i, j int) bool { return agentTargets[i].ID < agentTargets[j].ID })
		if len(agentTargets) == 0 {
			result = append(result, buildTargetCandidate(absolute, known, resourcehub.Target{ID: string(known.Agent) + "-user", Agent: known.Agent, Root: absolute, Scope: "global"}, false))
			continue
		}
		for _, target := range agentTargets {
			if strings.TrimSpace(target.Root) == "" {
				target.Root = absolute
			}
			result = append(result, buildTargetCandidate(absolute, known, target, true))
		}
	}
	// Preserve explicitly registered future adapters even if this binary does not
	// yet have a catalogue descriptor for them. They remain visible and immutable.
	for agent, agentTargets := range byAgent {
		if _, ok := knownEnvironmentForAgent(agent); ok {
			continue
		}
		for _, target := range agentTargets {
			result = append(result, TargetCandidate{
				ID: target.ID, Agent: agent, Name: humanAgentName(agent), Kind: "agent", Root: target.Root,
				Scope: target.Scope, Label: target.Label, Registered: true, Enabled: target.Enabled,
				DetectionState: "unsupported", SupportLevel: "unsupported", Message: "Registered target uses an adapter that is not available in this build.",
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if strings.EqualFold(result[i].Name, result[j].Name) {
			return result[i].ID < result[j].ID
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func buildTargetCandidate(home string, known knownEnvironment, target resourcehub.Target, registered bool) TargetCandidate {
	root := strings.TrimSpace(target.Root)
	if root == "" {
		root = home
	}
	candidate := TargetCandidate{
		ID: target.ID, Agent: known.Agent, Name: known.Name, Kind: known.Kind, Root: root,
		Scope: target.Scope, Label: target.Label, Registered: registered, Enabled: target.Enabled,
		SupportLevel: known.SupportLevel, Writable: known.Writable,
	}
	if candidate.Scope == "" {
		candidate.Scope = "global"
	}
	if candidate.Label == "" {
		candidate.Label = candidate.Scope
	}

	executableFound := false
	for _, executable := range known.Executables {
		path, err := exec.LookPath(executable)
		if err != nil {
			continue
		}
		executableFound = true
		candidate.Evidence = append(candidate.Evidence, TargetEvidence{Type: "executable", Value: filepath.Base(path), Verified: true, Message: "Executable resolved on the current PATH."})
		break
	}
	configurationFound := false
	for _, marker := range known.Markers {
		markerRoot := root
		if !registered || candidate.Scope == "global" {
			markerRoot = home
		}
		path := filepath.Join(markerRoot, filepath.FromSlash(marker))
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			configurationFound = true
			candidate.Evidence = append(candidate.Evidence, TargetEvidence{Type: "configuration", Value: filepath.ToSlash(marker), Verified: true, Message: "Expected configuration directory exists."})
			break
		}
	}
	if registered {
		candidate.Evidence = append(candidate.Evidence, TargetEvidence{Type: "registration", Value: target.ID, Verified: true, Message: "Resource Hub target is registered."})
	}

	switch {
	case known.SupportLevel == "unsupported":
		candidate.DetectionState = "unsupported"
		candidate.Confidence = 0
	case executableFound && (configurationFound || registered):
		candidate.DetectionState = "confirmed"
		candidate.Detected = true
		candidate.Confidence = 100
	case executableFound:
		candidate.DetectionState = "confirmed"
		candidate.Detected = true
		candidate.Confidence = 85
	case configurationFound || registered:
		candidate.DetectionState = "configuration-only"
		candidate.Detected = configurationFound
		candidate.Confidence = 70
	default:
		candidate.DetectionState = "not-detected"
		candidate.Confidence = 0
	}

	switch {
	case candidate.Registered && candidate.Enabled:
		candidate.Message = "Connected to AgentStack-managed resources."
	case candidate.Registered:
		candidate.Message = "Connection is paused."
	case !candidate.Writable:
		candidate.Message = "Detected target is catalogued for visibility; writable adapter verification is pending."
	case candidate.Detected:
		candidate.Message = "Detected locally. Connect to share reviewed AgentStack-managed resources."
	default:
		candidate.Message = "Verified adapter available. Connect a global or project target when this application is installed."
	}
	return candidate
}

func targetCandidateForAgent(home string, agent resourcehub.Agent, targets []resourcehub.Target) (TargetCandidate, bool) {
	known, ok := knownEnvironmentForAgent(agent)
	if !ok {
		return TargetCandidate{}, false
	}
	for _, candidate := range discoverTargetCandidates(home, targets) {
		if candidate.Agent == agent {
			return candidate, true
		}
	}
	return buildTargetCandidate(home, known, resourcehub.Target{ID: string(agent) + "-user", Agent: agent, Root: home}, false), true
}

func knownEnvironmentForAgent(agent resourcehub.Agent) (knownEnvironment, bool) {
	for _, known := range knownAgentEnvironments {
		if known.Agent == agent {
			return known, true
		}
	}
	return knownEnvironment{}, false
}

func targetMarkerExists(home string, agent resourcehub.Agent) bool {
	known, ok := knownEnvironmentForAgent(agent)
	if !ok {
		return false
	}
	for _, marker := range known.Markers {
		info, err := os.Stat(filepath.Join(home, filepath.FromSlash(marker)))
		if err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func humanAgentName(agent resourcehub.Agent) string {
	value := strings.ReplaceAll(string(agent), "-", " ")
	return strings.Title(value) //nolint:staticcheck // stable UI fallback for unknown adapters
}
