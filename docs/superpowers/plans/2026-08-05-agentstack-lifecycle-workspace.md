# AgentStack Lifecycle Workspace Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the overlapping manager UI with a four-destination lifecycle workspace, fix consumed-plan recovery, and add authenticated installation progress, environment inventory, transaction tracking, sharing, and sync visibility.

**Architecture:** Preserve the Go backend, single-use reviewed-plan boundary, operation polling transport, and dependency-free embedded web runtime. Extend the operation receipt with structured progress and client-safe failures, add a read-only environment aggregation service, and split the browser code by lifecycle responsibility under one `window.AgentStack` namespace.

**Tech Stack:** Go 1.26.5 release contract, Go standard library HTTP/embed APIs, existing state/runner/planner/resourcehub/mcplink packages, vanilla HTML/CSS/JavaScript, Playwright and axe in the existing UI test harness.

## Global Constraints

- No frontend framework, bundler, remote asset, web font, or new browser runtime dependency.
- Browser code remains intent-only; Resource Hub and `mcplink` retain exclusive mutation authority.
- Reviewed plans are consumed before external mutation and can never be replayed.
- Failed or partial work requires a newly inventoried and reviewed plan.
- Loopback bearer-token authentication and request headers remain unchanged.
- User-facing failures must not contain filesystem paths, command arguments, stderr, credentials, tokens, or raw process output.
- Primary navigation is exactly Home, Environments, Changes, and Activity.
- Minimum UI text is 12px; body and controls are at least 14px; mobile controls are at least 44px.
- Existing native, CGO-disabled, race, coverage, benchmark, governance, documentation, source-manifest, and cross-platform build gates remain mandatory.
- Do not commit automatically.

---

## File Structure

### Backend

- `internal/ui/failures.go` — maps internal errors to stable client-safe codes, messages, and recovery instructions.
- `internal/ui/operations.go` — owns operation lifecycle, progress snapshots, and client-safe failures.
- `internal/ui/environments.go` — aggregates read-only environment and connection state.
- `internal/ui/server.go` — exposes `/api/environments`, enhanced `/api/operations/{id}`, and progress-aware apply.
- `internal/app/progress.go` — defines apply progress types and maps plans/transactions into item progress.
- `internal/app/service.go` — adds `ApplyPlannedWithProgress`; retains `ApplyPlanned` as compatibility wrapper.
- `internal/reviewedplan/executor.go` — emits plan-loaded and transaction progress callbacks while preserving single-use consumption.
- `internal/runner/runner.go` — emits action-start callbacks before external execution.
- `internal/state/store.go` — lists minimized transaction history for Activity.

### Frontend

- `internal/ui/web/index.html` — four-destination lifecycle structure and accessible tracker markup.
- `internal/ui/web/styles.css` — distilled workspace layout and responsive states.
- `internal/ui/web/core.js` — shared state, API, navigation, error recovery, and operation polling.
- `internal/ui/web/changes.js` — plan preparation, inline review, apply, progress binding, and fresh-plan recovery.
- `internal/ui/web/environments.js` — environment inventory, resource details, connections, sharing, and sync-plan links.
- `internal/ui/web/activity.js` — active tracker, history, diagnostics, routines, and technical details.
- `internal/ui/web/app.js` — bootstrap only.

### Tests and Docs

- `internal/ui/failures_test.go`
- `internal/ui/environments_test.go`
- `internal/ui/operations_test.go`
- `internal/ui/server_test.go`
- `internal/ui/web_contract_test.go`
- `internal/app/service_test.go`
- `internal/reviewedplan/executor_test.go`
- `internal/runner/runner_test.go`
- `internal/state/store_test.go`
- `ui-tests/accessibility.mjs`
- `docs/USER_GUIDE.md`
- `docs/UX_DESIGN.md`
- `docs/UI_LIFECYCLE_WORKSPACE.md`
- `README.md`
- `CHANGELOG.md`
- `SOURCE_PROVENANCE.json`
- `SOURCE_MANIFEST.sha256`

---

