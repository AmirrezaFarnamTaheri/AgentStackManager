package resourcehub_test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentstack/agentstack/internal/cas"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

func setupTESTCASStore(t *testing.T) (cas.Store, string) {
	tmpDir, err := os.MkdirTemp("", "asm-cas-test-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	return cas.New(tmpDir), tmpDir
}

func TestRegisterSourceBundle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "asm-bundle-reg-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	now := time.Now().UTC()

	// Directory registration
	regDir, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindDirectory, tmpDir, "operator", now)
	if err != nil {
		t.Fatalf("register directory bundle: %v", err)
	}
	if regDir.Kind != resourcehub.BundleKindDirectory {
		t.Errorf("expected directory kind, got %s", regDir.Kind)
	}

	// Zip registration
	zipFile := filepath.Join(tmpDir, "test.zip")
	createTestZIP(t, zipFile, map[string][]byte{"file.txt": []byte("hello")})

	regZip, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindZIP, zipFile, "operator", now)
	if err != nil {
		t.Fatalf("register ZIP bundle: %v", err)
	}
	if regZip.Kind != resourcehub.BundleKindZIP {
		t.Errorf("expected ZIP kind, got %s", regZip.Kind)
	}

	// Unsupported kind registration
	_, err = resourcehub.RegisterSourceBundle("tar", tmpDir, "operator", now)
	if err == nil {
		t.Fatalf("expected error for unsupported kind 'tar', got nil")
	}
}

func TestAdmitDirectoryBundle(t *testing.T) {
	casStore, casDir := setupTESTCASStore(t)
	defer os.RemoveAll(casDir)

	tmpDir, err := os.MkdirTemp("", "asm-bundle-dir-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Populate directory
	if err := os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Title"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	subDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	now := time.Now().UTC()
	reg, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindDirectory, tmpDir, "operator", now)
	if err != nil {
		t.Fatalf("register bundle: %v", err)
	}

	receipt, err := resourcehub.AdmitSourceBundle(context.Background(), reg, casStore, resourcehub.DefaultSourceBundleBudgets(), now)
	if err != nil {
		t.Fatalf("admit directory bundle: %v", err)
	}

	if receipt.Status != resourcehub.AdmissionAdmitted {
		t.Errorf("expected status admitted, got %s", receipt.Status)
	}
	if receipt.TotalMembers != 3 { // README.md, src, src/main.go
		t.Errorf("expected 3 members, got %d", receipt.TotalMembers)
	}

	if err := resourcehub.VerifyAdmissionReceipt(receipt); err != nil {
		t.Errorf("verify admission receipt: %v", err)
	}

	members, err := resourcehub.InspectSourceBundle(receipt)
	if err != nil {
		t.Fatalf("inspect bundle: %v", err)
	}
	if len(members) != 3 {
		t.Errorf("expected 3 inspected members, got %d", len(members))
	}
}

func TestAdmitZIPBundle(t *testing.T) {
	casStore, casDir := setupTESTCASStore(t)
	defer os.RemoveAll(casDir)

	tmpDir, err := os.MkdirTemp("", "asm-bundle-zip-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	zipFile := filepath.Join(tmpDir, "valid.zip")
	createTestZIP(t, zipFile, map[string][]byte{
		"config.json":     []byte(`{"key":"value"}`),
		"scripts/run.sh":  []byte("#!/bin/bash\necho ok"),
	})

	now := time.Now().UTC()
	reg, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindZIP, zipFile, "operator", now)
	if err != nil {
		t.Fatalf("register ZIP: %v", err)
	}

	receipt, err := resourcehub.AdmitSourceBundle(context.Background(), reg, casStore, resourcehub.DefaultSourceBundleBudgets(), now)
	if err != nil {
		t.Fatalf("admit ZIP bundle: %v", err)
	}

	if receipt.Status != resourcehub.AdmissionAdmitted {
		t.Errorf("expected status admitted, got %s", receipt.Status)
	}

	if err := resourcehub.ReplayAdmissionReceipt(receipt, casStore); err != nil {
		t.Errorf("replay receipt: %v", err)
	}
}

