package conformance

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
)

type RunOptions struct {
	Environment adapters.Environment
	Targets     []string
}

func RunEmbedded(ctx context.Context, registry *adapters.Registry, options RunOptions) (Report, error) {
	corpus, err := LoadEmbedded()
	if err != nil {
		return Report{}, err
	}
	return Run(ctx, registry, corpus, options)
}

func Run(ctx context.Context, registry *adapters.Registry, corpus Corpus, options RunOptions) (Report, error) {
	if registry == nil {
		return Report{}, fmt.Errorf("adapter registry is unavailable")
	}
	if err := VerifyCorpus(corpus); err != nil {
		return Report{}, err
	}
	selected, err := selectFixtures(ctx, registry, corpus, options)
	if err != nil {
		return Report{}, err
	}
	report := Report{AdapterContract: adapters.ContractVersion, CorpusDigest: corpus.Digest}
	for _, fixture := range selected {
		report.Targets = append(report.Targets, runTarget(ctx, registry, fixture, options.Environment))
	}
	return SealReport(report)
}

func selectFixtures(ctx context.Context, registry *adapters.Registry, corpus Corpus, options RunOptions) ([]TargetFixture, error) {
	byTarget := make(map[string]TargetFixture, len(corpus.Targets))
	for _, fixture := range corpus.Targets {
		byTarget[fixture.Target] = fixture
	}
	if len(options.Targets) == 0 {
		return append([]TargetFixture(nil), corpus.Targets...), nil
	}
	selected := map[string]TargetFixture{}
	for _, requested := range options.Targets {
		adapter, err := registry.Get(requested)
		if err != nil {
			return nil, err
		}
		capability, err := adapter.Capabilities(ctx, options.Environment)
		if err != nil {
			return nil, err
		}
		if err := adapters.VerifyCapabilityForAdapter(adapter, capability); err != nil {
			return nil, err
		}
		fixture, ok := byTarget[capability.Target]
		if !ok {
			return nil, fmt.Errorf("conformance corpus has no fixture for target %q", capability.Target)
		}
		selected[fixture.Target] = fixture
	}
	result := make([]TargetFixture, 0, len(selected))
	for _, fixture := range selected {
		result = append(result, fixture)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Target < result[j].Target })
	return result, nil
}

func runTarget(ctx context.Context, registry *adapters.Registry, fixture TargetFixture, environment adapters.Environment) TargetResult {
	result := TargetResult{Target: fixture.Target, AdapterID: "unknown", CapabilityDigest: "sha256:" + strings.Repeat("0", 64)}
	adapter, err := registry.Get(fixture.Target)
	if err != nil {
		result.Cases = append(result.Cases, failedCase(fixture.Target+"/registry", fixture.Target, "registry", err))
		return result
	}
	capability, err := adapter.Capabilities(ctx, environment)
	if err != nil {
		result.Cases = append(result.Cases, failedCase(fixture.Target+"/capability", fixture.Target, "capability", err))
		return result
	}
	result.AdapterID = capability.AdapterID
	result.CapabilityDigest = capability.Digest
	result.Cases = append(result.Cases, evaluateCase(fixture.Target+"/capability", fixture.Target, "capability", func() (any, error) {
		if err := adapters.VerifyCapabilityForAdapter(adapter, capability); err != nil {
			return nil, err
		}
		if err := compareCapability(environment, capability, fixture); err != nil {
			return nil, err
		}
		return capability, nil
	}))
	result.Cases = append(result.Cases, evaluateCase(fixture.Target+"/aliases", fixture.Target, "registry", func() (any, error) {
		for _, alias := range fixture.Aliases {
			resolved, err := registry.Get(alias)
			if err != nil {
				return nil, err
			}
			if resolved.ID() != adapter.ID() {
				return nil, fmt.Errorf("alias %q resolved to %q instead of %q", alias, resolved.ID(), adapter.ID())
			}
		}
		return fixture.Aliases, nil
	}))
	for _, artifactFixture := range fixture.Artifacts {
		result.Cases = append(result.Cases, runArtifactCase(ctx, adapter, capability, fixture.Target, artifactFixture, environment))
	}
	result.Cases = append(result.Cases, runPlanMatrix(ctx, adapter, capability, fixture.Target, environment))
	return result
}

