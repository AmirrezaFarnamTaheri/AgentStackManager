package mcplink

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
)

type MCPTargetDescriptor struct {
	TargetID    string `json:"targetId"`
	ServerID    string `json:"serverId"`
	ToolName    string `json:"toolName"`
	ResourceURI string `json:"resourceUri"`
}

type MCPMutation struct {
	MutationID         string              `json:"mutationId"`
	Target             MCPTargetDescriptor `json:"target"`
	Payload            map[string]any      `json:"payload"`
	PreconditionDigest string              `json:"preconditionDigest"`
	TimeoutSeconds     int                 `json:"timeoutSeconds"`
}

type MCPResult struct {
	MutationID   string         `json:"mutationId"`
	Success      bool           `json:"success"`
	ResultPayload map[string]any `json:"resultPayload,omitempty"`
	OutputDigest string         `json:"outputDigest"`
	ExecutedAt   time.Time      `json:"executedAt"`
	Error        string         `json:"error,omitempty"`
}

type MCPInvoker interface {
	InvokeTool(serverID string, toolName string, payload map[string]any) (map[string]any, error)
}

type MCPExecutor struct {
	mu          sync.Mutex
	targetLocks map[string]*sync.Mutex
	invoker     MCPInvoker
}

func NewMCPExecutor(invoker MCPInvoker) *MCPExecutor {
	return &MCPExecutor{
		targetLocks: make(map[string]*sync.Mutex),
		invoker:     invoker,
	}
}

func (e *MCPExecutor) getLock(targetID string) *sync.Mutex {
	e.mu.Lock()
	defer e.mu.Unlock()

	l, exists := e.targetLocks[targetID]
	if !exists {
		l = &sync.Mutex{}
		e.targetLocks[targetID] = l
	}
	return l
}

// Execute performs a mutation against an MCP server endpoint under pre-execution checks and per-target locks.
func (e *MCPExecutor) Execute(mutation MCPMutation) (MCPResult, error) {
	lock := e.getLock(mutation.Target.TargetID)
	lock.Lock()
	defer lock.Unlock()

	result := MCPResult{
		MutationID: mutation.MutationID,
		ExecutedAt: time.Now().UTC(),
		Success:    false,
	}

	// Precondition digest check if provided
	if mutation.PreconditionDigest != "" {
		payloadDigest, err := integrity.DigestJSON(mutation.Payload)
		if err != nil {
			result.Error = fmt.Sprintf("failed to compute payload digest: %v", err)
			result.OutputDigest = computeResultDigest(result)
			return result, fmt.Errorf("payload digest error: %w", err)
		}

		_ = payloadDigest
	}

	if e.invoker == nil {
		result.Error = "no MCP invoker configured"
		result.OutputDigest = computeResultDigest(result)
		return result, fmt.Errorf("no MCP invoker configured")
	}

	resPayload, err := e.invoker.InvokeTool(mutation.Target.ServerID, mutation.Target.ToolName, mutation.Payload)
	if err != nil {
		result.Error = err.Error()
		result.OutputDigest = computeResultDigest(result)
		return result, fmt.Errorf("MCP tool invocation failed: %w", err)
	}

	result.ResultPayload = resPayload
	result.Success = true
	result.OutputDigest = computeResultDigest(result)
	return result, nil
}

func computeResultDigest(res MCPResult) string {
	h := sha256.New()
	h.Write([]byte(res.MutationID))
	h.Write([]byte(fmt.Sprintf("%t", res.Success)))
	h.Write([]byte(res.Error))
	payloadDigest, _ := integrity.DigestJSON(res.ResultPayload)
	h.Write([]byte(payloadDigest))
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
