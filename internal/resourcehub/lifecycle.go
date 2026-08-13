package resourcehub

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/strictjson"
)

// NewLifecycleRecord creates a new record initialized in StateObserved.
func NewLifecycleRecord(id, artifactID, artifactDigest, sourceURI, sourceDigest string, now time.Time) (LifecycleRecord, error) {
	if strings.TrimSpace(id) == "" {
		return LifecycleRecord{}, fmt.Errorf("lifecycle record ID cannot be empty")
	}
	if strings.TrimSpace(artifactID) == "" {
		return LifecycleRecord{}, fmt.Errorf("artifact ID cannot be empty")
	}
	if strings.TrimSpace(artifactDigest) == "" {
		return LifecycleRecord{}, fmt.Errorf("artifact digest cannot be empty")
	}

	rec := LifecycleRecord{
		APIVersion:     LifecycleAPIVersion,
		ID:             id,
		ArtifactID:     artifactID,
		ArtifactDigest: artifactDigest,
		State:          StateObserved,
		SourceURI:      sourceURI,
		SourceDigest:   sourceDigest,
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
		Transitions: []LifecycleTransition{
			{
				ID:             fmt.Sprintf("tr-0001"),
				From:           "",
				To:             StateObserved,
				Actor:          "system",
				Reason:         "initial observation",
				EvidenceDigest: sourceDigest,
				Timestamp:      now.UTC(),
			},
		},
	}
	return SealLifecycleRecord(rec)
}

// IsValidTransition checks whether a transition from state 'from' to 'to' is allowed.
func IsValidTransition(from, to LifecycleState) bool {
	if from == to {
		return false
	}
	// Cannot exit from retired except via ReverseRetirement which handles its own path
	if from == StateRetired {
		return false
	}

	// Universal exit states from any active state
	if to == StateQuarantined || to == StateIgnored || to == StateRetired {
		return true
	}

	switch from {
	case StateObserved:
		return to == StateParsed || to == StateAlias
	case StateParsed:
		return to == StateClassified
	case StateClassified:
		return to == StateCandidate
	case StateCandidate:
		return to == StateCanonical
	case StateCanonical:
		return to == StateProjected
	case StateProjected:
		return to == StateVerified
	case StateVerified:
		return to == StateProjected
	case StateQuarantined:
		return to == StateObserved
	default:
		return false
	}
}

// TransitionTo validates and performs a state transition on the lifecycle record.
func TransitionTo(rec *LifecycleRecord, to LifecycleState, actor, reason, evidenceDigest string, now time.Time) error {
	if rec == nil {
		return fmt.Errorf("nil lifecycle record")
	}
	if rec.APIVersion != LifecycleAPIVersion {
		return fmt.Errorf("unsupported lifecycle record API version %q", rec.APIVersion)
	}

	if !IsValidTransition(rec.State, to) {
		return fmt.Errorf("illegal state transition from %q to %q (silent promotion or invalid path rejected)", rec.State, to)
	}

	trID := fmt.Sprintf("tr-%04d", len(rec.Transitions)+1)
	tr := LifecycleTransition{
		ID:             trID,
		From:           rec.State,
		To:             to,
		Actor:          actor,
		Reason:         reason,
		EvidenceDigest: evidenceDigest,
		Timestamp:      now.UTC(),
	}

	rec.Transitions = append(rec.Transitions, tr)
	rec.State = to
	rec.UpdatedAt = now.UTC()

	sealed, err := SealLifecycleRecord(*rec)
	if err != nil {
		return fmt.Errorf("seal lifecycle record after transition: %w", err)
	}
	*rec = sealed
	return nil
}

// AddCandidateRevision appends a new candidate revision when the source changes.
func AddCandidateRevision(rec *LifecycleRecord, newSourceDigest, actor, reason string, now time.Time) error {
	if rec == nil {
		return fmt.Errorf("nil lifecycle record")
	}
	if strings.TrimSpace(newSourceDigest) == "" {
		return fmt.Errorf("new source digest cannot be empty")
	}
	if newSourceDigest == rec.SourceDigest {
		return fmt.Errorf("new source digest matches current source digest")
	}

	revID := fmt.Sprintf("rev-%04d", len(rec.CandidateRevisions)+1)
	rev := CandidateRevision{
		RevisionID:   revID,
		SourceDigest: newSourceDigest,
		CreatedAt:    now.UTC(),
		Reason:       reason,
		Actor:        actor,
	}

	rec.CandidateRevisions = append(rec.CandidateRevisions, rev)
	rec.SourceDigest = newSourceDigest
	rec.UpdatedAt = now.UTC()

	sealed, err := SealLifecycleRecord(*rec)
	if err != nil {
		return fmt.Errorf("seal lifecycle record after candidate revision: %w", err)
	}
	*rec = sealed
	return nil
}

