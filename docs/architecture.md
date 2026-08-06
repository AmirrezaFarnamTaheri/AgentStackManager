# Architecture

```text
CLI / loopback browser UI
          |
          v
Application service
  |-- immutable catalog + validation
  |-- minimized inventory + compatibility probes
  |-- sealed preservation planner
  |-- cross-process mutation lease
  |-- incremental transaction journal + ownership
  |-- postcondition verifier
  |-- indexed backup / preview / restore
  |-- privacy export, deletion, retention, diagnostics
  |-- MCP configuration, live doctor, registration repair
  |-- ownership-scoped lifecycle
          |
          +--> supervised process runtime --> WinGet / npm / uv / Git
          +--> MCP router --> managed child runtime --> pooled/one-shot stdio children
          +--> session launcher --> supervised Codex / AGY process tree

Release tooling
          +--> source provenance capsule --> exact manifest --> selected-file ZIP --> reopen verification
```

## Deep module boundaries

Four modules now own the highest-risk cross-cutting workflows instead of requiring callers to reproduce their invariants:

- `internal/supervisor.Runtime` is the only general process-execution boundary. It owns environment construction, deadlines, bounded output capture, process-tree lifecycle, and optional OS resource limits. Inventory probes, installer commands, sessions, MCP children, and Windows helper invocations delegate to it.
- `internal/reviewedplan.Executor` owns the complete reviewed-plan transaction: confirmation, mutation lease, plan loading, digest/expiry/catalog/inventory revalidation, lease renewal, pre-mutation consumption, journal persistence, and successful-install ownership recording.
- `internal/mcp.ManagedChildRuntime` is the caller-facing MCP child boundary. It chooses one-shot or persistent behavior from `ServerConfig` and hides launch, protocol negotiation, bounded framing, pooled reuse, idle expiry, failure eviction, and shutdown.
- `internal/releasepack` owns source provenance closure. It writes and verifies the exact source manifest, rejects runtime artifacts, packages only manifested regular files, reopens the resulting ZIP, and verifies exact membership and every digest before publication.

These interfaces are intentionally small. Platform-specific process enforcement remains behind `processctl`; domain-specific inventory, state, and ownership functions remain injected into their owning transaction boundary.

## Sources of truth

- `internal/catalog/default.json`: component/profile policy, versions, sources, publishers, platforms, dependencies, providers, MCP commands, and skill provenance.
- AgentStack state root: sealed plans, transaction journal, ownership, backups, diagnostics, and local events.
- Third-party package managers: actual package installation state.
- Codex/AGY configuration: client registration state; AgentStack changes only its named entry.

## State transitions

1. Inventory is collected with bounded probes and minimized before persistence.
2. Planner expands dependencies, resolves providers, and generates an ordered plan.
3. Service seals the plan with catalog and inventory digests and an expiry.
4. Apply acquires a machine-user mutation lease and revalidates the seal.
5. Each action is journaled before execution, run with bounded output/time, and verified by a postcondition scan.
6. Ownership is recorded only for successful AgentStack-created resources.
7. Router configuration is changed only after successful prerequisite installation and only when semantic content differs.

## MCP lifecycle

The router is one stdio MCP server with four stable tools. It negotiates the client protocol version, lazily starts a selected child, performs bounded request/response forwarding, and pools healthy children for the router lifetime. On Windows, each catalog-managed child tree receives a Job Object with kill-on-close, aggregate memory, CPU-rate, and active-process ceilings. Child event logs contain server digests, durations, and status—not raw arguments, environment values, or tool payloads.

## Recovery model

Windows PATH values cross the PowerShell process boundary as UTF-16LE/Base64 so non-ASCII and duplicate pre-existing segments remain intact; only the AgentStack bin segment is appended persistently.

Managed files are replaced through staging and rollback while carrying the destination POSIX mode or Windows DACL to the staged replacement. Every backup has an index record, original target, content digest, and reason. Preview is read-only. Restore revalidates digest/target/structure and performs a live MCP probe for router configuration. Startup recovery and retention run only while holding the global mutation lease; if another process owns that lease, its live transaction is left untouched for that process to complete or recover.

## Platform boundary

The supported distributable target is Windows x64 and Windows ARM64. Cross-platform Go code remains testable on Linux, but automatic catalog actions and the browser setup workflow are not advertised as a Linux product release.

## Unified fabric planes

The convergence release adds five deep modules without changing existing package-management, reviewed-plan, MCP-runtime, process-supervision, or release authorities.

1. `resourcehub` owns canonical agent resources and reviewed distribution.
2. `contextengine` owns deterministic project evidence and managed context blocks.
3. `workspace` owns hierarchy, scoped memory, prompt variables, and artifacts.
4. `mcplink` owns reviewed client link/unlink intent, digest-only plans, live reconstruction, minimal registration-only recovery records, and transactional cross-client apply for the existing MCP router.
5. `routines` owns schedules, sequential execution, redacted receipts, and restart reconciliation.

`app.Service` composes the planes. `cli.CLI` and the loopback UI are adapters; they do not own durable state. External commands flow through `runner.CommandRunner`. MCP process authority remains `mcp.ManagedChildRuntime` and the supervisor boundary.

### Canonical artifact graph preview

`internal/artifactgraph` introduces the additive `fabric.asm.dev/v1alpha1` canonical envelope required by future package, adapter, lockfile, and reconciliation layers. It owns stable canonical IDs, typed kinds, separate content and envelope digests, conservative execution classes, target bindings, source and field provenance, deterministic extension JSON, and graph-level digest verification.

`resourcehub.Manager.CanonicalSnapshot` projects the current version-1 Resource Hub registry into this envelope. The projection is read-only and available through `agentstack hub graph`. Resource Hub remains the sole authority for resource payloads, metadata, reviewed plans, backups, managed ownership, and target mutation.

