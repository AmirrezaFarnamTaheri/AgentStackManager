# Operation Recovery Console Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make AgentStack Manager report installation outcomes truthfully, explain shared and per-item failures safely, prevent invalid plans, and provide an accessible compact recovery workflow across Home, Changes, Environments, and Activity.

**Architecture:** Preserve the embedded Go + static HTML/CSS/JavaScript architecture and all existing route/field IDs. Add a typed public apply-outcome contract at the Go boundary, derive canonical counts and sanitized diagnostic groups before data reaches the browser, and render that model through one client-side outcome state. Keep execution progress separate from final outcome, validate dependencies before plan submission, and expose recovery actions without reusing a consumed review.

**Tech Stack:** Go 1.23 (language) / Go 1.26.5 release toolchain, embedded `net/http` UI, vanilla JavaScript, semantic HTML, CSS custom properties, Go tests, static web-contract tests, Playwright/axe browser assurance where the dependency is available.

## Global Constraints

- Preserve loopback-only authenticated API behavior, reviewed-plan identity, single-use approval, and mutation locking.
- Preserve all existing primary route IDs, form IDs, API routes, and the four destinations Home, Environments, Changes, and Activity.
- Never expose filesystem paths, commands, arguments, stdout, stderr, credentials, or secrets to the browser.
- Use one cold-neutral palette, restrained teal for action/selection, and semantic green/amber/red only for real state.
- Use Aptos / Segoe UI Variable / Segoe UI / system UI sans and Cascadia Code / Consolas for technical evidence; no network fonts.
- Keep primary controls at least 44px high, visible focus, atomic live regions, reduced-motion support, and no horizontal overflow at 320px, 390px, 768px, 1024px, and 1440px.
- Do not represent execution completion as successful outcome. `phase` and `outcome` are independent.
- The source export contains no `.git` directory, so verification checkpoints replace commit steps without initializing or mutating repository history.

---

### Task 1: Canonical public apply outcome and diagnostics

**Files:**
- Create: `internal/ui/apply_outcome.go`
- Create: `internal/ui/apply_outcome_test.go`
- Modify: `internal/ui/operations.go`
- Modify: `internal/ui/server.go`
- Modify: `internal/ui/failures.go`
- Modify: `internal/ui/failures_test.go`
- Modify: `internal/ui/server_test.go`

**Interfaces:**
- Consumes: `app.ApplyReport`, `model.Plan`, `model.Transaction`, internal execution error.
- Produces: `ApplyOutcome`, `ApplyDiagnostic`, `ApplyCauseGroup`, and `applyOperationResult` JSON used by the browser.

- [x] **Step 1: Write failing tests for truthful counts and shared-cause classification**

Add tests that construct a plan with 20 install/repair actions plus verified `keep` actions and a failed transaction. Assert:

```go
if outcome.Requested != 20 || outcome.Succeeded != 0 || outcome.Failed != 20 {
    t.Fatalf("outcome = %#v", outcome)
}
if outcome.Unchanged != 12 || outcome.Outcome != "failed" {
    t.Fatalf("outcome = %#v", outcome)
}
if len(outcome.Causes) != 1 || outcome.Causes[0].Count != 20 {
    t.Fatalf("causes = %#v", outcome.Causes)
}
```

Also assert that diagnostics contain action, method, category, exit code, retryability, and a path-free summary, while raw error/path/command output is absent from JSON.

