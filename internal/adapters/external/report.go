package external

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/agentstack/agentstack/internal/adapters"
	"github.com/agentstack/agentstack/internal/adapters/conformance"
	"github.com/agentstack/agentstack/internal/integrity"
)

type Mismatch struct {
	CaseID            string `json:"caseId"`
	Category          string `json:"category"`
	ReferenceStatus   string `json:"referenceStatus"`
	CandidateStatus   string `json:"candidateStatus"`
	ReferenceEvidence string `json:"referenceEvidence,omitempty"`
	CandidateEvidence string `json:"candidateEvidence,omitempty"`
	Reason            string `json:"reason"`
}

type DifferentialSummary struct {
	Cases        int `json:"cases"`
	Matched      int `json:"matched"`
	Mismatched   int `json:"mismatched"`
	Restrictions int `json:"restrictions"`
}

type ConformanceReport struct {
	APIVersion   string              `json:"apiVersion"`
	Protocol     string              `json:"protocol"`
	Descriptor   Descriptor          `json:"descriptor"`
	Intersection IntersectionReport  `json:"intersection"`
	Reference    conformance.Report  `json:"reference"`
	Candidate    conformance.Report  `json:"candidate"`
	Mismatches   []Mismatch          `json:"mismatches,omitempty"`
	Summary      DifferentialSummary `json:"summary"`
	Passed       bool                `json:"passed"`
	Digest       string              `json:"digest"`
}

func RunConformance(ctx context.Context, admission Admission, reference adapters.Adapter) (ConformanceReport, error) {
	candidate, err := Open(ctx, Config{Admission: admission, Reference: reference})
	if err != nil {
		return ConformanceReport{}, err
	}
	defer candidate.Close()
	_, _, intersection, err := candidate.CapabilityEvidence(ctx, admission.Environment)
	if err != nil {
		return ConformanceReport{}, err
	}
	referenceRegistry, err := adapters.NewRegistry(reference)
	if err != nil {
		return ConformanceReport{}, err
	}
	candidateRegistry, err := adapters.NewRegistry(candidate)
	if err != nil {
		return ConformanceReport{}, err
	}
	options := conformance.RunOptions{Environment: admission.Environment, Targets: []string{admission.Target}}
	referenceReport, err := conformance.RunEmbedded(ctx, referenceRegistry, options)
	if err != nil {
		return ConformanceReport{}, fmt.Errorf("run reviewed adapter conformance: %w", err)
	}
	if !referenceReport.Passed() {
		return ConformanceReport{}, fmt.Errorf("reviewed reference adapter failed its own conformance corpus")
	}
	candidateReport, err := conformance.RunEmbedded(ctx, candidateRegistry, options)
	if err != nil {
		return ConformanceReport{}, fmt.Errorf("run external adapter conformance: %w", err)
	}
	mismatches := compareConformanceReports(referenceReport, candidateReport)
	return SealConformanceReport(ConformanceReport{
		Protocol: ProtocolVersion, Descriptor: candidate.Descriptor(), Intersection: intersection,
		Reference: referenceReport, Candidate: candidateReport, Mismatches: mismatches,
	})
}

