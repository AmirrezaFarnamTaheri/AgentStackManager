package releasepack

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/safefile"
)

var fixedTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func sanitizePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}
	normalized := strings.ReplaceAll(prefix, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimSuffix(normalized, "/")
	cleaned, err := sanitizeArchivePath(normalized, "prefix")
	if err != nil {
		return "", err
	}
	if cleaned == "" {
		return "", nil
	}
	return cleaned + "/", nil
}

func sanitizeArchivePath(value, kind string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") || path.IsAbs(value) || strings.Contains(value, ":") {
		return "", fmt.Errorf("invalid %s archive path: %s", kind, value)
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return "", nil
	}
	if cleaned != value || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid %s archive path: %s", kind, value)
	}
	return cleaned, nil
}

func Pack(root, destination, prefix string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absDest, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve release destination: %w", err)
	}

	var files []string
	err = filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		absP, absErr := filepath.Abs(p)
		if absErr != nil {
			return fmt.Errorf("resolve release member %s: %w", p, absErr)
		}
		if absP == absDest {
			return nil
		}
		if strings.HasPrefix(filepath.Base(p), ".release-") && strings.HasSuffix(p, ".zip") {
			return nil
		}
		lstatInfo, lstatErr := os.Lstat(p)
		if lstatErr == nil && lstatInfo.Mode()&os.ModeSymlink != 0 {
			resolved, evalErr := filepath.EvalSymlinks(p)
			if evalErr != nil {
				return fmt.Errorf("invalid symlink %s: %w", p, evalErr)
			}
			rel, relErr := filepath.Rel(root, resolved)
			if relErr != nil || strings.HasPrefix(rel, "..") || rel == ".." {
				return fmt.Errorf("symlink %s points outside root directory: %s", p, resolved)
			}
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return err
	}
	return packRelativeFiles(root, destination, prefix, files)
}

func packRelativeFiles(root, destination, prefix string, relativeFiles []string) error {
	cleanPrefix, err := sanitizePrefix(prefix)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	files := append([]string(nil), relativeFiles...)
	sort.Strings(files)
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".release-*.zip")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	archive := zip.NewWriter(temp)
	for _, rel := range files {
		cleanRel, pathErr := sanitizeArchivePath(rel, "release member")
		if pathErr != nil || cleanRel == "" {
			_ = archive.Close()
			_ = temp.Close()
			if pathErr != nil {
				return pathErr
			}
			return fmt.Errorf("invalid release archive path: %s", rel)
		}
		pathStr := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(pathStr)
		if err != nil {
			_ = archive.Close()
			_ = temp.Close()
			return err
		}
		if !info.Mode().IsRegular() {
			_ = archive.Close()
			_ = temp.Close()
			return fmt.Errorf("release archive path is not a regular file: %s", rel)
		}
		entry := cleanPrefix + rel
		header := &zip.FileHeader{Name: entry, Method: zip.Deflate}
		header.SetModTime(fixedTime)
		mode := os.FileMode(0o644)
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		header.SetMode(mode)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			_ = archive.Close()
			_ = temp.Close()
			return err
		}
		input, err := os.Open(pathStr)
		if err != nil {
			_ = archive.Close()
			_ = temp.Close()
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			_ = archive.Close()
			_ = temp.Close()
			return copyErr
		}
		if closeErr != nil {
			_ = archive.Close()
			_ = temp.Close()
			return closeErr
		}
	}
	if err := archive.Close(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := safefile.Replace(name, destination); err != nil {
		return fmt.Errorf("publish release archive: %w", err)
	}
	return nil
}
