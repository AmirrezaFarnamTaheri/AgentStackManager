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

### Cross-plane identity

Workspace IDs may associate resource IDs and routine IDs, but each plane validates its own records. No cross-plane pointer grants authority. Plans carry their own identity, digest, expiry, and source-state evidence. Artifact and memory records use content digests. Workspace multi-file state uses a durable snapshot journal and live-reader fence. Routine receipts use independent schema/version identity.

Every durable convergence store crosses one shared admission boundary before decoding: the path must resolve to the expected regular file, the file must remain within a domain-specific byte ceiling, strict JSON must reject duplicate keys and trailing content, and the decoded graph must satisfy identity, digest, path, cardinality, and lifecycle invariants. Future schema versions are rejected rather than interpreted optimistically.

### Migration model

New stores use versioned envelopes. Legacy maps are read through compatibility paths and upgraded on the next write. Unknown future versions fail closed. No donor database, browser cache, Tauri store, or Node/Python runtime became an ASM source of truth.