### Task 1: Lock consumed-plan recovery and safe failure contracts

**Files:**
- Create: `internal/ui/failures.go`
- Create: `internal/ui/failures_test.go`
- Modify: `internal/reviewedplan/executor.go`
- Test: `internal/reviewedplan/executor_test.go`

**Interfaces:**
- Produces: `reviewedplan.ErrPlanUnavailable`.
- Produces: `ui.ClientFailure{Code, Message, Recovery string; Retryable bool}`.
- Produces: `ui.clientFailureFor(error) ClientFailure`.

- [ ] **Step 1: Write failing reviewed-plan tests**

Add a test that deletes the saved plan before execution and asserts:

```go
_, err := executor.Execute(ctx, Request{PlanID: plan.ID, Digest: plan.Digest, Confirmed: true})
if !errors.Is(err, ErrPlanUnavailable) {
    t.Fatalf("Execute() error = %v", err)
}
if strings.Contains(err.Error(), store.Root) {
    t.Fatalf("private store path leaked: %v", err)
}
```

- [ ] **Step 2: Write failing client-failure mapping tests**

Assert exact mappings:

```go
failure := clientFailureFor(reviewedplan.ErrPlanUnavailable)
if failure.Code != "plan_unavailable" || failure.Message != "This review is no longer available." {
    t.Fatalf("failure = %#v", failure)
}
if strings.Contains(failure.Message+failure.Recovery, `C:\`) {
    t.Fatal("client copy leaked a path")
}
```

Also cover `ErrPlanStale`, `ErrPlanMismatch`, confirmation-required, transaction failure, and an unknown internal error.

- [ ] **Step 3: Run the focused tests and verify they fail**

Run:

```bash
go test ./internal/reviewedplan ./internal/ui -run 'TestExecutorMissingPlan|TestClientFailure'
```

Expected: FAIL because `ErrPlanUnavailable` and `ClientFailure` do not exist.

- [ ] **Step 4: Implement the error boundary**

In `reviewedplan.Executor.Execute`, map `os.ErrNotExist` from `LoadPlan` to:

```go
var ErrPlanUnavailable = errors.New("reviewed plan is unavailable; create and review a fresh plan")
```

Return the sentinel without embedding the storage path. Implement `clientFailureFor` with stable, plain-language copy. Unknown errors return code `operation_failed`, message `AgentStack could not complete this operation.`, recovery `Review the technical details, refresh the system state, and try again.` and `Retryable: true`.

- [ ] **Step 5: Run focused tests**

```bash
go test ./internal/reviewedplan ./internal/ui -run 'TestExecutorMissingPlan|TestClientFailure'
```

Expected: PASS.

---

### Task 2: Add operation progress and failure envelopes

**Files:**
- Modify: `internal/ui/operations.go`
- Create: `internal/ui/operations_test.go`
- Modify: `internal/ui/server.go`
- Test: `internal/ui/server_test.go`

**Interfaces:**
- Produces: `OperationProgress`, `OperationProgressItem`, `ClientFailure` JSON fields on operation status.
- Produces: `type ProgressReporter func(OperationProgress)`.
- Changes: `operationStore.start(ctx, kind, prefix, work func(context.Context, ProgressReporter) (any, error))`.

- [ ] **Step 1: Write failing operation-store progress tests**

Start a blocked operation, report:

```go
report(OperationProgress{
    Phase: "installing", Completed: 1, Total: 3,
    CurrentID: "node", CurrentLabel: "Node.js",
})
```

Poll `get(id)` and assert the snapshot is visible while status remains `running`. Add a race-safe test that reports 100 updates while polling concurrently.

- [ ] **Step 2: Write failing failure-envelope server test**

Make a fake backend return `reviewedplan.ErrPlanUnavailable`. Poll the accepted operation and assert:

```json
{
  "status": "failed",
  "failure": {
    "code": "plan_unavailable",
    "message": "This review is no longer available.",
    "recovery": "Create a fresh plan from the current system state.",
    "retryable": true
  }
}
```

Assert the JSON does not contain the temporary store root or `plans\\`.

