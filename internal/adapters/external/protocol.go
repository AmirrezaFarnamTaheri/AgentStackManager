// Package external implements ASM's constrained out-of-process adapter protocol.
// External adapters are evidence-producing projection codecs. They are never
// registered as Resource Hub or mcplink mutation authorities by this package.
package external

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/integrity"
)

const (
	ProtocolVersion        = "fabric.asm.dev/external-adapter/v1alpha1"
	DescriptorAPIVersion   = "fabric.asm.dev/external-adapter-descriptor/v1alpha1"
	IntersectionAPIVersion = "fabric.asm.dev/external-adapter-intersection/v1alpha1"
	ReportAPIVersion       = "fabric.asm.dev/external-adapter-conformance-report/v1alpha1"
)

type Operation string

const (
	OperationHandshake    Operation = "handshake"
	OperationCapabilities Operation = "capabilities"
	OperationDiscover     Operation = "discover"
	OperationImport       Operation = "import"
	OperationRender       Operation = "render"
	OperationPlan         Operation = "plan"
	OperationVerify       Operation = "verify"
)

var requiredOperations = []Operation{
	OperationCapabilities,
	OperationDiscover,
	OperationImport,
	OperationRender,
	OperationPlan,
	OperationVerify,
}

type Request struct {
	APIVersion      string          `json:"apiVersion"`
	RequestID       string          `json:"requestId"`
	Operation       Operation       `json:"operation"`
	AdapterContract string          `json:"adapterContract"`
	Deadline        time.Time       `json:"deadline"`
	Payload         json.RawMessage `json:"payload"`
}

