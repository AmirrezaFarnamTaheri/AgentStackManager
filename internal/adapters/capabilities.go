package adapters

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
)

type SupportLevel string

const (
	SupportNative      SupportLevel = "native"
	SupportPassthrough SupportLevel = "passthrough"
	SupportFallback    SupportLevel = "fallback"
	SupportUnsupported SupportLevel = "unsupported"
)

type FieldSupport string

const (
	FieldNative      FieldSupport = "native"
	FieldMapped      FieldSupport = "mapped"
	FieldDefaulted   FieldSupport = "defaulted"
	FieldOmitted     FieldSupport = "omitted"
	FieldUnsupported FieldSupport = "unsupported"
)

type ArtifactCapability struct {
	Support    SupportLevel            `json:"support"`
	Scopes     []string                `json:"scopes,omitempty"`
	Directory  string                  `json:"directory,omitempty"`
	Format     string                  `json:"format,omitempty"`
	Fields     map[string]FieldSupport `json:"fields,omitempty"`
	Transports []string                `json:"transports,omitempty"`
}

type MCPRegistrationMode string

const (
	MCPRegistrationCommand  MCPRegistrationMode = "command"
	MCPRegistrationJSONFile MCPRegistrationMode = "json-file"
	MCPRegistrationNone     MCPRegistrationMode = "none"
)

type MCPClientCapability struct {
	Support          SupportLevel        `json:"support"`
	RegistrationMode MCPRegistrationMode `json:"registrationMode"`
	Location         string              `json:"location,omitempty"`
	RootKey          string              `json:"rootKey,omitempty"`
	EntryName        string              `json:"entryName,omitempty"`
	Transports       []string            `json:"transports,omitempty"`
}

type CapabilitySet struct {
	APIVersion         string                                    `json:"apiVersion"`
	AdapterID          string                                    `json:"adapterId"`
	AdapterVersion     string                                    `json:"adapterVersion"`
	Target             string                                    `json:"target"`
	Aliases            []string                                  `json:"aliases,omitempty"`
	TargetVersionRange string                                    `json:"targetVersionRange"`
	Artifacts          map[artifactgraph.Kind]ArtifactCapability `json:"artifacts,omitempty"`
	DeploymentModes    []string                                  `json:"deploymentModes,omitempty"`
	MCP                MCPClientCapability                       `json:"mcp"`
	Digest             string                                    `json:"digest"`
}

func SealCapabilitySet(value CapabilitySet) (CapabilitySet, error) {
	if value.APIVersion == "" {
		value.APIVersion = ContractVersion
	}
	value.AdapterID = strings.TrimSpace(value.AdapterID)
	value.AdapterVersion = strings.TrimSpace(value.AdapterVersion)
	value.Target = strings.TrimSpace(value.Target)
	value.TargetVersionRange = strings.TrimSpace(value.TargetVersionRange)
	if value.APIVersion != ContractVersion || value.AdapterID == "" || value.AdapterVersion == "" || value.Target == "" || value.TargetVersionRange == "" {
		return CapabilitySet{}, fmt.Errorf("invalid capability identity")
	}
	value.Aliases = sortedUnique(value.Aliases)
	value.DeploymentModes = sortedUnique(value.DeploymentModes)
	value.MCP.Location = strings.TrimSpace(value.MCP.Location)
	value.MCP.RootKey = strings.TrimSpace(value.MCP.RootKey)
	value.MCP.EntryName = strings.TrimSpace(value.MCP.EntryName)
	value.MCP.Transports = sortedUnique(value.MCP.Transports)
	if !validSupport(value.MCP.Support) {
		return CapabilitySet{}, fmt.Errorf("invalid MCP support %q", value.MCP.Support)
	}
	switch value.MCP.RegistrationMode {
	case MCPRegistrationCommand, MCPRegistrationJSONFile:
		if value.MCP.Support == SupportUnsupported || value.MCP.Location == "" || value.MCP.EntryName == "" {
			return CapabilitySet{}, fmt.Errorf("invalid active MCP capability")
		}
	case MCPRegistrationNone:
		if value.MCP.Support != SupportUnsupported {
			return CapabilitySet{}, fmt.Errorf("MCP registration mode none must be unsupported")
		}
		if value.MCP.Location != "" || value.MCP.RootKey != "" || value.MCP.EntryName != "" || len(value.MCP.Transports) != 0 {
			return CapabilitySet{}, fmt.Errorf("inactive MCP capability must not advertise registration fields")
		}
	default:
		return CapabilitySet{}, fmt.Errorf("invalid MCP registration mode %q", value.MCP.RegistrationMode)
	}
	artifacts := make(map[artifactgraph.Kind]ArtifactCapability, len(value.Artifacts))
	for kind, capability := range value.Artifacts {
		if kind == "" || !validSupport(capability.Support) {
			return CapabilitySet{}, fmt.Errorf("invalid capability for kind %q", kind)
		}
		capability.Directory = strings.TrimSpace(capability.Directory)
		capability.Format = strings.TrimSpace(capability.Format)
		capability.Scopes = sortedUnique(capability.Scopes)
		capability.Transports = sortedUnique(capability.Transports)
		if len(capability.Fields) > 0 {
			fields := make(map[string]FieldSupport, len(capability.Fields))
			for field, support := range capability.Fields {
				field = strings.TrimSpace(field)
				if field == "" || !validFieldSupport(support) {
					return CapabilitySet{}, fmt.Errorf("invalid field support for %q", field)
				}
				fields[field] = support
			}
			capability.Fields = fields
		}
		artifacts[kind] = capability
	}
	value.Artifacts = artifacts
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return CapabilitySet{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyCapabilitySet(value CapabilitySet) error {
	if !validSHA256(value.Digest) {
		return fmt.Errorf("invalid capability digest")
	}
	expected, err := SealCapabilitySet(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("capability digest mismatch")
	}
	return nil
}

func VerifyCapabilityForAdapter(adapter Adapter, value CapabilitySet) error {
	if adapter == nil {
		return fmt.Errorf("adapter is unavailable")
	}
	if adapter.SchemaVersion() != ContractVersion {
		return fmt.Errorf("adapter %q uses unsupported contract %q", adapter.ID(), adapter.SchemaVersion())
	}
	if err := VerifyCapabilitySet(value); err != nil {
		return err
	}
	if strings.TrimSpace(adapter.ID()) != value.Target {
		return fmt.Errorf("adapter %q returned capability for target %q", adapter.ID(), value.Target)
	}
	return nil
}

func validSupport(value SupportLevel) bool {
	switch value {
	case SupportNative, SupportPassthrough, SupportFallback, SupportUnsupported:
		return true
	default:
		return false
	}
}

func validFieldSupport(value FieldSupport) bool {
	switch value {
	case FieldNative, FieldMapped, FieldDefaulted, FieldOmitted, FieldUnsupported:
		return true
	default:
		return false
	}
}

func sortedUnique(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) == 0 {
		return nil
	}
	return result
}
