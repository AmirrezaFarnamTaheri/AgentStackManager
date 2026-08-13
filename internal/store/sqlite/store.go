// Package sqlite stores Fabric migration metadata in a versioned SQLite
// shadow database. Resource Hub version 1 and the immutable CAS remain the
// authoritative sources; this package records verified projections only.
package sqlite

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/agentstack/agentstack/internal/artifactgraph"
	"github.com/agentstack/agentstack/internal/migrations/asmv1"
	"github.com/agentstack/agentstack/internal/strictjson"
)

const (
	APIVersion           = "fabric.asm.dev/sqlite-shadow/v1alpha1"
	SchemaVersion        = 1
	minimumSQLiteVersion = 3037000
	applicationID        = 0x41534d46 // ASMF
	defaultBusyTimeoutMS = 5000
	maxDatabaseBytes     = 256 << 20
	maxReceiptBytes      = 64 << 20
	maxCanonicalRowBytes = 16 << 20
	resourceHubHead      = "resourcehub-v1"
)

var ErrUnavailable = errors.New("SQLite metadata backend is unavailable in this build")

type Store struct {
	Path string
}

type Summary struct {
	APIVersion        string    `json:"apiVersion"`
	Path              string    `json:"path"`
	SchemaVersion     int       `json:"schemaVersion"`
	SQLiteVersion     int       `json:"sqliteVersion"`
	JournalMode       string    `json:"journalMode"`
	ReadOnly          bool      `json:"readOnly"`
	ReceiptDigest     string    `json:"receiptDigest"`
	SourceGraphDigest string    `json:"sourceGraphDigest"`
	StagedGraphDigest string    `json:"stagedGraphDigest"`
	GeneratedAt       time.Time `json:"generatedAt"`
	ArtifactCount     int       `json:"artifactCount"`
	ResourceCount     int       `json:"resourceCount"`
	SnapshotCount     int       `json:"snapshotCount"`
	QuickCheck        string    `json:"quickCheck"`
}

type Inspection struct {
	Summary Summary
	Receipt asmv1.Receipt
}

func New(path string) Store {
	return Store{Path: path}
}

func Available() bool {
	return nativeSupported && nativeVersionNumber() >= minimumSQLiteVersion
}

func (s Store) Stage(receipt asmv1.Receipt) (Summary, error) {
	if !Available() {
		return Summary{}, ErrUnavailable
	}
	normalized, err := asmv1.SealReceipt(receipt)
	if err != nil {
		return Summary{}, fmt.Errorf("normalize ASM v1 receipt: %w", err)
	}
	if normalized.Digest != receipt.Digest {
		return Summary{}, fmt.Errorf("ASM v1 receipt is not sealed canonically")
	}
	receiptJSON, err := canonicalJSON(normalized, maxReceiptBytes)
	if err != nil {
		return Summary{}, fmt.Errorf("encode ASM v1 receipt: %w", err)
	}
	pathState, err := prepareDatabasePath(s.Path, true)
	if err != nil {
		return Summary{}, err
	}
	committed := false
	defer func() {
		if !committed && pathState.created {
			_ = removeCreatedDatabase(pathState)
		}
	}()
	db, err := openNative(pathState.path, nativeOpenReadWrite)
	if err != nil {
		return Summary{}, err
	}
	defer db.close()
	if err := confirmDatabaseIdentity(pathState); err != nil {
		return Summary{}, err
	}
	if err := configureWritable(db); err != nil {
		return Summary{}, err
	}
	if err := migrate(db); err != nil {
		return Summary{}, err
	}
	if err := confirmDatabaseIdentity(pathState); err != nil {
		return Summary{}, err
	}
	if err := secureDatabaseFiles(pathState.path); err != nil {
		return Summary{}, err
	}
	if err := db.exec("BEGIN IMMEDIATE"); err != nil {
		return Summary{}, err
	}
	transactionOpen := true
	defer func() {
		if transactionOpen {
			_ = db.exec("ROLLBACK")
		}
	}()
	if err := stageSnapshot(db, normalized, receiptJSON); err != nil {
		return Summary{}, err
	}
	if err := db.exec("COMMIT"); err != nil {
		return Summary{}, err
	}
	transactionOpen = false
	if err := db.exec("PRAGMA wal_checkpoint(FULL)"); err != nil {
		return Summary{}, err
	}
	if err := confirmDatabaseIdentity(pathState); err != nil {
		return Summary{}, err
	}
	committed = true
	inspection, err := inspectOpen(db, pathState.path, false)
	if err != nil {
		return Summary{}, err
	}
	return inspection.Summary, nil
}

