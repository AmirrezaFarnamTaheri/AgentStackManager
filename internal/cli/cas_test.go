package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentstack/agentstack/internal/migrations/asmv1"
	"github.com/agentstack/agentstack/internal/resourcehub"
)

func TestResourceHubCASStageVerifyAndRestoreCLI(t *testing.T) {
	command, output := testFabricCLI(t)
	source := filepath.Join(t.TempDir(), "rule.md")
	if err := os.WriteFile(source, []byte("preserve foreign state\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	runFabricCLI(t, command, output, "hub", "import", "--id", "preserve", "--kind", "rule", "--path", source)

	casRoot := filepath.Join(t.TempDir(), "cas")
	stageJSON := runFabricCLI(t, command, output, "hub", "cas-stage", "--root", casRoot)
	var receipt asmv1.Receipt
	if err := json.Unmarshal(stageJSON, &receipt); err != nil {
		t.Fatal(err)
	}
	if err := asmv1.VerifyReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(receiptPath, stageJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	verified := runFabricCLI(t, command, output, "hub", "cas-verify", "--root", casRoot, "--receipt", receiptPath)
	var verifyResult struct {
		Verified          bool   `json:"verified"`
		ReceiptDigest     string `json:"receiptDigest"`
		SourceGraphDigest string `json:"sourceGraphDigest"`
		StagedGraphDigest string `json:"stagedGraphDigest"`
	}
	if err := json.Unmarshal(verified, &verifyResult); err != nil {
		t.Fatal(err)
	}
	if !verifyResult.Verified || verifyResult.ReceiptDigest != receipt.Digest || verifyResult.SourceGraphDigest != receipt.SourceGraphDigest || verifyResult.StagedGraphDigest != receipt.StagedGraph.Digest {
		t.Fatalf("verify output=%s", verified)
	}

	destination := filepath.Join(t.TempDir(), "restored")
	runFabricCLI(t, command, output, "hub", "cas-restore", "--root", casRoot, "--receipt", receiptPath, "--resource", "preserve", "--destination", destination, "--yes")
	digest, err := resourcehub.DigestPath(destination)
	if err != nil {
		t.Fatal(err)
	}
	if digest != receipt.Resources[0].LegacyDigest {
		t.Fatalf("restored digest=%s receipt=%#v", digest, receipt.Resources[0])
	}
}

func TestReadMigrationReceiptRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	target := filepath.Join(t.TempDir(), "receipt.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "receipt-link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	var receipt asmv1.Receipt
	if err := readMigrationReceipt(link, &receipt); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("expected symlink receipt rejection, got %v", err)
	}
}

func TestResourceHubCASRestoreRequiresConfirmation(t *testing.T) {
	command, output := testFabricCLI(t)
	output.Reset()
	code := command.Run(context.Background(), []string{"hub", "cas-restore", "--receipt", "receipt.json", "--resource", "x", "--destination", "out"})
	if code != 2 {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
}