### Content-addressed shadow store and ASM v1 migration stage

`internal/cas` stores immutable raw blobs and deterministic flat tree manifests beneath `objects/sha256`. Object installation is atomic, no-replace, and deduplicated: complete same-directory temporary files are published with hard links, so a concurrent writer cannot be overwritten. Reads reject symlink objects and symlink object directories, verify raw content digests, validate sorted and path-confined tree manifests, recursively verify referenced blobs, and expose a deterministic reachability mark set without deleting data. Materialization is permitted only to a new destination. Blob destinations use atomic no-replace file publication; tree destinations and every contained entry use exclusive creation and retain an incomplete marker on failure. This avoids Unix rename replacement races and never merges with or deletes concurrently introduced content.

`internal/migrations/asmv1` is a reversible shadow stage, not an authority transfer. It maps every current Resource Hub payload to a CAS tree, reconstructs it in an isolated directory, compares the result with the legacy Resource Hub digest, produces a CAS-backed canonical graph, and seals the source graph digest, staged graph, and object map into a migration receipt. `agentstack hub cas-stage`, `cas-verify`, and `cas-restore` expose this boundary. A changed source graph makes the receipt stale. No CAS command rewrites `registry.json`, `resources/`, target files, plans, managed state, or backups.

### SQLite shadow metadata

`internal/store/sqlite` transactionally indexes sealed ASM v1 migration receipts, canonical artifacts, resource-to-CAS records, and an explicitly non-authoritative Resource Hub shadow head. The schema uses WAL, full synchronous durability, foreign keys, an application ID, an explicit migration ledger, strict tables, and bounded canonical JSON rows. Every inspection runs SQLite quick and foreign-key checks, verifies the sealed receipt, and compares all artifact and resource rows byte-for-byte with that receipt.

`agentstack hub db-stage` is the only metadata write entry point and runs under the shared Fabric lease. It first stages and verifies CAS, commits one metadata transaction, then re-verifies SQLite, CAS, and current Resource Hub identity. `db-inspect` and `db-verify` are read-only. `db-backup` uses SQLite online backup into an incomplete temporary file, verifies the copy, and publishes it without replacement. Resource Hub v1 remains the sole resource metadata and payload authority; SQLite is disposable and rebuildable shadow evidence.

The native backend is selected only for CGO builds with SQLite 3.37 or newer. CGO-disabled builds compile a fail-closed unavailable stub, preserving the rest of ASM without silently substituting another persistence format.

### Versioned target adapter plane

`internal/adapters` defines the pure `fabric.asm.dev/adapter/v1alpha1` contract. Adapters declare digest-bound structural capabilities, normalize core-supplied observations, validate imports, render deterministic projections, classify closed plan transitions, and compare postconditions. They cannot write files, execute target CLIs, approve plans, or own persistence.

`internal/adapters/builtin` extracts the existing Codex, Claude, Cursor, AGY/Gemini, OpenCode, GitHub Copilot, and generic target mappings behind that contract. Resource Hub and `mcplink` bind reviewed plans to capability snapshots and machine-readable fidelity/loss reports. Apply re-resolves capabilities and verifies per-operation loss evidence before using the existing mutation and rollback paths. Resource Hub remains authoritative for resource deployment; `mcplink` remains authoritative for MCP registration.

`internal/adapters/conformance` embeds the reviewed `fabric.asm.dev/adapter-conformance/v1alpha1` oracle. It differentially verifies every built-in capability and declared artifact projection, candidate-preserving import, the closed plan matrix, and postconditions, then emits a digest-bound read-only report. The corpus and report are evidence only; they do not load external code or acquire mutation authority.

### Constrained external adapter evidence plane

`internal/adapters/external` implements `fabric.asm.dev/external-adapter/v1alpha1` as a one-request-per-process JSON boundary. Admission requires an absolute regular non-symlink executable, an exact SHA-256 digest, bounded fixed arguments, and a stable opened-file identity. ASM copies and rehashes the bytes into a private session directory before execution, replaces the inherited environment with synthetic home/work paths, enforces deadlines and byte ceilings, and rejects crashes, stderr-on-success, malformed envelopes, identity drift, target-root escape, and divergence from core plan/postcondition semantics.

The host intersects candidate capabilities with the reviewed built-in adapter ceiling and runs the Phase 5 corpus against both reference and candidate implementations. Reports seal executable and argument identities, capability restrictions, complete reference/candidate evidence, and per-case mismatches. No external adapter is registered with Resource Hub or `mcplink`; the command is read-only compatibility evidence. The current process boundary does not provide network/filesystem namespaces, syscall filtering, CPU or memory quotas, Windows Job Objects, signatures, or WASI isolation, so external activation remains prohibited.

### Cross-plane identity

Workspace IDs may associate resource IDs and routine IDs, but each plane validates its own records. No cross-plane pointer grants authority. Plans carry their own identity, digest, expiry, and source-state evidence. Artifact and memory records use content digests. Workspace multi-file state uses a durable snapshot journal and live-reader fence. Routine receipts use independent schema/version identity.

Every durable convergence store crosses one shared admission boundary before decoding: the path must resolve to the expected regular file, the file must remain within a domain-specific byte ceiling, strict JSON must reject duplicate keys and trailing content, and the decoded graph must satisfy identity, digest, path, cardinality, and lifecycle invariants. Future schema versions are rejected rather than interpreted optimistically.

### Migration model

New stores use versioned envelopes. Legacy maps are read through compatibility paths and upgraded on the next write. Unknown future versions fail closed. No donor database, browser cache, Tauri store, or Node/Python runtime became an ASM source of truth.