func compareCapability(environment adapters.Environment, actual adapters.CapabilitySet, expected TargetFixture) error {
	if actual.Target != expected.Target || actual.AdapterID != expected.AdapterID || actual.AdapterVersion != expected.AdapterVersion || actual.TargetVersionRange != expected.TargetVersionRange {
		return fmt.Errorf("capability identity mismatch")
	}
	if !reflect.DeepEqual(actual.Aliases, expected.Aliases) {
		return fmt.Errorf("aliases mismatch: got %v want %v", actual.Aliases, expected.Aliases)
	}
	if !reflect.DeepEqual(actual.DeploymentModes, expected.DeploymentModes) {
		return fmt.Errorf("deployment modes mismatch: got %v want %v", actual.DeploymentModes, expected.DeploymentModes)
	}
	if err := compareMCP(environment, actual.MCP, expected.MCP); err != nil {
		return err
	}
	expectedSupported := map[artifactgraph.Kind]ArtifactFixture{}
	for _, fixture := range expected.Artifacts {
		if fixture.Support != adapters.SupportUnsupported {
			expectedSupported[fixture.Kind] = fixture
		}
	}
	if len(actual.Artifacts) != len(expectedSupported) {
		return fmt.Errorf("artifact capability count mismatch: got %d want %d", len(actual.Artifacts), len(expectedSupported))
	}
	for kind, fixture := range expectedSupported {
		capability, ok := actual.Artifacts[kind]
		if !ok {
			return fmt.Errorf("missing artifact capability %q", kind)
		}
		if capability.Support != fixture.Support || capability.Directory != fixture.Directory || capability.Format != fixture.Format || !reflect.DeepEqual(capability.Fields, fixture.Fields) {
			return fmt.Errorf("artifact capability %q mismatch", kind)
		}
	}
	return nil
}

func compareMCP(environment adapters.Environment, actual adapters.MCPClientCapability, expected MCPFixture) error {
	if actual.Support != expected.Support || actual.RegistrationMode != expected.RegistrationMode || actual.RootKey != expected.RootKey || actual.EntryName != expected.EntryName || !reflect.DeepEqual(actual.Transports, expected.Transports) {
		return fmt.Errorf("MCP capability structure mismatch")
	}
	var want string
	switch expected.LocationKind {
	case "literal":
		want = expected.Location
	case "project-relative":
		absolute, err := filepath.Abs(environment.ProjectRoot)
		if err != nil {
			return err
		}
		want = filepath.Join(absolute, filepath.FromSlash(expected.Location))
	case "agy-config":
		absolute, err := filepath.Abs(environment.AgyConfig)
		if err != nil {
			return err
		}
		want = absolute
	case "none":
		if actual.Location != "" {
			return fmt.Errorf("inactive MCP capability has location %q", actual.Location)
		}
		want = ""
	default:
		return fmt.Errorf("unknown MCP location kind %q", expected.LocationKind)
	}
	if filepath.Clean(actual.Location) != filepath.Clean(want) {
		return fmt.Errorf("MCP location mismatch: got %q want %q", actual.Location, want)
	}
	return nil
}

func runArtifactCase(ctx context.Context, adapter adapters.Adapter, capability adapters.CapabilitySet, target string, fixture ArtifactFixture, environment adapters.Environment) CaseResult {
	id := target + "/artifact/" + strings.ToLower(string(fixture.Kind))
	return evaluateCase(id, target, "artifact-round-trip", func() (any, error) {
		artifact, err := fixtureArtifact(fixture.Kind)
		if err != nil {
			return nil, err
		}
		rendered, report, renderErr := adapter.Render(ctx, adapters.RenderRequest{Environment: environment, Artifact: artifact, SourcePath: "/conformance/source/" + strings.ToLower(string(fixture.Kind))})
		if fixture.Support == adapters.SupportUnsupported {
			if renderErr == nil {
				return nil, fmt.Errorf("unsupported artifact rendered successfully")
			}
			if err := compareLossReport(report, fixture, artifact.ID); err != nil {
				return nil, err
			}
			return report, nil
		}
		if renderErr != nil {
			return nil, renderErr
		}
		if err := adapters.VerifyRenderedSet(rendered); err != nil {
			return nil, err
		}
		if len(rendered.Outputs) != 1 {
			return nil, fmt.Errorf("rendered %d outputs", len(rendered.Outputs))
		}
		output := rendered.Outputs[0]
		wantRelative := strings.Replace(fixture.RelativeDestination, "{{name}}", artifact.Metadata.Name, 1)
		targetRoot, err := filepath.Abs(environment.TargetRoot)
		if err != nil {
			return nil, err
		}
		wantDestination := filepath.Join(targetRoot, filepath.FromSlash(wantRelative))
		if output.RelativeDestination != wantRelative || filepath.Clean(output.Destination) != filepath.Clean(wantDestination) || output.Support != fixture.Support || output.ArtifactID != artifact.ID || output.DesiredDigest != artifact.Content.Digest {
			return nil, fmt.Errorf("rendered projection mismatch")
		}
		if err := compareLossReport(report, fixture, artifact.ID); err != nil {
			return nil, err
		}
		observed := adapters.ObservedArtifact{ArtifactID: artifact.ID, Kind: artifact.Kind, Location: output.Destination, Digest: output.DesiredDigest, BaseDigest: output.DesiredDigest, Exists: true, Owned: true, Equivalent: true}
		discovered, err := adapter.Discover(ctx, adapters.DiscoverRequest{Environment: environment, Candidates: []adapters.ObservedArtifact{{ArtifactID: artifact.ID + "-later", Kind: artifact.Kind, Location: output.Destination + ".z", Digest: output.DesiredDigest}, observed}})
		if err != nil {
			return nil, err
		}
		if len(discovered) != 2 || discovered[0].ArtifactID != artifact.ID {
			return nil, fmt.Errorf("discovery normalization is not deterministic")
		}
		imported, importReport, err := adapter.Import(ctx, adapters.ImportRequest{Environment: environment, Observed: observed, Candidate: artifact})
		if err != nil {
			return nil, err
		}
		if err := artifactgraph.Verify(imported); err != nil {
			return nil, err
		}
		if imported.Digest != artifact.Digest {
			return nil, fmt.Errorf("candidate-preserving import changed canonical digest")
		}
		if importReport.Digest != report.Digest {
			return nil, fmt.Errorf("render and import loss reports differ")
		}
		operations, err := adapter.Plan(ctx, adapters.PlanRequest{Environment: environment, Mode: adapters.PresencePresent, Rendered: output, Capability: capability, LossReport: report})
		if err != nil {
			return nil, err
		}
		if len(operations) != 1 || operations[0].Action != adapters.ActionCreate {
			return nil, fmt.Errorf("absent projection did not produce create")
		}
		verification, err := adapter.Verify(ctx, adapters.VerifyRequest{Environment: environment, Operation: operations[0], Observed: observed})
		if err != nil {
			return nil, err
		}
		if !verification.Verified {
			return nil, fmt.Errorf("postcondition verification failed: %s", verification.Reason)
		}
		return struct {
			Rendered string `json:"rendered"`
			Loss     string `json:"loss"`
			Artifact string `json:"artifact"`
		}{rendered.Digest, report.Digest, imported.Digest}, nil
	})
}