- [ ] **Step 3: Run focused tests and verify failure**

```bash
go test ./internal/ui -run 'TestOperationStoreProgress|TestApplyOperationReturnsClientFailure'
```

Expected: FAIL because progress and failure fields are absent.

- [ ] **Step 4: Implement progress snapshots and safe failure storage**

Add:

```go
type operationStatus struct {
    OperationID string             `json:"operationId"`
    Kind        string             `json:"kind"`
    Status      string             `json:"status"`
    Progress    *OperationProgress `json:"progress,omitempty"`
    Result      any                `json:"result,omitempty"`
    Failure     *ClientFailure     `json:"failure,omitempty"`
    StartedAt   time.Time          `json:"startedAt"`
    FinishedAt  time.Time          `json:"finishedAt,omitempty"`
}
```

The reporter deep-copies item slices under the store mutex. When work fails, store `clientFailureFor(err)` instead of raw error text. Preserve `Result` so partial transaction evidence remains available.

- [ ] **Step 5: Update all operation start call sites**

Non-progress operations ignore the reporter. Apply passes it to the progress-aware backend in Task 4.

- [ ] **Step 6: Run focused and race tests**

```bash
go test ./internal/ui -run 'TestOperationStoreProgress|TestApplyOperationReturnsClientFailure'
go test -race ./internal/ui -run TestOperationStoreProgress
```

Expected: PASS.

---

### Task 3: Emit action-level progress from runner and reviewed-plan execution

**Files:**
- Modify: `internal/runner/runner.go`
- Test: `internal/runner/runner_test.go`
- Modify: `internal/reviewedplan/executor.go`
- Test: `internal/reviewedplan/executor_test.go`

**Interfaces:**
- Adds: `runner.ApplyOptions.OnActionStart func(model.PlanAction) error`.
- Adds: `reviewedplan.Executor.OnPlanReady func(model.Plan)`.
- Adds: `reviewedplan.Executor.OnActionStart func(model.PlanAction)`.
- Adds: `reviewedplan.Executor.OnTransaction func(model.Transaction)`.

- [ ] **Step 1: Write failing runner callback tests**

Use a two-action plan and assert callback order:

```go
want := []string{"start:tool-a", "done:tool-a", "start:tool-b", "done:tool-b"}
```

The start callback must happen before `CommandRunner.Run`. A callback error must fail the transaction before the command executes.

- [ ] **Step 2: Write failing executor callback tests**

Assert `OnPlanReady` fires after validation and before plan deletion, `OnActionStart` sees each installer action, and `OnTransaction` receives persisted checkpoints.

- [ ] **Step 3: Run focused tests and verify failure**

```bash
go test ./internal/runner ./internal/reviewedplan -run 'TestApplyActionStart|TestExecutorProgressCallbacks'
```

- [ ] **Step 4: Implement callbacks without changing journal semantics**

Call `OnActionStart` immediately before external execution. Do not append an incomplete action to the transaction. Continue using `OnUpdate` only for durable transaction checkpoints. In the executor, invoke callbacks around the existing runner hooks; callback failures fail closed.

- [ ] **Step 5: Run focused tests**

```bash
go test ./internal/runner ./internal/reviewedplan -run 'TestApplyActionStart|TestExecutorProgressCallbacks'
```

Expected: PASS.

---

### Task 4: Add progress-aware apply service and partial-result summaries

**Files:**
- Create: `internal/app/progress.go`
- Modify: `internal/app/service.go`
- Test: `internal/app/service_test.go`
- Modify: `internal/ui/server.go`
- Test: `internal/ui/server_test.go`

**Interfaces:**
- Produces: `app.ApplyProgress` and `app.ApplyProgressItem`.
- Produces: `func (s *Service) ApplyPlannedWithProgress(ctx context.Context, planID, digest string, confirmed bool, onProgress func(ApplyProgress)) (ApplyReport, error)`.
- Preserves: existing `ApplyPlanned` delegates to the new method with `nil` callback.
- UI optional interface:

