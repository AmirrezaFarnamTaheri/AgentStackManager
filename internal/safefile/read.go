package safefile

import (
	"fmt"
	"io"
	"os"
)

// ReadBoundedRegular reads a regular, non-symlink file while enforcing a hard
// byte ceiling before and during the read. It rejects filesystem substitutions
// that would otherwise turn durable state reads into unbounded allocations.
func ReadBoundedRegular(path string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("read limit must be positive")
	}
	linkInfo, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 || !linkInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("bounded read requires a regular non-symlink file: %s", path)
	}
	if linkInfo.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d bytes: %s", limit, path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(linkInfo, openedInfo) {
		_ = file.Close()
		return nil, fmt.Errorf("file changed before bounded read: %s", path)
	}
	if openedInfo.Size() > limit {
		_ = file.Close()
		return nil, fmt.Errorf("file exceeds %d bytes: %s", limit, path)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file grew beyond %d bytes while reading: %s", limit, path)
	}
	return data, nil
}