- [x] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/ui -run 'TestBuildApplyOutcome|TestApplyOperationFailure' -count=1 -v
```

Expected: FAIL because the outcome contract and classifiers do not exist.

- [x] **Step 3: Implement the typed outcome contract**

Implement these stable fields:

```go
type ApplyOutcome struct {
    Phase       string            `json:"phase"`
    Outcome     string            `json:"outcome"`
    Requested   int               `json:"requested"`
    Processed   int               `json:"processed"`
    Succeeded   int               `json:"succeeded"`
    Failed      int               `json:"failed"`
    Skipped     int               `json:"skipped"`
    Unchanged   int               `json:"unchanged"`
    Retryable   bool              `json:"retryable"`
    Summary     string            `json:"summary"`
    Detail      string            `json:"detail"`
    Diagnostics []ApplyDiagnostic `json:"diagnostics,omitempty"`
    Causes      []ApplyCauseGroup `json:"causes,omitempty"`
}
```

Classify failures into `installer_unavailable`, `permission_denied`, `dependency_failed`, `verification_failed`, `network_unavailable`, `timeout`, and `installation_failed`. Map install methods to safe labels such as `WinGet`, `npm`, `uv`, `Skill package`, and `AgentStack configuration`. Group causes by category + method + safe summary.

- [x] **Step 4: Return the same result shape for success and failure**

Use:

```go
type applyOperationResult struct {
    Report  app.ApplyReport `json:"report"`
    Outcome ApplyOutcome    `json:"outcome"`
}
```

The apply operation must always store this typed result. On failure, let it provide a count-aware `ClientFailure`: zero succeeded means `No requested changes were applied.`; mixed result means `Some requested changes were applied.` Existing verified items are described as unchanged, not successful changes.

- [x] **Step 5: Extend progress snapshots with result counts**

Add `Processed`, `Succeeded`, `Failed`, and `Skipped` to `OperationProgress`; derive them from item statuses. Keep the existing `Completed` JSON field for compatibility, but define it as processed work and stop using the word “completed” in browser copy.

- [x] **Step 6: Run focused and full Go tests**

Run:

```bash
go test ./internal/ui ./internal/app -count=1
go test ./... -count=1
```

Expected: PASS.

### Task 2: Proactive dependency and provider validation

**Files:**
- Modify: `internal/ui/web/index.html`
- Modify: `internal/ui/web/changes.js`
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web_contract_test.go`
- Modify: `ui-tests/accessibility.mjs`

**Interfaces:**
- Consumes: catalog `dependsOn`, `capability`, `credentialRequired`, current selection, provider override.
- Produces: `validateSelection()`, `ensureRequirements()`, inline issue list, disabled plan submission while unresolved.

- [x] **Step 1: Write failing static/browser assertions**

Require the HTML to contain `selectionIssues`, and JavaScript to contain `validateSelection`, `ensureRequirements`, provider dependency handling, and an inline `role="alert"`/atomic status. Browser test must select a provider, verify required components are included, and verify no raw provider-ID error toast is shown.

- [x] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/ui -run TestWebContract -count=1 -v
```

Expected: FAIL because proactive validation markers are absent.

- [x] **Step 3: Implement recursive requirement selection**

Selecting a component or browser provider recursively selects dependencies. Attempting to deselect a required dependency is rejected with a nearby plain-language message. Provider options explain added requirements using product names, not component IDs.

- [x] **Step 4: Implement form-level validation**

Render unresolved issues directly under Installation preferences, move focus to the issue summary after an invalid submit, and keep `Create changes` disabled until the selection is valid. Do not use a transient toast for form validation.

- [x] **Step 5: Run contract tests**

Run:

```bash
go test ./internal/ui -run TestWebContract -count=1 -v
```

Expected: PASS.

### Task 3: Truthful tracker and compact recovery console

**Files:**
- Modify: `internal/ui/web/index.html`
- Modify: `internal/ui/web/activity.js`
- Modify: `internal/ui/web/core.js`
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web_contract_test.go`
- Modify: `ui-tests/accessibility.mjs`

**Interfaces:**
- Consumes: `operation.progress`, `operation.result.outcome`, `operation.failure`.
- Produces: final outcome header, segmented counts, cause panel, filterable result table, per-item disclosures, retry/review/fresh-plan actions.

- [x] **Step 1: Write failing tests for final failure semantics**

Assert a mocked `0 succeeded / 20 failed / 12 unchanged` operation renders:

- `Run finished`
- `No requested changes were applied`
- `0 succeeded`
- `20 failed`
- `12 unchanged`
- `Retry failed items`

Assert it does not render `Changes complete`, `20 of 20 completed`, a success class, or raw JSON.

- [x] **Step 2: Run focused tests and verify RED**

Run static contract tests. Run browser assurance when Playwright is available. Expected: failure on current wording and tracker structure.

