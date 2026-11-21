package conformance

import (
	"fmt"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/integrity"
)

type Status string

const (
	StatusPassed Status = "passed"
	StatusFailed Status = "failed"
)

type CaseResult struct {
	ID             string `json:"id"`
	Target         string `json:"target"`
	Category       string `json:"category"`
	Status         Status `json:"status"`
	EvidenceDigest string `json:"evidenceDigest,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type TargetResult struct {
	Target           string       `json:"target"`
	AdapterID        string       `json:"adapterId"`
	CapabilityDigest string       `json:"capabilityDigest"`
	Passed           bool         `json:"passed"`
	Cases            []CaseResult `json:"cases"`
}

type Summary struct {
	Targets int `json:"targets"`
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
}

type Report struct {
	APIVersion      string         `json:"apiVersion"`
	AdapterContract string         `json:"adapterContract"`
	CorpusDigest    string         `json:"corpusDigest"`
	Targets         []TargetResult `json:"targets"`
	Summary         Summary        `json:"summary"`
	Digest          string         `json:"digest"`
}

func SealReport(value Report) (Report, error) {
	if value.APIVersion == "" {
		value.APIVersion = ReportAPIVersion
	}
	value.AdapterContract = strings.TrimSpace(value.AdapterContract)
	value.CorpusDigest = strings.TrimSpace(value.CorpusDigest)
	if value.APIVersion != ReportAPIVersion || value.AdapterContract == "" || !validDigest(value.CorpusDigest) {
		return Report{}, fmt.Errorf("invalid conformance report identity")
	}
	value.Targets = append([]TargetResult(nil), value.Targets...)
	seenTargets := map[string]struct{}{}
	value.Summary = Summary{Targets: len(value.Targets)}
	for i := range value.Targets {
		target := &value.Targets[i]
		target.Target = strings.TrimSpace(target.Target)
		target.AdapterID = strings.TrimSpace(target.AdapterID)
		target.CapabilityDigest = strings.TrimSpace(target.CapabilityDigest)
		if target.Target == "" || target.AdapterID == "" || !validDigest(target.CapabilityDigest) {
			return Report{}, fmt.Errorf("invalid target result at index %d", i)
		}
		if _, duplicate := seenTargets[target.Target]; duplicate {
			return Report{}, fmt.Errorf("duplicate target result %q", target.Target)
		}
		seenTargets[target.Target] = struct{}{}
		target.Cases = append([]CaseResult(nil), target.Cases...)
		if len(target.Cases) == 0 {
			return Report{}, fmt.Errorf("target %q has no conformance cases", target.Target)
		}
		seenCases := map[string]struct{}{}
		target.Passed = true
		for j := range target.Cases {
			item := &target.Cases[j]
			item.ID = strings.TrimSpace(item.ID)
			item.Target = strings.TrimSpace(item.Target)
			item.Category = strings.TrimSpace(item.Category)
			item.EvidenceDigest = strings.TrimSpace(item.EvidenceDigest)
			item.Reason = strings.TrimSpace(item.Reason)
			if item.ID == "" || item.Target != target.Target || item.Category == "" {
				return Report{}, fmt.Errorf("invalid case result at target %q index %d", target.Target, j)
			}
			if _, duplicate := seenCases[item.ID]; duplicate {
				return Report{}, fmt.Errorf("duplicate case result %q", item.ID)
			}
			seenCases[item.ID] = struct{}{}
			switch item.Status {
			case StatusPassed:
				if item.Reason != "" || !validDigest(item.EvidenceDigest) {
					return Report{}, fmt.Errorf("passed case %q has invalid evidence", item.ID)
				}
				value.Summary.Passed++
			case StatusFailed:
				if item.Reason == "" || item.EvidenceDigest != "" {
					return Report{}, fmt.Errorf("failed case %q has invalid failure evidence", item.ID)
				}
				target.Passed = false
				value.Summary.Failed++
			default:
				return Report{}, fmt.Errorf("case %q has invalid status %q", item.ID, item.Status)
			}
			value.Summary.Total++
		}
		sort.Slice(target.Cases, func(i, j int) bool { return target.Cases[i].ID < target.Cases[j].ID })
	}
	sort.Slice(value.Targets, func(i, j int) bool { return value.Targets[i].Target < value.Targets[j].Target })
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return Report{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyReport(value Report) error {
	if !validDigest(value.Digest) {
		return fmt.Errorf("invalid conformance report digest")
	}
	expected, err := SealReport(value)
	if err != nil {
		return err
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("conformance report digest mismatch")
	}
	return nil
}

func (value Report) Passed() bool { return value.Summary.Failed == 0 }
