package observation

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ResolvePhysicalIdentity resolves physical identity, handling Windows reparse points, symlinks, and junctions.
func ResolvePhysicalIdentity(path string, maxHops int) (PhysicalIdentity, []string, ObjectType, os.FileInfo, error) {
	cleanPath := filepath.Clean(path)
	reparseChain := []string{cleanPath}

	current := cleanPath
	hops := 0

	for {
		info, err := os.Lstat(current)
		if err != nil {
			return PhysicalIdentity{}, reparseChain, ObjectTypeReparse, nil, fmt.Errorf("lstat %s: %w", current, err)
		}

		// Check for symlink / reparse point / junction
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			target, err := os.Readlink(current)
			if err != nil {
				// Broken link / junction
				return PhysicalIdentity{
					Key: fmt.Sprintf("broken:%s", current),
				}, reparseChain, ObjectTypeJunction, info, fmt.Errorf("broken junction or unreadable reparse target %s: %w", current, err)
			}

			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(current), target)
			}
			target = filepath.Clean(target)

			hops++
			if hops > maxHops {
				return PhysicalIdentity{
					Key: fmt.Sprintf("cycle:%s", current),
				}, reparseChain, ObjectTypeReparse, info, fmt.Errorf("reparse hop limit (%d) exceeded at %s", maxHops, current)
			}

			// Check for reparse cycle
			for _, visited := range reparseChain {
				if strings.EqualFold(visited, target) {
					reparseChain = append(reparseChain, target)
					return PhysicalIdentity{
						Key: fmt.Sprintf("cycle:%s", target),
					}, reparseChain, ObjectTypeReparse, info, fmt.Errorf("reparse cycle detected at %s -> %s", current, target)
				}
			}

			reparseChain = append(reparseChain, target)
			current = target
			continue
		}

		// Resolved to final physical target
		targetInfo, err := os.Stat(current)
		if err != nil {
			return PhysicalIdentity{}, reparseChain, ObjectTypeReparse, nil, fmt.Errorf("stat %s: %w", current, err)
		}

		objType := ObjectTypeFile
		if targetInfo.IsDir() {
			objType = ObjectTypeDirectory
		} else if len(reparseChain) > 1 {
			objType = ObjectTypeJunction
		}

		identityKey := fmt.Sprintf("phys:%s|%d|%d", strings.ToLower(current), targetInfo.Size(), targetInfo.ModTime().UnixNano())

		return PhysicalIdentity{
			Key: identityKey,
		}, reparseChain, objType, targetInfo, nil
	}
}

// ComputePayloadDigest hashes a file's physical payload once.
func ComputePayloadDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}
