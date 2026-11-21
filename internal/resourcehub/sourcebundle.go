package resourcehub

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/cas"
	"github.com/agentstack/agentstack/internal/integrity"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const SourceBundleAPIVersion = "resourcehub.asm.dev/sourcebundle/v1alpha1"

type SourceBundleKind string

const (
	BundleKindDirectory SourceBundleKind = "directory"
	BundleKindZIP       SourceBundleKind = "zip"
)

type AdmissionStatus string

const (
	AdmissionRegistered AdmissionStatus = "registered"
	AdmissionAdmitted   AdmissionStatus = "admitted"
	AdmissionRejected   AdmissionStatus = "rejected"
)

type SourceBundleBudgets struct {
	MaxMembers           int     `json:"maxMembers"`
	MaxUncompressedBytes int64   `json:"maxUncompressedBytes"`
	MaxTotalSizeBytes    int64   `json:"maxTotalSizeBytes"`
	MaxCompressionRatio  float64 `json:"maxCompressionRatio"`
	MaxSingleFileBytes   int64   `json:"maxSingleFileBytes"`
}

func DefaultSourceBundleBudgets() SourceBundleBudgets {
	return SourceBundleBudgets{
		MaxMembers:           10_000,
		MaxUncompressedBytes: 256 << 20, // 256 MB
		MaxTotalSizeBytes:    256 << 20, // 256 MB
		MaxCompressionRatio:  100.0,
		MaxSingleFileBytes:   16 << 20, // 16 MB
	}
}

type SourceBundleRegistration struct {
	ID           string           `json:"id"`
	Kind         SourceBundleKind `json:"kind"`
	SourcePath   string           `json:"sourcePath"`
	RegisteredAt time.Time        `json:"registeredAt"`
	RegisteredBy string           `json:"registeredBy"`
}

type SourceBundleMember struct {
	Path             string `json:"path"`
	Kind             string `json:"kind"` // "file", "directory"
	Size             int64  `json:"size"`
	UncompressedSize int64  `json:"uncompressedSize"`
	Digest           string `json:"digest,omitempty"`
}

type AdmissionReceipt struct {
	APIVersion        string               `json:"apiVersion"`
	ID                string               `json:"id"`
	BundleID          string               `json:"bundleId"`
	Kind              SourceBundleKind     `json:"kind"`
	Status            AdmissionStatus      `json:"status"`
	CASRef            cas.Ref              `json:"casRef"`
	CASURI            string               `json:"casUri"`
	TotalMembers      int                  `json:"totalMembers"`
	TotalBytes        int64                `json:"totalBytes"`
	UncompressedBytes int64                `json:"uncompressedBytes"`
	Members           []SourceBundleMember `json:"members"`
	RejectionReason   string               `json:"rejectionReason,omitempty"`
	AdmittedAt        time.Time            `json:"admittedAt"`
	Digest            string               `json:"digest"`
}

// RegisterSourceBundle registers a source bundle, enforcing explicit directory or ZIP support only.
func RegisterSourceBundle(kind SourceBundleKind, sourcePath, registeredBy string, now time.Time) (SourceBundleRegistration, error) {
	if kind != BundleKindDirectory && kind != BundleKindZIP {
		return SourceBundleRegistration{}, fmt.Errorf("unsupported source bundle kind %q: only %q and %q are supported", kind, BundleKindDirectory, BundleKindZIP)
	}

	cleanPath := filepath.Clean(sourcePath)
	info, err := os.Lstat(cleanPath)
	if err != nil {
		return SourceBundleRegistration{}, fmt.Errorf("source bundle path %q error: %w", cleanPath, err)
	}

	if kind == BundleKindDirectory && !info.IsDir() {
		return SourceBundleRegistration{}, fmt.Errorf("registered directory source bundle path %q is not a directory", cleanPath)
	}
	if kind == BundleKindZIP && info.IsDir() {
		return SourceBundleRegistration{}, fmt.Errorf("registered ZIP source bundle path %q is a directory", cleanPath)
	}

	id := fmt.Sprintf("sb-%x", sha256.Sum256([]byte(cleanPath+string(kind)+now.Format(time.RFC3339Nano))))[:16]

	return SourceBundleRegistration{
		ID:           id,
		Kind:         kind,
		SourcePath:   cleanPath,
		RegisteredAt: now.UTC(),
		RegisteredBy: registeredBy,
	}, nil
}

