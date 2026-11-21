package observation

import (
	"fmt"
	"sort"
	"time"

	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const APIVersion = "observation.asm.dev/v1alpha1"

type ObjectType string

const (
	ObjectTypeFile      ObjectType = "file"
	ObjectTypeDirectory ObjectType = "directory"
	ObjectTypeSymlink   ObjectType = "symlink"
	ObjectTypeJunction  ObjectType = "junction"
	ObjectTypeReparse   ObjectType = "reparse"
)

type DiagnosticSeverity string

const (
	SeverityError   DiagnosticSeverity = "error"
	SeverityWarning DiagnosticSeverity = "warning"
	SeverityInfo    DiagnosticSeverity = "info"
)

type DiagnosticKind string

const (
	DiagBrokenJunction   DiagnosticKind = "broken_junction"
	DiagCycle            DiagnosticKind = "cycle"
	DiagPermissionError  DiagnosticKind = "permission_error"
	DiagCaseCollision    DiagnosticKind = "case_collision"
	DiagUnrecognizedType DiagnosticKind = "unrecognized_type"
	DiagExcludedPath     DiagnosticKind = "excluded_path"
	DiagBudgetExceeded   DiagnosticKind = "budget_exceeded"
)

type ScannerBudgets struct {
	MaxDepth       int   `json:"maxDepth"`
	MaxFiles       int   `json:"maxFiles"`
	MaxBytes       int64 `json:"maxBytes"`
	MaxDiagnostics int   `json:"maxDiagnostics"`
	MaxReparseHops int   `json:"maxReparseHops"`
}

func DefaultScannerBudgets() ScannerBudgets {
	return ScannerBudgets{
		MaxDepth:       32,
		MaxFiles:       50_000,
		MaxBytes:       512 << 20, // 512 MB
		MaxDiagnostics: 1_000,
		MaxReparseHops: 10,
	}
}

type ScanPolicy struct {
	ExcludedNames  []string `json:"excludedNames"`
	FollowSymlinks bool     `json:"followSymlinks"`
}

func DefaultScanPolicy() ScanPolicy {
	return ScanPolicy{
		ExcludedNames: []string{
			".omx", "skills_old", "backups", "sync-state",
			"node_modules", ".git", "vendor", "dist", "build", "cache", "coverage",
		},
		FollowSymlinks: false,
	}
}

type PhysicalIdentity struct {
	Key    string `json:"key"`
	SHA256 string `json:"sha256,omitempty"`
}

type ObjectObservation struct {
	ID               string           `json:"id"`
	PhysicalIdentity PhysicalIdentity `json:"physicalIdentity"`
	Type             ObjectType       `json:"type"`
	Size             int64            `json:"size"`
	Digest           string           `json:"digest,omitempty"`
	PrimaryPath      string           `json:"primaryPath"`
	Aliases          []string         `json:"aliases,omitempty"`
	ReparseChain     []string         `json:"reparseChain,omitempty"`
}

type Diagnostic struct {
	Kind     DiagnosticKind     `json:"kind"`
	Severity DiagnosticSeverity `json:"severity"`
	Path     string             `json:"path"`
	Message  string             `json:"message"`
	Error    string             `json:"error,omitempty"`
}

type ScanResult struct {
	APIVersion   string              `json:"apiVersion"`
	ScanID       string              `json:"scanId"`
	RootPath     string              `json:"rootPath"`
	StartedAt    time.Time           `json:"startedAt"`
	CompletedAt  time.Time           `json:"completedAt"`
	Observations []ObjectObservation `json:"observations"`
	Diagnostics  []Diagnostic        `json:"diagnostics"`
	FilesScanned int                 `json:"filesScanned"`
	BytesScanned int64               `json:"bytesScanned"`
	Digest       string              `json:"digest"`
}

// SealScanResult normalizes observations, diagnostics, and computes a SHA-256 digest.
func SealScanResult(result ScanResult) (ScanResult, error) {
	if result.APIVersion == "" {
		result.APIVersion = APIVersion
	}
	if result.Observations == nil {
		result.Observations = []ObjectObservation{}
	} else {
		for i := range result.Observations {
			if result.Observations[i].Aliases == nil {
				result.Observations[i].Aliases = []string{}
			} else {
				sort.Strings(result.Observations[i].Aliases)
			}
			if result.Observations[i].ReparseChain == nil {
				result.Observations[i].ReparseChain = []string{}
			}
		}
		sort.Slice(result.Observations, func(i, j int) bool {
			return result.Observations[i].PrimaryPath < result.Observations[j].PrimaryPath
		})
	}
	if result.Diagnostics == nil {
		result.Diagnostics = []Diagnostic{}
	} else {
		sort.Slice(result.Diagnostics, func(i, j int) bool {
			if result.Diagnostics[i].Path != result.Diagnostics[j].Path {
				return result.Diagnostics[i].Path < result.Diagnostics[j].Path
			}
			return result.Diagnostics[i].Kind < result.Diagnostics[j].Kind
		})
	}

	result.Digest = ""
	digest, err := integrity.DigestJSON(result)
	if err != nil {
		return ScanResult{}, fmt.Errorf("digest scan result: %w", err)
	}
	result.Digest = digest
	return result, nil
}

// VerifyScanResult checks scan result digest against its sealed digest.
func VerifyScanResult(result ScanResult) error {
	if result.APIVersion != APIVersion {
		return fmt.Errorf("unsupported scan result API version %q", result.APIVersion)
	}
	if result.Digest == "" {
		return fmt.Errorf("scan result digest is empty")
	}
	expected, err := SealScanResult(result)
	if err != nil {
		return err
	}
	if expected.Digest != result.Digest {
		return fmt.Errorf("scan result %q digest mismatch: got %q, expected %q", result.ScanID, result.Digest, expected.Digest)
	}
	return nil
}

// UnmarshalScanResultJSON decodes and verifies a scan result from JSON bytes.
func UnmarshalScanResultJSON(data []byte) (ScanResult, error) {
	var result ScanResult
	if err := strictjson.Decode(data, &result); err != nil {
		return ScanResult{}, fmt.Errorf("decode scan result: %w", err)
	}
	if result.APIVersion != APIVersion {
		return ScanResult{}, fmt.Errorf("unsupported scan result API version %q", result.APIVersion)
	}
	if err := VerifyScanResult(result); err != nil {
		return ScanResult{}, fmt.Errorf("verify scan result: %w", err)
	}
	return result, nil
}
