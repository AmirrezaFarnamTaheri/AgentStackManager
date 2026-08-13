package external

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
)

type CapabilityChangeKind string

const (
	CapabilityRestricted       CapabilityChangeKind = "restricted"
	CapabilityCandidateLimited CapabilityChangeKind = "candidate-limited"
)

type CapabilityChange struct {
	Path   string               `json:"path"`
	Kind   CapabilityChangeKind `json:"kind"`
	Reason string               `json:"reason"`
}

type IntersectionReport struct {
	APIVersion      string             `json:"apiVersion"`
	Target          string             `json:"target"`
	AdapterID       string             `json:"adapterId"`
	AdapterVersion  string             `json:"adapterVersion"`
	RawDigest       string             `json:"rawDigest"`
	CeilingDigest   string             `json:"ceilingDigest"`
	EffectiveDigest string             `json:"effectiveDigest"`
	Changes         []CapabilityChange `json:"changes,omitempty"`
	Digest          string             `json:"digest"`
}

func IntersectCapabilities(raw, ceiling adapters.CapabilitySet) (adapters.CapabilitySet, IntersectionReport, error) {
	if err := adapters.VerifyCapabilitySet(raw); err != nil {
		return adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("verify external capability: %w", err)
	}
	if err := adapters.VerifyCapabilitySet(ceiling); err != nil {
		return adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("verify capability ceiling: %w", err)
	}
	if raw.Target != ceiling.Target || raw.AdapterID != ceiling.AdapterID || raw.AdapterVersion != ceiling.AdapterVersion {
		return adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("external capability semantic identity differs from the reviewed ceiling")
	}
	if raw.TargetVersionRange != ceiling.TargetVersionRange {
		return adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("external capability target version range differs from the reviewed ceiling")
	}
	changes := []CapabilityChange{}
	effective := adapters.CapabilitySet{
		AdapterID: ceiling.AdapterID, AdapterVersion: ceiling.AdapterVersion,
		Target: ceiling.Target, TargetVersionRange: ceiling.TargetVersionRange,
		Aliases:         intersectStrings(raw.Aliases, ceiling.Aliases),
		DeploymentModes: intersectStrings(raw.DeploymentModes, ceiling.DeploymentModes),
		Artifacts:       map[artifactgraph.Kind]adapters.ArtifactCapability{},
	}
	appendSetChanges(&changes, "/aliases", raw.Aliases, ceiling.Aliases)
	appendSetChanges(&changes, "/deploymentModes", raw.DeploymentModes, ceiling.DeploymentModes)

	for kind := range raw.Artifacts {
		if _, ok := ceiling.Artifacts[kind]; !ok {
			changes = append(changes, CapabilityChange{Path: "/artifacts/" + string(kind), Kind: CapabilityRestricted, Reason: "artifact kind is outside the reviewed target capability ceiling"})
		}
	}
	for kind, allowed := range ceiling.Artifacts {
		candidate, ok := raw.Artifacts[kind]
		if !ok || candidate.Support == adapters.SupportUnsupported {
			changes = append(changes, CapabilityChange{Path: "/artifacts/" + string(kind), Kind: CapabilityCandidateLimited, Reason: "external adapter does not provide a reviewed artifact capability"})
			continue
		}
		if candidate.Directory != allowed.Directory || candidate.Format != allowed.Format {
			return adapters.CapabilitySet{}, IntersectionReport{}, fmt.Errorf("external artifact projection %q differs from the reviewed directory or format", kind)
		}
		support, restricted := intersectSupport(candidate.Support, allowed.Support)
		if restricted {
			changes = append(changes, CapabilityChange{Path: "/artifacts/" + string(kind) + "/support", Kind: CapabilityRestricted, Reason: "external support claim exceeded the reviewed target ceiling"})
		} else if support != allowed.Support {
			changes = append(changes, CapabilityChange{Path: "/artifacts/" + string(kind) + "/support", Kind: CapabilityCandidateLimited, Reason: "external adapter provides weaker support than the reviewed target capability"})
		}
		if support == adapters.SupportUnsupported {
			continue
		}
		fields := intersectFields(candidate.Fields, allowed.Fields, "/artifacts/"+string(kind)+"/fields", &changes)
		effective.Artifacts[kind] = adapters.ArtifactCapability{
			Support: support, Directory: allowed.Directory, Format: allowed.Format,
			Scopes:     intersectStrings(candidate.Scopes, allowed.Scopes),
			Transports: intersectStrings(candidate.Transports, allowed.Transports),
			Fields:     fields,
		}
		appendSetChanges(&changes, "/artifacts/"+string(kind)+"/scopes", candidate.Scopes, allowed.Scopes)
		appendSetChanges(&changes, "/artifacts/"+string(kind)+"/transports", candidate.Transports, allowed.Transports)
	}

	mcp, mcpChanges, err := intersectMCP(raw.MCP, ceiling.MCP)
	if err != nil {
		return adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	effective.MCP = mcp
	changes = append(changes, mcpChanges...)

	sealed, err := adapters.SealCapabilitySet(effective)
	if err != nil {
		return adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	report, err := SealIntersectionReport(IntersectionReport{
		Target: ceiling.Target, AdapterID: ceiling.AdapterID, AdapterVersion: ceiling.AdapterVersion,
		RawDigest: raw.Digest, CeilingDigest: ceiling.Digest, EffectiveDigest: sealed.Digest, Changes: changes,
	})
	if err != nil {
		return adapters.CapabilitySet{}, IntersectionReport{}, err
	}
	return sealed, report, nil
}

func SealIntersectionReport(value IntersectionReport) (IntersectionReport, error) {
	if value.APIVersion == "" {
		value.APIVersion = IntersectionAPIVersion
	}
	value.Target = strings.TrimSpace(value.Target)
	value.AdapterID = strings.TrimSpace(value.AdapterID)
	value.AdapterVersion = strings.TrimSpace(value.AdapterVersion)
	value.RawDigest = strings.TrimSpace(value.RawDigest)
	value.CeilingDigest = strings.TrimSpace(value.CeilingDigest)
	value.EffectiveDigest = strings.TrimSpace(value.EffectiveDigest)
	if value.APIVersion != IntersectionAPIVersion || value.Target == "" || value.AdapterID == "" || value.AdapterVersion == "" || !validDigest(value.RawDigest) || !validDigest(value.CeilingDigest) || !validDigest(value.EffectiveDigest) {
		return IntersectionReport{}, fmt.Errorf("invalid capability intersection identity")
	}
	value.Changes = append([]CapabilityChange(nil), value.Changes...)
	for i := range value.Changes {
		change := &value.Changes[i]
		change.Path = strings.TrimSpace(change.Path)
		change.Reason = strings.TrimSpace(change.Reason)
		if change.Path == "" || change.Reason == "" || (change.Kind != CapabilityRestricted && change.Kind != CapabilityCandidateLimited) {
			return IntersectionReport{}, fmt.Errorf("invalid capability intersection change at index %d", i)
		}
	}
	sort.Slice(value.Changes, func(i, j int) bool {
		if value.Changes[i].Path != value.Changes[j].Path {
			return value.Changes[i].Path < value.Changes[j].Path
		}
		if value.Changes[i].Kind != value.Changes[j].Kind {
			return value.Changes[i].Kind < value.Changes[j].Kind
		}
		return value.Changes[i].Reason < value.Changes[j].Reason
	})
	for i := 1; i < len(value.Changes); i++ {
		if value.Changes[i-1] == value.Changes[i] {
			return IntersectionReport{}, fmt.Errorf("duplicate capability intersection change at %q", value.Changes[i].Path)
		}
	}
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return IntersectionReport{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyIntersectionReport(value IntersectionReport) error {
	if !validDigest(value.Digest) {
		return fmt.Errorf("invalid capability intersection digest")
	}
	expected, err := SealIntersectionReport(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("capability intersection digest mismatch")
	}
	return nil
}

func intersectMCP(raw, ceiling adapters.MCPClientCapability) (adapters.MCPClientCapability, []CapabilityChange, error) {
	if ceiling.Support == adapters.SupportUnsupported {
		changes := []CapabilityChange{}
		if raw.Support != adapters.SupportUnsupported || raw.RegistrationMode != adapters.MCPRegistrationNone {
			changes = append(changes, CapabilityChange{Path: "/mcp", Kind: CapabilityRestricted, Reason: "MCP registration is outside the reviewed target capability ceiling"})
		}
		return adapters.MCPClientCapability{Support: adapters.SupportUnsupported, RegistrationMode: adapters.MCPRegistrationNone}, changes, nil
	}
	if raw.Support == adapters.SupportUnsupported || raw.RegistrationMode == adapters.MCPRegistrationNone {
		return adapters.MCPClientCapability{Support: adapters.SupportUnsupported, RegistrationMode: adapters.MCPRegistrationNone}, []CapabilityChange{{Path: "/mcp", Kind: CapabilityCandidateLimited, Reason: "external adapter does not provide the reviewed MCP registration capability"}}, nil
	}
	if raw.RegistrationMode != ceiling.RegistrationMode || raw.Location != ceiling.Location || raw.RootKey != ceiling.RootKey || raw.EntryName != ceiling.EntryName {
		return adapters.MCPClientCapability{}, nil, fmt.Errorf("external MCP registration structure differs from the reviewed ceiling")
	}
	support, restricted := intersectSupport(raw.Support, ceiling.Support)
	changes := []CapabilityChange{}
	if restricted {
		changes = append(changes, CapabilityChange{Path: "/mcp/support", Kind: CapabilityRestricted, Reason: "external MCP support claim exceeded the reviewed target ceiling"})
	} else if support != ceiling.Support {
		changes = append(changes, CapabilityChange{Path: "/mcp/support", Kind: CapabilityCandidateLimited, Reason: "external adapter provides weaker MCP support than the reviewed target capability"})
	}
	transports := intersectStrings(raw.Transports, ceiling.Transports)
	appendSetChanges(&changes, "/mcp/transports", raw.Transports, ceiling.Transports)
	if support == adapters.SupportUnsupported {
		return adapters.MCPClientCapability{Support: adapters.SupportUnsupported, RegistrationMode: adapters.MCPRegistrationNone}, changes, nil
	}
	return adapters.MCPClientCapability{
		Support: support, RegistrationMode: ceiling.RegistrationMode, Location: ceiling.Location,
		RootKey: ceiling.RootKey, EntryName: ceiling.EntryName, Transports: transports,
	}, changes, nil
}

func intersectSupport(raw, ceiling adapters.SupportLevel) (adapters.SupportLevel, bool) {
	if supportRank(raw) > supportRank(ceiling) {
		return ceiling, true
	}
	return raw, false
}

func supportRank(value adapters.SupportLevel) int {
	switch value {
	case adapters.SupportNative:
		return 3
	case adapters.SupportPassthrough:
		return 2
	case adapters.SupportFallback:
		return 1
	case adapters.SupportUnsupported:
		return 0
	default:
		return -1
	}
}

func intersectFields(raw, ceiling map[string]adapters.FieldSupport, path string, changes *[]CapabilityChange) map[string]adapters.FieldSupport {
	result := map[string]adapters.FieldSupport{}
	for field := range raw {
		if _, ok := ceiling[field]; !ok {
			*changes = append(*changes, CapabilityChange{Path: path + "/" + field, Kind: CapabilityRestricted, Reason: "field capability is outside the reviewed target ceiling"})
		}
	}
	for field, allowed := range ceiling {
		candidate, ok := raw[field]
		if !ok {
			*changes = append(*changes, CapabilityChange{Path: path + "/" + field, Kind: CapabilityCandidateLimited, Reason: "external adapter omitted a reviewed field capability"})
			continue
		}
		if fieldRank(candidate) > fieldRank(allowed) {
			result[field] = allowed
			*changes = append(*changes, CapabilityChange{Path: path + "/" + field, Kind: CapabilityRestricted, Reason: "external field support claim exceeded the reviewed target ceiling"})
		} else {
			result[field] = candidate
			if candidate != allowed {
				*changes = append(*changes, CapabilityChange{Path: path + "/" + field, Kind: CapabilityCandidateLimited, Reason: "external adapter provides weaker field support than the reviewed target capability"})
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func fieldRank(value adapters.FieldSupport) int {
	switch value {
	case adapters.FieldNative:
		return 4
	case adapters.FieldMapped:
		return 3
	case adapters.FieldDefaulted:
		return 2
	case adapters.FieldOmitted:
		return 1
	case adapters.FieldUnsupported:
		return 0
	default:
		return -1
	}
}

func intersectStrings(raw, ceiling []string) []string {
	allowed := make(map[string]struct{}, len(ceiling))
	for _, value := range ceiling {
		allowed[value] = struct{}{}
	}
	result := []string{}
	for _, value := range raw {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return sortedUniqueStrings(result)
}

func appendSetChanges(changes *[]CapabilityChange, path string, raw, ceiling []string) {
	allowed := make(map[string]struct{}, len(ceiling))
	for _, value := range ceiling {
		allowed[value] = struct{}{}
	}
	present := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		present[value] = struct{}{}
		if _, ok := allowed[value]; !ok {
			*changes = append(*changes, CapabilityChange{Path: path + "/" + value, Kind: CapabilityRestricted, Reason: "value is outside the reviewed capability ceiling"})
		}
	}
	for _, value := range ceiling {
		if _, ok := present[value]; !ok {
			*changes = append(*changes, CapabilityChange{Path: path + "/" + value, Kind: CapabilityCandidateLimited, Reason: "external adapter omitted a reviewed capability value"})
		}
	}
}