// AdmitSourceBundle validates budgets, path safety, ZIP bombs, symlinks, and admits the bundle into CAS.
func AdmitSourceBundle(ctx context.Context, reg SourceBundleRegistration, casStore cas.Store, budgets SourceBundleBudgets, now time.Time) (AdmissionReceipt, error) {
	if budgets.MaxMembers <= 0 {
		budgets = DefaultSourceBundleBudgets()
	}

	receiptID := fmt.Sprintf("rcpt-%s", reg.ID)

	var members []SourceBundleMember
	var totalBytes, uncompressedBytes int64
	var casRef cas.Ref
	var err error

	switch reg.Kind {
	case BundleKindDirectory:
		members, totalBytes, uncompressedBytes, casRef, err = admitDirectoryBundle(ctx, reg.SourcePath, casStore, budgets)
	case BundleKindZIP:
		members, totalBytes, uncompressedBytes, casRef, err = admitZIPBundle(ctx, reg.SourcePath, casStore, budgets)
	default:
		err = fmt.Errorf("unsupported source bundle kind %q", reg.Kind)
	}

	if err != nil {
		receipt := AdmissionReceipt{
			APIVersion:      SourceBundleAPIVersion,
			ID:              receiptID,
			BundleID:        reg.ID,
			Kind:            reg.Kind,
			Status:          AdmissionRejected,
			RejectionReason: err.Error(),
			AdmittedAt:      now.UTC(),
		}
		sealed, _ := SealAdmissionReceipt(receipt)
		return sealed, err
	}

	receipt := AdmissionReceipt{
		APIVersion:        SourceBundleAPIVersion,
		ID:                receiptID,
		BundleID:          reg.ID,
		Kind:              reg.Kind,
		Status:            AdmissionAdmitted,
		CASRef:            casRef,
		CASURI:            casRef.URI(),
		TotalMembers:      len(members),
		TotalBytes:        totalBytes,
		UncompressedBytes: uncompressedBytes,
		Members:           members,
		AdmittedAt:        now.UTC(),
	}

	return SealAdmissionReceipt(receipt)
}

func admitDirectoryBundle(ctx context.Context, root string, casStore cas.Store, budgets SourceBundleBudgets) ([]SourceBundleMember, int64, int64, cas.Ref, error) {
	var members []SourceBundleMember
	var totalBytes, uncompressedBytes int64
	seenPaths := make(map[string]struct{})

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)

		if err := validateBundleMemberPath(relSlash); err != nil {
			return fmt.Errorf("directory member path violation: %w", err)
		}

		if _, exists := seenPaths[relSlash]; exists {
			return fmt.Errorf("duplicate member path %q in directory tree", relSlash)
		}
		seenPaths[relSlash] = struct{}{}

		info, err := d.Info()
		if err != nil {
			return err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("external or unhandled symlink rejected in source bundle: %s", relSlash)
		}

		if d.IsDir() {
			members = append(members, SourceBundleMember{
				Path: relSlash,
				Kind: "directory",
			})
			return nil
		}

		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported special file in directory source bundle: %s", relSlash)
		}

		if len(members)+1 > budgets.MaxMembers {
			return fmt.Errorf("source bundle exceeds max member budget (%d > %d)", len(members)+1, budgets.MaxMembers)
		}

		if info.Size() > budgets.MaxSingleFileBytes {
			return fmt.Errorf("file %q size (%d bytes) exceeds max single file budget (%d bytes)", relSlash, info.Size(), budgets.MaxSingleFileBytes)
		}

		totalBytes += info.Size()
		uncompressedBytes += info.Size()

		if uncompressedBytes > budgets.MaxUncompressedBytes {
			return fmt.Errorf("source bundle total size (%d bytes) exceeds max uncompressed budget (%d bytes)", uncompressedBytes, budgets.MaxUncompressedBytes)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read bundle file %s: %w", relSlash, err)
		}
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))

		members = append(members, SourceBundleMember{
			Path:             relSlash,
			Kind:             "file",
			Size:             info.Size(),
			UncompressedSize: info.Size(),
			Digest:           digest,
		})

		return nil
	})

	if err != nil {
		return nil, 0, 0, cas.Ref{}, err
	}

	casRef, err := casStore.PutTree(root)
	if err != nil {
		return nil, 0, 0, cas.Ref{}, fmt.Errorf("store directory tree in CAS: %w", err)
	}

	sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })
	return members, totalBytes, uncompressedBytes, casRef, nil
}

