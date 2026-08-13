# Unified Desktop Sync Orchestrator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Deliver AgentStack Manager as a unified Windows desktop application with multi-target connections, canonical sharing/sync inventory, bounded parallel installations, robust evidence-based discovery, deduplication, and structured backend-only diagnostics.

**Architecture:** Preserve the existing Go service as the single source of truth. Add focused orchestration packages for target capabilities, resource inspection, and concurrent scheduling; expose typed UI endpoints; render a new Sharing & Sync workspace; launch the local UI inside a native Windows WebView2 host with a browser fallback only when the runtime is unavailable.

**Tech Stack:** Go 1.23 (language) / Go 1.26.5 release toolchain, embedded HTML/CSS/JavaScript, Windows WebView2/Win32, existing Resource Hub and reviewed-plan infrastructure.

## Global Constraints

- No filesystem mutation before a fresh digest-bound reviewed plan is approved.
- Raw command lines, stdout, stderr, package-manager animation, environment variables, and tokens remain backend-only.
- Unmanaged resources are never silently deleted or overwritten.
- Independent operations may execute concurrently; shared installers, dependency chains, and shared roots are serialized.
- Every applied operation must be cancellable, idempotent, verified, and recorded in a sanitized receipt.
- Known targets and verified writable adapters are separate states.
- The Windows executable must use a GUI subsystem, include branded icon resources, and not open a console window.

---

### Task 1: Evidence-based target catalogue and multi-target discovery

**Files:**
- Modify: `internal/resourcehub/types.go`
- Modify: `internal/ui/environments.go`
- Modify: `internal/ui/target_discovery.go`
- Test: `internal/ui/target_discovery_test.go`
- Test: `internal/ui/environments_test.go`

- [x] Add typed support level, detection state, scope, executable evidence, configuration evidence, and confidence fields.
- [x] Add verified catalog entries plus known/read-only entries for additional apps, IDEs, and CLIs.
- [x] Preserve every registered target instead of collapsing to one target per agent.
- [x] Add red/green tests for multiple profiles, configuration-only detection, and unsupported adapters.

### Task 2: Batch connection management

**Files:**
- Modify: `internal/ui/server.go`
- Modify: `internal/ui/environments.go`
- Modify: `internal/ui/web/environments.js`
- Test: `internal/ui/server_test.go`
- Test: `internal/ui/web_contract_test.go`

- [x] Add a typed batch connect/pause request that prevalidates all targets before mutation.
- [x] Return per-target results and retain independent failures.
- [x] Add multi-select controls and Connect selected / Pause selected actions.
- [x] Verify multiple simultaneous connections remain visible and independently controllable.

### Task 3: Canonical sharing, sync, and deduplication inventory

**Files:**
- Create: `internal/resourcehub/inspect.go`
- Create: `internal/resourcehub/inspect_test.go`
- Modify: `internal/resourcehub/types.go`
- Modify: `internal/ui/server.go`

- [x] Build canonical identities from kind, normalized namespace/name, version, and content digest.
- [x] Report installations separately from resources.
- [x] Classify exact duplicates, equivalent managed copies, version duplicates, collisions, shadowing, drift, orphans, and foreign resources.
- [x] Produce aggregate counts for Already installed, Contained, In sync, Drifted, Duplicates, Conflicts, and Unmanaged.

### Task 4: Reviewed multi-target sync planning and parallel apply

**Files:**
- Create: `internal/resourcehub/batch_sync.go`
- Create: `internal/resourcehub/batch_sync_test.go`
- Modify: `internal/resourcehub/types.go`
- Modify: `internal/ui/server.go`

- [x] Generate one immutable parent plan containing per-target child plans and a parent digest.
- [x] Revalidate the registry, capabilities, roots, and child digests before any mutation.
- [x] Apply independent target plans concurrently with a configurable global limit and per-root lock.
- [x] Preserve completed independent targets when another target fails and return structured receipts.

### Task 5: Bounded parallel installer scheduler

**Files:**
- Modify: `internal/runner/runner.go`
- Create: `internal/runner/parallel_test.go`
- Modify: `internal/app/service.go`

- [x] Add global and per-installer concurrency limits.
- [x] Respect catalog dependencies and serialize skill/configuration mutations.
- [x] Keep transaction action order deterministic while publishing concurrent progress safely.
- [x] Cancel active process trees and stop scheduling new work when the context is cancelled.

### Task 6: Backend-only process evidence and sanitized public receipts

**Files:**
- Modify: `internal/supervisor/runtime.go`
- Create: `internal/supervisor/process_windows.go`
- Create: `internal/supervisor/process_other.go`
- Modify: `internal/ui/public_report.go`
- Modify: `internal/ui/server.go`
- Test: `internal/ui/public_report_test.go`
- Test: `internal/ui/web_contract_test.go`

- [x] Hide child process windows on Windows.
- [x] Retain bounded raw evidence only in local backend state.
- [x] Expose normalized category, error code, observed evidence, root cause, and recovery without raw output.
- [x] Ensure transaction/history endpoints cannot serialize commands, arguments, stdout, stderr, or private paths.

### Task 7: Sharing & Sync workspace widgets

**Files:**
- Modify: `internal/ui/web/index.html`
- Create: `internal/ui/web/sync.js`
- Modify: `internal/ui/web/core.js`
- Modify: `internal/ui/web/app.js`
- Modify: `internal/ui/web/styles.css`
- Test: `internal/ui/web_contract_test.go`

- [x] Add Sharing & Sync primary navigation.
- [x] Render Overview, Managed resources, Installed, Contained, In sync, Drifted, Duplicates, Conflicts, Plans, and History.
- [x] Add target/resource search, state filters, duplicate-group explanations, and reviewed-plan actions.
- [x] Verify desktop, mobile, dark mode, keyboard focus, empty states, and no horizontal overflow.

### Task 8: Native desktop host and branded Windows binary

**Files:**
- Create: `internal/desktop/desktop.go`
- Create: `internal/desktop/desktop_windows.go`
- Create: `internal/desktop/desktop_other.go`
- Modify: `internal/ui/server.go`
- Modify: `internal/cli/cli.go`
- Modify: `cmd/agentstack/icon.rc`
- Modify: `scripts/build.sh`
- Modify: `scripts/build.ps1`
- Test: `internal/desktop/desktop_test.go`
- Test: `cmd/agentstack/main_test.go`

- [x] Launch the local session URL in a dedicated, address-bar-free desktop application window on Windows, with server lifetime owned by that window.
- [x] Use a visible startup error when no supported Windows desktop engine can initialize; ordinary browser mode remains explicit through `ui --browser`.
- [x] Build as a Windows GUI application and embed app icon plus manifest resources.
- [x] Verify closing the native window shuts down the local server and child operations cleanly.

### Task 9: Full verification and delivery

**Files:**
- Modify: `CHANGELOG.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `SOURCE_MANIFEST.sha256`

- [x] Run targeted red/green regression tests.
- [x] Run `go test ./...`, `go test -race ./...`, and `go vet ./...`.
- [x] Run JavaScript syntax and UI contract checks.
- [x] Render desktop and mobile QA screenshots with interaction proof.
- [x] Produce two byte-identical Windows x64 builds, source archive, delivery archive, checksums, and QA evidence.
