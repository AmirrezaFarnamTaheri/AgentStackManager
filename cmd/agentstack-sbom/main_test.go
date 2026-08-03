package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunWritesCatalogSBOM(t *testing.T) {
	licenses, err := filepath.Abs(filepath.Join("..", "..", "supply-chain", "component-licenses.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "catalog.cdx.json")
	var stderr bytes.Buffer
	code := run([]string{"--version", "test", "--licenses", licenses, "--out", out}, &stderr)
	if code != 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if document["bomFormat"] != "CycloneDX" {
		t.Fatalf("bomFormat = %v", document["bomFormat"])
	}
}

func TestRunReportsLicenseFailure(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"--licenses", filepath.Join(t.TempDir(), "missing.json")}, &stderr)
	if code != 1 || stderr.Len() == 0 {
		t.Fatalf("run() code = %d, stderr = %q", code, stderr.String())
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	var stderr bytes.Buffer
	if code := run([]string{"extra"}, &stderr); code != 2 {
		t.Fatalf("run() code = %d, want 2", code)
	}
}
