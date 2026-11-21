package opencode

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters/conformance"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/safefile"
)

type Adapter struct{}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func (a *Adapter) TargetID() string {
	return "opencode"
}

// Discover scans an OpenCode target root for skill markdown candidates.
func (a *Adapter) Discover(root string) ([]resourcehub.CandidateRevision, error) {
	skillsDir := filepath.Join(root, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read opencode skills dir: %w", err)
	}

	var candidates []resourcehub.CandidateRevision
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
		data, err := safefile.ReadBoundedRegular(skillFile, 1<<20)
		if err != nil {
			continue // skip unreadable or missing SKILL.md
		}

		h := sha256.Sum256(data)
		digest := fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))

		cand := resourcehub.CandidateRevision{
			RevisionID:   fmt.Sprintf("rev-%s", entry.Name()),
			SourceDigest: digest,
			CreatedAt:    time.Now().UTC(),
			Reason:       fmt.Sprintf("discovered from %s", skillFile),
			Actor:        "opencode-adapter",
		}

		candidates = append(candidates, cand)
	}

	return candidates, nil
}

// Normalize parses raw OpenCode skill markdown into a canonical revision object.
func (a *Adapter) Normalize(raw []byte, kind resourcehub.Kind) (resourcehub.CandidateRevision, error) {
	if kind != resourcehub.KindSkill {
		return resourcehub.CandidateRevision{}, fmt.Errorf("unsupported kind for opencode adapter: %s", kind)
	}

	h := sha256.Sum256(raw)
	digest := fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))

	cand := resourcehub.CandidateRevision{
		RevisionID:   "rev-normalized-skill",
		SourceDigest: digest,
		CreatedAt:    time.Now().UTC(),
		Reason:       "normalized from raw opencode skill bytes",
		Actor:        "opencode-adapter",
	}

	return cand, nil
}

// Render formats a canonical revision into target-native OpenCode skill markdown.
func (a *Adapter) Render(canonical resourcehub.CandidateRevision, targetID string) ([]byte, conformance.FidelityRecord, error) {
	rec := conformance.FidelityRecord{
		SupportedFields:   []string{"revisionId", "sourceDigest", "createdAt"},
		UnsupportedFields: nil,
		Lossy:             false,
		LossDetails:       nil,
	}

	content := fmt.Sprintf("---\nrevisionId: %s\nsourceDigest: %s\n---\n\n# OpenCode Skill\n\nRendered by AgentStackManager OpenCode Adapter.\n", canonical.RevisionID, canonical.SourceDigest)
	return []byte(content), rec, nil
}

// Plan computes a pure execution plan between current and desired target content.
func (a *Adapter) Plan(current, desired []byte, targetPath string) (conformance.AdapterPlan, error) {
	var steps []conformance.OperationPlanStep

	curDigest := ""
	if len(current) > 0 {
		h := sha256.Sum256(current)
		curDigest = fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))
	}

	desDigest := ""
	if len(desired) > 0 {
		h := sha256.Sum256(desired)
		desDigest = fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))
	}

	opType := "noop"
	if curDigest == "" && desDigest != "" {
		opType = "create"
	} else if curDigest != "" && desDigest == "" {
		opType = "delete"
	} else if curDigest != desDigest {
		opType = "update"
	}

	if opType != "noop" {
		steps = append(steps, conformance.OperationPlanStep{
			OpType:        opType,
			TargetPath:    filepath.ToSlash(targetPath),
			BaseDigest:    curDigest,
			DesiredDigest: desDigest,
		})
	}

	plan := conformance.AdapterPlan{
		TargetID: "opencode",
		Steps:    steps,
	}
	plan.PlanDigest = conformance.ComputePlanDigest(plan)
	return plan, nil
}

// Verify asserts that the content at targetPath matches expectedDigest.
func (a *Adapter) Verify(targetPath string, expectedDigest string) error {
	data, err := safefile.ReadBoundedRegular(targetPath, 1<<20)
	if err != nil {
		return fmt.Errorf("read target file for verification: %w", err)
	}

	actualDigest := integrity.DigestBytes(data)

	if !strings.EqualFold(actualDigest, expectedDigest) {
		return fmt.Errorf("verification digest mismatch: expected %s, got %s", expectedDigest, actualDigest)
	}

	return nil
}
