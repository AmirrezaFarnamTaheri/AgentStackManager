# ASM Fabric Phase 2: Content-Addressed Shadow Store

## Evidence basis

Phase 2 follows the dependency order established by the five uploaded research reports and Phase 1's canonical artifact envelope.

- `deep-research-report (9).md` requires a content-addressed object store with atomic writes, deduplication, garbage-collection marks, and corruption verification, followed by a current-store migration that preserves all Resource Hub records.
- `deep-research-report (1).md` requires incremental dual storage, deterministic export, reversible migration, downgrade recovery, and retention of the existing ASM safety model.
- `deep-research-report (5).md` requires content-addressed blobs, transactional updates, explicit ownership, and migration that does not silently transfer authority.

The implementation remains target-native: no donor database, package manager, synchronizer, or runtime was embedded.

## Converged decision

Phase 2 adds a verified shadow store before SQLite metadata or any authoritative migration. This is the smallest coherent vertical slice that proves content immutability, deduplication, reversible reconstruction, stale-source detection, and operator-visible receipts while keeping Resource Hub version 1 as the only source of truth.

## Implemented contract

### `internal/cas`

The local CAS provides:

- raw SHA-256-addressed blob objects;
- deterministic tree manifests containing sorted paths, regular-file blob references, empty directories, sizes, and permission bits;
- atomic no-replace object installation through same-directory temporary files and hard-link publication;
- deduplication that verifies an existing object before accepting it;
- hard ceilings of 10,000 files, 16 MiB per blob, 64 MiB per tree payload, and 8 MiB per manifest;
- rejection of symlink inputs, special files, traversal paths, duplicate or unsorted entries, missing directory parents, malformed references, symlink store roots, and symlink object-prefix directories;
- recursive blob and tree corruption verification;
- deterministic reachability closure for later mark-and-sweep garbage collection, with no deletion in this phase;
- kind-qualified object URIs such as `cas://tree/sha256/<digest>` so a digest cannot ambiguously identify both a blob and a tree;
- materialization only to a new destination with exclusive file and directory creation, per-entry no-overwrite enforcement, post-write modes, and an incomplete marker retained on failure.

Object paths are implementation details under:

```text
<cas-root>/objects/sha256/<prefix>/<digest>.blob
<cas-root>/objects/sha256/<prefix>/<digest>.tree.json
```

### `internal/migrations/asmv1`

The ASM v1 migration stage:

1. captures the current read-only canonical Resource Hub graph;
2. stores every authoritative `resources/<id>/content` directory as a CAS tree;
3. verifies every tree and referenced blob;
4. reconstructs each resource in an isolated temporary directory;
5. requires the reconstructed legacy Resource Hub digest to equal the registry digest;
6. emits a CAS-backed canonical graph while preserving the legacy semantic content digest;
7. records the CAS object under the `asm.cas.v1` extension and field provenance;
8. seals the source graph digest, staged graph, resource/object map, and timestamp in a migration receipt.

`VerifyCurrent` rejects changed Resource Hub state as stale and repeats all CAS and round-trip checks. `RestoreResource` reconstructs one receipt-bound resource to a new path. It never replaces existing content. If final legacy-digest verification fails, the destination is retained and the error names it for inspection; ASM does not delete a path after publication because another actor may have introduced content concurrently.

## Operator surface

```text
agentstack hub cas-stage [--root PATH] > receipt.json
agentstack hub cas-verify --receipt receipt.json [--root PATH]
agentstack hub cas-restore --receipt receipt.json --resource ID --destination PATH --yes [--root PATH]
```

The default store is `<data-root>/fabric/cas`. Receipt input is strict JSON, bounded to 32 MiB, and rejected when it is a symlink or non-regular file.

## Authority and recovery boundary

| State | Authority after Phase 2 | CAS operation |
| --- | --- | --- |
| Resource registry and metadata | Resource Hub v1 | Read only |
| Resource payload | `resources/<id>/content` | Verified shadow copy |
| Resource sync/refresh plans | Existing Resource Hub planners | No access |
| Target files and ownership | Existing Resource Hub sync engine | No access |
| CAS blobs and tree manifests | Immutable local CAS | Add and verify only |
| Migration receipt | Operator-retained strict JSON | Verify and restore to a new path |
| Garbage collection | Not implemented | Reachability marks only |

A failed stage may leave unreferenced immutable objects. They are harmless and intentionally retained until a later reviewed garbage-collection slice exists.

## Validation coverage

Tests cover:

- deterministic tree identity independent of filesystem creation order;
- blob and tree deduplication;
- empty-directory and file-mode reconstruction;
- recursive reachability marks;
- corruption, size, malformed-reference, traversal, special-file, symlink-input, symlink-root, symlink-prefix, and symlink-destination-parent rejection;
- refusal to overwrite restore destinations;
- refusal to overwrite a raced tree entry or blob destination;
- retained incomplete-tree markers instead of destructive cleanup;
- verification that read-only CAS checks do not create a missing store;
- deterministic Resource Hub shadow staging and repeated-stage object reuse;
- complete legacy-digest round trips;
- stale Resource Hub rejection;
- migration-receipt tamper rejection;
- CLI stage, verify, restore, and confirmation gates.

## Explicit non-goals

Phase 2 does not:

- move Resource Hub authority to CAS;
- change `registry.json` or the `resources/` layout;
- add SQLite metadata, WAL, schema migrations, or a database backup API;
- garbage-collect unreferenced objects;
- implement packages, dependencies, lockfiles, registries, adapters, or semantic merge;
- restore directly over authoritative Resource Hub or target paths;
- sign receipts or provide publisher authentication.

Phase 3 implements the next SQLite shadow-metadata slice in `FABRIC_PHASE3.md`. The remaining dependency-ordered work begins with the adapter capability and loss-report contract.
