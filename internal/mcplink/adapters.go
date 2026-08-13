package mcplink

import (
	"context"
	"fmt"
	"runtime"
	"sort"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/builtin"
)

func (m Manager) adapterRegistry() *adapters.Registry {
	if m.Adapters != nil {
		return m.Adapters
	}
	return builtin.MustRegistry()
}

func (m Manager) adapterEnvironment() adapters.Environment {
	return adapters.Environment{
		OS: runtime.GOOS, Architecture: runtime.GOARCH,
		ProjectRoot: m.Options.ProjectRoot, TargetRoot: m.Options.ProjectRoot,
		Home: m.Options.Home, AgyConfig: m.Options.AgyConfig,
	}
}

func (m Manager) clientAdapter(client ClientKind) (adapters.Adapter, adapters.CapabilitySet, adapters.LossReport, error) {
	adapter, err := m.adapterRegistry().Get(string(client))
	if err != nil {
		return nil, adapters.CapabilitySet{}, adapters.LossReport{}, err
	}
	capability, err := adapter.Capabilities(context.Background(), m.adapterEnvironment())
	if err != nil {
		return nil, adapters.CapabilitySet{}, adapters.LossReport{}, err
	}
	if err := adapters.VerifyCapabilityForAdapter(adapter, capability); err != nil {
		return nil, adapters.CapabilitySet{}, adapters.LossReport{}, err
	}
	if capability.MCP.Support == adapters.SupportUnsupported || capability.MCP.RegistrationMode == adapters.MCPRegistrationNone {
		return nil, adapters.CapabilitySet{}, adapters.LossReport{}, fmt.Errorf("target %q does not support MCP router registration", client)
	}
	report, err := adapters.SealLossReport(adapters.LossReport{
		Target: capability.Target, AdapterID: capability.AdapterID,
		AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest,
	})
	if err != nil {
		return nil, adapters.CapabilitySet{}, adapters.LossReport{}, err
	}
	return adapter, capability, report, nil
}

func mcpAction(value adapters.Action) (Action, error) {
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

func applyMCPAdapterMetadata(operation *Operation, proposal adapters.ProposedOperation, report adapters.LossReport) {
	operation.AdapterID = proposal.AdapterID
	operation.AdapterVersion = proposal.AdapterVersion
	operation.CapabilityDigest = proposal.CapabilityDigest
	operation.LossReportDigest = proposal.LossReportDigest
	operation.Fidelity = report.Fidelity
	operation.Losses = append([]adapters.Loss(nil), report.Losses...)
}

func sortCapabilitySnapshots(values []adapters.CapabilitySet) {
	sort.Slice(values, func(i, j int) bool { return values[i].Target < values[j].Target })
}

func sortLossReports(values []adapters.LossReport) {
	sort.Slice(values, func(i, j int) bool { return values[i].Target < values[j].Target })
}

func (m Manager) verifyAdapterPlan(plan Plan) error {
	if plan.AdapterContract != adapters.ContractVersion {
		return fmt.Errorf("unsupported adapter contract %q", plan.AdapterContract)
	}
	capabilities := map[string]adapters.CapabilitySet{}
	reports := map[string]adapters.LossReport{}
	for _, capability := range plan.CapabilitySnapshots {
		if err := adapters.VerifyCapabilitySet(capability); err != nil {
			return err
		}
		if _, exists := capabilities[capability.Target]; exists {
			return fmt.Errorf("duplicate capability snapshot for %q", capability.Target)
		}
		capabilities[capability.Target] = capability
	}
	for _, report := range plan.LossReports {
		if err := adapters.VerifyLossReport(report); err != nil {
			return err
		}
		if _, exists := reports[report.Target]; exists {
			return fmt.Errorf("duplicate loss report for %q", report.Target)
		}
		reports[report.Target] = report
	}
	for _, operation := range plan.Operations {
		capability, ok := capabilities[string(operation.Client)]
		if !ok {
			return fmt.Errorf("missing capability snapshot for %q", operation.Client)
		}
		report, ok := reports[string(operation.Client)]
		if !ok {
			return fmt.Errorf("missing loss report for %q", operation.Client)
		}
		if capability.AdapterID != operation.AdapterID || capability.AdapterVersion != operation.AdapterVersion || capability.Digest != operation.CapabilityDigest || report.Digest != operation.LossReportDigest {
			return fmt.Errorf("adapter identity mismatch for %q", operation.Client)
		}
		operationReport, err := adapters.SealLossReport(adapters.LossReport{
			Target: capability.Target, AdapterID: capability.AdapterID,
			AdapterVersion: capability.AdapterVersion, CapabilityDigest: capability.Digest,
			Losses: append([]adapters.Loss(nil), operation.Losses...),
		})
		if err != nil {
			return err
		}
		if operationReport.Digest != report.Digest || operationReport.Fidelity != operation.Fidelity {
			return fmt.Errorf("adapter loss report mismatch for %q", operation.Client)
		}
		_, current, currentReport, err := m.clientAdapter(operation.Client)
		if err != nil {
			return err
		}
		if current.AdapterID != capability.AdapterID || current.AdapterVersion != capability.AdapterVersion || current.Digest != capability.Digest || currentReport.Digest != report.Digest {
			return fmt.Errorf("adapter capability changed after plan review for %q", operation.Client)
		}
	}
	return nil
}
