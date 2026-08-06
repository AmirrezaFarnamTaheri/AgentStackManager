package adapters

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/integrity"
)

type Fidelity string

const (
	FidelityFull    Fidelity = "full"
	FidelityPartial Fidelity = "partial"
	FidelityLossy   Fidelity = "lossy"
	FidelityBlocked Fidelity = "blocked"
)

type LossKind string

const (
	LossTransformation LossKind = "transformation"
	LossFallback       LossKind = "fallback"
	LossOmission       LossKind = "omission"
	LossUnsupported    LossKind = "unsupported"
)

type Loss struct {
	ArtifactID string   `json:"artifactId"`
	Field      string   `json:"field"`
	Kind       LossKind `json:"kind"`
	Code       string   `json:"code"`
	Reason     string   `json:"reason"`
	Required   bool     `json:"required,omitempty"`
}

type LossReport struct {
	APIVersion       string   `json:"apiVersion"`
	Target           string   `json:"target"`
	AdapterID        string   `json:"adapterId"`
	AdapterVersion   string   `json:"adapterVersion"`
	CapabilityDigest string   `json:"capabilityDigest"`
	Fidelity         Fidelity `json:"fidelity"`
	Losses           []Loss   `json:"losses,omitempty"`
	Digest           string   `json:"digest"`
}

func SealLossReport(value LossReport) (LossReport, error) {
	if value.APIVersion == "" {
		value.APIVersion = ContractVersion
	}
	value.Target = strings.TrimSpace(value.Target)
	value.AdapterID = strings.TrimSpace(value.AdapterID)
	value.AdapterVersion = strings.TrimSpace(value.AdapterVersion)
	value.CapabilityDigest = strings.TrimSpace(value.CapabilityDigest)
	if value.APIVersion != ContractVersion || value.Target == "" || value.AdapterID == "" || value.AdapterVersion == "" || !validSHA256(value.CapabilityDigest) {
		return LossReport{}, fmt.Errorf("invalid loss report identity")
	}
	value.Losses = append([]Loss(nil), value.Losses...)
	for i := range value.Losses {
		loss := &value.Losses[i]
		loss.ArtifactID = strings.TrimSpace(loss.ArtifactID)
		loss.Field = strings.TrimSpace(loss.Field)
		loss.Code = strings.TrimSpace(loss.Code)
		loss.Reason = strings.TrimSpace(loss.Reason)
		if loss.ArtifactID == "" || loss.Field == "" || loss.Code == "" || loss.Reason == "" || !validLossKind(loss.Kind) {
			return LossReport{}, fmt.Errorf("invalid loss at index %d", i)
		}
	}
	sort.Slice(value.Losses, func(i, j int) bool {
		left, right := value.Losses[i], value.Losses[j]
		if left.ArtifactID != right.ArtifactID {
			return left.ArtifactID < right.ArtifactID
		}
		if left.Field != right.Field {
			return left.Field < right.Field
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Code < right.Code
	})
	for i := 1; i < len(value.Losses); i++ {
		if sameLossIdentity(value.Losses[i-1], value.Losses[i]) {
			return LossReport{}, fmt.Errorf("duplicate loss %s %s %s", value.Losses[i].ArtifactID, value.Losses[i].Field, value.Losses[i].Code)
		}
	}
	value.Fidelity = deriveFidelity(value.Losses)
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return LossReport{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyLossReport(value LossReport) error {
	if !validSHA256(value.Digest) {
		return fmt.Errorf("invalid loss report digest")
	}
	expected, err := SealLossReport(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("loss report digest mismatch")
	}
	return nil
}

func MergeLossReports(target, adapterID, adapterVersion, capabilityDigest string, reports ...LossReport) (LossReport, error) {
	value := LossReport{Target: target, AdapterID: adapterID, AdapterVersion: adapterVersion, CapabilityDigest: capabilityDigest}
	for _, report := range reports {
		if report.Digest != "" {
			if err := VerifyLossReport(report); err != nil {
				return LossReport{}, err
			}
			if report.Target != target || report.AdapterID != adapterID || report.AdapterVersion != adapterVersion || report.CapabilityDigest != capabilityDigest {
				return LossReport{}, fmt.Errorf("loss report identity mismatch")
			}
		}
		value.Losses = append(value.Losses, report.Losses...)
	}
	return SealLossReport(value)
}

func (value LossReport) HasLosses() bool { return len(value.Losses) > 0 }

func deriveFidelity(losses []Loss) Fidelity {
	result := FidelityFull
	for _, loss := range losses {
		switch loss.Kind {
		case LossUnsupported:
			return FidelityBlocked
		case LossOmission:
			result = FidelityLossy
		case LossFallback:
			if result != FidelityLossy {
				result = FidelityLossy
			}
		case LossTransformation:
			if result == FidelityFull {
				result = FidelityPartial
			}
		}
	}
	return result
}

func validLossKind(value LossKind) bool {
	switch value {
	case LossTransformation, LossFallback, LossOmission, LossUnsupported:
		return true
	default:
		return false
	}
}

func sameLossIdentity(left, right Loss) bool {
	return left.ArtifactID == right.ArtifactID && left.Field == right.Field && left.Kind == right.Kind && left.Code == right.Code
}

func validSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}