- [x] **Step 3: Separate phase from outcome in rendering**

During execution, show neutral `Processed X of Y`. At terminal state, use outcome copy and semantic class. The progress bar remains neutral while running and receives success/error/partial styling only after outcome is known.

- [x] **Step 4: Replace oversized cards with a compact result table**

Use a semantic table with columns Tool, Action, Result, Cause, and Next action. Add All / Failed / Succeeded / Skipped filters with counts. Rows expand to a structured detail panel containing method, category, exit code when available, safe summary, retryability, and recommendation.

- [x] **Step 5: Add shared root-cause panel**

When multiple items share a cause, show one cause summary and affected count above the table. Do not repeat identical error prose on every row.

- [x] **Step 6: Replace raw JSON technical output**

Keep `Technical details` as a disclosure, but render a bounded sanitized definition list and diagnostic table. No JSON blob, nested scroll trap, private path, command, arguments, stdout, or stderr.

- [x] **Step 7: Implement recovery hierarchy**

Primary: `Retry failed items`. Secondary: `Review failed items`. Quiet action: `Create fresh plan`. All paths refresh inventory and create a new reviewed plan; none retries the consumed plan.

- [x] **Step 8: Run Go and contract tests**

Run:

```bash
go test ./internal/ui -count=1
go test ./... -count=1
```

Expected: PASS.

### Task 4: Canonical Home, header, Activity, and environment terminology

**Files:**
- Modify: `internal/ui/web/app.js`
- Modify: `internal/ui/web/environments.js`
- Modify: `internal/ui/web/activity.js`
- Modify: `internal/ui/web/index.html`
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/environments.go`
- Modify: `internal/ui/environments_test.go`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Consumes: current inventory/environment overview plus `state.lastOutcome`.
- Produces: one `renderOperationalStatus()` source of truth and contextual state labels.

- [x] **Step 1: Write failing tests**

Require Home and header to use last-run outcome counts after a terminal operation. Require supported AI apps/IDEs that are not linked to use state `not-connected`, not `available`. Require Activity transaction summaries to separate requested changes from unchanged `keep` actions.

- [x] **Step 2: Run focused tests and verify RED**

Run:

```bash
go test ./internal/ui -run 'TestBuildEnvironmentOverview|TestWebContract' -count=1 -v
```

Expected: FAIL on `available` environment state and missing canonical outcome rendering.

- [x] **Step 3: Centralize operational status**

Store the latest terminal outcome in `state.lastOutcome`. Render the header, Home summary, Home metric label/value, recommendation, and Activity summary from it. Inventory health remains explicitly labelled `Tool health issues` and is never confused with last-run failures.

- [x] **Step 4: Make Activity a real timeline**

Show start/finish time, duration, requested/succeeded/failed/unchanged counts, outcome, and recovery availability. Do not merely repeat the notice message.

- [x] **Step 5: Normalize environment vocabulary**

Use `Not connected`, `Connected`, `Paused`, `Installed`, `Available to add`, `Needs attention`, and `Not supported here` contextually. Resource counts are labelled as `managed resources`; installed-tool counts are not compared to resource counts without explanation.

- [x] **Step 6: Run focused and full tests**

Run:

```bash
go test ./internal/ui -count=1
go test ./... -count=1
```

Expected: PASS.

### Task 5: Visual hierarchy, density, responsive behavior, and accessibility

**Files:**
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web/accessibility.css`
- Modify: `internal/ui/web/index.html`
- Modify: `ui-tests/accessibility.mjs`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Produces: compact operational layout with a documented 14px panel / 8px control radius system and no decorative side-tab borders.

- [x] **Step 1: Add failing assurance checks**

Require result rows to remain readable at 320px and 390px, no nested horizontal/vertical scroll trap, minimum 44px interactive targets, minimum 12px visible text, visible focus, atomic live regions, reduced motion, and no serious axe violations in light/dark modes.

- [x] **Step 2: Run available checks and verify RED**

Run static contract tests. Attempt Playwright. If the pinned dependency cannot be fetched, record the registry blocker and use Chromium headless screenshot/DOM checks plus Go/static tests.

