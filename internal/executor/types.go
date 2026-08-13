package executor

import (
	"time"

	"github.com/agentstack/agentstack/internal/resourcehub"
)

type FileDescriptor struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	Exists bool   `json:"exists"`
	IsDir  bool   `json:"isDir"`
}

type FileOperation struct {
	OperationID       string                      `json:"operationId"`
	TargetID          string                      `json:"targetId"`
	TargetPath        string                      `json:"targetPath"`
	OpType            string                      `json:"opType"` // "create", "update", "delete"
	BaseDescriptor    FileDescriptor              `json:"baseDescriptor"`
	DesiredDescriptor FileDescriptor              `json:"desiredDescriptor"`
	DesiredContent    []byte                      `json:"-"`
	OwnershipMarker   resourcehub.OwnershipManifest `json:"ownershipMarker"`
	BackupRequired    bool                        `json:"backupRequired"`
}

type BackupState struct {
	BackupID       string      `json:"backupId"`
	TargetPath     string      `json:"targetPath"`
	BackupPath     string      `json:"backupPath"`
	Existed        bool        `json:"existed"`
	IsDir          bool        `json:"isDir"`
	OriginalDigest string      `json:"originalDigest"`
	OriginalMode   uint32      `json:"originalMode"`
	CreatedAt      time.Time   `json:"createdAt"`
}

type DeploymentReceipt struct {
	ReceiptID    string        `json:"receiptId"`
	PlanDigest   string        `json:"planDigest"`
	TargetID     string        `json:"targetId"`
	AppliedAt    time.Time     `json:"appliedAt"`
	Operations   []string      `json:"operations"`
	BackupStates []BackupState `json:"backupStates"`
	Success      bool          `json:"success"`
	ErrorReason  string        `json:"errorReason,omitempty"`
	ReceiptDigest string       `json:"receiptDigest"`
}
