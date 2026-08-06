package external

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/strictjson"
)

type Config struct {
	Admission Admission
	Reference adapters.Adapter
}

type capabilityEvidence struct {
	raw          adapters.CapabilitySet
	effective    adapters.CapabilitySet
	intersection IntersectionReport
}

type Adapter struct {
	staged     stagedExecutable
	reference  adapters.Adapter
	descriptor Descriptor
	sequence   atomic.Uint64
	closed     atomic.Bool
	cacheMu    sync.Mutex
	cache      map[string]capabilityEvidence
}

func Open(ctx context.Context, config Config) (*Adapter, error) {
	if config.Reference == nil {
		return nil, fmt.Errorf("external adapter requires a reviewed reference adapter")
	}
	admission, err := normalizeAdmission(config.Admission)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.Reference.ID()) != admission.Target || config.Reference.SchemaVersion() != adapters.ContractVersion {
		return nil, fmt.Errorf("external adapter target does not match the reviewed reference")
	}
	staged, err := stage(admission)
	if err != nil {
		return nil, err
	}
	candidate := &Adapter{staged: staged, reference: config.Reference, cache: map[string]capabilityEvidence{}}
	cleanup := true
	defer func() {
		if cleanup {
			_ = candidate.Close()
		}
	}()
	var handshake HandshakeResult
	if err := candidate.invoke(ctx, OperationHandshake, HandshakeRequest{
		SupportedProtocols: []string{ProtocolVersion}, AdapterContract: adapters.ContractVersion,
		Target: admission.Target, Environment: admission.Environment,
	}, &handshake); err != nil {
		return nil, fmt.Errorf("external adapter handshake: %w", err)
	}
	if err := validateHandshake(handshake, admission.Target); err != nil {
		return nil, err
	}
	referenceCapability, err := config.Reference.Capabilities(ctx, admission.Environment)
	if err != nil {
		return nil, fmt.Errorf("load reviewed adapter capability: %w", err)
	}
	if err := adapters.VerifyCapabilityForAdapter(config.Reference, referenceCapability); err != nil {
		return nil, err
	}
	if handshake.AdapterID != referenceCapability.AdapterID || handshake.AdapterVersion != referenceCapability.AdapterVersion {
		return nil, fmt.Errorf("external adapter semantic identity differs from the reviewed reference")
	}
	descriptor, err := SealDescriptor(Descriptor{
		ProtocolVersion: ProtocolVersion, AdapterContract: adapters.ContractVersion,
		Target: handshake.Target, AdapterID: handshake.AdapterID, AdapterVersion: handshake.AdapterVersion,
		Aliases: handshake.Aliases, Operations: handshake.Operations,
		ExecutableDigest: staged.digest, ExecutableSize: staged.size, ArgumentsDigest: staged.argumentsDigest,
	})
	if err != nil {
		return nil, err
	}
	candidate.descriptor = descriptor
	if _, _, _, err := candidate.CapabilityEvidence(ctx, admission.Environment); err != nil {
		return nil, fmt.Errorf("external adapter capability admission: %w", err)
	}
	cleanup = false
	return candidate, nil
}

func (a *Adapter) ID() string {
	if a == nil {
		return ""
	}
	return a.descriptor.Target
}

func (a *Adapter) Aliases() []string {
	if a == nil {
		return nil
	}
	return append([]string(nil), a.descriptor.Aliases...)
}

func (a *Adapter) SchemaVersion() string { return adapters.ContractVersion }

func (a *Adapter) Descriptor() Descriptor {
	if a == nil {
		return Descriptor{}
	}
	return a.descriptor
}

func (a *Adapter) Close() error {
	if a == nil || !a.closed.CompareAndSwap(false, true) {
		return nil
	}
	return a.staged.cleanup()
}

func (a *Adapter) Capabilities(ctx context.Context, environment adapters.Environment) (adapters.CapabilitySet, error) {
	_, effective, _, err := a.CapabilityEvidence(ctx, environment)
	return effective, err
}