```go
type progressApplyBackend interface {
    ApplyPlannedWithProgress(context.Context, string, string, bool, func(app.ApplyProgress)) (app.ApplyReport, error)
}
```

- [ ] **Step 1: Write failing service progress tests**

For two installer actions and one no-op action, assert progress phases and counts:

```text
preparing 0/2
installing current=tool-a 0/2
installing 1/2
installing current=tool-b 1/2
verifying 2/2
complete 2/2
```

For partial failure, assert successful and failed item states remain in the final progress payload and `ApplyReport.Transaction` is returned with the error.

- [ ] **Step 2: Write failing server translation test**

Use a progress-capable fake backend. Poll during apply and assert app progress is translated to `OperationProgress` with item labels and stable status values.

- [ ] **Step 3: Run focused tests and verify failure**

```bash
go test ./internal/app ./internal/ui -run 'TestApplyPlannedWithProgress|TestApplyEndpointPublishesProgress'
```

- [ ] **Step 4: Implement `ApplyPlannedWithProgress`**

Build initial items from selected install, repair, and configure actions. Emit phase changes at validation, installer action start, transaction checkpoints, router configuration, verification, and completion. Never emit raw commands, arguments, output, or paths.

- [ ] **Step 5: Wire apply endpoint progress**

If the backend implements `progressApplyBackend`, pass progress to the operation reporter. Otherwise preserve the compatibility path used by simple tests and alternate backends.

- [ ] **Step 6: Run focused tests**

```bash
go test ./internal/app ./internal/ui -run 'TestApplyPlannedWithProgress|TestApplyEndpointPublishesProgress'
```

Expected: PASS.

---

### Task 5: Add minimized transaction history and environment overview

**Files:**
- Modify: `internal/state/store.go`
- Test: `internal/state/store_test.go`
- Create: `internal/ui/environments.go`
- Create: `internal/ui/environments_test.go`
- Modify: `internal/ui/server.go`
- Test: `internal/ui/server_test.go`

**Interfaces:**
- Produces: `state.Store.ListTransactions(limit int) ([]model.Transaction, error)` returning minimized records sorted newest-first.
- Produces: `EnvironmentOverview`, `Environment`, `EnvironmentResource`, and `ConnectionState`.
- Adds optional backend interfaces for transaction history, workspace/resource summaries, and MCP client status.
- Adds authenticated read-only endpoints `GET /api/environments` and `GET /api/transactions?limit=20`.

- [ ] **Step 1: Write failing transaction-list tests**

Persist three transactions with different timestamps. Assert newest-first ordering, limit enforcement, invalid JSON fail-closed behavior, and that returned actions contain no args or output.

- [ ] **Step 2: Write failing environment aggregation tests**

Use a catalog and inventory containing installed Node, available Git, a routed MCP server, and unsupported Windows-only Winget on Linux. Assert states `installed`, `available`, `needs-attention`, `not-detected`, and `not-supported`. Assert resource lists group components under their environment.

- [ ] **Step 3: Write failing endpoint tests**

Assert both endpoints require the bearer token, accept only GET, and return stable JSON without exposing credentials or raw configuration file contents.

- [ ] **Step 4: Run focused tests and verify failure**

```bash
go test ./internal/state ./internal/ui -run 'TestListTransactions|TestEnvironmentOverview|TestEnvironmentEndpoints'
```

- [ ] **Step 5: Implement transaction listing**

Read only valid `.json` files from the transactions directory, decode strict records, sort by `StartedAt` descending, clamp limit to `1..100`, and return the already minimized on-disk data.

- [ ] **Step 6: Implement environment aggregation**

Normalize existing catalog/inventory data into AI app, IDE, CLI, MCP, and workspace groups. Known client targets are Codex, Claude, Cursor, AGY, OpenCode, and GitHub Copilot. Connection state is observational and must not infer a successful link without existing adapter or client evidence.

- [ ] **Step 7: Run focused tests**

```bash
go test ./internal/state ./internal/ui -run 'TestListTransactions|TestEnvironmentOverview|TestEnvironmentEndpoints'
```

Expected: PASS.

