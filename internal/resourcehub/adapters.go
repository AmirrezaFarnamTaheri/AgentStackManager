package resourcehub

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/builtin"
)

func (m Manager) adapterRegistry() *adapters.Registry {
	if m.Adapters != nil {
		return m.Adapters
	}
	return builtin.MustRegistry()
}

func targetEnvironment(target Target) adapters.Environment {
	return builtin.RuntimeEnvironment(target.Root, target.Root, "", "")
}

func (m Manager) targetCapability(target Target) (adapters.Adapter, adapters.CapabilitySet, error) {
	adapter, err := m.adapterRegistry().Get(string(target.Agent))
	if err != nil {
		return nil, adapters.CapabilitySet{}, err
	}
	capability, err := adapter.Capabilities(context.Background(), adapters.Environment{
		OS: runtime.GOOS, Architecture: runtime.GOARCH, ProjectRoot: target.Root, TargetRoot: target.Root,
	})
	if err != nil {
		return nil, adapters.CapabilitySet{}, err
	}
	if err := adapters.VerifyCapabilityForAdapter(adapter, capability); err != nil {
		return nil, adapters.CapabilitySet{}, err
	}
	return adapter, capability, nil
}

func (m Manager) renderResource(adapter adapters.Adapter, target Target, resource Resource) (adapters.RenderedArtifact, adapters.LossReport, error) {
	artifact, err := canonicalArtifact(resource)
	if err != nil {
		return adapters.RenderedArtifact{}, adapters.LossReport{}, err
	}
	rendered, report, err := adapter.Render(context.Background(), adapters.RenderRequest{
		Environment: targetEnvironment(target), Artifact: artifact, SourcePath: m.resourceSource(resource),
	})
	if err != nil {
		return adapters.RenderedArtifact{}, report, err
	}
	if err := adapters.VerifyRenderedSet(rendered); err != nil {
		return adapters.RenderedArtifact{}, adapters.LossReport{}, err
	}
	if err := adapters.VerifyLossReport(report); err != nil {
		return adapters.RenderedArtifact{}, adapters.LossReport{}, err
	}
	if len(rendered.Outputs) != 1 {
		return adapters.RenderedArtifact{}, adapters.LossReport{}, fmt.Errorf("adapter %q rendered %d outputs for resource %q; exactly one is required in Resource Hub v1", adapter.ID(), len(rendered.Outputs), resource.ID)
	}
	return rendered.Outputs[0], report, nil
}

func syncAction(value adapters.Action) (SyncAction, error) {
	switch value {
	case adapters.ActionCreate:
		return ActionCreate, nil
	case adapters.ActionUpdate:
		return ActionUpdate, nil
	case adapters.ActionRemove:
		return ActionRemove, nil
	case adapters.ActionNoop:
		return ActionNoop, nil
	case adapters.ActionConflict:
		return ActionConflict, nil
	default:
		return "", fmt.Errorf("unsupported adapter action %q", value)
	}
}

func applyAdapterMetadata(operation *SyncOperation, proposal adapters.ProposedOperation, report adapters.LossReport) {
	operation.AdapterID = proposal.AdapterID
	operation.AdapterVersion = proposal.AdapterVersion
	operation.CapabilityDigest = proposal.CapabilityDigest
	operation.LossReportDigest = proposal.LossReportDigest
	operation.Fidelity = report.Fidelity
	operation.Losses = append([]adapters.Loss(nil), report.Losses...)
}

func verifySyncOperationLoss(capability adapters.CapabilitySet, operation SyncOperation) error {
	report, err := adapters.SealLossReport(adapters.LossReport{
		Target: capability.Target, AdapterID: capability.AdapterID,
		AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest,
		Losses: append([]adapters.Loss(nil), operation.Losses...),
	})
	if err != nil {
		return err
	}
	if report.Digest != operation.LossReportDigest || report.Fidelity != operation.Fidelity {
		return fmt.Errorf("sync operation loss report mismatch at %s", operation.Destination)
	}
	return nil
}

// targetDestination remains as a compatibility helper for existing internal
// tests and callers. The authoritative projection now comes from the adapter.
func targetDestination(target Target, resource Resource) (string, error) {
	manager := Manager{}
	adapter, _, err := manager.targetCapability(target)
	if err != nil {
		return "", err
	}
	if resource.Digest == "" {
		resource.Digest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	}
	if resource.Entry == "" {
		resource.Entry = "content"
	}
	if resource.ImportedAt.IsZero() {
		resource.ImportedAt = time.Unix(1, 0).UTC()
	}
	if resource.UpdatedAt.IsZero() {
		resource.UpdatedAt = resource.ImportedAt
	}
	artifact, err := canonicalArtifact(resource)
	if err != nil {
		return "", err
	}
	rendered, _, err := adapter.Render(context.Background(), adapters.RenderRequest{
		Environment: targetEnvironment(target), Artifact: artifact,
	})
	if err != nil {
		return "", err
	}
	if len(rendered.Outputs) != 1 {
		return "", fmt.Errorf("adapter %q returned %d outputs", adapter.ID(), len(rendered.Outputs))
	}
	return rendered.Outputs[0].Destination, nil
}
