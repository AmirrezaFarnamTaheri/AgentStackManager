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
          +--> bounded command runner --> WinGet / npm / uv / Git
          +--> MCP router --> pooled stdio children
          +--> session launcher --> Codex / AGY process tree
```

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