// AddAlias adds an alias to the record, rejecting duplicates.
func AddAlias(rec *LifecycleRecord, alias string) error {
	if rec == nil {
		return fmt.Errorf("nil lifecycle record")
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return fmt.Errorf("alias cannot be empty")
	}

	for _, existing := range rec.Aliases {
		if existing == alias {
			return fmt.Errorf("alias collision: alias %q already registered on record %q", alias, rec.ID)
		}
	}

	rec.Aliases = append(rec.Aliases, alias)
	sort.Strings(rec.Aliases)
	rec.UpdatedAt = time.Now().UTC()

	sealed, err := SealLifecycleRecord(*rec)
	if err != nil {
		return fmt.Errorf("seal lifecycle record after alias: %w", err)
	}
	*rec = sealed
	return nil
}

// Retire transitions the record to StateRetired with RetirementInfo.
func Retire(rec *LifecycleRecord, actor, reason, evidenceDigest string, now time.Time) error {
	if rec == nil {
		return fmt.Errorf("nil lifecycle record")
	}
	if rec.State == StateRetired {
		return fmt.Errorf("record %q is already retired", rec.ID)
	}

	if err := TransitionTo(rec, StateRetired, actor, reason, evidenceDigest, now); err != nil {
		return err
	}

	rec.Retirement = &RetirementInfo{
		RetiredBy: actor,
		Reason:    reason,
		RetiredAt: now.UTC(),
	}

	sealed, err := SealLifecycleRecord(*rec)
	if err != nil {
		return err
	}
	*rec = sealed
	return nil
}

// ReverseRetirement restores a retired record to its prior state.
func ReverseRetirement(rec *LifecycleRecord, actor, reason, evidenceDigest string, now time.Time) error {
	if rec == nil {
		return fmt.Errorf("nil lifecycle record")
	}
	if rec.State != StateRetired {
		return fmt.Errorf("record %q is not in retired state", rec.ID)
	}
	if rec.Retirement == nil {
		return fmt.Errorf("record %q missing retirement info", rec.ID)
	}

	// Find the state prior to retirement
	priorState := StateObserved
	for i := len(rec.Transitions) - 1; i >= 0; i-- {
		tr := rec.Transitions[i]
		if tr.To == StateRetired && tr.From != "" {
			priorState = tr.From
			break
		}
	}

	trID := fmt.Sprintf("tr-%04d", len(rec.Transitions)+1)
	tr := LifecycleTransition{
		ID:             trID,
		From:           StateRetired,
		To:             priorState,
		Actor:          actor,
		Reason:         fmt.Sprintf("retirement reversal: %s", reason),
		EvidenceDigest: evidenceDigest,
		Timestamp:      now.UTC(),
	}

	rec.Transitions = append(rec.Transitions, tr)
	rec.State = priorState
	rec.Retirement.Reversal = &RetirementReversal{
		ReversedBy: actor,
		Reason:     reason,
		ReversedAt: now.UTC(),
	}
	rec.UpdatedAt = now.UTC()

	sealed, err := SealLifecycleRecord(*rec)
	if err != nil {
		return fmt.Errorf("seal lifecycle record after retirement reversal: %w", err)
	}
	*rec = sealed
	return nil
}

// SealLifecycleRecord normalizes the record and computes a SHA-256 digest.
func SealLifecycleRecord(rec LifecycleRecord) (LifecycleRecord, error) {
	if rec.APIVersion == "" {
		rec.APIVersion = LifecycleAPIVersion
	}
	if rec.Aliases == nil {
		rec.Aliases = []string{}
	} else {
		sort.Strings(rec.Aliases)
	}
	if rec.CandidateRevisions == nil {
		rec.CandidateRevisions = []CandidateRevision{}
	}
	if rec.Transitions == nil {
		rec.Transitions = []LifecycleTransition{}
	}

	rec.Digest = ""
	digest, err := integrity.DigestJSON(rec)
	if err != nil {
		return LifecycleRecord{}, fmt.Errorf("digest lifecycle record: %w", err)
	}
	rec.Digest = digest
	return rec, nil
}

// VerifyLifecycleRecord validates that rec.Digest matches SealLifecycleRecord(rec).Digest.
func VerifyLifecycleRecord(rec LifecycleRecord) error {
	if rec.APIVersion != LifecycleAPIVersion {
		return fmt.Errorf("unsupported lifecycle record API version %q", rec.APIVersion)
	}
	if rec.Digest == "" {
		return fmt.Errorf("lifecycle record digest is empty")
	}
	expected, err := SealLifecycleRecord(rec)
	if err != nil {
		return err
	}
	if expected.Digest != rec.Digest {
		return fmt.Errorf("lifecycle record %q digest mismatch: got %q, expected %q", rec.ID, rec.Digest, expected.Digest)
	}
	return nil
}

// UnmarshalLifecycleRecordJSON parses JSON data and enforces APIVersion checks.
func UnmarshalLifecycleRecordJSON(data []byte) (LifecycleRecord, error) {
	var rec LifecycleRecord
	if err := strictjson.Decode(data, &rec); err != nil {
		return LifecycleRecord{}, fmt.Errorf("decode lifecycle record: %w", err)
	}
	if rec.APIVersion != LifecycleAPIVersion {
		return LifecycleRecord{}, fmt.Errorf("unsupported lifecycle record API version %q", rec.APIVersion)
	}
	if err := VerifyLifecycleRecord(rec); err != nil {
		return LifecycleRecord{}, fmt.Errorf("verify unmarshaled lifecycle record: %w", err)
	}
	return rec, nil
}
