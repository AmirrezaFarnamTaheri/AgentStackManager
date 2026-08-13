package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/agentstack/agentstack/internal/donormanifest"
)

type donorLedger struct {
	SchemaVersion int             `json:"schemaVersion"`
	SourceRoot    string          `json:"sourceRoot"`
	Snapshots     []donorSnapshot `json:"snapshots"`
}

type donorModule struct {
	Path        string `json:"path"`
	Disposition string `json:"disposition"`
	Destination string `json:"destination"`
	Reason      string `json:"reason"`
}

type donorSnapshot struct {
	ID      string        `json:"id"`
	Path    string        `json:"path"`
	Files   int           `json:"files"`
	Modules []donorModule `json:"modules"`
}

type ManifestIndex struct {
	SchemaVersion  int                  `json:"schemaVersion"`
	TotalSnapshots int                  `json:"totalSnapshots"`
	TotalFiles     int                  `json:"totalFiles"`
	TotalSizeBytes int64                `json:"totalSizeBytes"`
	Snapshots      []SnapshotIndexEntry `json:"snapshots"`
}

type SnapshotIndexEntry struct {
	ID             string `json:"id"`
	SourcePath     string `json:"sourcePath"`
	ManifestDigest string `json:"manifestDigest"`
	EmptySnapshot  bool   `json:"emptySnapshot"`
	FileCount      int    `json:"fileCount"`
	TotalSizeBytes int64  `json:"totalSizeBytes"`
	ManifestFile   string `json:"manifestFile"`
	ReceiptFile    string `json:"receiptFile"`
}

func main() {
	var root string
	var ledgerPath string
	var outDir string

	flag.StringVar(&root, "root", ".", "Repository root path")
	flag.StringVar(&ledgerPath, "ledger", "docs/convergence/DONOR_ADOPTION_LEDGER.json", "Path to DONOR_ADOPTION_LEDGER.json")
	flag.StringVar(&outDir, "out", "docs/convergence/manifests", "Output directory for manifest artifacts")
	flag.Parse()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		log.Fatalf("Failed to resolve repository root: %v", err)
	}

	absLedger := filepath.Join(absRoot, ledgerPath)
	ledgerData, err := os.ReadFile(absLedger)
	if err != nil {
		log.Fatalf("Failed to read donor ledger %s: %v", absLedger, err)
	}

	var ledger donorLedger
	if err := json.Unmarshal(ledgerData, &ledger); err != nil {
		log.Fatalf("Failed to parse donor ledger %s: %v", absLedger, err)
	}

	absOutDir := filepath.Join(absRoot, outDir)
	if err := os.MkdirAll(absOutDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory %s: %v", absOutDir, err)
	}

	var indexEntries []SnapshotIndexEntry
	var grandTotalFiles int
	var grandTotalBytes int64

	for _, snap := range ledger.Snapshots {
		snapSource := filepath.Join(absRoot, ledger.SourceRoot, snap.Path)
		fmt.Printf("Generating manifest for snapshot %q at %s...\n", snap.ID, snapSource)

		var rules []donormanifest.DispositionRule
		for _, m := range snap.Modules {
			rules = append(rules, donormanifest.DispositionRule{
				Pattern:     m.Path,
				Disposition: m.Disposition,
				Destination: m.Destination,
				Reason:      m.Reason,
			})
		}

		manifest, receipt, err := donormanifest.Generate(snap.ID, snapSource, donormanifest.WithDispositionRules(rules))
		if err != nil {
			log.Fatalf("Failed to generate manifest for %s: %v", snap.ID, err)
		}

		manifestJSON, err := donormanifest.MarshalManifestJSON(manifest)
		if err != nil {
			log.Fatalf("Failed to marshal manifest for %s: %v", snap.ID, err)
		}

		receiptJSON, err := donormanifest.MarshalReceiptJSON(receipt)
		if err != nil {
			log.Fatalf("Failed to marshal receipt for %s: %v", snap.ID, err)
		}

		manifestFile := snap.ID + ".manifest.json"
		receiptFile := snap.ID + ".receipt.json"

		if err := os.WriteFile(filepath.Join(absOutDir, manifestFile), manifestJSON, 0644); err != nil {
			log.Fatalf("Failed to write %s: %v", manifestFile, err)
		}

		if err := os.WriteFile(filepath.Join(absOutDir, receiptFile), receiptJSON, 0644); err != nil {
			log.Fatalf("Failed to write %s: %v", receiptFile, err)
		}

		fmt.Printf("  - %s: %d files, %d bytes, digest: %s\n", snap.ID, manifest.FileCount, manifest.TotalSizeBytes, receipt.ManifestDigest[:16])

		grandTotalFiles += manifest.FileCount
		grandTotalBytes += manifest.TotalSizeBytes

		indexEntries = append(indexEntries, SnapshotIndexEntry{
			ID:             snap.ID,
			SourcePath:     filepath.ToSlash(filepath.Join(ledger.SourceRoot, snap.Path)),
			ManifestDigest: receipt.ManifestDigest,
			EmptySnapshot:  manifest.EmptySnapshot,
			FileCount:      manifest.FileCount,
			TotalSizeBytes: manifest.TotalSizeBytes,
			ManifestFile:   manifestFile,
			ReceiptFile:    receiptFile,
		})
	}

	sort.Slice(indexEntries, func(i, j int) bool {
		return indexEntries[i].ID < indexEntries[j].ID
	})

	index := ManifestIndex{
		SchemaVersion:  donormanifest.SchemaVersion,
		TotalSnapshots: len(indexEntries),
		TotalFiles:     grandTotalFiles,
		TotalSizeBytes: grandTotalBytes,
		Snapshots:      indexEntries,
	}

	indexJSON, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal manifest index: %v", err)
	}
	indexJSON = append(indexJSON, '\n')

	if err := os.WriteFile(filepath.Join(absOutDir, "INDEX.json"), indexJSON, 0644); err != nil {
		log.Fatalf("Failed to write INDEX.json: %v", err)
	}

	fmt.Printf("\nCompleted donor manifests generation!\nTotal snapshots: %d, Total files: %d, Total bytes: %d\n", len(indexEntries), grandTotalFiles, grandTotalBytes)
}