func (s Store) Inspect() (Inspection, error) {
	if !Available() {
		return Inspection{}, ErrUnavailable
	}
	pathState, err := prepareDatabasePath(s.Path, false)
	if err != nil {
		return Inspection{}, err
	}
	db, err := openNative(pathState.path, nativeOpenReadOnly)
	if err != nil {
		return Inspection{}, err
	}
	defer db.close()
	if err := confirmDatabaseIdentity(pathState); err != nil {
		return Inspection{}, err
	}
	if err := configureReadOnly(db); err != nil {
		return Inspection{}, err
	}
	return inspectOpen(db, pathState.path, true)
}

func (s Store) Backup(destination string) (Summary, error) {
	if !Available() {
		return Summary{}, ErrUnavailable
	}
	if _, err := s.Inspect(); err != nil {
		return Summary{}, fmt.Errorf("verify source SQLite metadata: %w", err)
	}
	sourceState, err := prepareDatabasePath(s.Path, false)
	if err != nil {
		return Summary{}, err
	}
	destinationState, err := prepareBackupDestination(destination)
	if err != nil {
		return Summary{}, err
	}
	defer func() {
		_ = os.Remove(destinationState.temporary)
		_ = os.Remove(destinationState.temporary + "-wal")
		_ = os.Remove(destinationState.temporary + "-shm")
	}()
	if err := backupNative(sourceState.path, destinationState.temporary); err != nil {
		return Summary{}, err
	}
	if err := confirmDatabaseIdentity(sourceState); err != nil {
		return Summary{}, err
	}
	if err := confirmDatabaseIdentity(databasePathState{path: destinationState.temporary, info: destinationState.temporaryInfo}); err != nil {
		return Summary{}, err
	}
	if err := secureDatabaseFiles(destinationState.temporary); err != nil {
		return Summary{}, err
	}
	backupStore := New(destinationState.temporary)
	inspection, err := backupStore.Inspect()
	if err != nil {
		return Summary{}, fmt.Errorf("verify SQLite metadata backup: %w", err)
	}
	if err := publishNoReplace(destinationState.temporary, destinationState.destination); err != nil {
		return Summary{}, err
	}
	published, err := New(destinationState.destination).Inspect()
	if err != nil {
		return Summary{}, fmt.Errorf("SQLite backup was published at %s but final verification failed: %w", destinationState.destination, err)
	}
	if published.Receipt.Digest != inspection.Receipt.Digest {
		return Summary{}, fmt.Errorf("SQLite backup was published at %s but receipt identity changed", destinationState.destination)
	}
	return published.Summary, nil
}

