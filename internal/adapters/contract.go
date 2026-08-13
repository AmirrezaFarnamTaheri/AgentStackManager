// Package adapters defines ASM's versioned, non-authoritative target adapter
// contract. Adapters normalize observations, render projections, and propose
// operations; Resource Hub and mcplink retain all I/O and mutation authority.
package adapters

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
)

const ContractVersion = "fabric.asm.dev/adapter/v1alpha1"

type Environment struct {
	OS            string `json:"os"`
	Architecture  string `json:"architecture"`
	ProjectRoot   string `json:"projectRoot,omitempty"`
	TargetRoot    string `json:"targetRoot,omitempty"`
	Home          string `json:"home,omitempty"`
	AgyConfig     string `json:"agyConfig,omitempty"`
	TargetVersion string `json:"targetVersion,omitempty"`
}

type ObservedArtifact struct {
	ArtifactID string             `json:"artifactId"`
	Kind       artifactgraph.Kind `json:"kind"`
	Location   string             `json:"location"`
	Digest     string             `json:"digest"`
	BaseDigest string             `json:"baseDigest,omitempty"`
	Exists     bool               `json:"exists"`
	Owned      bool               `json:"owned"`
	Equivalent bool               `json:"equivalent"`
}

type DiscoverRequest struct {
	Environment Environment        `json:"environment"`
	Candidates  []ObservedArtifact `json:"candidates"`
}

type ImportRequest struct {
	Environment Environment            `json:"environment"`
	Observed    ObservedArtifact       `json:"observed"`
	Candidate   artifactgraph.Artifact `json:"candidate"`
}

type RenderRequest struct {
	Environment Environment            `json:"environment"`
	Artifact    artifactgraph.Artifact `json:"artifact"`
	SourcePath  string                 `json:"sourcePath"`
}

type PresenceMode string

const (
	PresencePresent PresenceMode = "present"
	PresenceAbsent  PresenceMode = "absent"
)

type PlanRequest struct {
	Environment Environment      `json:"environment"`
	Mode        PresenceMode     `json:"mode"`
	Rendered    RenderedArtifact `json:"rendered"`
	Observed    ObservedArtifact `json:"observed"`
	Capability  CapabilitySet    `json:"capability"`
	LossReport  LossReport       `json:"lossReport"`
}

type VerifyRequest struct {
	Environment Environment       `json:"environment"`
	Operation   ProposedOperation `json:"operation"`
	Observed    ObservedArtifact  `json:"observed"`
}

type VerificationResult struct {
	Verified bool   `json:"verified"`
	Reason   string `json:"reason"`
}

// Adapter is intentionally pure with respect to mutation. Implementations may
// normalize core-provided observations and compute projections, but they do not
// write files, execute target CLIs, or grant themselves authority.
type Adapter interface {
	ID() string
	SchemaVersion() string
	Capabilities(context.Context, Environment) (CapabilitySet, error)
	Discover(context.Context, DiscoverRequest) ([]ObservedArtifact, error)
	Import(context.Context, ImportRequest) (artifactgraph.Artifact, LossReport, error)
	Render(context.Context, RenderRequest) (RenderedSet, LossReport, error)
	Plan(context.Context, PlanRequest) ([]ProposedOperation, error)
	Verify(context.Context, VerifyRequest) (VerificationResult, error)
}

type Action string

const (
	ActionCreate   Action = "create"
	ActionUpdate   Action = "update"
	ActionRemove   Action = "remove"
	ActionNoop     Action = "noop"
	ActionConflict Action = "conflict"
)

type ProposedOperation struct {
	AdapterID        string `json:"adapterId"`
	AdapterVersion   string `json:"adapterVersion"`
	CapabilityDigest string `json:"capabilityDigest"`
	LossReportDigest string `json:"lossReportDigest"`
	Target           string `json:"target"`
	ArtifactID       string `json:"artifactId"`
	Location         string `json:"location"`
	Action           Action `json:"action"`
	BeforeDigest     string `json:"beforeDigest"`
	AfterDigest      string `json:"afterDigest,omitempty"`
	Reason           string `json:"reason"`
}

type RenderedArtifact struct {
	ArtifactID          string             `json:"artifactId"`
	Kind                artifactgraph.Kind `json:"kind"`
	SourcePath          string             `json:"sourcePath"`
	Destination         string             `json:"destination"`
	RelativeDestination string             `json:"relativeDestination"`
	DesiredDigest       string             `json:"desiredDigest"`
	Support             SupportLevel       `json:"support"`
}

type RenderedSet struct {
	APIVersion     string             `json:"apiVersion"`
	AdapterID      string             `json:"adapterId"`
	AdapterVersion string             `json:"adapterVersion"`
	Target         string             `json:"target"`
	Outputs        []RenderedArtifact `json:"outputs"`
	Digest         string             `json:"digest"`
}

func SealRenderedSet(value RenderedSet) (RenderedSet, error) {
	if value.APIVersion == "" {
		value.APIVersion = ContractVersion
	}
	value.AdapterID = strings.TrimSpace(value.AdapterID)
	value.AdapterVersion = strings.TrimSpace(value.AdapterVersion)
	value.Target = strings.TrimSpace(value.Target)
	if value.APIVersion != ContractVersion || value.AdapterID == "" || value.AdapterVersion == "" || value.Target == "" {
		return RenderedSet{}, fmt.Errorf("invalid rendered set identity")
	}
	value.Outputs = append([]RenderedArtifact(nil), value.Outputs...)
	for i := range value.Outputs {
		output := &value.Outputs[i]
		output.ArtifactID = strings.TrimSpace(output.ArtifactID)
		output.SourcePath = strings.TrimSpace(output.SourcePath)
		output.Destination = strings.TrimSpace(output.Destination)
		output.RelativeDestination = strings.ReplaceAll(strings.TrimSpace(output.RelativeDestination), "\\", "/")
		output.DesiredDigest = strings.TrimSpace(output.DesiredDigest)
		if output.ArtifactID == "" || output.Kind == "" || output.Destination == "" || output.RelativeDestination == "" || !validSHA256(output.DesiredDigest) {
			return RenderedSet{}, fmt.Errorf("invalid rendered output at index %d", i)
		}
		cleanRelative := path.Clean(output.RelativeDestination)
		if cleanRelative == "." || cleanRelative == ".." || strings.HasPrefix(cleanRelative, "../") || strings.HasPrefix(cleanRelative, "/") {
			return RenderedSet{}, fmt.Errorf("invalid rendered relative destination at index %d", i)
		}
		output.RelativeDestination = cleanRelative
		if !validSupport(output.Support) {
			return RenderedSet{}, fmt.Errorf("invalid support %q for %s", output.Support, output.ArtifactID)
		}
	}
	sort.Slice(value.Outputs, func(i, j int) bool {
		if value.Outputs[i].Destination == value.Outputs[j].Destination {
			return value.Outputs[i].ArtifactID < value.Outputs[j].ArtifactID
		}
		return value.Outputs[i].Destination < value.Outputs[j].Destination
	})
	for i := 1; i < len(value.Outputs); i++ {
		if value.Outputs[i-1].Destination == value.Outputs[i].Destination {
			return RenderedSet{}, fmt.Errorf("duplicate rendered destination %q", value.Outputs[i].Destination)
		}
	}
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return RenderedSet{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyRenderedSet(value RenderedSet) error {
	if !validSHA256(value.Digest) {
		return fmt.Errorf("invalid rendered set digest")
	}
	expected, err := SealRenderedSet(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("rendered set digest mismatch")
	}
	return nil
}
