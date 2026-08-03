# Convergence Release Pre-Mortem

## Failure scenario

It is the day after the convergence release. ASM was rolled back because the new fabric created competing sources of truth, a donor-derived automation crossed an authority boundary, local state could not be migrated safely, or a large donor surface was declared integrated without behavior-specific evidence.

## Prioritized failure paths

### 1. Duplicate authorities diverged

**Failure conditions:** donor registries, MCP configs, workspace caches, or UI stores survived as independent write paths.

**Prevention requirements:** one canonical owner per plane; UI read-only for authority; reviewed plans revalidate source and destination; foreign state is preserved or blocks apply.

**Owner:** architecture maintainer
**Checkpoint:** adoption ledger target nodes and cross-plane tests
**Ship gate:** no duplicate durable write path in code review

### 2. Automation executed broader authority than reviewed

**Failure conditions:** shell strings, implicit network connectors, unconfirmed schedules, hidden Git hooks, or remote marketplace installation entered the runtime.

**Prevention requirements:** direct binary/argument execution only; explicit confirmation; bounded output/time; read-only Git; remote source must become a local reviewed checkout; secrets stay external.

**Owner:** runtime/security maintainer
**Checkpoint:** routine, context Git, resource import, and secret-redaction negative tests
**Ship gate:** race/security suite and manual trust-boundary review

### 3. Persistence migration corrupted local state

**Failure conditions:** legacy maps were overwritten without compatibility, future schemas were misread, or multi-file routine receipt/state writes were interrupted.

**Prevention requirements:** versioned envelopes, legacy readers, fail-closed future versions, atomic replacement, receipt reconciliation, clean migration tests.

**Owner:** state maintainer
**Checkpoint:** workspace and routine migration/recovery tests
**Ship gate:** clean legacy fixture upgrade on exact release candidate

### 4. “Integrated” meant only file-level accounting

**Failure conditions:** root records covered deep behavior, shared smoke tests represented multiple semantics, or donor-specific negative paths were absent.

**Prevention requirements:** one independently meaningful ledger row per value unit; unique test nodes; bidirectional surface links; donor-root hash verification; omission ladder through state, recovery, operator, and deep-leaf surfaces.

**Owner:** convergence reviewer
**Checkpoint:** ledger validator and omission audit
**Ship gate:** zero unresolved critical/high evidence gaps

### 5. Convergence degraded established ASM guarantees

**Failure conditions:** new scans or memory searches became unbounded, MCP/process rules were bypassed, or source packaging included donor caches/build products.

**Prevention requirements:** benchmark ceilings, existing supervisor reuse, full regression/race/vet/coverage, source-manifest-only packaging, clean extraction verification, deterministic archive reproduction.

**Owner:** release manager
**Checkpoint:** benchmark and release receipts
**Ship gate:** exact artifact clean-install and checksum match

## Plan deltas enforced

- Unique discriminating tests replace shared umbrella evidence.
- Routine receipts now redact structured/text secrets before persistence.
- Context paths, reads, searches, and Git operations have explicit confinement and resource limits.
- Workspace and routine stores have migration tests and future-version rejection.
- Source archives will be regenerated from the closed target manifest only after the complete validation run.

## Final prevention gates added during implementation

- MCP plans store digests and intent instead of copying client configurations; recovery records contain only the prior ASM registration, so unrelated credentials never enter plan or backup state.
- Duplicate JSON keys, secret-bearing same-name MCP entries, unsafe registration extras, and post-review configuration drift fail closed.
- Resource import and audit enforce per-file, total-byte, and file-count ceilings; UTF-8 snippets cannot split encoded characters.
- Workspace multi-file mutations use durable transaction journals, fence live readers, and restore stale interrupted transactions.
- Resource sync, source refresh, context refresh, workspace artifacts, and multi-client MCP changes have failure-injection rollback tests.
- Routine definitions enforce step, argument, parameter, command, value, definition-size, confirmation, and total-runtime bounds; secret-bearing arguments are rejected before persistence.
- Routine receipts and structured outputs cross the shared credential-redaction boundary before durable storage.
- The convergence ledger and surface matrix are hash-verified against all seven exact donor roots; traceability tests resolve 60 donor record, composition, and negative-path links to declared tests.
- Source release uses manifest-only deterministic packing and must pass independent extraction, manifest verification, tests, vet, and command builds before promotion.