---

### Task 6: Replace the HTML information architecture

**Files:**
- Modify: `internal/ui/web/index.html`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Produces exactly four primary section IDs: `home`, `environments`, `changes`, `activity`.
- Preserves required field IDs used by backend contracts where practical; renamed controls receive compatibility aliases only during the JS transition.

- [ ] **Step 1: Replace old contract assertions with failing lifecycle assertions**

Require:

```go
for _, id := range []string{"home", "environments", "changes", "activity"} {
    requireFragment(t, html, `data-section="`+id+`"`)
}
```

Reject primary navigation labels `Fabric`, `MCP testing`, `Routines`, `Workspaces`, `Sharing`, `Diagnostics`, `Setup`, `Tools`, `Review`, and `Operate`. Reject user-facing terms `sealed plan`, `responsibility plane`, `record seal`, and `inventory-bound` case-insensitively.

Require tracker IDs `installProgress`, `installStage`, `installCount`, `installItems`, and recovery actions `reviewFailedBtn`, `createFreshPlanBtn`, `technicalDetails`.

- [ ] **Step 2: Run contract test and verify failure**

```bash
go test ./internal/ui -run TestEmbeddedUILifecycleWorkspaceContract
```

- [ ] **Step 3: Rewrite the document structure**

Build:

- Header: product title, concise system state, Refresh.
- Rail: Home, Environments, Changes, Activity.
- Home: readiness summary, next action, active tracker slot, recent activity.
- Environments: visible categories and environment detail region.
- Changes: profile/tool inputs, inline pending summary, exact change list, confirmation, apply.
- Activity: current/recent transactions, routines, diagnostics, technical details.

Use one application settings popover. Remove duplicate banners, output panels, metrics walls, and nested card shells.

- [ ] **Step 4: Run contract test**

```bash
go test ./internal/ui -run TestEmbeddedUILifecycleWorkspaceContract
```

Expected: PASS.

---

### Task 7: Split browser code and implement lifecycle state

**Files:**
- Create: `internal/ui/web/core.js`
- Create: `internal/ui/web/changes.js`
- Create: `internal/ui/web/environments.js`
- Create: `internal/ui/web/activity.js`
- Replace: `internal/ui/web/app.js`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Produces: `window.AgentStack = {state, api, ui, operations, changes, environments, activity}`.
- `core.js` exports `api`, `navigate`, `runOperation`, `renderFailure`, `formatElapsed`.
- `changes.js` exports `buildPlan`, `applyPlan`, `createFreshPlan`, `renderPlan`.
- `environments.js` exports `load`, `render`, `selectEnvironment`.
- `activity.js` exports `loadTransactions`, `renderProgress`, `renderFailureSummary`, `loadDiagnostics`.
- `app.js` calls module initializers in deterministic order.

- [ ] **Step 1: Add failing static module contract tests**

Assert all five scripts are embedded in order, `app.js` is under 160 lines, and banned duplicate implementations such as a second `api` or `runOperation` are absent.

- [ ] **Step 2: Run the contract and verify failure**

```bash
go test ./internal/ui -run TestEmbeddedUIBrowserModuleContract
```

- [ ] **Step 3: Extract shared runtime into `core.js`**

Move token/base handling, API fetch, authenticated polling, navigation, theme, focus, toast, operation locking, and safe failure rendering. `waitForOperation` must forward every progress snapshot and throw an error whose `data` contains the full final operation status.

- [ ] **Step 4: Implement `changes.js`**

Keep selection and plan generation in-place. At apply start:

```js
state.planConsumed = true;
setApplyEnabled(false);
```

On success, clear the plan and refresh. On failure, never re-enable Apply for that plan. Use the operation result report to summarize successes and failures, then offer a fresh-plan action.

- [ ] **Step 5: Implement `environments.js` and `activity.js`**

Render flat categorized rows, environment resources, connection state, progress bar, item tracker, recent transactions, routines, diagnostics, and sanitized technical details. Sharing and sync buttons may only open or create existing reviewed plans; they cannot call mutation endpoints directly.