func TestZIPBombDetection(t *testing.T) {
	casStore, casDir := setupTESTCASStore(t)
	defer os.RemoveAll(casDir)

	tmpDir, err := os.MkdirTemp("", "asm-zip-bomb-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a highly compressible payload (2MB of repeated zeroes)
	hugeData := bytes.Repeat([]byte{0}, 2<<20)
	zipFile := filepath.Join(tmpDir, "bomb.zip")
	createTestZIP(t, zipFile, map[string][]byte{
		"zeroes.bin": hugeData,
	})

	budgets := resourcehub.DefaultSourceBundleBudgets()
	budgets.MaxCompressionRatio = 5.0 // Set low threshold to trigger bomb check

	now := time.Now().UTC()
	reg, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindZIP, zipFile, "operator", now)
	if err != nil {
		t.Fatalf("register zip: %v", err)
	}

	receipt, err := resourcehub.AdmitSourceBundle(context.Background(), reg, casStore, budgets, now)
	if err == nil {
		t.Fatalf("expected ZIP bomb error, got nil")
	}
	if receipt.Status != resourcehub.AdmissionRejected {
		t.Errorf("expected rejected status in receipt, got %s", receipt.Status)
	}
	if !strings.Contains(receipt.RejectionReason, "ZIP bomb detected") {
		t.Errorf("expected ZIP bomb rejection reason, got %q", receipt.RejectionReason)
	}
}

func TestPathTraversalRejection(t *testing.T) {
	casStore, casDir := setupTESTCASStore(t)
	defer os.RemoveAll(casDir)

	tmpDir, err := os.MkdirTemp("", "asm-path-traversal-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	zipFile := filepath.Join(tmpDir, "traversal.zip")
	createTestZIP(t, zipFile, map[string][]byte{
		"../secret.txt": []byte("stolen"),
	})

	now := time.Now().UTC()
	reg, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindZIP, zipFile, "operator", now)
	if err != nil {
		t.Fatalf("register zip: %v", err)
	}

	receipt, err := resourcehub.AdmitSourceBundle(context.Background(), reg, casStore, resourcehub.DefaultSourceBundleBudgets(), now)
	if err == nil {
		t.Fatalf("expected path traversal rejection error, got nil")
	}
	if receipt.Status != resourcehub.AdmissionRejected {
		t.Errorf("expected rejected status, got %s", receipt.Status)
	}
	if !strings.Contains(receipt.RejectionReason, "path traversal") {
		t.Errorf("expected path traversal rejection reason, got %q", receipt.RejectionReason)
	}
}

func TestMemberCountBudgetExceeded(t *testing.T) {
	casStore, casDir := setupTESTCASStore(t)
	defer os.RemoveAll(casDir)

	tmpDir, err := os.MkdirTemp("", "asm-budget-exceeded-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	files := make(map[string][]byte)
	for i := 0; i < 15; i++ {
		files[filepath.Join("dir", string(rune('a'+i))+".txt")] = []byte("content")
	}

	zipFile := filepath.Join(tmpDir, "many.zip")
	createTestZIP(t, zipFile, files)

	budgets := resourcehub.DefaultSourceBundleBudgets()
	budgets.MaxMembers = 5 // Cap at 5 members

	now := time.Now().UTC()
	reg, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindZIP, zipFile, "operator", now)
	if err != nil {
		t.Fatalf("register zip: %v", err)
	}

	_, err = resourcehub.AdmitSourceBundle(context.Background(), reg, casStore, budgets, now)
	if err == nil {
		t.Fatalf("expected member budget error, got nil")
	}
}

func TestContextCancellation(t *testing.T) {
	casStore, casDir := setupTESTCASStore(t)
	defer os.RemoveAll(casDir)

	tmpDir, err := os.MkdirTemp("", "asm-cancel-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	zipFile := filepath.Join(tmpDir, "cancel.zip")
	createTestZIP(t, zipFile, map[string][]byte{"file.txt": []byte("test")})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Pre-cancel context

	now := time.Now().UTC()
	reg, err := resourcehub.RegisterSourceBundle(resourcehub.BundleKindZIP, zipFile, "operator", now)
	if err != nil {
		t.Fatalf("register zip: %v", err)
	}

	_, err = resourcehub.AdmitSourceBundle(ctx, reg, casStore, resourcehub.DefaultSourceBundleBudgets(), now)
	if err == nil {
		t.Fatalf("expected context cancellation error, got nil")
	}
}

func createTestZIP(t *testing.T, targetPath string, files map[string][]byte) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, content := range files {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip header %s: %v", name, err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatalf("write zip content %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := os.WriteFile(targetPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip file %s: %v", targetPath, err)
	}
}
