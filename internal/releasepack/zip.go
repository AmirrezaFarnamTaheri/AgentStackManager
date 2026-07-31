package releasepack

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var fixedTime = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func Pack(root, destination, prefix string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	var files []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, path)
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
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		entry := filepath.ToSlash(rel)
		if prefix != "" {
			entry = strings.TrimSuffix(prefix, "/") + "/" + entry
		}
		header := &zip.FileHeader{Name: entry, Method: zip.Deflate}
		header.SetModTime(fixedTime)
		mode := os.FileMode(0o644)
		if info, err := os.Stat(path); err == nil && info.Mode()&0o111 != 0 {
			mode = 0o755
		}
		header.SetMode(mode)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			archive.Close()
			temp.Close()
			return err
		}
		input, err := os.Open(path)
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