type Response struct {
	APIVersion string          `json:"apiVersion"`
	RequestID  string          `json:"requestId"`
	Operation  Operation       `json:"operation"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      *ProtocolError  `json:"error,omitempty"`
}

type ErrorCode string

const (
	ErrorInvalidRequest       ErrorCode = "invalid-request"
	ErrorUnsupportedOperation ErrorCode = "unsupported-operation"
	ErrorAdapter              ErrorCode = "adapter-error"
	ErrorInternal             ErrorCode = "internal-error"
)

type ProtocolError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return "external adapter protocol error"
	}
	return fmt.Sprintf("external adapter %s: %s", e.Code, e.Message)
}

type HandshakeRequest struct {
	SupportedProtocols []string             `json:"supportedProtocols"`
	AdapterContract    string               `json:"adapterContract"`
	Target             string               `json:"target"`
	Environment        adapters.Environment `json:"environment"`
}

type HandshakeResult struct {
	ProtocolVersion string      `json:"protocolVersion"`
	AdapterContract string      `json:"adapterContract"`
	Target          string      `json:"target"`
	AdapterID       string      `json:"adapterId"`
	AdapterVersion  string      `json:"adapterVersion"`
	Aliases         []string    `json:"aliases,omitempty"`
	Operations      []Operation `json:"operations"`
}

type CapabilitiesResult struct {
	Capability adapters.CapabilitySet `json:"capability"`
}

type DiscoverResult struct {
	Artifacts []adapters.ObservedArtifact `json:"artifacts"`
}

type ImportResult struct {
	Artifact   artifactgraph.Artifact `json:"artifact"`
	LossReport adapters.LossReport    `json:"lossReport"`
}

type RenderResult struct {
	Rendered   adapters.RenderedSet `json:"rendered"`
	LossReport adapters.LossReport  `json:"lossReport"`
}

type PlanResult struct {
	Operations []adapters.ProposedOperation `json:"operations"`
}

type VerifyResult struct {
	Verification adapters.VerificationResult `json:"verification"`
}

type Descriptor struct {
	APIVersion       string      `json:"apiVersion"`
	ProtocolVersion  string      `json:"protocolVersion"`
	AdapterContract  string      `json:"adapterContract"`
	Target           string      `json:"target"`
	AdapterID        string      `json:"adapterId"`
	AdapterVersion   string      `json:"adapterVersion"`
	Aliases          []string    `json:"aliases,omitempty"`
	Operations       []Operation `json:"operations"`
	ExecutableDigest string      `json:"executableDigest"`
	ExecutableSize   int64       `json:"executableSize"`
	ArgumentsDigest  string      `json:"argumentsDigest"`
	Digest           string      `json:"digest"`
}

func SealDescriptor(value Descriptor) (Descriptor, error) {
	if value.APIVersion == "" {
		value.APIVersion = DescriptorAPIVersion
	}
	value.ProtocolVersion = strings.TrimSpace(value.ProtocolVersion)
	value.AdapterContract = strings.TrimSpace(value.AdapterContract)
	value.Target = strings.TrimSpace(value.Target)
	value.AdapterID = strings.TrimSpace(value.AdapterID)
	value.AdapterVersion = strings.TrimSpace(value.AdapterVersion)
	value.ExecutableDigest = strings.TrimSpace(value.ExecutableDigest)
	value.ArgumentsDigest = strings.TrimSpace(value.ArgumentsDigest)
	if value.APIVersion != DescriptorAPIVersion || value.ProtocolVersion != ProtocolVersion || value.AdapterContract != adapters.ContractVersion {
		return Descriptor{}, fmt.Errorf("invalid external adapter descriptor protocol identity")
	}
	if value.Target == "" || value.AdapterID == "" || value.AdapterVersion == "" || value.ExecutableSize <= 0 || !validDigest(value.ExecutableDigest) || !validDigest(value.ArgumentsDigest) {
		return Descriptor{}, fmt.Errorf("invalid external adapter descriptor identity")
	}
	value.Aliases = sortedUniqueStrings(value.Aliases)
	value.Operations = sortedUniqueOperations(value.Operations)
	if err := requireOperations(value.Operations); err != nil {
		return Descriptor{}, err
	}
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return Descriptor{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyDescriptor(value Descriptor) error {
	if !validDigest(value.Digest) {
		return fmt.Errorf("invalid external adapter descriptor digest")
	}
	expected, err := SealDescriptor(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("external adapter descriptor digest mismatch")
	}
	return nil
}

func validateHandshake(value HandshakeResult, target string) error {
	value.ProtocolVersion = strings.TrimSpace(value.ProtocolVersion)
	value.AdapterContract = strings.TrimSpace(value.AdapterContract)
	value.Target = strings.TrimSpace(value.Target)
	value.AdapterID = strings.TrimSpace(value.AdapterID)
	value.AdapterVersion = strings.TrimSpace(value.AdapterVersion)
	if value.ProtocolVersion != ProtocolVersion || value.AdapterContract != adapters.ContractVersion {
		return fmt.Errorf("external adapter selected unsupported protocol or contract")
	}
	if value.Target != strings.TrimSpace(target) || value.AdapterID == "" || value.AdapterVersion == "" {
		return fmt.Errorf("external adapter handshake identity mismatch")
	}
	if err := requireOperations(sortedUniqueOperations(value.Operations)); err != nil {
		return err
	}
	return nil
}

func requireOperations(values []Operation) error {
	set := make(map[Operation]struct{}, len(values))
	for _, value := range values {
		if !validOperation(value) || value == OperationHandshake {
			return fmt.Errorf("invalid external adapter operation %q", value)
		}
		set[value] = struct{}{}
	}
	for _, required := range requiredOperations {
		if _, ok := set[required]; !ok {
			return fmt.Errorf("external adapter does not implement required operation %q", required)
		}
	}
	return nil
}

func validOperation(value Operation) bool {
	switch value {
	case OperationHandshake, OperationCapabilities, OperationDiscover, OperationImport, OperationRender, OperationPlan, OperationVerify:
		return true
	default:
		return false
	}
}

func validErrorCode(value ErrorCode) bool {
	switch value {
	case ErrorInvalidRequest, ErrorUnsupportedOperation, ErrorAdapter, ErrorInternal:
		return true
	default:
		return false
	}
}

func sortedUniqueOperations(values []Operation) []Operation {
	set := map[Operation]struct{}{}
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]Operation, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func sortedUniqueStrings(values []string) []string {
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
	return result
}
