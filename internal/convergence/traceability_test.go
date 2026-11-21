package convergence_test

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvergenceEvidenceLinksResolveToTestsAndRecords(t *testing.T) {
	root := repositoryRoot(t)
	ledgers := [][]map[string]string{
		readCSV(t, filepath.Join(root, "docs", "convergence", "ADOPTION.csv")),
		readCSV(t, filepath.Join(root, "docs", "convergence", "FABRIC_ADOPTION.csv")),
	}
	trace := readCSV(t, filepath.Join(root, "docs", "convergence", "TEST_TRACEABILITY.csv"))

	records := map[string]struct{}{}
	ledgerTests := map[string]struct{}{}
	for _, ledger := range ledgers {
		for _, row := range ledger {
			id := strings.TrimSpace(row["record_id"])
			if id == "" {
				t.Fatal("convergence ledger contains an empty record_id")
			}
			if _, exists := records[id]; exists {
				t.Fatalf("duplicate convergence record %q", id)
			}
			records[id] = struct{}{}
			declared := strings.TrimSpace(row["test_node"])
			if declared == "" {
				declared = strings.TrimSpace(row["test_nodes"])
			}
			for _, testNode := range strings.Split(declared, ";") {
				testNode = strings.TrimSpace(testNode)
				if testNode != "" && testNode != "n/a" && strings.Contains(testNode, "#") {
					ledgerTests[testNode] = struct{}{}
				}
			}
		}
	}

	traced := map[string]struct{}{}
	for _, row := range trace {
		testNode := strings.TrimSpace(row["test_node"])
		if testNode == "" {
			t.Fatal("traceability row has an empty test_node")
		}
		if _, exists := traced[testNode]; exists {
			t.Fatalf("duplicate traceability test %q", testNode)
		}
		traced[testNode] = struct{}{}
		for _, recordID := range strings.Split(row["record_ids"], ";") {
			recordID = strings.TrimSpace(recordID)
			if _, exists := records[recordID]; !exists {
				t.Fatalf("traceability test %q references unknown record %q", testNode, recordID)
			}
		}
		assertTestNodeExists(t, root, testNode)
	}
	for testNode := range ledgerTests {
		if _, exists := traced[testNode]; !exists {
			t.Fatalf("ledger test %q is absent from TEST_TRACEABILITY.csv", testNode)
		}
	}
}

func TestEveryFabricSurfaceLinksToKnownEvidence(t *testing.T) {
	root := repositoryRoot(t)
	ledger := readCSV(t, filepath.Join(root, "docs", "convergence", "FABRIC_ADOPTION.csv"))
	surfaces := readCSV(t, filepath.Join(root, "docs", "convergence", "FABRIC_SURFACES.csv"))
	records := map[string]struct{}{}
	for _, row := range ledger {
		records[strings.TrimSpace(row["record_id"])] = struct{}{}
	}
	if len(surfaces) == 0 {
		t.Fatal("Fabric surface matrix is empty")
	}
	seen := map[string]struct{}{}
	for index, row := range surfaces {
		path := strings.TrimSpace(row["path"])
		if path == "" {
			t.Fatalf("Fabric surface row %d has an empty path", index+2)
		}
		if _, duplicate := seen[path]; duplicate {
			t.Fatalf("duplicate Fabric surface path %q", path)
		}
		seen[path] = struct{}{}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatalf("Fabric surface %q is unavailable: %v", path, err)
		}
		refs := strings.TrimSpace(row["record_ids"])
		if refs == "" {
			t.Fatalf("Fabric surface row %d (%s) has no evidence", index+2, path)
		}
		for _, recordID := range strings.Split(refs, ";") {
			recordID = strings.TrimSpace(recordID)
			if _, exists := records[recordID]; !exists {
				t.Fatalf("Fabric surface row %d references unknown record %q", index+2, recordID)
			}
		}
	}
}

func TestEveryDonorSurfaceLinksToKnownEvidence(t *testing.T) {
	root := repositoryRoot(t)
	ledger := readCSV(t, filepath.Join(root, "docs", "convergence", "ADOPTION.csv"))
	surfaces := readCSV(t, filepath.Join(root, "docs", "convergence", "SURFACES.csv"))
	records := map[string]struct{}{}
	for _, row := range ledger {
		records[strings.TrimSpace(row["record_id"])] = struct{}{}
	}
	if len(surfaces) != 2673 {
		t.Fatalf("unexpected donor surface count: got %d want 2673", len(surfaces))
	}
	for index, row := range surfaces {
		refs := strings.TrimSpace(row["semantic_record_ids"])
		if refs == "" {
			t.Fatalf("surface row %d (%s:%s) has no semantic evidence", index+2, row["donor"], row["path"])
		}
		for _, recordID := range strings.Split(refs, ";") {
			if _, exists := records[strings.TrimSpace(recordID)]; !exists {
				t.Fatalf("surface row %d references unknown record %q", index+2, recordID)
			}
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readCSV(t *testing.T, path string) []map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	headers, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]map[string]string, 0, len(records))
	for _, record := range records {
		if len(record) != len(headers) {
			t.Fatalf("CSV row in %s has %d values for %d headers", path, len(record), len(headers))
		}
		row := make(map[string]string, len(headers))
		for index, header := range headers {
			row[header] = record[index]
		}
		rows = append(rows, row)
	}
	return rows
}

func assertTestNodeExists(t *testing.T, root, testNode string) {
	t.Helper()
	parts := strings.SplitN(testNode, "#", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Fatalf("invalid test node %q", testNode)
	}
	path := filepath.Join(root, filepath.FromSlash(parts[0]))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("traceability source %q is unavailable: %v", testNode, err)
	}
	declaration := fmt.Sprintf("func %s(", parts[1])
	if !strings.Contains(string(data), declaration) {
		t.Fatalf("traceability source %q does not declare %s", testNode, declaration)
	}
}
