# ASM Fabric Phase 3: Reversible SQLite Shadow Metadata

## Evidence basis

Phase 3 implements the next dependency-ordered migration slice from the five uploaded research reports:

- `deep-research-report (9).md` proposes `internal/store/sqlite` with WAL, migrations, backup, and read-only recovery while retaining deterministic export and existing ASM authority.
- `deep-research-report (1).md` requires dual storage to remain reversible and explicitly preserves Resource Hub compatibility until promotion is separately reviewed.
- `deep-research-report (5).md` requires transactional metadata, content-addressed references, bounded admission, and preservation of the local-first mutation boundary.

No donor database, daemon, package manager, or synchronizer was embedded. The implementation uses ASM's canonical graph, CAS receipt, shared fabric lease, and existing Resource Hub authority.

## Converged decision

Phase 3 adds a versioned SQLite **shadow metadata index**. Resource Hub version 1 remains the only authoritative source for resource metadata and payload ownership. CAS remains the immutable payload shadow. SQLite stores a verified historical projection of sealed CAS migration receipts and a movable non-authoritative `resourcehub-v1` shadow head.

The promotion sequence remains:

```text
Resource Hub v1 authority
  -> canonical snapshot
  -> immutable CAS objects
  -> sealed ASM v1 migration receipt
  -> transactional SQLite shadow metadata
  -> read-only verification
```

A later migration decision may introduce dual-read behavior. This phase does not redirect production reads or writes.

## Implemented contract

### `internal/store/sqlite`

The metadata store provides:

- schema version `1` and SQLite application identity `ASMF`;
- WAL journal mode, `synchronous=FULL`, foreign-key enforcement, trusted-schema disablement, and a bounded busy timeout;
- strict tables for immutable snapshots, canonical artifacts, Resource Hub resource/CAS records, schema migrations, and named shadow heads;
- transactionally staged receipts, artifacts, resources, and head updates;
- idempotent restaging of an identical receipt and retained immutable history when the authoritative graph changes;
- byte-for-byte canonical JSON comparison between every database row and its sealed migration receipt;
- `PRAGMA quick_check`, `foreign_key_check`, schema/application identity checks, migration-ledger checks, count checks, and receipt digest verification on every inspection;
- refusal to adopt an unrelated existing SQLite database, including application-ID and pre-existing-schema checks;
- regular-file, no-symlink, size, and private-permission admission for database, WAL, and shared-memory files;
- read-only inspection that never creates a missing database or parent directory;
- verified SQLite online backup to a same-directory incomplete file followed by atomic no-replace hard-link publication;
- preservation of all prior snapshots rather than deletion or compaction.

The database stores CAS references and canonical metadata, not payload bytes or secret values.

### Native backend boundary

The source includes a narrow SQLite C-API backend selected when CGO is enabled. It requires SQLite 3.37 or newer at build/runtime. CGO-disabled builds retain the complete existing ASM product and compile a fail-closed metadata stub returning `ErrUnavailable`; no fallback silently writes a different format.

This phase verifies the native backend on Linux amd64. Windows and macOS source compatibility is checked through CGO-disabled compilation; protected native builders must provide and test the SQLite toolchain before the metadata preview is enabled in a release.

## Operator surface

```text
agentstack hub db-stage [--db PATH] [--cas-root PATH]
agentstack hub db-inspect [--db PATH]
agentstack hub db-verify [--db PATH] [--cas-root PATH]
agentstack hub db-backup [--db PATH] --destination PATH --yes
```

Defaults:

```text
<data-root>/fabric/metadata.db
<data-root>/fabric/cas
```

`db-stage` runs under the shared Fabric lease. It creates a fresh verified CAS receipt, commits its canonical metadata in one SQLite transaction, reopens and verifies the stored projection, then verifies the receipt against the still-current Resource Hub and CAS.

`db-inspect` is read-only and reports the current shadow head, schema, SQLite version, journal mode, counts, and integrity result.

`db-verify` additionally proves that the shadow head still matches authoritative Resource Hub state and that all referenced CAS objects still round-trip to their legacy digests.

`db-backup` requires explicit confirmation, verifies the source before copying, uses SQLite's online backup API, verifies the temporary copy, and refuses to replace an existing destination.

## Authority and recovery boundary

| State | Authority after Phase 3 | SQLite relationship |
| --- | --- | --- |
| Resource registry and metadata | Resource Hub v1 | Verified shadow rows only |
| Resource payloads | Resource Hub v1 | CAS references only |
| Immutable payload shadow | CAS | Referenced and reverified |
| Current migration evidence | Sealed ASM v1 receipt | Stored byte-for-byte |
| SQLite head | Non-authoritative local index | May move only under Fabric lease |
| Target files and ownership | Existing Resource Hub sync engine | No access |
| Reviewed plans and backups | Existing ASM modules | No access |
| Database backup | Operator-selected new path | Verified no-overwrite copy |

Deleting or losing the SQLite database does not remove Resource Hub or CAS state. Rebuilding the shadow index from the authoritative Resource Hub and CAS is supported through `db-stage`.

## Validation coverage

Tests cover:

- transactional stage and read-only inspection;
- schema, application ID, WAL, strict row identity, and private file permissions;
- idempotent repeated stage and immutable multi-snapshot history;
- sealed receipt, artifact-row, resource-row, count, migration-ledger, quick-check, and foreign-key verification;
- refusal to adopt unrelated SQLite databases;
- missing-path read-only behavior and symlink rejection;
- verified backup and no-overwrite publication;
- complete CLI stage, inspect, current-state verify, stale-authority rejection, backup, and confirmation gates;
- CGO-disabled package and CLI compilation with a fail-closed unavailable backend.

## Explicit non-goals

Phase 3 does not:

- read Resource Hub production state from SQLite;
- write or remove Resource Hub registry data;
- store resource payload bytes outside CAS;
- restore a database over an active database path;
- delete old snapshots or CAS objects;
- implement schema downgrade, package resolution, registries, adapters, or semantic merge;
- bundle or attest a release SQLite library for Windows/macOS;
- claim Windows/macOS native SQLite execution without protected native evidence.

The next dependency-ordered slice is the versioned adapter capability contract and machine-readable loss reports, followed by extraction of existing target logic behind that non-mutating adapter boundary.
