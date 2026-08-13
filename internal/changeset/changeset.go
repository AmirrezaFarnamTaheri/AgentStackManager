package changeset

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/agentstack/agentstack/internal/adapters/conformance"
	"github.com/agentstack/agentstack/internal/adapters/mcplink"
	"github.com/agentstack/agentstack/internal/executor"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

type ChangeSet struct {
	ChangeSetID        string                              `json:"changeSetId"`
	TargetID           string                              `json:"targetId"`
	FileOps            []executor.FileOperation           `json:"fileOps,omitempty"`
	MCPMutations       []mcplink.MCPMutation               `json:"mcpMutations,omitempty"`
	BeforeDescriptors  map[string]executor.FileDescriptor `json:"beforeDescriptors,omitempty"`
	AfterDescriptors   map[string]executor.FileDescriptor `json:"afterDescriptors,omitempty"`
	FidelityRecords    map[string]conformance.FidelityRecord `json:"fidelityRecords,omitempty"`
	OwnershipManifests []resourcehub.OwnershipManifest    `json:"ownershipManifests,omitempty"`
	RetryPolicy        string                              `json:"retryPolicy"`
	RecoveryDetail     string                              `json:"recoveryDetail"`
	CreatedAt          time.Time                           `json:"createdAt"`
	ChangeSetDigest    string                              `json:"changeSetDigest"`
}

type ChangeSetResult struct {
	ChangeSetID       string                     `json:"changeSetId"`
	Success           bool                       `json:"success"`
	PartialSuccess    bool                       `json:"partialSuccess"`
	DeploymentReceipt executor.DeploymentReceipt `json:"deploymentReceipt"`
	MCPResults        []mcplink.MCPResult        `json:"mcpResults,omitempty"`
	ExecutedAt        time.Time                  `json:"executedAt"`
	ResultDigest      string                     `json:"resultDigest"`
}

// ComposeChangeSet builds a validated, deterministically sealed ChangeSet.
func ComposeChangeSet(
	targetID string,
	fileOps []executor.FileOperation,
	mcpMutations []mcplink.MCPMutation,
	fidelities map[string]conformance.FidelityRecord,
	ownerships []resourcehub.OwnershipManifest,
	retryPolicy string,
	recoveryDetail string,
	now time.Time,
) (ChangeSet, error) {
	if targetID == "" {
		return ChangeSet{}, fmt.Errorf("target ID cannot be empty")
	}

	cs := ChangeSet{
		ChangeSetID:        fmt.Sprintf("cs-%s-%d", targetID, now.UnixNano()),
		TargetID:           targetID,
		FileOps:            fileOps,
		MCPMutations:       mcpMutations,
		BeforeDescriptors:  make(map[string]executor.FileDescriptor),
		AfterDescriptors:   make(map[string]executor.FileDescriptor),
		FidelityRecords:    fidelities,
		OwnershipManifests: ownerships,
		RetryPolicy:        retryPolicy,
		RecoveryDetail:     recoveryDetail,
		CreatedAt:          now.UTC(),
	}

	for _, op := range fileOps {
		cs.BeforeDescriptors[op.TargetPath] = op.BaseDescriptor
		cs.AfterDescriptors[op.TargetPath] = op.DesiredDescriptor
	}

	sealed, err := SealChangeSet(cs)
	if err != nil {
		return ChangeSet{}, fmt.Errorf("seal changeset: %w", err)
	}
	return sealed, nil
}

// SealChangeSet computes deterministic SHA-256 digest of the ChangeSet.
func SealChangeSet(cs ChangeSet) (ChangeSet, error) {
	cs.ChangeSetDigest = ""
	d, err := integrity.DigestJSON(cs)
	if err != nil {
		return ChangeSet{}, fmt.Errorf("digest changeset: %w", err)
	}
	cs.ChangeSetDigest = d
	return cs, nil
}

// ExecuteChangeSet runs both file deployment and MCP mutations, reporting honest partial success status.
func ExecuteChangeSet(
	cs ChangeSet,
	depExec *executor.DeploymentExecutor,
	mcpExec *mcplink.MCPExecutor,
) (ChangeSetResult, error) {
	res := ChangeSetResult{
		ChangeSetID: cs.ChangeSetID,
		ExecutedAt:  time.Now().UTC(),
		Success:     false,
	}

	var depErr error
	if len(cs.FileOps) > 0 && depExec != nil {
		receipt, err := depExec.Execute(cs.TargetID, cs.ChangeSetDigest, cs.FileOps)
		res.DeploymentReceipt = receipt
		if err != nil {
			depErr = err
		}
	}

	mcpSuccessCount := 0
	mcpFailCount := 0
	if len(cs.MCPMutations) > 0 && mcpExec != nil {
		for _, mut := range cs.MCPMutations {
			mcpRes, err := mcpExec.Execute(mut)
			res.MCPResults = append(res.MCPResults, mcpRes)
			if err != nil || !mcpRes.Success {
				mcpFailCount++
			} else {
				mcpSuccessCount++
			}
		}
	}

	// Honest status evaluation
	filesOk := res.DeploymentReceipt.Success || len(cs.FileOps) == 0
	mcpOk := mcpFailCount == 0 || len(cs.MCPMutations) == 0

	if filesOk && mcpOk {
		res.Success = true
		res.PartialSuccess = false
	} else if (res.DeploymentReceipt.Success && mcpFailCount > 0) || (mcpSuccessCount > 0 && (!res.DeploymentReceipt.Success || depErr != nil)) {
		res.Success = false
		res.PartialSuccess = true
	} else {
		res.Success = false
		res.PartialSuccess = false
	}

	res.ResultDigest = computeResultDigest(res)
	if !res.Success && !res.PartialSuccess {
		return res, fmt.Errorf("changeset execution failed: depErr=%v, mcpFails=%d", depErr, mcpFailCount)
	}
	return res, nil
}

func computeResultDigest(res ChangeSetResult) string {
	h := sha256.New()
	h.Write([]byte(res.ChangeSetID))
	h.Write([]byte(fmt.Sprintf("%t:%t", res.Success, res.PartialSuccess)))
	h.Write([]byte(res.DeploymentReceipt.ReceiptDigest))
	for _, m := range res.MCPResults {
		h.Write([]byte(m.OutputDigest))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
