//go:build windows

package safefile

import "github.com/agentstack/agentstack/internal/winsecurity"

type fileMetadata struct {
	dacl winsecurity.DACL
}

func captureFileMetadata(path string) (fileMetadata, error) {
	dacl, err := winsecurity.CaptureFileDACL(path)
	if err != nil {
		return fileMetadata{}, err
	}
	return fileMetadata{dacl: dacl}, nil
}

func applyFileMetadata(path string, metadata fileMetadata) error {
	return winsecurity.ApplyFileDACL(path, metadata.dacl)
}

func fileMetadataMatches(path string, metadata fileMetadata) (bool, error) {
	current, err := winsecurity.CaptureFileDACL(path)
	if err != nil {
		return false, err
	}
	return current.SecurityInformation == metadata.dacl.SecurityInformation && current.SDDL == metadata.dacl.SDDL, nil
}