func SealConformanceReport(value ConformanceReport) (ConformanceReport, error) {
	if value.APIVersion == "" {
		value.APIVersion = ReportAPIVersion
	}
	value.Protocol = strings.TrimSpace(value.Protocol)
	if value.APIVersion != ReportAPIVersion || value.Protocol != ProtocolVersion {
		return ConformanceReport{}, fmt.Errorf("invalid external conformance report identity")
	}
	if err := VerifyDescriptor(value.Descriptor); err != nil {
		return ConformanceReport{}, err
	}
	if err := VerifyIntersectionReport(value.Intersection); err != nil {
		return ConformanceReport{}, err
	}
	if err := conformance.VerifyReport(value.Reference); err != nil {
		return ConformanceReport{}, fmt.Errorf("verify reference conformance report: %w", err)
	}
	if err := conformance.VerifyReport(value.Candidate); err != nil {
		return ConformanceReport{}, fmt.Errorf("verify candidate conformance report: %w", err)
	}
	if value.Descriptor.Target != value.Intersection.Target || value.Descriptor.AdapterID != value.Intersection.AdapterID || value.Descriptor.AdapterVersion != value.Intersection.AdapterVersion {
		return ConformanceReport{}, fmt.Errorf("external conformance descriptor and intersection identities differ")
	}
	value.Mismatches = append([]Mismatch(nil), value.Mismatches...)
	for i := range value.Mismatches {
		mismatch := &value.Mismatches[i]
		mismatch.CaseID = strings.TrimSpace(mismatch.CaseID)
		mismatch.Category = strings.TrimSpace(mismatch.Category)
		mismatch.ReferenceStatus = strings.TrimSpace(mismatch.ReferenceStatus)
		mismatch.CandidateStatus = strings.TrimSpace(mismatch.CandidateStatus)
		mismatch.ReferenceEvidence = strings.TrimSpace(mismatch.ReferenceEvidence)
		mismatch.CandidateEvidence = strings.TrimSpace(mismatch.CandidateEvidence)
		mismatch.Reason = strings.TrimSpace(mismatch.Reason)
		if mismatch.CaseID == "" || mismatch.Category == "" || mismatch.ReferenceStatus == "" || mismatch.CandidateStatus == "" || mismatch.Reason == "" {
			return ConformanceReport{}, fmt.Errorf("invalid external conformance mismatch at index %d", i)
		}
	}
	sort.Slice(value.Mismatches, func(i, j int) bool { return value.Mismatches[i].CaseID < value.Mismatches[j].CaseID })
	for i := 1; i < len(value.Mismatches); i++ {
		if value.Mismatches[i-1].CaseID == value.Mismatches[i].CaseID {
			return ConformanceReport{}, fmt.Errorf("duplicate external conformance mismatch %q", value.Mismatches[i].CaseID)
		}
	}
	expectedMismatches := compareConformanceReports(value.Reference, value.Candidate)
	if !equalMismatches(expectedMismatches, value.Mismatches) {
		return ConformanceReport{}, fmt.Errorf("external conformance mismatch set is inconsistent with the embedded reports")
	}
	cases := value.Reference.Summary.Total
	value.Summary = DifferentialSummary{
		Cases: cases, Matched: cases - len(value.Mismatches), Mismatched: len(value.Mismatches),
		Restrictions: len(value.Intersection.Changes),
	}
	value.Passed = value.Reference.Passed() && value.Candidate.Passed() && len(value.Mismatches) == 0
	value.Digest = ""
	digest, err := integrity.DigestJSON(value)
	if err != nil {
		return ConformanceReport{}, err
	}
	value.Digest = digest
	return value, nil
}

func VerifyConformanceReport(value ConformanceReport) error {
	if !validDigest(value.Digest) {
		return fmt.Errorf("invalid external conformance report digest")
	}
	originalSummary := value.Summary
	originalPassed := value.Passed
	expected, err := SealConformanceReport(value)
	if err != nil {
		return err
	}
	if expected.Summary != originalSummary || expected.Passed != originalPassed {
		return fmt.Errorf("external conformance derived summary mismatch")
	}
	if expected.Digest != value.Digest {
		return fmt.Errorf("external conformance report digest mismatch")
	}
	return nil
}

func compareConformanceReports(reference, candidate conformance.Report) []Mismatch {
	referenceCases := flattenCases(reference)
	candidateCases := flattenCases(candidate)
	ids := map[string]struct{}{}
	for id := range referenceCases {
		ids[id] = struct{}{}
	}
	for id := range candidateCases {
		ids[id] = struct{}{}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	mismatches := []Mismatch{}
	for _, id := range ordered {
		left, leftOK := referenceCases[id]
		right, rightOK := candidateCases[id]
		if !leftOK {
			mismatches = append(mismatches, Mismatch{CaseID: id, Category: right.Category, ReferenceStatus: "missing", CandidateStatus: string(right.Status), CandidateEvidence: right.EvidenceDigest, Reason: "candidate produced a case absent from the reviewed reference"})
			continue
		}
		if !rightOK {
			mismatches = append(mismatches, Mismatch{CaseID: id, Category: left.Category, ReferenceStatus: string(left.Status), CandidateStatus: "missing", ReferenceEvidence: left.EvidenceDigest, Reason: "candidate omitted a reviewed conformance case"})
			continue
		}
		if left.Status != right.Status || left.EvidenceDigest != right.EvidenceDigest {
			reason := "candidate evidence differs from the reviewed reference"
			if right.Reason != "" {
				reason = right.Reason
			}
			mismatches = append(mismatches, Mismatch{
				CaseID: id, Category: left.Category, ReferenceStatus: string(left.Status), CandidateStatus: string(right.Status),
				ReferenceEvidence: left.EvidenceDigest, CandidateEvidence: right.EvidenceDigest, Reason: reason,
			})
		}
	}
	return mismatches
}

func flattenCases(report conformance.Report) map[string]conformance.CaseResult {
	result := map[string]conformance.CaseResult{}
	for _, target := range report.Targets {
		for _, item := range target.Cases {
			result[item.ID] = item
		}
	}
	return result
}

func equalMismatches(left, right []Mismatch) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
