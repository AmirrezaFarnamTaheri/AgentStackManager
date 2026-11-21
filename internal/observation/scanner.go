package observation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Scanner struct {
	Budgets ScannerBudgets
	Policy  ScanPolicy
}

func NewScanner(budgets ScannerBudgets, policy ScanPolicy) *Scanner {
	if budgets.MaxFiles <= 0 {
		budgets = DefaultScannerBudgets()
	}
	if len(policy.ExcludedNames) == 0 {
		policy = DefaultScanPolicy()
	}
	return &Scanner{
		Budgets: budgets,
		Policy:  policy,
	}
}

func (s *Scanner) Scan(ctx context.Context, root string) (ScanResult, error) {
	cleanRoot := filepath.Clean(root)
	startTime := time.Now().UTC()

	scanID := fmt.Sprintf("scan-%x", sha256.Sum256([]byte(cleanRoot+startTime.Format(time.RFC3339Nano))))[:16]

	info, err := os.Stat(cleanRoot)
	if err != nil {
		return ScanResult{}, fmt.Errorf("stat scan root %s: %w", cleanRoot, err)
	}

	if !info.IsDir() {
		return ScanResult{}, fmt.Errorf("scan root %s is not a directory", cleanRoot)
	}

	obsMap := make(map[string]*ObjectObservation) // Key: PhysicalIdentity.Key -> ObjectObservation
	var diagnostics []Diagnostic
	caseMap := make(map[string]string) // Lowercase path -> original path for case collision check

	filesScanned := 0
	var bytesScanned int64

	excludedSet := make(map[string]struct{})
	for _, name := range s.Policy.ExcludedNames {
		excludedSet[strings.ToLower(name)] = struct{}{}
	}

	addDiag := func(diag Diagnostic) {
		if len(diagnostics) < s.Budgets.MaxDiagnostics {
			diagnostics = append(diagnostics, diag)
		}
	}

	err = filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if walkErr != nil {
			addDiag(Diagnostic{
				Kind:     DiagPermissionError,
				Severity: SeverityWarning,
				Path:     path,
				Message:  "access permission error during walk",
				Error:    walkErr.Error(),
			})
			return nil
		}

		if path == cleanRoot {
			return nil
		}

		rel, err := filepath.Rel(cleanRoot, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)

		// Check depth budget
		depth := strings.Count(relSlash, "/") + 1
		if depth > s.Budgets.MaxDepth {
			addDiag(Diagnostic{
				Kind:     DiagBudgetExceeded,
				Severity: SeverityWarning,
				Path:     relSlash,
				Message:  fmt.Sprintf("scan depth budget (%d) exceeded", s.Budgets.MaxDepth),
			})
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Check case collision
		lowerPath := strings.ToLower(relSlash)
		if origPath, exists := caseMap[lowerPath]; exists && origPath != relSlash {
			addDiag(Diagnostic{
				Kind:     DiagCaseCollision,
				Severity: SeverityError,
				Path:     relSlash,
				Message:  fmt.Sprintf("case collision detected between %q and %q", relSlash, origPath),
			})
		} else {
			caseMap[lowerPath] = relSlash
		}

		// Check excluded native-state paths / backup trees
		baseName := strings.ToLower(filepath.Base(path))
		if _, excluded := excludedSet[baseName]; excluded {
			addDiag(Diagnostic{
				Kind:     DiagExcludedPath,
				Severity: SeverityInfo,
				Path:     relSlash,
				Message:  fmt.Sprintf("excluded native-state directory/path %q", baseName),
			})
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		// Resolve physical identity
		physIdent, chain, objType, fileInfo, err := ResolvePhysicalIdentity(path, s.Budgets.MaxReparseHops)
		if err != nil {
			if strings.Contains(err.Error(), "cycle") {
				addDiag(Diagnostic{
					Kind:     DiagCycle,
					Severity: SeverityError,
					Path:     relSlash,
					Message:  "reparse cycle detected",
					Error:    err.Error(),
				})
			} else if strings.Contains(err.Error(), "broken") {
				addDiag(Diagnostic{
					Kind:     DiagBrokenJunction,
					Severity: SeverityError,
					Path:     relSlash,
					Message:  "broken junction or link",
					Error:    err.Error(),
				})
			} else {
				addDiag(Diagnostic{
					Kind:     DiagUnrecognizedType,
					Severity: SeverityWarning,
					Path:     relSlash,
					Message:  "unresolved physical identity",
					Error:    err.Error(),
				})
			}
			return nil
		}

		// Check if physical identity already visited (deduplicate into logical visibility edges/aliases)
		if existing, exists := obsMap[physIdent.Key]; exists {
			existing.Aliases = append(existing.Aliases, relSlash)
			return nil
		}

		// Budget checks
		if filesScanned+1 > s.Budgets.MaxFiles {
			addDiag(Diagnostic{
				Kind:     DiagBudgetExceeded,
				Severity: SeverityWarning,
				Path:     relSlash,
				Message:  fmt.Sprintf("max files budget (%d) exceeded", s.Budgets.MaxFiles),
			})
			return fmt.Errorf("max files budget (%d) exceeded", s.Budgets.MaxFiles)
		}

		filesScanned++
		var digest string
		var size int64

		if objType == ObjectTypeFile {
			size = fileInfo.Size()
			bytesScanned += size

			if bytesScanned > s.Budgets.MaxBytes {
				addDiag(Diagnostic{
					Kind:     DiagBudgetExceeded,
					Severity: SeverityWarning,
					Path:     relSlash,
					Message:  fmt.Sprintf("max bytes budget (%d) exceeded", s.Budgets.MaxBytes),
				})
				return fmt.Errorf("max bytes budget (%d) exceeded", s.Budgets.MaxBytes)
			}

			// Compute SHA256 payload digest once per physical object
			dHex, err := ComputePayloadDigest(chain[len(chain)-1])
			if err == nil {
				digest = dHex
				physIdent.SHA256 = digest
			}
		}

		obsID := fmt.Sprintf("obs-%s-%06d", scanID, filesScanned)
		obs := &ObjectObservation{
			ID:               obsID,
			PhysicalIdentity: physIdent,
			Type:             objType,
			Size:             size,
			Digest:           digest,
			PrimaryPath:      relSlash,
			Aliases:          []string{},
			ReparseChain:     chain,
		}

		obsMap[physIdent.Key] = obs
		return nil
	})

	if ctx.Err() != nil {
		return ScanResult{}, ctx.Err()
	}

	if err != nil && err != fs.SkipDir && !strings.Contains(err.Error(), "budget") {
		addDiag(Diagnostic{
			Kind:     DiagPermissionError,
			Severity: SeverityError,
			Path:     cleanRoot,
			Message:  "scanner stopped with error",
			Error:    err.Error(),
		})
	}

	var observations []ObjectObservation
	for _, obs := range obsMap {
		observations = append(observations, *obs)
	}

	sort.Slice(observations, func(i, j int) bool {
		return observations[i].PrimaryPath < observations[j].PrimaryPath
	})

	result := ScanResult{
		APIVersion:   APIVersion,
		ScanID:       scanID,
		RootPath:     cleanRoot,
		StartedAt:    startTime,
		CompletedAt:  time.Now().UTC(),
		Observations: observations,
		Diagnostics:  diagnostics,
		FilesScanned: filesScanned,
		BytesScanned: bytesScanned,
	}

	return SealScanResult(result)
}
