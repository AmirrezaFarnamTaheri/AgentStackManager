package cas

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStorePutTreeDeduplicatesVerifiesAndMaterializes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "nested", "empty"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("canonical\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "data.json"), []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	store := New(filepath.Join(root, "store"))
	first, err := store.PutTree(source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.PutTree(source)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("dedup refs differ: %#v %#v", first, second)
	}
	if err := store.Verify(first); err != nil {
		t.Fatal(err)
	}

	marks, err := store.Reachable(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(marks) != 3 || marks[0].Kind != KindBlob || marks[2].Kind != KindTree {
		t.Fatalf("unexpected closure: %#v", marks)
	}

	destination := filepath.Join(root, "restored")
	if err := store.Materialize(first, destination); err != nil {
		t.Fatal(err)
	}
	for rel, expected := range map[string]string{
		"README.md":        "canonical\n",
		"nested/data.json": `{"ok":true}`,
	} {
		data, err := os.ReadFile(filepath.Join(destination, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != expected {
			t.Fatalf("%s=%q want %q", rel, data, expected)
		}
	}
	if info, err := os.Stat(filepath.Join(destination, "nested", "empty")); err != nil || !info.IsDir() {
		t.Fatalf("empty directory not restored: info=%v err=%v", info, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(destination, "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("mode=%o want 640", got)
		}
	}
}

func TestStoreTreeDigestIsIndependentOfCreationOrder(t *testing.T) {
	root := t.TempDir()
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	for _, dir := range []string{left, right} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(left, "b"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(left, "a"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "a"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(right, "b"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(filepath.Join(root, "store"))
	leftRef, err := store.PutTree(left)
	if err != nil {
		t.Fatal(err)
	}
	rightRef, err := store.PutTree(right)
	if err != nil {
		t.Fatal(err)
	}
	if leftRef != rightRef {
		t.Fatalf("creation order changed tree identity: %#v %#v", leftRef, rightRef)
	}
}

func TestStoreRejectsSymlinkInputAndExistingDestination(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(filepath.Join(root, "store"))
	ref, err := store.PutTree(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "existing")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.Materialize(ref, destination); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing destination rejection, got %v", err)
	}

	if runtime.GOOS == "windows" {
		return
	}
	outside := filepath.Join(root, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(source, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutTree(source); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestStoreDetectsCorruptBlob(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(filepath.Join(root, "store"))
	ref, err := store.PutTree(source)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := store.loadTree(ref)
	if err != nil {
		t.Fatal(err)
	}
	blob := manifest.Entries[0].Blob
	if err := os.WriteFile(store.objectPath(blob), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(ref); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected corruption detection, got %v", err)
	}
}

func TestStoreRejectsTraversalManifest(t *testing.T) {
	root := t.TempDir()
	store := New(filepath.Join(root, "store"))
	blob, err := store.PutBlob([]byte("escape"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := TreeManifest{
		APIVersion: APIVersion,
		Entries:    []TreeEntry{{Path: "../escape", Kind: EntryFile, Mode: 0o600, Size: 6, Blob: blob}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	ref := Ref{Kind: KindTree, Digest: digestBytes(data), Size: int64(len(data))}
	if err := store.ensure(); err != nil {
		t.Fatal(err)
	}
	path := store.objectPath(ref)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(ref); err == nil || !strings.Contains(err.Error(), "unsafe tree path") {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func TestStoreRejectsEmptyRoot(t *testing.T) {
	if _, err := New("").PutBlob([]byte("data")); err == nil || !strings.Contains(err.Error(), "root is empty") {
		t.Fatalf("expected empty root rejection, got %v", err)
	}
}

func TestStoreRejectsSymlinkObjectPrefix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	root := filepath.Join(t.TempDir(), "store")
	store := New(root)
	data := []byte("prefix escape")
	ref := Ref{Kind: KindBlob, Digest: digestBytes(data), Size: int64(len(data))}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "objects", "sha256"), 0o700); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(root, "objects", "sha256", strings.TrimPrefix(ref.Digest, "sha256:")[:2])
	if err := os.Symlink(outside, prefix); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutBlob(data); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink prefix rejection, got %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("CAS escaped through symlink: %#v", entries)
	}
}

func TestStoreVerifyRejectsEmptyRoot(t *testing.T) {
	ref := Ref{Kind: KindBlob, Digest: digestBytes([]byte("data")), Size: 4}
	if err := New("").Verify(ref); err == nil || !strings.Contains(err.Error(), "root is empty") {
		t.Fatalf("expected empty root rejection, got %v", err)
	}
}

func TestStoreMaterializeRejectsSymlinkParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation commonly requires elevated Windows privileges")
	}
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(filepath.Join(root, "store"))
	ref, err := store.PutTree(source)
	if err != nil {
		t.Fatal(err)
	}
	realParent := filepath.Join(root, "real-parent")
	if err := os.Mkdir(realParent, 0o700); err != nil {
		t.Fatal(err)
	}
	linkParent := filepath.Join(root, "link-parent")
	if err := os.Symlink(realParent, linkParent); err != nil {
		t.Fatal(err)
	}
	if err := store.Materialize(ref, filepath.Join(linkParent, "restored")); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink parent rejection, got %v", err)
	}
}

func TestStoreVerifyDoesNotCreateMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	ref := Ref{Kind: KindBlob, Digest: digestBytes([]byte("data")), Size: 4}
	if err := New(root).Verify(ref); err == nil {
		t.Fatal("expected missing store verification failure")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("verify created CAS root: %v", err)
	}
}

func TestStorePopulateTreeNeverOverwritesExistingEntry(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "owned.txt"), []byte("canonical"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(filepath.Join(root, "store"))
	ref, err := store.PutTree(source)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := store.loadTree(ref)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "destination")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(destination, "owned.txt")
	if err := os.WriteFile(existing, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.populateTree(manifest, destination); err == nil || !strings.Contains(strings.ToLower(err.Error()), "exist") {
		t.Fatalf("expected exclusive-entry failure, got %v", err)
	}
	data, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "foreign" {
		t.Fatalf("existing content was overwritten: %q", data)
	}
}

func TestStoreMaterializeBlobNeverOverwritesDestination(t *testing.T) {
	root := t.TempDir()
	store := New(filepath.Join(root, "store"))
	ref, err := store.PutBlob([]byte("canonical"))
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(destination, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Materialize(ref, destination); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected existing destination rejection, got %v", err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "foreign" {
		t.Fatalf("existing blob destination was overwritten: %q", data)
	}
}

func TestRefURIIncludesObjectKind(t *testing.T) {
	ref := Ref{Kind: KindTree, Digest: "sha256:" + strings.Repeat("a", 64), Size: 0}
	if got, want := ref.URI(), "cas://tree/sha256/"+strings.Repeat("a", 64); got != want {
		t.Fatalf("URI=%q want %q", got, want)
	}
}

func TestStoreFailedTreeRestoreRetainsMarkerAndForeignCollision(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 256; index++ {
		name := filepath.Join(source, fmt.Sprintf("a-%03d.txt", index))
		if err := os.WriteFile(name, []byte("canonical"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "zz-collision.txt"), []byte("canonical"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(filepath.Join(root, "store"))
	ref, err := store.PutTree(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "restored")
	collisionResult := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			markers, _ := filepath.Glob(filepath.Join(destination, ".agentstack-restore-incomplete-*"))
			if len(markers) > 0 {
				path := filepath.Join(destination, "zz-collision.txt")
				file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
				if err == nil {
					_, err = file.Write([]byte("foreign"))
					if closeErr := file.Close(); err == nil {
						err = closeErr
					}
				}
				collisionResult <- err
				return
			}
			time.Sleep(time.Millisecond)
		}
		collisionResult <- fmt.Errorf("restore marker was not observed")
	}()
	materializeErr := store.Materialize(ref, destination)
	if err := <-collisionResult; err != nil {
		t.Fatal(err)
	}
	if materializeErr == nil || !strings.Contains(materializeErr.Error(), "incomplete destination retained") {
		t.Fatalf("expected retained incomplete restore, got %v", materializeErr)
	}
	data, err := os.ReadFile(filepath.Join(destination, "zz-collision.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "foreign" {
		t.Fatalf("foreign collision was changed: %q", data)
	}
	markers, err := filepath.Glob(filepath.Join(destination, ".agentstack-restore-incomplete-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 1 {
		t.Fatalf("incomplete restore marker count=%d paths=%#v", len(markers), markers)
	}
}

func TestStoreRejectsAmbiguousAndNonCanonicalTreeDocuments(t *testing.T) {
	for name, data := range map[string][]byte{
		"duplicate-key": []byte(`{"apiVersion":"fabric.asm.dev/cas/v1alpha1","entries":[],"entries":[]}`),
		"unknown-field": []byte(`{"apiVersion":"fabric.asm.dev/cas/v1alpha1","entries":[],"extra":true}`),
		"non-canonical": []byte(`{ "apiVersion": "fabric.asm.dev/cas/v1alpha1", "entries": [] }`),
	} {
		t.Run(name, func(t *testing.T) {
			store := New(filepath.Join(t.TempDir(), "store"))
			ref := Ref{Kind: KindTree, Digest: digestBytes(data), Size: 0}
			if err := store.ensure(); err != nil {
				t.Fatal(err)
			}
			path := store.objectPath(ref)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := store.Verify(ref); err == nil {
				t.Fatalf("invalid tree document was accepted: %s", data)
			}
		})
	}
}

func TestValidateRefEnforcesKindDigestAndSizeBounds(t *testing.T) {
	valid := Ref{Kind: KindBlob, Digest: "sha256:" + strings.Repeat("0", 64), Size: 1}
	if err := ValidateRef(valid); err != nil {
		t.Fatal(err)
	}
	for _, ref := range []Ref{
		{Kind: "unknown", Digest: valid.Digest, Size: 1},
		{Kind: KindBlob, Digest: "sha256:not-a-digest", Size: 1},
		{Kind: KindBlob, Digest: valid.Digest, Size: maxBlobBytes + 1},
		{Kind: KindTree, Digest: valid.Digest, Size: maxTreeBytes + 1},
	} {
		if err := ValidateRef(ref); err == nil {
			t.Fatalf("invalid ref was accepted: %#v", ref)
		}
	}
}
