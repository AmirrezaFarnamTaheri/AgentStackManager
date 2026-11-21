//go:build !cgo

package sqlite

import "fmt"

const nativeSupported = false
const (
	nativeOpenReadOnly  = 0
	nativeOpenReadWrite = 0
)

type nativeDB struct{}
type nativeStmt struct{}

func nativeVersionNumber() int                           { return 0 }
func openNative(string, int) (*nativeDB, error)          { return nil, ErrUnavailable }
func (db *nativeDB) close() error                        { return nil }
func (db *nativeDB) exec(string) error                   { return ErrUnavailable }
func (db *nativeDB) prepare(string) (*nativeStmt, error) { return nil, ErrUnavailable }
func (stmt *nativeStmt) close() error                    { return nil }
func (stmt *nativeStmt) bindText(int, string) error      { return ErrUnavailable }
func (stmt *nativeStmt) bindBlob(int, []byte) error      { return ErrUnavailable }
func (stmt *nativeStmt) bindInt64(int, int64) error      { return ErrUnavailable }
func (stmt *nativeStmt) step() (bool, error)             { return false, ErrUnavailable }
func (stmt *nativeStmt) columnText(int) string           { return "" }
func (stmt *nativeStmt) columnBlob(int) []byte           { return nil }
func (stmt *nativeStmt) columnInt64(int) int64           { return 0 }
func backupNative(string, string) error {
	return fmt.Errorf("backup SQLite metadata: %w", ErrUnavailable)
}
