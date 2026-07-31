//go:build !windows

package safefile

import "os"

type fileMetadata struct {
	mode os.FileMode
}

func captureFileMetadata(path string) (fileMetadata, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileMetadata{}, err
	}
	const preserved = os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	return fileMetadata{mode: info.Mode() & preserved}, nil
}

func applyFileMetadata(path string, metadata fileMetadata) error {
	return os.Chmod(path, metadata.mode)
}

func fileMetadataMatches(path string, metadata fileMetadata) (bool, error) {
	current, err := captureFileMetadata(path)
	if err != nil {
		return false, err
	}
	return current.mode == metadata.mode, nil
}