func configureWritable(db *nativeDB) error {
	if nativeVersionNumber() < minimumSQLiteVersion {
		return fmt.Errorf("SQLite %d is older than required version %d", nativeVersionNumber(), minimumSQLiteVersion)
	}
	// These pragmas are connection-local. Persistent database settings are
	// applied only after migrate has admitted the existing file as ASM-owned or
	// empty, so a mistaken path cannot alter a foreign database.
	statements := []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA trusted_schema=OFF",
		fmt.Sprintf("PRAGMA busy_timeout=%d", defaultBusyTimeoutMS),
	}
	for _, statement := range statements {
		if err := db.exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func configureReadOnly(db *nativeDB) error {
	statements := []string{
		"PRAGMA query_only=ON",
		"PRAGMA foreign_keys=ON",
		"PRAGMA trusted_schema=OFF",
		fmt.Sprintf("PRAGMA busy_timeout=%d", defaultBusyTimeoutMS),
	}
	for _, statement := range statements {
		if err := db.exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func migrate(db *nativeDB) error {
	currentApplicationID, err := querySingleInt(db, "PRAGMA application_id")
	if err != nil {
		return err
	}
	currentVersion, err := querySingleInt(db, "PRAGMA user_version")
	if err != nil {
		return err
	}
	userObjects, err := querySingleInt(db, "SELECT COUNT(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'")
	if err != nil {
		return err
	}
	if currentApplicationID != 0 && currentApplicationID != applicationID {
		return fmt.Errorf("SQLite file belongs to application_id %d, not ASM Fabric", currentApplicationID)
	}
	if currentApplicationID == 0 && userObjects != 0 {
		return fmt.Errorf("refusing to adopt an unrecognized existing SQLite database")
	}
	if currentVersion > SchemaVersion {
		return fmt.Errorf("SQLite metadata schema version %d is newer than supported version %d", currentVersion, SchemaVersion)
	}
	for _, statement := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA wal_autocheckpoint=1000"} {
		if err := db.exec(statement); err != nil {
			return err
		}
	}
	if err := db.exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	open := true
	defer func() {
		if open {
			_ = db.exec("ROLLBACK")
		}
	}()
	statements := []string{
		fmt.Sprintf("PRAGMA application_id=%d", applicationID),
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS fabric_snapshots (
			receipt_digest TEXT PRIMARY KEY,
			api_version TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			source_graph_digest TEXT NOT NULL,
			staged_graph_digest TEXT NOT NULL,
			artifact_count INTEGER NOT NULL CHECK (artifact_count >= 0),
			resource_count INTEGER NOT NULL CHECK (resource_count >= 0),
			receipt_json BLOB NOT NULL CHECK (length(receipt_json) <= 67108864)
		) STRICT, WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS fabric_artifacts (
			receipt_digest TEXT NOT NULL REFERENCES fabric_snapshots(receipt_digest) ON DELETE CASCADE,
			artifact_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			content_digest TEXT NOT NULL,
			envelope_digest TEXT NOT NULL,
			artifact_json BLOB NOT NULL CHECK (length(artifact_json) <= 16777216),
			PRIMARY KEY (receipt_digest, artifact_id)
		) STRICT, WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS fabric_resources (
			receipt_digest TEXT NOT NULL REFERENCES fabric_snapshots(receipt_digest) ON DELETE CASCADE,
			resource_id TEXT NOT NULL,
			artifact_id TEXT NOT NULL,
			legacy_digest TEXT NOT NULL,
			object_kind TEXT NOT NULL,
			object_digest TEXT NOT NULL,
			object_size INTEGER NOT NULL CHECK (object_size >= 0),
			record_json BLOB NOT NULL CHECK (length(record_json) <= 16777216),
			PRIMARY KEY (receipt_digest, resource_id),
			FOREIGN KEY (receipt_digest, artifact_id) REFERENCES fabric_artifacts(receipt_digest, artifact_id) ON DELETE CASCADE
		) STRICT, WITHOUT ROWID`,
		`CREATE TABLE IF NOT EXISTS fabric_heads (
			name TEXT PRIMARY KEY,
			receipt_digest TEXT NOT NULL REFERENCES fabric_snapshots(receipt_digest),
			updated_at TEXT NOT NULL
		) STRICT, WITHOUT ROWID`,
		fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion),
	}
	for _, statement := range statements {
		if err := db.exec(statement); err != nil {
			return err
		}
	}
	migration, err := db.prepare("INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES(?, ?)")
	if err != nil {
		return err
	}
	defer migration.close()
	if err := migration.bindInt64(1, SchemaVersion); err != nil {
		return err
	}
	if err := migration.bindText(2, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if row, err := migration.step(); err != nil || row {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected row while recording SQLite schema migration")
	}
	if err := db.exec("COMMIT"); err != nil {
		return err
	}
	open = false
	return nil
}

func stageSnapshot(db *nativeDB, receipt asmv1.Receipt, receiptJSON []byte) error {
	insertSnapshot, err := db.prepare(`INSERT OR IGNORE INTO fabric_snapshots(
		receipt_digest, api_version, generated_at, source_graph_digest, staged_graph_digest,
		artifact_count, resource_count, receipt_json
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	if err := bindSnapshot(insertSnapshot, receipt, receiptJSON); err != nil {
		_ = insertSnapshot.close()
		return err
	}
	if row, err := insertSnapshot.step(); err != nil || row {
		_ = insertSnapshot.close()
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected row while staging SQLite snapshot")
	}
	if err := insertSnapshot.close(); err != nil {
		return err
	}
	storedReceipt, err := selectBlob(db, "SELECT receipt_json FROM fabric_snapshots WHERE receipt_digest=?", receipt.Digest, maxReceiptBytes)
	if err != nil {
		return err
	}
	if !bytes.Equal(storedReceipt, receiptJSON) {
		return fmt.Errorf("SQLite snapshot digest collision or stored receipt mismatch")
	}

	artifacts := append([]artifactgraph.Artifact(nil), receipt.StagedGraph.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	for _, artifact := range artifacts {
		artifactJSON, err := canonicalJSON(artifact, maxCanonicalRowBytes)
		if err != nil {
			return fmt.Errorf("encode artifact %q: %w", artifact.ID, err)
		}
		statement, err := db.prepare(`INSERT OR IGNORE INTO fabric_artifacts(
			receipt_digest, artifact_id, kind, content_digest, envelope_digest, artifact_json
		) VALUES(?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		values := []string{receipt.Digest, artifact.ID, string(artifact.Kind), artifact.Content.Digest, artifact.Digest}
		for index, value := range values {
			if err := statement.bindText(index+1, value); err != nil {
				_ = statement.close()
				return err
			}
		}
		if err := statement.bindBlob(6, artifactJSON); err != nil {
			_ = statement.close()
			return err
		}
		if row, err := statement.step(); err != nil || row {
			_ = statement.close()
			if err != nil {
				return err
			}
			return fmt.Errorf("unexpected row while staging artifact %q", artifact.ID)
		}
		if err := statement.close(); err != nil {
			return err
		}
		stored, err := selectBlob2(db, "SELECT artifact_json FROM fabric_artifacts WHERE receipt_digest=? AND artifact_id=?", receipt.Digest, artifact.ID, maxCanonicalRowBytes)
		if err != nil {
			return err
		}
		if !bytes.Equal(stored, artifactJSON) {
			return fmt.Errorf("stored artifact %q does not match receipt", artifact.ID)
		}
	}
	for _, record := range receipt.Resources {
		recordJSON, err := canonicalJSON(record, maxCanonicalRowBytes)
		if err != nil {
			return fmt.Errorf("encode resource %q: %w", record.ResourceID, err)
		}
		statement, err := db.prepare(`INSERT OR IGNORE INTO fabric_resources(
			receipt_digest, resource_id, artifact_id, legacy_digest, object_kind, object_digest, object_size, record_json
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			return err
		}
		values := []string{receipt.Digest, record.ResourceID, record.ArtifactID, record.LegacyDigest, string(record.Object.Kind), record.Object.Digest}
		for index, value := range values {
			if err := statement.bindText(index+1, value); err != nil {
				_ = statement.close()
				return err
			}
		}
		if err := statement.bindInt64(7, record.Object.Size); err != nil {
			_ = statement.close()
			return err
		}
		if err := statement.bindBlob(8, recordJSON); err != nil {
			_ = statement.close()
			return err
		}
		if row, err := statement.step(); err != nil || row {
			_ = statement.close()
			if err != nil {
				return err
			}
			return fmt.Errorf("unexpected row while staging resource %q", record.ResourceID)
		}
		if err := statement.close(); err != nil {
			return err
		}
		stored, err := selectBlob2(db, "SELECT record_json FROM fabric_resources WHERE receipt_digest=? AND resource_id=?", receipt.Digest, record.ResourceID, maxCanonicalRowBytes)
		if err != nil {
			return err
		}
		if !bytes.Equal(stored, recordJSON) {
			return fmt.Errorf("stored resource %q does not match receipt", record.ResourceID)
		}
	}
	head, err := db.prepare(`INSERT INTO fabric_heads(name, receipt_digest, updated_at) VALUES(?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET receipt_digest=excluded.receipt_digest, updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer head.close()
	if err := head.bindText(1, resourceHubHead); err != nil {
		return err
	}
	if err := head.bindText(2, receipt.Digest); err != nil {
		return err
	}
	if err := head.bindText(3, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if row, err := head.step(); err != nil || row {
		if err != nil {
			return err
		}
		return fmt.Errorf("unexpected row while updating SQLite shadow head")
	}
	return nil
}

func bindSnapshot(statement *nativeStmt, receipt asmv1.Receipt, receiptJSON []byte) error {
	textValues := []string{
		receipt.Digest,
		receipt.APIVersion,
		receipt.GeneratedAt.UTC().Format(time.RFC3339Nano),
		receipt.SourceGraphDigest,
		receipt.StagedGraph.Digest,
	}
	for index, value := range textValues {
		if err := statement.bindText(index+1, value); err != nil {
			return err
		}
	}
	if err := statement.bindInt64(6, int64(len(receipt.StagedGraph.Artifacts))); err != nil {
		return err
	}
	if err := statement.bindInt64(7, int64(len(receipt.Resources))); err != nil {
		return err
	}
	return statement.bindBlob(8, receiptJSON)
}

func inspectOpen(db *nativeDB, path string, readOnly bool) (Inspection, error) {
	quick, err := querySingleText(db, "PRAGMA quick_check")
	if err != nil {
		return Inspection{}, err
	}
	if quick != "ok" {
		return Inspection{}, fmt.Errorf("SQLite quick_check failed: %s", quick)
	}
	if err := requireNoRows(db, "PRAGMA foreign_key_check"); err != nil {
		return Inspection{}, fmt.Errorf("SQLite foreign_key_check failed: %w", err)
	}
	appID, err := querySingleInt(db, "PRAGMA application_id")
	if err != nil {
		return Inspection{}, err
	}
	if appID != applicationID {
		return Inspection{}, fmt.Errorf("unexpected SQLite application_id %d", appID)
	}
	version, err := querySingleInt(db, "PRAGMA user_version")
	if err != nil {
		return Inspection{}, err
	}
	if version != SchemaVersion {
		return Inspection{}, fmt.Errorf("unsupported SQLite metadata schema version %d", version)
	}
	migrationCount, err := querySingleInt(db, fmt.Sprintf("SELECT COUNT(*) FROM schema_migrations WHERE version=%d", SchemaVersion))
	if err != nil {
		return Inspection{}, err
	}
	if migrationCount != 1 {
		return Inspection{}, fmt.Errorf("SQLite metadata migration ledger is incomplete")
	}
	journalMode, err := querySingleText(db, "PRAGMA journal_mode")
	if err != nil {
		return Inspection{}, err
	}
	head, err := db.prepare(`SELECT s.receipt_digest, s.source_graph_digest, s.staged_graph_digest,
		s.generated_at, s.artifact_count, s.resource_count, s.receipt_json
		FROM fabric_heads h JOIN fabric_snapshots s ON s.receipt_digest=h.receipt_digest
		WHERE h.name=?`)
	if err != nil {
		return Inspection{}, err
	}
	defer head.close()
	if err := head.bindText(1, resourceHubHead); err != nil {
		return Inspection{}, err
	}
	row, err := head.step()
	if err != nil {
		return Inspection{}, err
	}
	if !row {
		return Inspection{}, fmt.Errorf("SQLite metadata has no Resource Hub shadow head")
	}
	receiptDigest := head.columnText(0)
	sourceGraphDigest := head.columnText(1)
	stagedGraphDigest := head.columnText(2)
	generatedAtText := head.columnText(3)
	artifactCount := int(head.columnInt64(4))
	resourceCount := int(head.columnInt64(5))
	receiptJSON := head.columnBlob(6)
	if len(receiptJSON) == 0 || len(receiptJSON) > maxReceiptBytes {
		return Inspection{}, fmt.Errorf("SQLite metadata receipt exceeds admission bounds")
	}
	if more, err := head.step(); err != nil {
		return Inspection{}, err
	} else if more {
		return Inspection{}, fmt.Errorf("SQLite metadata has duplicate Resource Hub heads")
	}
	var receipt asmv1.Receipt
	if err := strictjson.Decode(receiptJSON, &receipt); err != nil {
		return Inspection{}, fmt.Errorf("decode SQLite migration receipt: %w", err)
	}
	if err := asmv1.VerifyReceipt(receipt); err != nil {
		return Inspection{}, fmt.Errorf("verify SQLite migration receipt: %w", err)
	}
	if receipt.Digest != receiptDigest || receipt.SourceGraphDigest != sourceGraphDigest || receipt.StagedGraph.Digest != stagedGraphDigest {
		return Inspection{}, fmt.Errorf("SQLite snapshot identity does not match its sealed receipt")
	}
	generatedAt, err := time.Parse(time.RFC3339Nano, generatedAtText)
	if err != nil || !generatedAt.Equal(receipt.GeneratedAt) {
		return Inspection{}, fmt.Errorf("SQLite snapshot generatedAt does not match its sealed receipt")
	}
	if artifactCount != len(receipt.StagedGraph.Artifacts) || resourceCount != len(receipt.Resources) {
		return Inspection{}, fmt.Errorf("SQLite snapshot counts do not match its sealed receipt")
	}
	if err := verifyArtifactRows(db, receipt); err != nil {
		return Inspection{}, err
	}
	if err := verifyResourceRows(db, receipt); err != nil {
		return Inspection{}, err
	}
	snapshotCount64, err := querySingleInt(db, "SELECT COUNT(*) FROM fabric_snapshots")
	if err != nil {
		return Inspection{}, err
	}
	return Inspection{
		Summary: Summary{
			APIVersion: APIVersion, Path: path, SchemaVersion: version,
			SQLiteVersion: nativeVersionNumber(), JournalMode: strings.ToLower(journalMode), ReadOnly: readOnly,
			ReceiptDigest: receipt.Digest, SourceGraphDigest: receipt.SourceGraphDigest,
			StagedGraphDigest: receipt.StagedGraph.Digest, GeneratedAt: receipt.GeneratedAt,
			ArtifactCount: artifactCount, ResourceCount: resourceCount, SnapshotCount: int(snapshotCount64), QuickCheck: quick,
		},
		Receipt: receipt,
	}, nil
}

func verifyArtifactRows(db *nativeDB, receipt asmv1.Receipt) error {
	statement, err := db.prepare("SELECT artifact_id, artifact_json FROM fabric_artifacts WHERE receipt_digest=? ORDER BY artifact_id")
	if err != nil {
		return err
	}
	defer statement.close()
	if err := statement.bindText(1, receipt.Digest); err != nil {
		return err
	}
	expected := append([]artifactgraph.Artifact(nil), receipt.StagedGraph.Artifacts...)
	sort.Slice(expected, func(i, j int) bool { return expected[i].ID < expected[j].ID })
	index := 0
	for {
		row, err := statement.step()
		if err != nil {
			return err
		}
		if !row {
			break
		}
		if index >= len(expected) {
			return fmt.Errorf("SQLite metadata has extra artifact rows")
		}
		artifactID := statement.columnText(0)
		artifactJSON := statement.columnBlob(1)
		expectedJSON, err := canonicalJSON(expected[index], maxCanonicalRowBytes)
		if err != nil {
			return err
		}
		if artifactID != expected[index].ID || !bytes.Equal(artifactJSON, expectedJSON) {
			return fmt.Errorf("SQLite artifact row %q does not match sealed receipt", artifactID)
		}
		index++
	}
	if index != len(expected) {
		return fmt.Errorf("SQLite metadata is missing artifact rows")
	}
	return nil
}

func verifyResourceRows(db *nativeDB, receipt asmv1.Receipt) error {
	statement, err := db.prepare("SELECT resource_id, record_json FROM fabric_resources WHERE receipt_digest=? ORDER BY resource_id")
	if err != nil {
		return err
	}
	defer statement.close()
	if err := statement.bindText(1, receipt.Digest); err != nil {
		return err
	}
	expected := append([]asmv1.ResourceRecord(nil), receipt.Resources...)
	sort.Slice(expected, func(i, j int) bool { return expected[i].ResourceID < expected[j].ResourceID })
	index := 0
	for {
		row, err := statement.step()
		if err != nil {
			return err
		}
		if !row {
			break
		}
		if index >= len(expected) {
			return fmt.Errorf("SQLite metadata has extra resource rows")
		}
		resourceID := statement.columnText(0)
		recordJSON := statement.columnBlob(1)
		expectedJSON, err := canonicalJSON(expected[index], maxCanonicalRowBytes)
		if err != nil {
			return err
		}
		if resourceID != expected[index].ResourceID || !bytes.Equal(recordJSON, expectedJSON) {
			return fmt.Errorf("SQLite resource row %q does not match sealed receipt", resourceID)
		}
		index++
	}
	if index != len(expected) {
		return fmt.Errorf("SQLite metadata is missing resource rows")
	}
	return nil
}

func canonicalJSON(value any, limit int) ([]byte, error) {
	encoded, err := strictjson.MarshalCanonical(value)
	if err != nil {
		return nil, err
	}
	if len(encoded) > limit {
		return nil, fmt.Errorf("canonical JSON exceeds %d bytes", limit)
	}
	return encoded, nil
}

func selectBlob(db *nativeDB, query, value string, limit int) ([]byte, error) {
	statement, err := db.prepare(query)
	if err != nil {
		return nil, err
	}
	defer statement.close()
	if err := statement.bindText(1, value); err != nil {
		return nil, err
	}
	row, err := statement.step()
	if err != nil {
		return nil, err
	}
	if !row {
		return nil, fmt.Errorf("SQLite row is missing")
	}
	blob := statement.columnBlob(0)
	if len(blob) > limit {
		return nil, fmt.Errorf("SQLite row exceeds %d bytes", limit)
	}
	if extra, err := statement.step(); err != nil {
		return nil, err
	} else if extra {
		return nil, fmt.Errorf("SQLite query returned duplicate rows")
	}
	return blob, nil
}

func selectBlob2(db *nativeDB, query, first, second string, limit int) ([]byte, error) {
	statement, err := db.prepare(query)
	if err != nil {
		return nil, err
	}
	defer statement.close()
	if err := statement.bindText(1, first); err != nil {
		return nil, err
	}
	if err := statement.bindText(2, second); err != nil {
		return nil, err
	}
	row, err := statement.step()
	if err != nil {
		return nil, err
	}
	if !row {
		return nil, fmt.Errorf("SQLite row is missing")
	}
	blob := statement.columnBlob(0)
	if len(blob) > limit {
		return nil, fmt.Errorf("SQLite row exceeds %d bytes", limit)
	}
	if extra, err := statement.step(); err != nil {
		return nil, err
	} else if extra {
		return nil, fmt.Errorf("SQLite query returned duplicate rows")
	}
	return blob, nil
}

func querySingleText(db *nativeDB, query string) (string, error) {
	statement, err := db.prepare(query)
	if err != nil {
		return "", err
	}
	defer statement.close()
	row, err := statement.step()
	if err != nil {
		return "", err
	}
	if !row {
		return "", fmt.Errorf("SQLite query returned no row")
	}
	value := statement.columnText(0)
	if extra, err := statement.step(); err != nil {
		return "", err
	} else if extra {
		return "", fmt.Errorf("SQLite query returned multiple rows")
	}
	return value, nil
}

func querySingleInt(db *nativeDB, query string) (int, error) {
	statement, err := db.prepare(query)
	if err != nil {
		return 0, err
	}
	defer statement.close()
	row, err := statement.step()
	if err != nil {
		return 0, err
	}
	if !row {
		return 0, fmt.Errorf("SQLite query returned no row")
	}
	value := int(statement.columnInt64(0))
	if extra, err := statement.step(); err != nil {
		return 0, err
	} else if extra {
		return 0, fmt.Errorf("SQLite query returned multiple rows")
	}
	return value, nil
}

func requireNoRows(db *nativeDB, query string) error {
	statement, err := db.prepare(query)
	if err != nil {
		return err
	}
	defer statement.close()
	row, err := statement.step()
	if err != nil {
		return err
	}
	if row {
		return fmt.Errorf("constraint violation: %s", statement.columnText(0))
	}
	return nil
}

func (s Store) absolutePath() (string, error) {
	if strings.TrimSpace(s.Path) == "" {
		return "", fmt.Errorf("SQLite metadata path is required")
	}
	return filepath.Abs(s.Path)
}
