# AgentStack Deep-Module Convergence

**Status:** Implemented and verified in an unreleased remediation workspace.

## Goal

Close the four highest-value architectural seams identified by the forensic reconciliation without weakening preservation, authorization, process, or release guarantees.

## Implemented boundaries

### 1. Closed source-provenance capsule

`internal/releasepack` now owns source metadata validation, exact manifest generation, manifest verification, selected-file deterministic ZIP creation, archive reopening, exact-member comparison, and digest verification. Source packaging never walks arbitrary ignored content into the output; it packages only verified manifest paths plus `SOURCE_MANIFEST.sha256`.

### 2. Reviewed-plan transaction

`internal/reviewedplan.Executor` owns the complete authorization-to-journal sequence. It requires confirmation, acquires and renews the mutation lease, validates digest/expiry/catalog/inventory state, consumes the plan before mutation, persists incremental and final transactions, and records ownership only through an injected post-success hook.

### 3. Managed MCP child runtime

`internal/mcp.ManagedChildRuntime` is the single caller-facing child runtime. `ServerConfig.Persistent` selects pooled or one-shot operation internally. Launch, initialization, bounded protocol framing, idle expiry, invalid-session eviction, observer events, and shutdown remain hidden behind `ChildClient` and `Close`.

The previous monolithic child implementation is split by responsibility:

- `child_client.go`
- `child_session.go`
- `child_pool.go`
- `child_protocol.go`
- `child_buffer.go`
- `child_types.go`

### 4. Supervised process runtime

`internal/supervisor.Runtime` centralizes command construction, environment propagation, deadlines, bounded stdout/stderr capture, exit-code mapping, attached-stream execution, piped execution, graceful close, forced termination, and optional `processctl` resource limits.

The runner, inventory, session, MCP, self-install, and Windows PATH helper paths delegate to this boundary. Browser-opening and OS identity probes remain separate because they are intentionally not managed workload executions.

## Mutation assurance

The default Gremlins matrix contains no blanket path or platform exclusions. Go build constraints naturally limit each host run to active files. A native Windows job generates exact exclusions for all non-`_windows.go` production files, leaving platform-specific implementations exclusively targeted under the same efficacy and mutant-coverage thresholds.

## Compatibility decisions

- Existing public CLI and JSON contracts remain unchanged, except `releasepack` adds explicit source-manifest modes and emits concise success text for the standalone helper.
- Existing `processctl` remains the platform enforcement adapter; the supervisor does not duplicate Job Object or Unix process-group logic.
- Existing plan format, state files, transaction model, router tools, and child wire protocol remain unchanged.
- No lock-free state rewrite, new RPC protocol, memory-mapped storage, HMAC overlay, GUI rewrite, or Wasm runtime was introduced because the source did not justify those migrations.

## Verification requirements

- focused boundary tests;
- all Go tests;
- race detector;
- vet and formatting;
- critical coverage gate;
- governance and documentation contracts;
- Linux native build;
- Windows amd64 and arm64 compile/test builds;
- source manifest rewrite and exact verification;
- source ZIP reopen and member/digest verification.