- [x] **Step 3: Implement the compact visual system**

Reduce oversized row padding, remove redundant state dots, reserve red outlines for the cause summary and failed status text, improve surface hierarchy, avoid nested scroll regions, and keep action proximity to the failure message.

- [x] **Step 4: Implement responsive table topology**

Desktop uses a table. Below 760px, each row becomes a labelled grid/card without horizontal scrolling; controls stack with one-line labels. Filters scroll only when necessary and remain keyboard accessible.

- [x] **Step 5: Audit interaction states**

Verify hover, active, disabled, loading, empty, error, partial, success, focus, dark, and reduced-motion states. Ensure dynamic outcome changes are announced once through an atomic live region.

- [x] **Step 6: Run static and browser checks**

Run:

```bash
go test ./internal/ui -count=1
```

Then run `npm test` in `ui-tests` if dependencies are available; otherwise run the documented Chromium fallback.

### Task 6: Documentation and source contracts

**Files:**
- Modify: `docs/UI_LIFECYCLE_WORKSPACE.md`
- Modify: `docs/OPERATIONS.md`
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `DELIVERY_STATUS.md`
- Modify: `internal/ui/web_contract_test.go`

**Interfaces:**
- Produces: documented outcome taxonomy, recovery behavior, and sanitized diagnostics contract.

- [x] **Step 1: Write failing documentation-contract assertions**

Require documentation to define `queued/running/finished` phase separately from `succeeded/partially-failed/failed/cancelled` outcome and to explain that retry creates a fresh reviewed plan.

- [x] **Step 2: Run documentation/static checks and verify RED**

Run:

```bash
bash scripts/check-docs.sh
go test ./internal/ui -run TestWebContract -count=1
```

Expected: FAIL until docs and contract strings are updated.

- [x] **Step 3: Update documentation**

Document the canonical counts, root-cause grouping, per-item sanitized fields, proactive dependency selection, result-table filters, and Home/Activity behavior. Remove claims that “completed” means success.

- [x] **Step 4: Run docs and full tests**

Run:

```bash
bash scripts/check-docs.sh
go test ./... -count=1
```

Expected: PASS.

### Task 7: Bounded finish QA and deliverable archive

**Files:**
- Modify as required by the first visual QA pass only.
- Generate outside source tree: `/mnt/data/AgentStackManager-lifecycle-workspace-final-fixed.zip`

**Interfaces:**
- Produces: verified source archive and evidence screenshots outside the repository.

- [x] **Step 1: Run the Impeccable detector once**

Run:

```bash
node /home/oai/skills/impeccable-main/.kiro/skills/impeccable/scripts/detect.mjs --json internal/ui/web/index.html internal/ui/web/styles.css internal/ui/web/core.js internal/ui/web/changes.js internal/ui/web/environments.js internal/ui/web/activity.js internal/ui/web/app.js
```

Fix every applicable finding in one batch.

- [x] **Step 2: Build and run the application**

Run:

```bash
go build -o agentstack ./cmd/agentstack
./agentstack ui --no-open --listen 127.0.0.1:0
```

Capture desktop and mobile evidence with the Browser plugin if available, otherwise Chromium headless. Exercise Home, Environments, Changes, provider validation, mocked/fixture failure outcome, recovery action, Activity, light/dark, and reduced motion.

- [x] **Step 3: Make one bounded visual correction pass**

Batch all defects from desktop/mobile screenshots and DOM/console evidence into one edit. Rebuild and confirm once. Do not enter an open-ended polishing loop.

- [x] **Step 4: Run final verification**

Run:

```bash
go test ./... -count=1
bash scripts/check-docs.sh
bash scripts/check-governance.sh
```

Run UI browser assurance when its dependency is available. Record any external registry limitation precisely.

- [x] **Step 5: Create the source archive**

Exclude generated binary and dependency directories:

```bash
rm -f agentstack
zip -qr /mnt/data/AgentStackManager-lifecycle-workspace-final-fixed.zip . -x 'ui-tests/node_modules/*' -x '.git/*'
```

Confirm the archive exists and list its SHA-256 digest.
