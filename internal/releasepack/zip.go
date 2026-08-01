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
)

var fixedTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func sanitizePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}
	prefix = strings.ReplaceAll(prefix, "\\", "/")
	prefix = strings.TrimPrefix(prefix, "./")
	prefix = strings.TrimPrefix(prefix, "/")
	cleaned := path.Clean(prefix)
	if cleaned == "." || cleaned == "" {
		return "", nil
	}
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("invalid prefix path traversal: %s", prefix)
	}
	return cleaned + "/", nil
}

func Pack(root, destination, prefix string) error {
	cleanPrefix, err := sanitizePrefix(prefix)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	absDest, _ := filepath.Abs(destination)

	var files []string
	err = filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		absP, _ := filepath.Abs(p)
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
		files = append(files, p)
		return nil
	})
	if err != nil {
		return err
	}
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
	for _, pathStr := range files {
		rel, err := filepath.Rel(root, pathStr)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		entry := cleanPrefix + filepath.ToSlash(rel)
		header := &zip.FileHeader{Name: entry, Method: zip.Deflate}
		header.SetModTime(fixedTime)
		mode := os.FileMode(0o644)
		if info, err := os.Stat(pathStr); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		header.SetMode(mode)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		input, err := os.Open(pathStr)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			archive.Close()
			temp.Close()
			return copyErr
		}
		if closeErr != nil {
			archive.Close()
			temp.Close()
			return closeErr
		}
	}
	if err := archive.Close(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, destination); err != nil {
		return fmt.Errorf("publish release archive: %w", err)
	}
	return nil
}
