package adapters

import "fmt"

func Plan(req PlanRequest) ([]ProposedOperation, error) {
	if err := VerifyCapabilitySet(req.Capability); err != nil {
		return nil, err
	}
	if err := VerifyLossReport(req.LossReport); err != nil {
		return nil, err
	}
	if req.Capability.Digest != req.LossReport.CapabilityDigest ||
		req.Capability.Target != req.LossReport.Target ||
		req.Capability.AdapterID != req.LossReport.AdapterID ||
		req.Capability.AdapterVersion != req.LossReport.AdapterVersion {
		return nil, fmt.Errorf("capability and loss report identity mismatch")
	}
	if req.Rendered.ArtifactID == "" {
		return nil, fmt.Errorf("rendered artifact identity is empty")
	}
	if req.Observed.ArtifactID != "" && req.Observed.ArtifactID != req.Rendered.ArtifactID {
		return nil, fmt.Errorf("rendered and observed artifact identities differ")
	}
	if req.Observed.Exists && !validSHA256(req.Observed.Digest) {
		return nil, fmt.Errorf("observed artifact digest is invalid")
	}
	if req.Observed.BaseDigest != "" && !validSHA256(req.Observed.BaseDigest) {
		return nil, fmt.Errorf("observed base digest is invalid")
	}
	if req.Mode == PresencePresent && !validSHA256(req.Rendered.DesiredDigest) {
		return nil, fmt.Errorf("rendered desired digest is invalid")
	}
	location := req.Rendered.Destination
	if location == "" {
		location = req.Observed.Location
	}
	if location == "" {
		return nil, fmt.Errorf("operation location is empty")
	}
	if req.Rendered.Destination != "" && req.Observed.Location != "" && req.Rendered.Destination != req.Observed.Location {
		return nil, fmt.Errorf("rendered and observed locations differ")
	}
	operation := ProposedOperation{
		AdapterID: req.Capability.AdapterID, AdapterVersion: req.Capability.AdapterVersion,
		CapabilityDigest: req.Capability.Digest, LossReportDigest: req.LossReport.Digest,
		Target: req.Capability.Target, ArtifactID: req.Rendered.ArtifactID, Location: location,
		BeforeDigest: req.Observed.Digest,
	}
	if operation.BeforeDigest == "" {
		operation.BeforeDigest = "absent"
	}
	switch req.Mode {
	case PresencePresent:
		operation.AfterDigest = req.Rendered.DesiredDigest
		switch {
		case !req.Observed.Exists:
			operation.Action, operation.Reason = ActionCreate, "target projection is absent"
		case req.Observed.Equivalent:
			operation.Action, operation.Reason = ActionNoop, "target projection is equivalent"
		case req.Observed.Owned && (req.Observed.BaseDigest == "" || req.Observed.Digest == req.Observed.BaseDigest):
			operation.Action, operation.Reason = ActionUpdate, "owned target projection is unchanged from its reviewed base"
		case req.Observed.Owned:
			operation.Action, operation.Reason = ActionConflict, "owned target projection diverged from its reviewed base"
		default:
			operation.Action, operation.Reason = ActionConflict, "foreign target projection is preserved"
		}
	case PresenceAbsent:
		operation.AfterDigest = "absent"
		switch {
		case !req.Observed.Exists:
			operation.Action, operation.Reason = ActionNoop, "target projection is absent"
		case req.Observed.Owned && req.Observed.BaseDigest != "" && req.Observed.Digest != req.Observed.BaseDigest:
			operation.Action, operation.Reason = ActionConflict, "owned target projection diverged from its reviewed base"
		case req.Observed.Owned:
			operation.Action, operation.Reason = ActionRemove, "remove owned target projection"
		default:
			operation.Action, operation.Reason = ActionConflict, "foreign target projection is preserved"
		}
	default:
		return nil, fmt.Errorf("unsupported presence mode %q", req.Mode)
	}
	return []ProposedOperation{operation}, nil
}

func Verify(req VerifyRequest) VerificationResult {
	switch req.Operation.Action {
	case ActionCreate, ActionUpdate:
		if req.Observed.Exists && req.Observed.Digest == req.Operation.AfterDigest {
			return VerificationResult{Verified: true, Reason: "target digest matches the reviewed projection"}
		}
		return VerificationResult{Reason: "target digest does not match the reviewed projection"}
	case ActionNoop:
		if req.Operation.AfterDigest == "absent" {
			if !req.Observed.Exists {
				return VerificationResult{Verified: true, Reason: "target projection remains absent"}
			}
			return VerificationResult{Reason: "target projection unexpectedly exists"}
		}
		if req.Observed.Exists && (req.Observed.Digest == req.Operation.AfterDigest || req.Observed.Equivalent) {
			return VerificationResult{Verified: true, Reason: "target projection remains equivalent to the reviewed projection"}
		}
		return VerificationResult{Reason: "target projection is not equivalent to the reviewed projection"}
	case ActionRemove:
		if !req.Observed.Exists {
			return VerificationResult{Verified: true, Reason: "target projection is absent"}
		}
		return VerificationResult{Reason: "target projection still exists"}
	case ActionConflict:
		return VerificationResult{Reason: "conflict operations are not executable"}
	default:
		return VerificationResult{Reason: "unknown operation action"}
	}
}