func (a *Adapter) CapabilityEvidence(ctx context.Context, environment adapters.Environment) (adapters.CapabilitySet, adapters.CapabilitySet, IntersectionReport, error) {
	if err := a.ensureOpen(); err != nil {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	var result CapabilitiesResult
	if err := a.invoke(ctx, OperationCapabilities, environment, &result); err != nil {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	raw := result.Capability
	if err := adapters.VerifyCapabilitySet(raw); err != nil {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("verify external capability: %w", err)
	}
	if raw.Target != a.descriptor.Target || raw.AdapterID != a.descriptor.AdapterID || raw.AdapterVersion != a.descriptor.AdapterVersion {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("external capability identity drifted from the admitted descriptor")
	}
	ceiling, err := a.reference.Capabilities(ctx, environment)
	if err != nil {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("load reviewed capability ceiling: %w", err)
	}
	if err := adapters.VerifyCapabilityForAdapter(a.reference, ceiling); err != nil {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	effective, intersection, err := IntersectCapabilities(raw, ceiling)
	if err != nil {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	key, err := integrity.DigestJSON(environment)
	if err != nil {
		return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if prior, ok := a.cache[key]; ok {
		if prior.raw.Digest != raw.Digest || prior.effective.Digest != effective.Digest || prior.intersection.Digest != intersection.Digest {
			return adapters.CapabilitySet{}, adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("external adapter capability changed during the admitted session")
		}
	} else {
		a.cache[key] = capabilityEvidence{raw: raw, effective: effective, intersection: intersection}
	}
	return raw, effective, intersection, nil
}

func (a *Adapter) Discover(ctx context.Context, request adapters.DiscoverRequest) ([]adapters.ObservedArtifact, error) {
	if err := a.ensureOpen(); err != nil {
		return nil, err
	}
	var result DiscoverResult
	if err := a.invoke(ctx, OperationDiscover, request, &result); err != nil {
		return nil, err
	}
	expected, err := normalizeObserved(request.Candidates)
	if err != nil {
		return nil, err
	}
	actual, err := normalizeObserved(result.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("validate external discovery result: %w", err)
	}
	if !reflect.DeepEqual(actual, expected) {
		return nil, fmt.Errorf("external discovery invented, removed, or changed core observations")
	}
	return actual, nil
}

func (a *Adapter) Import(ctx context.Context, request adapters.ImportRequest) (artifactgraph.Artifact, adapters.LossReport, error) {
	if err := a.ensureOpen(); err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	candidate, err := artifactgraph.Seal(request.Candidate)
	if err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	request.Candidate = candidate
	raw, effective, _, err := a.CapabilityEvidence(ctx, request.Environment)
	if err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	var result ImportResult
	if err := a.invoke(ctx, OperationImport, request, &result); err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	if err := artifactgraph.Verify(result.Artifact); err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, fmt.Errorf("verify external imported artifact: %w", err)
	}
	if result.Artifact.Digest != candidate.Digest {
		return artifactgraph.Artifact{}, adapters.LossReport{}, fmt.Errorf("external import changed the canonical candidate")
	}
	report, err := rebindLossReport(result.LossReport, raw, effective)
	if err != nil {
		return artifactgraph.Artifact{}, adapters.LossReport{}, err
	}
	return result.Artifact, report, nil
}

func (a *Adapter) Render(ctx context.Context, request adapters.RenderRequest) (adapters.RenderedSet, adapters.LossReport, error) {
	if err := a.ensureOpen(); err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	if err := artifactgraph.Verify(request.Artifact); err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	raw, effective, _, err := a.CapabilityEvidence(ctx, request.Environment)
	if err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	var result RenderResult
	if err := a.invoke(ctx, OperationRender, request, &result); err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	report, err := rebindLossReport(result.LossReport, raw, effective)
	if err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	if report.Fidelity == adapters.FidelityBlocked {
		return adapters.RenderedSet{}, report, fmt.Errorf("external adapter reports the artifact projection as blocked")
	}
	if err := validateRendered(result.Rendered, request, effective); err != nil {
		return adapters.RenderedSet{}, adapters.LossReport{}, err
	}
	return result.Rendered, report, nil
}

func (a *Adapter) Plan(ctx context.Context, request adapters.PlanRequest) ([]adapters.ProposedOperation, error) {
	if err := a.ensureOpen(); err != nil {
		return nil, err
	}
	var result PlanResult
	if err := a.invoke(ctx, OperationPlan, request, &result); err != nil {
		return nil, err
	}
	expected, err := adapters.Plan(request)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(result.Operations, expected) {
		return nil, fmt.Errorf("external adapter plan diverges from the reviewed core state machine")
	}
	return result.Operations, nil
}

func (a *Adapter) Verify(ctx context.Context, request adapters.VerifyRequest) (adapters.VerificationResult, error) {
	if err := a.ensureOpen(); err != nil {
		return adapters.VerificationResult{}, err
	}
	var result VerifyResult
	if err := a.invoke(ctx, OperationVerify, request, &result); err != nil {
		return adapters.VerificationResult{}, err
	}
	expected := adapters.Verify(request)
	if result.Verification != expected {
		return adapters.VerificationResult{}, fmt.Errorf("external adapter verification diverges from the reviewed core postcondition")
	}
	return result.Verification, nil
}

func (a *Adapter) invoke(ctx context.Context, operation Operation, payload any, destination any) error {
	if err := a.ensureOpenFor(operation); err != nil {
		return err
	}
	encodedPayload, err := strictjson.MarshalCanonical(payload)
	if err != nil {
		return fmt.Errorf("encode external adapter %s payload: %w", operation, err)
	}
	requestID := fmt.Sprintf("%s-%06d", strings.TrimPrefix(a.staged.digest, "sha256:")[:12], a.sequence.Add(1))
	deadline := invocationDeadline(ctx, a.staged.limits.Timeout)
	request := Request{
		APIVersion: ProtocolVersion, RequestID: requestID, Operation: operation,
		AdapterContract: adapters.ContractVersion, Deadline: deadline, Payload: encodedPayload,
	}
	data, err := strictjson.MarshalCanonical(request)
	if err != nil {
		return fmt.Errorf("encode external adapter request: %w", err)
	}
	stdout, _, err := a.staged.execute(ctx, data)
	if err != nil {
		return err
	}
	var response Response
	if err := strictjson.Decode(stdout, &response); err != nil {
		return fmt.Errorf("decode external adapter response: %w", err)
	}
	if response.APIVersion != ProtocolVersion || response.RequestID != requestID || response.Operation != operation {
		return fmt.Errorf("external adapter response identity mismatch")
	}
	if response.Error != nil {
		response.Error.Message = strings.TrimSpace(response.Error.Message)
		if !validErrorCode(response.Error.Code) || response.Error.Message == "" || len(response.Result) != 0 {
			return fmt.Errorf("invalid external adapter protocol error")
		}
		return response.Error
	}
	if len(response.Result) == 0 {
		return fmt.Errorf("external adapter response has neither result nor error")
	}
	if destination == nil {
		return fmt.Errorf("external adapter response destination is unavailable")
	}
	if err := strictjson.Decode(response.Result, destination); err != nil {
		return fmt.Errorf("decode external adapter %s result: %w", operation, err)
	}
	return nil
}

func (a *Adapter) ensureOpen() error {
	if a == nil || a.closed.Load() {
		return fmt.Errorf("external adapter session is closed")
	}
	return nil
}

func (a *Adapter) ensureOpenFor(operation Operation) error {
	if a == nil || a.closed.Load() {
		return fmt.Errorf("external adapter session is closed")
	}
	if !validOperation(operation) {
		return fmt.Errorf("invalid external adapter operation %q", operation)
	}
	if a.descriptor.Digest != "" && operation != OperationHandshake {
		if !containsOperation(a.descriptor.Operations, operation) {
			return fmt.Errorf("external adapter did not admit operation %q", operation)
		}
	}
	return nil
}

func invocationDeadline(ctx context.Context, limit time.Duration) time.Time {
	deadline := time.Now().Add(limit)
	if current, ok := ctx.Deadline(); ok && current.Before(deadline) {
		return current
	}
	return deadline
}

func containsOperation(values []Operation, expected Operation) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func normalizeObserved(values []adapters.ObservedArtifact) ([]adapters.ObservedArtifact, error) {
	result := append([]adapters.ObservedArtifact(nil), values...)
	seen := map[string]struct{}{}
	for i := range result {
		item := &result[i]
		item.ArtifactID = strings.TrimSpace(item.ArtifactID)
		item.Location = strings.TrimSpace(item.Location)
		item.Digest = strings.TrimSpace(item.Digest)
		item.BaseDigest = strings.TrimSpace(item.BaseDigest)
		if item.ArtifactID == "" || item.Kind == "" || item.Location == "" {
			return nil, fmt.Errorf("invalid observed artifact at index %d", i)
		}
		if item.Digest != "" && !validDigest(item.Digest) {
			return nil, fmt.Errorf("invalid observed digest at index %d", i)
		}
		if item.BaseDigest != "" && !validDigest(item.BaseDigest) {
			return nil, fmt.Errorf("invalid observed base digest at index %d", i)
		}
		key := item.ArtifactID + "\x00" + item.Location
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("duplicate observed artifact %q", item.ArtifactID)
		}
		seen[key] = struct{}{}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Location == result[j].Location {
			return result[i].ArtifactID < result[j].ArtifactID
		}
		return result[i].Location < result[j].Location
	})
	return result, nil
}

func rebindLossReport(report adapters.LossReport, raw, effective adapters.CapabilitySet) (adapters.LossReport, error) {
	if err := adapters.VerifyLossReport(report); err != nil {
		return adapters.LossReport{}, fmt.Errorf("verify external loss report: %w", err)
	}
	if report.Target != raw.Target || report.AdapterID != raw.AdapterID || report.AdapterVersion != raw.AdapterVersion || report.CapabilityDigest != raw.Digest {
		return adapters.LossReport{}, fmt.Errorf("external loss report does not bind the raw capability")
	}
	return adapters.SealLossReport(adapters.LossReport{
		Target: effective.Target, AdapterID: effective.AdapterID, AdapterVersion: effective.AdapterVersion,
		CapabilityDigest: effective.Digest, Losses: report.Losses,
	})
}

func validateRendered(value adapters.RenderedSet, request adapters.RenderRequest, capability adapters.CapabilitySet) error {
	if err := adapters.VerifyRenderedSet(value); err != nil {
		return fmt.Errorf("verify external rendered set: %w", err)
	}
	if value.AdapterID != capability.AdapterID || value.AdapterVersion != capability.AdapterVersion || value.Target != capability.Target {
		return fmt.Errorf("external rendered set identity mismatch")
	}
	if len(value.Outputs) == 0 || len(value.Outputs) > 32 {
		return fmt.Errorf("external rendered set output count is outside bounds")
	}
	root, err := filepath.Abs(request.Environment.TargetRoot)
	if err != nil {
		return err
	}
	artifactCapability, ok := capability.Artifacts[request.Artifact.Kind]
	if !ok || artifactCapability.Support == adapters.SupportUnsupported {
		return fmt.Errorf("external rendered a capability excluded by the reviewed intersection")
	}
	for i, output := range value.Outputs {
		if output.ArtifactID != request.Artifact.ID || output.Kind != request.Artifact.Kind || output.DesiredDigest != request.Artifact.Content.Digest || output.SourcePath != request.SourcePath {
			return fmt.Errorf("external rendered output %d does not bind the requested artifact", i)
		}
		if supportRank(output.Support) > supportRank(artifactCapability.Support) {
			return fmt.Errorf("external rendered output %d exceeds effective support", i)
		}
		destination, err := filepath.Abs(output.Destination)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, destination)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("external rendered output %d escapes the target root", i)
		}
		if filepath.ToSlash(filepath.Clean(relative)) != output.RelativeDestination {
			return fmt.Errorf("external rendered output %d relative destination mismatch", i)
		}
	}
	return nil
}
