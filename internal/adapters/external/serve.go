package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/strictjson"
)

// ServeOne serves exactly one protocol request. It is intended as the reference
// entry point for an external adapter executable. The executable should call
// ServeOne with its pure adapter implementation, then exit.
func ServeOne(parent context.Context, adapter adapters.Adapter, input io.Reader, output io.Writer) error {
	if adapter == nil {
		return fmt.Errorf("external adapter implementation is unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(input, defaultMaxRequestBytes+1))
	if err != nil {
		return fmt.Errorf("read external adapter request: %w", err)
	}
	if len(data) == 0 || int64(len(data)) > defaultMaxRequestBytes {
		return fmt.Errorf("external adapter request exceeds protocol bounds")
	}
	var request Request
	if err := strictjson.Decode(data, &request); err != nil {
		return fmt.Errorf("decode external adapter request: %w", err)
	}
	if err := validateRequest(request); err != nil {
		return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
	}
	ctx, cancel := context.WithDeadline(parent, request.Deadline)
	defer cancel()
	var result any
	switch request.Operation {
	case OperationHandshake:
		var payload HandshakeRequest
		if err := strictjson.Decode(request.Payload, &payload); err != nil {
			return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
		}
		if !containsString(payload.SupportedProtocols, ProtocolVersion) || payload.AdapterContract != adapters.ContractVersion || strings.TrimSpace(payload.Target) != strings.TrimSpace(adapter.ID()) {
			return writeProtocolError(output, request, ErrorInvalidRequest, "protocol, contract, or target negotiation failed")
		}
		capability, err := adapter.Capabilities(ctx, payload.Environment)
		if err != nil {
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		if err := adapters.VerifyCapabilityForAdapter(adapter, capability); err != nil {
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		result = HandshakeResult{
			ProtocolVersion: ProtocolVersion, AdapterContract: adapters.ContractVersion,
			Target: capability.Target, AdapterID: capability.AdapterID, AdapterVersion: capability.AdapterVersion,
			Aliases: capability.Aliases, Operations: append([]Operation(nil), requiredOperations...),
		}
	case OperationCapabilities:
		var environment adapters.Environment
		if err := strictjson.Decode(request.Payload, &environment); err != nil {
			return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
		}
		capability, err := adapter.Capabilities(ctx, environment)
		if err != nil {
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		result = CapabilitiesResult{Capability: capability}
	case OperationDiscover:
		var payload adapters.DiscoverRequest
		if err := strictjson.Decode(request.Payload, &payload); err != nil {
			return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
		}
		items, err := adapter.Discover(ctx, payload)
		if err != nil {
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		result = DiscoverResult{Artifacts: items}
	case OperationImport:
		var payload adapters.ImportRequest
		if err := strictjson.Decode(request.Payload, &payload); err != nil {
			return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
		}
		artifact, report, err := adapter.Import(ctx, payload)
		if err != nil {
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		result = ImportResult{Artifact: artifact, LossReport: report}
	case OperationRender:
		var payload adapters.RenderRequest
		if err := strictjson.Decode(request.Payload, &payload); err != nil {
			return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
		}
		rendered, report, err := adapter.Render(ctx, payload)
		if err != nil {
			// Unsupported projections still carry a machine-readable loss report.
			if report.Digest != "" {
				result = RenderResult{Rendered: rendered, LossReport: report}
				return writeProtocolResult(output, request, result)
			}
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		result = RenderResult{Rendered: rendered, LossReport: report}
	case OperationPlan:
		var payload adapters.PlanRequest
		if err := strictjson.Decode(request.Payload, &payload); err != nil {
			return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
		}
		operations, err := adapter.Plan(ctx, payload)
		if err != nil {
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		result = PlanResult{Operations: operations}
	case OperationVerify:
		var payload adapters.VerifyRequest
		if err := strictjson.Decode(request.Payload, &payload); err != nil {
			return writeProtocolError(output, request, ErrorInvalidRequest, err.Error())
		}
		verification, err := adapter.Verify(ctx, payload)
		if err != nil {
			return writeProtocolError(output, request, ErrorAdapter, err.Error())
		}
		result = VerifyResult{Verification: verification}
	default:
		return writeProtocolError(output, request, ErrorUnsupportedOperation, fmt.Sprintf("unsupported operation %q", request.Operation))
	}
	return writeProtocolResult(output, request, result)
}

func validateRequest(value Request) error {
	value.RequestID = strings.TrimSpace(value.RequestID)
	value.AdapterContract = strings.TrimSpace(value.AdapterContract)
	if value.APIVersion != ProtocolVersion || value.RequestID == "" || !validOperation(value.Operation) || value.AdapterContract != adapters.ContractVersion {
		return fmt.Errorf("invalid external adapter request identity")
	}
	if value.Deadline.IsZero() || !value.Deadline.After(time.Now()) {
		return fmt.Errorf("external adapter request deadline is missing or expired")
	}
	if len(bytes.TrimSpace(value.Payload)) == 0 {
		return fmt.Errorf("external adapter request payload is empty")
	}
	return nil
}

func writeProtocolResult(output io.Writer, request Request, value any) error {
	result, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeResponse(output, Response{APIVersion: ProtocolVersion, RequestID: request.RequestID, Operation: request.Operation, Result: result})
}

func writeProtocolError(output io.Writer, request Request, code ErrorCode, message string) error {
	return writeResponse(output, Response{APIVersion: ProtocolVersion, RequestID: request.RequestID, Operation: request.Operation, Error: &ProtocolError{Code: code, Message: strings.TrimSpace(message)}})
}

func writeResponse(output io.Writer, response Response) error {
	data, err := strictjson.MarshalCanonical(response)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = output.Write(data)
	return err
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