func runPlanMatrix(ctx context.Context, adapter adapters.Adapter, capability adapters.CapabilitySet, target string, environment adapters.Environment) CaseResult {
	return evaluateCase(target+"/plan-matrix", target, "plan-state-machine", func() (any, error) {
		loss, err := adapters.SealLossReport(adapters.LossReport{Target: capability.Target, AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest})
		if err != nil {
			return nil, err
		}
		desired := "sha256:" + strings.Repeat("a", 64)
		current := "sha256:" + strings.Repeat("b", 64)
		base := "sha256:" + strings.Repeat("c", 64)
		rendered := adapters.RenderedArtifact{ArtifactID: "conformance/Rule/plan", Kind: artifactgraph.KindRule, Destination: "/conformance/plan", RelativeDestination: "conformance/plan", DesiredDigest: desired, Support: adapters.SupportNative}
		cases := []struct {
			name     string
			mode     adapters.PresenceMode
			observed adapters.ObservedArtifact
			want     adapters.Action
		}{
			{"create", adapters.PresencePresent, adapters.ObservedArtifact{}, adapters.ActionCreate},
			{"noop", adapters.PresencePresent, adapters.ObservedArtifact{Exists: true, Equivalent: true, Digest: current}, adapters.ActionNoop},
			{"update", adapters.PresencePresent, adapters.ObservedArtifact{Exists: true, Owned: true, Digest: current, BaseDigest: current}, adapters.ActionUpdate},
			{"diverged", adapters.PresencePresent, adapters.ObservedArtifact{Exists: true, Owned: true, Digest: current, BaseDigest: base}, adapters.ActionConflict},
			{"foreign", adapters.PresencePresent, adapters.ObservedArtifact{Exists: true, Digest: current}, adapters.ActionConflict},
			{"remove", adapters.PresenceAbsent, adapters.ObservedArtifact{Exists: true, Owned: true, Digest: current}, adapters.ActionRemove},
			{"remove-diverged", adapters.PresenceAbsent, adapters.ObservedArtifact{Exists: true, Owned: true, Digest: current, BaseDigest: base}, adapters.ActionConflict},
			{"remove-absent", adapters.PresenceAbsent, adapters.ObservedArtifact{}, adapters.ActionNoop},
		}
		evidence := make([]adapters.ProposedOperation, 0, len(cases))
		for _, item := range cases {
			item.observed.ArtifactID = rendered.ArtifactID
			item.observed.Kind = rendered.Kind
			item.observed.Location = rendered.Destination
			operations, err := adapter.Plan(ctx, adapters.PlanRequest{Environment: environment, Mode: item.mode, Rendered: rendered, Observed: item.observed, Capability: capability, LossReport: loss})
			if err != nil {
				return nil, fmt.Errorf("%s: %w", item.name, err)
			}
			if len(operations) != 1 || operations[0].Action != item.want {
				return nil, fmt.Errorf("%s: got %v want %v", item.name, operations, item.want)
			}
			post := adapters.ObservedArtifact{ArtifactID: rendered.ArtifactID, Kind: rendered.Kind, Location: rendered.Destination}
			switch item.want {
			case adapters.ActionCreate, adapters.ActionUpdate:
				post.Exists = true
				post.Digest = operations[0].AfterDigest
			case adapters.ActionNoop:
				if item.mode == adapters.PresenceAbsent {
					post.Exists = false
				} else {
					post = item.observed
				}
			case adapters.ActionRemove:
				post.Exists = false
			case adapters.ActionConflict:
				post = item.observed
			}
			verification, err := adapter.Verify(ctx, adapters.VerifyRequest{Environment: environment, Operation: operations[0], Observed: post})
			if err != nil {
				return nil, fmt.Errorf("%s verify: %w", item.name, err)
			}
			if item.want == adapters.ActionConflict && verification.Verified {
				return nil, fmt.Errorf("%s conflict verified as executable", item.name)
			}
			if item.want != adapters.ActionConflict && !verification.Verified {
				return nil, fmt.Errorf("%s postcondition failed: %s", item.name, verification.Reason)
			}
			evidence = append(evidence, operations[0])
		}
		return evidence, nil
	})
}