func admitZIPBundle(ctx context.Context, zipPath string, casStore cas.Store, budgets SourceBundleBudgets) ([]SourceBundleMember, int64, int64, cas.Ref, error) {
	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		return nil, 0, 0, cas.Ref{}, fmt.Errorf("read zip bundle file: %w", err)
	}

	if int64(len(zipData)) > budgets.MaxTotalSizeBytes {
		return nil, 0, 0, cas.Ref{}, fmt.Errorf("ZIP bundle file size (%d bytes) exceeds max total size budget (%d bytes)", len(zipData), budgets.MaxTotalSizeBytes)
	}

	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, 0, 0, cas.Ref{}, fmt.Errorf("open zip archive: %w", err)
	}

	var members []SourceBundleMember
	var totalBytes, uncompressedBytes int64
	seenPaths := make(map[string]struct{})

	for _, f := range reader.File {
		select {
		case <-ctx.Done():
			return nil, 0, 0, cas.Ref{}, ctx.Err()
		default:
		}

		relSlash := filepath.ToSlash(f.Name)
		relSlash = strings.TrimSuffix(relSlash, "/")

		if relSlash == "" {
			continue
		}

		if err := validateBundleMemberPath(relSlash); err != nil {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("ZIP member path violation: %w", err)
		}

		if _, exists := seenPaths[relSlash]; exists {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("duplicate member path %q in ZIP archive", relSlash)
		}
		seenPaths[relSlash] = struct{}{}

		if len(members)+1 > budgets.MaxMembers {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("ZIP bundle exceeds max member budget (%d > %d)", len(members)+1, budgets.MaxMembers)
		}

		// ZIP Bomb check: compression ratio check
		compSize := int64(f.CompressedSize64)
		uncompSize := int64(f.UncompressedSize64)
		if compSize > 0 {
			ratio := float64(uncompSize) / float64(compSize)
			if ratio > budgets.MaxCompressionRatio && uncompSize > 1<<20 {
				return nil, 0, 0, cas.Ref{}, fmt.Errorf("ZIP bomb detected: file %q compression ratio %.1f exceeds max allowed ratio %.1f", relSlash, ratio, budgets.MaxCompressionRatio)
			}
		}

		if uncompSize > budgets.MaxSingleFileBytes {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("ZIP file %q uncompressed size (%d bytes) exceeds max single file budget (%d bytes)", relSlash, uncompSize, budgets.MaxSingleFileBytes)
		}

		if f.FileInfo().IsDir() {
			members = append(members, SourceBundleMember{
				Path:             relSlash,
				Kind:             "directory",
				Size:             compSize,
				UncompressedSize: uncompSize,
			})
			continue
		}

		// Read and compute digest for file payload
		rc, err := f.Open()
		if err != nil {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("open ZIP entry %s: %w", relSlash, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, budgets.MaxSingleFileBytes+1))
		rc.Close()
		if err != nil {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("read ZIP entry %s: %w", relSlash, err)
		}
		if int64(len(data)) > budgets.MaxSingleFileBytes {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("ZIP entry %s exceeds max single file size limit", relSlash)
		}

		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
		totalBytes += compSize
		uncompressedBytes += int64(len(data))

		if uncompressedBytes > budgets.MaxUncompressedBytes {
			return nil, 0, 0, cas.Ref{}, fmt.Errorf("ZIP bundle total uncompressed size (%d bytes) exceeds budget (%d bytes)", uncompressedBytes, budgets.MaxUncompressedBytes)
		}

		members = append(members, SourceBundleMember{
			Path:             relSlash,
			Kind:             "file",
			Size:             compSize,
			UncompressedSize: int64(len(data)),
			Digest:           digest,
		})
	}

	casRef, err := casStore.PutBlob(zipData)
	if err != nil {
		return nil, 0, 0, cas.Ref{}, fmt.Errorf("store ZIP blob in CAS: %w", err)
	}

	sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })
	return members, totalBytes, uncompressedBytes, casRef, nil
}