- [ ] **Step 6: Replace `app.js` with bootstrap**

Initialize theme, menus, navigation, module events, and initial data load. Do not retain duplicate render paths.

- [ ] **Step 7: Run syntax and contract tests**

```bash
node --check internal/ui/web/core.js
node --check internal/ui/web/changes.js
node --check internal/ui/web/environments.js
node --check internal/ui/web/activity.js
node --check internal/ui/web/app.js
go test ./internal/ui -run 'TestEmbeddedUIBrowserModuleContract|TestEmbeddedUILifecycleWorkspaceContract'
```

Expected: PASS.

---

### Task 8: Distill the visual system around progress and environment rows

**Files:**
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Produces responsive layout for `.workspace-rail`, `.environment-list`, `.change-workspace`, `.install-tracker`, `.transaction-list`, and `.technical-details`.

- [ ] **Step 1: Add failing CSS contract assertions**

Require explicit styles for `progress`, `[aria-current="page"]`, environment state badges, item tracker states, mobile 44px controls, reduced motion, dark theme, and focus-visible. Reject selectors for removed primary sections and duplicate legacy card systems.

- [ ] **Step 2: Run contract test and verify failure**

```bash
go test ./internal/ui -run TestEmbeddedUILifecycleStylesContract
```

- [ ] **Step 3: Rewrite layout and components**

Use the existing Conservation Ledger tokens. Replace nested panels with flat divided lists. Keep one raised work surface per destination. Make the active tracker persistent but compact. Use a native `<progress>` element with visible text and per-item status rows.

- [ ] **Step 4: Implement responsive topology**

At mobile width, convert the rail to a four-item bottom or top navigation, stack Changes controls before the review list, preserve 44px targets, and avoid horizontally scrolling tables by switching to labeled rows.

- [ ] **Step 5: Run contract tests**

```bash
go test ./internal/ui -run TestEmbeddedUILifecycleStylesContract
```

Expected: PASS.

---

### Task 9: Add browser behavior, accessibility, and recovery tests

**Files:**
- Modify: `ui-tests/accessibility.mjs`
- Modify: `ui-tests/package.json` only if a new existing-script alias is needed; add no dependency.

**Interfaces:**
- Uses current Playwright and axe harness.

- [ ] **Step 1: Add mocked API fixtures**

Provide catalog, inventory, environments, transaction history, plan, running operation progress, succeeded operation, partial failure operation, routines, and diagnostics responses. The failed operation must include a report with three succeeded and two failed items plus `failure.code = "installation_failed"`.

- [ ] **Step 2: Add lifecycle flow tests**

Verify:

1. Initial page shows four destinations and one next action.
2. Environments lists AI apps, IDEs, CLI runtimes, MCP servers, and their resources.
3. Selection updates pending changes inline without navigation.
4. Apply disables immediately and renders stage/count/item progress.
5. Partial failure shows `3 completed, 2 need attention`, never shows a local path, and never re-enables the consumed plan.
6. Create fresh plan refreshes inventory and returns to Changes.
7. Sharing/sync state is visible without executing mutation.
8. Activity shows the failed transaction and technical details only after disclosure.

- [ ] **Step 3: Add accessibility and responsive assertions**

Run axe at 1440×1000 and 390×844. Assert no horizontal overflow, minimum 12px text, minimum 44px interactive targets on mobile, focus preservation, atomic live regions, keyboard settings dismissal, and progress semantics.

- [ ] **Step 4: Run browser suite**

```bash
cd ui-tests
npm test
```

Expected: all lifecycle, accessibility, desktop, and mobile assertions pass with no console errors.

---

### Task 10: Update user documentation and source evidence