func compareLossReport(actual adapters.LossReport, expected ArtifactFixture, artifactID string) error {
	if err := adapters.VerifyLossReport(actual); err != nil {
		return err
	}
	if actual.Fidelity != expected.Fidelity {
		return fmt.Errorf("fidelity mismatch: got %q want %q", actual.Fidelity, expected.Fidelity)
	}
	if len(actual.Losses) != len(expected.Losses) {
		return fmt.Errorf("loss count mismatch: got %d want %d", len(actual.Losses), len(expected.Losses))
	}
	for i, fixture := range expected.Losses {
		loss := actual.Losses[i]
		if loss.ArtifactID != artifactID || loss.Field != fixture.Field || loss.Kind != fixture.Kind || loss.Code != fixture.Code || loss.Required != fixture.Required {
			return fmt.Errorf("loss at index %d mismatch", i)
		}
	}
	return nil
}

func fixtureArtifact(kind artifactgraph.Kind) (artifactgraph.Artifact, error) {
	name := strings.ToLower(string(kind)) + "-fixture"
	execution := artifactgraph.ExecutionDeclarative
	if kind == artifactgraph.KindSkill {
		execution = artifactgraph.ExecutionSandboxed
	} else if kind == artifactgraph.KindCommand {
		execution = artifactgraph.ExecutionInterpreted
	} else if kind == artifactgraph.KindMCPServer {
		execution = artifactgraph.ExecutionPrivileged
	}
	return artifactgraph.Seal(artifactgraph.Artifact{
		ID:   "conformance/" + string(kind) + "/" + name,
		Kind: kind,
		Metadata: artifactgraph.Metadata{
			Namespace: "conformance", Name: name, DisplayName: "Conformance " + string(kind),
			Description: "adapter conformance fixture", Scope: "project",
			Tags: []string{"conformance", "fixture"}, Labels: map[string]string{"suite": "adapter"},
		},
		Content:    artifactgraph.ContentReference{Ref: "fixture://" + name, Digest: "sha256:" + strings.Repeat("d", 64), MediaType: "text/markdown"},
		Source:     artifactgraph.SourceReference{Type: "fixture", URI: "fixture://" + name, Revision: "v1"},
		Security:   artifactgraph.SecurityClassification{ExecutionClass: execution},
		Targets:    []artifactgraph.TargetBinding{{Target: "fixture", Scope: "project", Mode: "copy"}},
		Provenance: artifactgraph.Provenance{Origin: "conformance-corpus", ImportedBy: "adapter-conformance", ImportedAt: time.Unix(1, 0), UpdatedAt: time.Unix(1, 0)},
	})
}

func evaluateCase(id, target, category string, operation func() (any, error)) CaseResult {
	evidence, err := operation()
	if err != nil {
		return failedCase(id, target, category, err)
	}
	digest, err := integrity.DigestJSON(evidence)
	if err != nil {
		return failedCase(id, target, category, err)
	}
	return CaseResult{ID: id, Target: target, Category: category, Status: StatusPassed, EvidenceDigest: digest}
}

func failedCase(id, target, category string, err error) CaseResult {
	return CaseResult{ID: id, Target: target, Category: category, Status: StatusFailed, Reason: err.Error()}
}
