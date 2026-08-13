//go:build cgo

package sqlite

/*
#cgo pkg-config: sqlite3
#include <sqlite3.h>
#include <stdlib.h>

static int asm_bind_text(sqlite3_stmt *stmt, int index, const char *value, int length) {
	return sqlite3_bind_text(stmt, index, value, length, SQLITE_TRANSIENT);
}

static int asm_bind_blob(sqlite3_stmt *stmt, int index, const void *value, int length) {
	return sqlite3_bind_blob(stmt, index, value, length, SQLITE_TRANSIENT);
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const nativeSupported = true

const (
	nativeOpenReadOnly  = int(C.SQLITE_OPEN_READONLY | C.SQLITE_OPEN_FULLMUTEX)
	nativeOpenReadWrite = int(C.SQLITE_OPEN_READWRITE | C.SQLITE_OPEN_CREATE | C.SQLITE_OPEN_FULLMUTEX)
)

type nativeDB struct {
	ptr *C.sqlite3
}

type nativeStmt struct {
	db  *nativeDB
	ptr *C.sqlite3_stmt
}

func nativeVersionNumber() int {
	return int(C.sqlite3_libversion_number())
}

func openNative(path string, flags int) (*nativeDB, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	var ptr *C.sqlite3
	rc := C.sqlite3_open_v2(cPath, &ptr, C.int(flags), nil)
	if rc != C.SQLITE_OK {
		message := "unknown SQLite open error"
		if ptr != nil {
			message = C.GoString(C.sqlite3_errmsg(ptr))
			_ = C.sqlite3_close_v2(ptr)
		}
		return nil, fmt.Errorf("open SQLite database: %s", message)
	}
	return &nativeDB{ptr: ptr}, nil
}

func (db *nativeDB) close() error {
	if db == nil || db.ptr == nil {
		return nil
	}
	rc := C.sqlite3_close_v2(db.ptr)
	db.ptr = nil
	if rc != C.SQLITE_OK {
		return fmt.Errorf("close SQLite database: code %d", int(rc))
	}
	return nil
}

func (db *nativeDB) error(rc C.int, operation string) error {
	if rc == C.SQLITE_OK {
		return nil
	}
	return fmt.Errorf("%s: %s (code %d)", operation, C.GoString(C.sqlite3_errmsg(db.ptr)), int(rc))
}

func (db *nativeDB) exec(query string) error {
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))
	var message *C.char
	rc := C.sqlite3_exec(db.ptr, cQuery, nil, nil, &message)
	if rc == C.SQLITE_OK {
		return nil
	}
	text := C.GoString(C.sqlite3_errmsg(db.ptr))
	if message != nil {
		text = C.GoString(message)
		C.sqlite3_free(unsafe.Pointer(message))
	}
	return fmt.Errorf("execute SQLite statement: %s (code %d)", text, int(rc))
}

func (db *nativeDB) prepare(query string) (*nativeStmt, error) {
	cQuery := C.CString(query)
	defer C.free(unsafe.Pointer(cQuery))
	var ptr *C.sqlite3_stmt
	rc := C.sqlite3_prepare_v2(db.ptr, cQuery, -1, &ptr, nil)
	if err := db.error(rc, "prepare SQLite statement"); err != nil {
		return nil, err
	}
	return &nativeStmt{db: db, ptr: ptr}, nil
}

func (stmt *nativeStmt) close() error {
	if stmt == nil || stmt.ptr == nil {
		return nil
	}
	rc := C.sqlite3_finalize(stmt.ptr)
	stmt.ptr = nil
	if rc != C.SQLITE_OK {
		return stmt.db.error(rc, "finalize SQLite statement")
	}
	return nil
}

func (stmt *nativeStmt) bindText(index int, value string) error {
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cValue))
	rc := C.asm_bind_text(stmt.ptr, C.int(index), cValue, C.int(len(value)))
	return stmt.db.error(rc, "bind SQLite text")
}

func (stmt *nativeStmt) bindBlob(index int, value []byte) error {
	if len(value) == 0 {
		rc := C.sqlite3_bind_zeroblob(stmt.ptr, C.int(index), 0)
		return stmt.db.error(rc, "bind SQLite blob")
	}
	rc := C.asm_bind_blob(stmt.ptr, C.int(index), unsafe.Pointer(&value[0]), C.int(len(value)))
	return stmt.db.error(rc, "bind SQLite blob")
}

func (stmt *nativeStmt) bindInt64(index int, value int64) error {
	rc := C.sqlite3_bind_int64(stmt.ptr, C.int(index), C.sqlite3_int64(value))
	return stmt.db.error(rc, "bind SQLite integer")
}

func (stmt *nativeStmt) step() (bool, error) {
	rc := C.sqlite3_step(stmt.ptr)
	switch rc {
	case C.SQLITE_ROW:
		return true, nil
	case C.SQLITE_DONE:
		return false, nil
	default:
		return false, stmt.db.error(rc, "step SQLite statement")
	}
}

func (stmt *nativeStmt) columnText(index int) string {
	value := C.sqlite3_column_text(stmt.ptr, C.int(index))
	if value == nil {
		return ""
	}
	length := C.sqlite3_column_bytes(stmt.ptr, C.int(index))
	return C.GoStringN((*C.char)(unsafe.Pointer(value)), length)
}

func (stmt *nativeStmt) columnBlob(index int) []byte {
	value := C.sqlite3_column_blob(stmt.ptr, C.int(index))
	length := C.sqlite3_column_bytes(stmt.ptr, C.int(index))
	if value == nil || length == 0 {
		return nil
	}
	return C.GoBytes(value, length)
}

func (stmt *nativeStmt) columnInt64(index int) int64 {
	return int64(C.sqlite3_column_int64(stmt.ptr, C.int(index)))
}

func backupNative(sourcePath, destinationPath string) error {
	source, err := openNative(sourcePath, nativeOpenReadOnly)
	if err != nil {
		return err
	}
	defer source.close()
	destination, err := openNative(destinationPath, nativeOpenReadWrite)
	if err != nil {
		return err
	}
	defer destination.close()
	mainName := C.CString("main")
	defer C.free(unsafe.Pointer(mainName))
	backup := C.sqlite3_backup_init(destination.ptr, mainName, source.ptr, mainName)
	if backup == nil {
		return fmt.Errorf("initialize SQLite backup: %s", C.GoString(C.sqlite3_errmsg(destination.ptr)))
	}
	rc := C.sqlite3_backup_step(backup, -1)
	finishRC := C.sqlite3_backup_finish(backup)
	if rc != C.SQLITE_DONE {
		return destination.error(rc, "copy SQLite backup")
	}
	if finishRC != C.SQLITE_OK {
		return destination.error(finishRC, "finish SQLite backup")
	}
	return nil
}