**Files:**
- Create: `docs/UI_LIFECYCLE_WORKSPACE.md`
- Modify: `README.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `docs/UX_DESIGN.md`
- Modify: `CHANGELOG.md`
- Modify: `DELIVERY_STATUS.md`
- Modify: `SOURCE_PROVENANCE.json`
- Modify: `SOURCE_MANIFEST.sha256`

**Interfaces:**
- Documents the four-destination lifecycle, single-use recovery, progress tracker, environment states, sharing/sync authority, and remaining release limitations.

- [ ] **Step 1: Write documentation contract text**

Document exact user flows and state terms. Include the invariant: “A failed or partial apply keeps successful changes, consumes the prior approval, and requires a fresh plan for remaining work.”

- [ ] **Step 2: Remove obsolete UI language**

Search documentation for the former primary navigation and revise it where it describes the live UI. Keep protocol terms only in architecture/protocol documentation.

- [ ] **Step 3: Run docs and governance checks**

```bash
bash scripts/check-docs.sh
bash scripts/check-governance.sh
```

Expected: PASS.

- [ ] **Step 4: Regenerate source manifest**

```bash
bash scripts/write-source-manifest.sh
bash scripts/verify-source-manifest.sh
```

Expected: every source file is listed exactly once and verifies.

---

### Task 11: Full project verification and release packaging

**Files:**
- Create final source ZIP, upgrade patch, platform convenience packages, SBOM, receipt, checksums, and implementation report under `/mnt/data`.

**Interfaces:**
- Produces a new lifecycle-workspace release label without claiming signing, attestation, deployment, or public publication.

- [ ] **Step 1: Run formatting, tests, static analysis, and no-CGO gates**

```bash
gofmt -w internal/app internal/reviewedplan internal/runner internal/state internal/ui
go test ./...
CGO_ENABLED=0 go test ./...
go vet ./...
```

Expected: PASS.

- [ ] **Step 2: Run race, coverage, benchmark, fuzz, docs, and governance gates**

Use repository scripts and bounded package groups where the runner has a global process ceiling:

```bash
go test -race ./internal/app ./internal/reviewedplan ./internal/runner ./internal/state ./internal/ui
bash scripts/check-critical-coverage.sh
bash scripts/check-benchmarks.sh
bash scripts/fuzz.sh
bash scripts/check-docs.sh
bash scripts/check-governance.sh
bash scripts/verify-source-manifest.sh
```

Expected: PASS; any environment timeout is rerun in independently bounded commands and not counted as success until completed.

- [ ] **Step 3: Run browser tests**

```bash
cd ui-tests && npm test
```

Expected: desktop and mobile lifecycle tests pass with no accessibility or console failures.

- [ ] **Step 4: Cross-compile supported targets**

Compile Windows amd64/arm64, macOS amd64/arm64, and Linux arm64 with `CGO_ENABLED=0`; build Linux amd64 native and portable variants. Verify every binary’s embedded release label and source revision.

- [ ] **Step 5: Generate and verify artifacts**

Build:

- `AgentStackManager-lifecycle-final-project.zip`
- `AgentStackManager-lifecycle-final-source.zip`
- `AgentStackManager-due-diligence-to-lifecycle.patch`
- Six platform ZIPs
- CycloneDX SBOM
- JSON receipt
- SHA-256 checksum sets
- Lifecycle implementation report

Independently extract every ZIP, reject traversal/absolute/backslash/NUL/duplicate/case-collision entries, verify CRC and expansion limits, compare the source tree byte-for-byte, apply the patch to a clean due-diligence baseline, and rerun native/no-CGO tests there.

- [ ] **Step 6: Record honest release status**

State explicitly that local binaries are unsigned convenience builds. Do not claim commit, push, tag, signing, attestation, deployment, release upload, or production authority migration.

---

## Self-Review

- **Spec coverage:** Recovery, progress, environment inventory, sharing/sync visibility, transaction history, IA consolidation, jargon cleanup, responsive behavior, accessibility, documentation, verification, and packaging all map to tasks.
- **Placeholder scan:** No TBD, TODO, “similar to”, or unspecified test steps remain.
- **Type consistency:** `ApplyProgress` maps to `OperationProgress`; browser polling consumes `operation.progress`; client failures use `ClientFailure` across store, server, and browser.
- **Scope control:** WASI, remote sync, SSE, pure-Go SQLite, new acquisition, and egress enforcement remain deferred.
