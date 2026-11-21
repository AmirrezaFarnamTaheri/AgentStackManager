# AgentStack Full Reliability and Intelligence Upgrade Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Deliver a Windows x64 AgentStack Manager build that elevates reliably, detects and connects supported AI apps, explains installation failures with specific root causes, prevents stale embedded UI assets, and adds a compact health-and-repair experience without reintroducing layout regressions.

**Architecture:** Keep the existing Go service, operation store, Resource Hub, and embedded HTML/CSS/JavaScript architecture. Add a Windows launch-elevation boundary, a target-discovery/connection service around Resource Hub, richer canonical diagnostic fields in `ApplyOutcome`, cache-proof embedded asset delivery, and small UI widgets that consume those backend contracts. No external runtime dependencies are added.

**Tech Stack:** Go 1.23 (language) / Go 1.26.5 release toolchain, Windows syscall bindings through the standard library, embedded HTTP assets, vanilla JavaScript, CSS, Go tests, Playwright-compatible static UI harness.

## Global Constraints

- The Windows app requests administrator privileges at launch and must not create an elevation loop.
- AI app connection state is based on registered/detected target roots, not merely application installation.
- Every failed requested change must expose a sanitized root cause, method, normalized error code when known, and concrete recovery action.
- The UI must never render “Shared root cause above” or “Use the recovery action above.”
- Embedded assets use no-store headers and versioned URLs so an upgraded executable cannot render stale JavaScript or CSS.
- Desktop and mobile result layouts must not collapse text into one-character columns or create horizontal overflow.
- No secrets, command lines, raw stdout/stderr, user paths, or tokens are exposed in the browser result model.
- Existing successful changes remain preserved during partial failure.

---

### Task 1: Cache-proof Embedded UI Delivery

**Files:**
- Modify: `internal/ui/server.go`
- Modify: `internal/ui/server_test.go`
- Modify: `internal/ui/web/index.html`

**Interfaces:**
- Produces: `assetVersion(version, revision string) string`
- Produces: versioned asset URLs and `Cache-Control: no-store, max-age=0` on HTML and assets.

- [x] Write a handler test that fetches HTML and CSS and asserts no-store headers plus a version query on every stylesheet/script URL.
- [x] Run `go test ./internal/ui -run 'Cache|Asset' -count=1` and confirm the new test fails.
- [x] Add a static-asset wrapper and replace `__AGENTSTACK_ASSET_VERSION__` in HTML.
- [x] Run the targeted test and confirm it passes.

### Task 2: Windows Administrator Launch

**Files:**
- Create: `cmd/agentstack/elevation_windows.go`
- Create: `cmd/agentstack/elevation_other.go`
- Modify: `cmd/agentstack/main.go`
- Modify: `cmd/agentstack/main_test.go`

**Interfaces:**
- Produces: `ensureElevated(args []string) (relaunched bool, err error)`
- Uses a private `--agentstack-elevated` marker to prevent relaunch loops.

- [x] Write tests for marker stripping, already-elevated behavior, and relaunch failure propagation through injected launch dependencies.
- [x] Run `go test ./cmd/agentstack -run Elevat -count=1` and confirm failure.
- [x] Implement Windows token-membership detection and `ShellExecuteW(..., "runas", ...)` relaunch; non-Windows returns no-op.
- [x] Invoke elevation before service initialization for UI/default launch while preserving CLI help/version behavior.
- [x] Run targeted tests and confirm pass.

### Task 3: AI App Discovery and Connection

**Files:**
- Create: `internal/ui/target_discovery.go`
- Create: `internal/ui/target_discovery_test.go`
- Modify: `internal/ui/server.go`
- Modify: `internal/ui/server_test.go`
- Modify: `internal/ui/environments.go`
- Modify: `internal/ui/environments_test.go`
- Modify: `internal/ui/web/index.html`
- Modify: `internal/ui/web/environments.js`
- Modify: `internal/ui/web/styles.css`

**Interfaces:**
- Produces: `TargetCandidate { Agent, ID, Name, Root, Detected, Registered, Enabled, Message }`
- Produces backend interface `RegisterResourceTarget(resourcehub.Target) error`.
- Adds `GET /api/environment-targets` and `POST /api/environment-targets/connect`.

- [x] Write discovery tests using a temporary home with Codex, Claude, Cursor, OpenCode, Copilot, and AGY marker directories.
- [x] Write server tests proving a detected app can be connected and immediately appears as `connected`.
- [x] Run targeted UI tests and confirm failure.
- [x] Implement safe target-root discovery using explicit home/project roots and regular-directory validation.
- [x] Implement registration endpoint with strict agent allowlist, absolute-root validation, and `ModeCopy` default.
- [x] Add Connect/Reconnect/Pause controls and a compact connection-status widget in the environment detail panel.
- [x] Run targeted tests and confirm pass.

### Task 4: Root-Cause Intelligence and Repair Guidance

**Files:**
- Modify: `internal/ui/apply_outcome.go`
- Modify: `internal/ui/apply_outcome_test.go`
- Modify: `internal/ui/web/activity.js`
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Extends `ApplyDiagnostic` with `Cause`, `Evidence`, `Severity`, and `RepairHint` sanitized fields.
- Extends `ApplyCauseGroup` with `Title` and `AffectedLabels`.
- Produces `classifyApplyFailure(raw, method string, exitCode int) failureClassification`.

- [x] Add failing tests for WinGet `0x8A150017`, `0x8A150014`, access denied, installer unavailable, verification failed with exit code 0, dependency failure, timeout, network failure, and an unknown non-empty error.
- [x] Run `go test ./internal/ui -run ApplyOutcome -count=1` and confirm failures.
- [x] Replace tuple classification with a typed classification and ensure unknown failures still include a sanitized evidence phrase rather than generic copy.
- [x] Render direct reason and recovery per row; root-cause groups list affected tools and one repair action.
- [x] Ensure in-progress rows never expose final retry guidance or expandable technical details.
- [x] Run targeted tests and contract tests.

### Task 5: Health, Repair, and History Widgets

**Files:**
- Modify: `internal/ui/environments.go`
- Modify: `internal/ui/web/index.html`
- Modify: `internal/ui/web/environments.js`
- Modify: `internal/ui/web/activity.js`
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Adds environment health fields: `healthScore`, `issueCount`, `recommendedAction`.
- Adds client-side widgets: connection health summary, repair queue summary, searchable history.

- [x] Write contract tests for health score, repair queue, history search, empty states, and accessible status text.
- [x] Run contract tests and confirm failure.
- [x] Calculate deterministic health scores from connected, installed, attention, and unavailable states.
- [x] Add compact widgets with no nested scrolling and no dashboard-wall duplication.
- [x] Add history text search and status filtering without changing stored transaction data.
- [x] Run UI contract tests.

### Task 6: Layout Hardening and Delivery

**Files:**
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web_contract_test.go`
- Modify: `CHANGELOG.md`
- Create: `docs/audit/2026-08-06-full-upgrade-verification.md`

**Interfaces:**
- Produces Windows x64 executable and checksums under `/mnt/data`.

- [x] Add regression contracts prohibiting one-character recovery columns, fixed-width detail expansion, and stale generic root-cause phrases.
- [x] Run `go test ./...`, `go test -race ./...`, and `go vet ./...`.
- [x] Run JavaScript syntax checks for all embedded scripts.
- [x] Build Windows x64 twice with identical flags and compare SHA-256 hashes.
- [x] Package final source, executable, checksums, and verification report into one delivery ZIP.
