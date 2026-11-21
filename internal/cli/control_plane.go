package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/adapters/opencode"
	"github.com/agentstack/agentstack/internal/executor"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/observation"
	"github.com/agentstack/agentstack/internal/resourcehub"
	"github.com/agentstack/agentstack/internal/similarity"
	"github.com/agentstack/agentstack/internal/targetcatalog"
)

type ControlPlaneAuditReport struct {
	TotalDiscovered int               `json:"totalDiscovered"`
	TotalTargets    int               `json:"totalTargets"`
	DuplicatesFound int               `json:"duplicatesFound"`
	DriftDetected   int               `json:"driftDetected"`
	TargetStatuses  map[string]string `json:"targetStatuses"`
}

func (c *CLI) runControlPlane(args []string) int {
	if len(args) == 0 {
		return c.failUsage(fmt.Errorf("control-plane requires subcommand: audit, reconcile, or verify"))
	}
	switch args[0] {
	case "audit":
		return c.runAudit(args[1:])
	case "reconcile":
		return c.runReconcile(args[1:])
	default:
		return c.failUsage(fmt.Errorf("unknown control-plane subcommand: %s", args[0]))
	}
}

func (c *CLI) runAudit(args []string) int {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	rootDir := fs.String("root", ".", "Root directory of repository")
	mode := fs.String("mode", "shadow", "Execution mode: shadow or write")
	outPath := fs.String("out", "", "Output path for JSON audit report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	report := ControlPlaneAuditReport{
		TargetStatuses: make(map[string]string),
	}

	// 1. Observe candidates using scanner
	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	scanRes, err := scanner.Scan(context.Background(), filepath.Join(*rootDir, ".gemini"))
	if err == nil {
		report.TotalDiscovered = len(scanRes.Observations)
	}

	// 2. Build target catalog
	cat, err := targetcatalog.New()
	if err == nil {
		targets := cat.Schema().Targets
		report.TotalTargets = len(targets)
		for _, t := range targets {
			report.TargetStatuses[t.ID] = "registered"
		}
	}

	// 3. Identity & similarity indexing
	idx := resourcehub.NewIdentityIndex()
	for _, obs := range scanRes.Observations {
		idx.IndexPayload(obs.ID, obs.ID, obs.PrimaryPath, obs.Digest)
	}
	report.DuplicatesFound = len(idx.ExactDuplicates())

	// 4. Drift check across target projections
	targetDir := filepath.Join(*rootDir, ".agents", "skills")
	isManaged, manifest, err := resourcehub.InspectManagedProjection(targetDir)
	if err == nil && isManaged {
		skillPath := filepath.Join(targetDir, filepath.Base(manifest.ResourceID), "SKILL.md")
		data, readErr := os.ReadFile(skillPath)
		if readErr != nil {
			report.DriftDetected++
		} else {
			actualDigest := integrity.DigestBytes(data)
			if !strings.EqualFold(actualDigest, manifest.ArtifactDigest) {
				report.DriftDetected++
			}
		}
	}

	if *outPath != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			_ = os.WriteFile(*outPath, data, 0o644)
		}
	}

	return c.printJSON(map[string]any{
		"mode":    *mode,
		"rootDir": *rootDir,
		"report":  report,
	})
}

func (c *CLI) runReconcile(args []string) int {
	fs := flag.NewFlagSet("reconcile", flag.ContinueOnError)
	fs.SetOutput(c.errWriter())
	rootDir := fs.String("root", ".", "Root directory")
	targetID := fs.String("target", "opencode", "Target adapter ID")
	mode := fs.String("mode", "shadow", "Execution mode: shadow or write")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	scanner := observation.NewScanner(observation.DefaultScannerBudgets(), observation.DefaultScanPolicy())
	scanRes, _ := scanner.Scan(context.Background(), filepath.Join(*rootDir, ".gemini"))

	idx := resourcehub.NewIdentityIndex()
	for _, obs := range scanRes.Observations {
		idx.IndexPayload(obs.ID, obs.ID, obs.PrimaryPath, obs.Digest)
	}

	duplicates := idx.ExactDuplicates()
	var reconciliations []map[string]any

	for _, dup := range duplicates {
		var candList []similarity.ConsolidationCandidate
		for i, loc := range dup.Locations {
			candList = append(candList, similarity.ConsolidationCandidate{
				ResourceID:        fmt.Sprintf("%s-%d", dup.CanonicalID, i),
				IsOperatorChoice:  i == 0,
				IsGovernedSource:  strings.Contains(loc, ".gemini"),
				CompletenessScore: 100,
				IsValidSchema:     true,
			})
		}
		winner, err := similarity.SelectCanonicalWinner(candList)
		if err == nil {
			dec, _ := similarity.CreateConsolidationDecision(
				dup.CanonicalID,
				winner.ResourceID,
				"identity-reconciliation",
				"target-update",
				"backup-restore",
				similarity.RelationshipAlternative,
				dup.Locations[:1],
				dup.Locations[1:],
				"agentstack-cli",
				time.Now().UTC(),
			)
			reconciliations = append(reconciliations, map[string]any{
				"canonicalID": dup.CanonicalID,
				"winnerID":    dec.CanonicalWinnerID,
				"decision":    dec,
			})
		}
	}

	if *mode == "write" {
		adapter := opencode.NewAdapter()
		targetFile := filepath.Join(*rootDir, ".agents", "skills", "reconciled", "SKILL.md")
		plan, err := adapter.Plan(nil, []byte("# Reconciled Skill\n"), targetFile)
		if err == nil {
			var fileOps []executor.FileOperation
			for i, step := range plan.Steps {
				fileOps = append(fileOps, executor.FileOperation{
					OperationID: fmt.Sprintf("op-%d", i),
					TargetID:    plan.TargetID,
					TargetPath:  step.TargetPath,
					OpType:      step.OpType,
					BaseDescriptor: executor.FileDescriptor{
						Path:   step.TargetPath,
						Digest: step.BaseDigest,
						Exists: step.BaseDigest != "",
					},
					DesiredDescriptor: executor.FileDescriptor{
						Path:   step.TargetPath,
						Digest: step.DesiredDigest,
						Exists: true,
					},
					DesiredContent: []byte("# Reconciled Skill\n"),
					OwnershipMarker: resourcehub.OwnershipManifest{
						ResourceID: "reconciled-skill",
						ArtifactID: fmt.Sprintf("op-%d", i),
					},
					BackupRequired: true,
				})
			}
			exec := executor.NewDeploymentExecutor(filepath.Join(*rootDir, ".agentstack", "backups"))
			receipt, err := exec.Execute(*targetID, plan.PlanDigest, fileOps)
			if err == nil {
				return c.printJSON(map[string]any{
					"mode":            *mode,
					"targetID":        *targetID,
					"reconciliations": reconciliations,
					"receipt":         receipt,
				})
			}
		}
	}

	return c.printJSON(map[string]any{
		"mode":            *mode,
		"targetID":        *targetID,
		"reconciliations": reconciliations,
	})
}