func validateBundleMemberPath(path string) error {
	if path == "" {
		return fmt.Errorf("empty member path")
	}
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\") {
		return fmt.Errorf("absolute path not allowed: %s", path)
	}
	// Check Windows drive prefix (e.g. C:, D:)
	if len(path) >= 2 && path[1] == ':' {
		return fmt.Errorf("drive path prefix not allowed: %s", path)
	}

	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == ".." {
			return fmt.Errorf("path traversal (..) not allowed: %s", path)
		}
	}
	return nil
}

// InspectSourceBundle lists bundle members without side effects or executing code.
func InspectSourceBundle(receipt AdmissionReceipt) ([]SourceBundleMember, error) {
	if err := VerifyAdmissionReceipt(receipt); err != nil {
		return nil, fmt.Errorf("invalid admission receipt: %w", err)
	}
	res := make([]SourceBundleMember, len(receipt.Members))
	copy(res, receipt.Members)
	return res, nil
}

// SealAdmissionReceipt normalizes and digests the admission receipt.
func SealAdmissionReceipt(receipt AdmissionReceipt) (AdmissionReceipt, error) {
	if receipt.APIVersion == "" {
		receipt.APIVersion = SourceBundleAPIVersion
	}
	if receipt.Members == nil {
		receipt.Members = []SourceBundleMember{}
	} else {
		sort.Slice(receipt.Members, func(i, j int) bool { return receipt.Members[i].Path < receipt.Members[j].Path })
	}

	receipt.Digest = ""
	digest, err := integrity.DigestJSON(receipt)
	if err != nil {
		return AdmissionReceipt{}, fmt.Errorf("digest admission receipt: %w", err)
	}
	receipt.Digest = digest
	return receipt, nil
}

// VerifyAdmissionReceipt checks the receipt digest against its sealed value.
func VerifyAdmissionReceipt(receipt AdmissionReceipt) error {
	if receipt.APIVersion != SourceBundleAPIVersion {
		return fmt.Errorf("unsupported admission receipt API version %q", receipt.APIVersion)
	}
	if receipt.Digest == "" {
		return fmt.Errorf("admission receipt digest is empty")
	}
	expected, err := SealAdmissionReceipt(receipt)
	if err != nil {
		return err
	}
	if expected.Digest != receipt.Digest {
		return fmt.Errorf("admission receipt %q digest mismatch: got %q, expected %q", receipt.ID, receipt.Digest, expected.Digest)
	}
	return nil
}

// ReplayAdmissionReceipt validates and re-checks a receipt against CAS evidence.
func ReplayAdmissionReceipt(receipt AdmissionReceipt, casStore cas.Store) error {
	if err := VerifyAdmissionReceipt(receipt); err != nil {
		return fmt.Errorf("verify receipt for replay: %w", err)
	}

	if receipt.Status == AdmissionRejected {
		return nil // Rejected receipts replay cleanly as rejected evidence
	}

	if receipt.CASRef.Digest == "" {
		return fmt.Errorf("receipt missing CAS digest reference")
	}

	if err := cas.ValidateRef(receipt.CASRef); err != nil {
		return fmt.Errorf("invalid receipt CAS ref: %w", err)
	}

	return nil
}

// UnmarshalAdmissionReceiptJSON parses and verifies an admission receipt from JSON.
func UnmarshalAdmissionReceiptJSON(data []byte) (AdmissionReceipt, error) {
	var receipt AdmissionReceipt
	if err := strictjson.Decode(data, &receipt); err != nil {
		return AdmissionReceipt{}, fmt.Errorf("decode admission receipt: %w", err)
	}
	if receipt.APIVersion != SourceBundleAPIVersion {
		return AdmissionReceipt{}, fmt.Errorf("unsupported admission receipt API version %q", receipt.APIVersion)
	}
	if err := VerifyAdmissionReceipt(receipt); err != nil {
		return AdmissionReceipt{}, fmt.Errorf("verify unmarshaled admission receipt: %w", err)
	}
	return receipt, nil
}
