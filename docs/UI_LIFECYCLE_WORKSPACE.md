# Embedded manager lifecycle workspace

## Purpose

The embedded manager is organized around the work an operator is trying to complete, not around AgentStack's internal subsystems. It shows what exists, prepares a reviewed set of changes, applies that set once, and preserves enough progress and history to recover from partial failure without retrying stale authorization.

The browser remains an intent and evidence surface. Resource Hub, `mcplink`, reviewed-plan validation, backup, apply, and postcondition verification remain server-side authorities.

## Five-area model

### Home

Home answers three questions without presenting a dashboard wall:

- What is already ready?
- What needs attention?
- What is the single recommended next action?

It shows a concise environment/tool summary and recent activity. It does not duplicate the full inventory, change list, or transaction log.

### Environments

Environments provides one read-only inventory of AI applications, IDEs, command-line tools, MCP servers, and workspaces. Each environment shows detected state, available resources, version or health evidence where known, and managed connection status.

Sharing and synchronization status is visible here, but connection mutation does not bypass its own reviewed sync plan. The UI does not imply that generic tool installation changes an agent-resource connection.

### Changes

Changes combines selection, preview, review, and authorization in one continuous workspace:

1. Choose a profile and individual tools.
2. See an inline estimate of additions, repairs, and preserved installations.
3. Generate the exact pending-change list.
4. Review every consequential item.
5. Confirm once and apply.

There is no separate Review destination and no confirmation modal. Changing any selection invalidates the current plan. A reviewed plan is single-use.

### Activity

Activity owns operational feedback:

- active installation stage and processed count;
- per-item waiting, running, succeeded, failed, or skipped state;
- terminal transaction history with requested, succeeded, failed, and unchanged counts;
- a compact filterable result table and grouped root-cause recovery;
- maintenance actions and system checks;
- sanitized diagnostics behind an explicit disclosure.

A global notice may summarize the outcome, but it does not duplicate the tracker or expose private paths, commands, arguments, stdout, or stderr.

## Installation lifecycle

The authenticated operation endpoint exposes safe progress receipts. The browser renders the server-reported lifecycle rather than simulating elapsed time:

1. **Preparing** — validates the reviewed change set and current inventory.
2. **Installing** — performs package or tool actions one at a time.
3. **Configuring** — applies AgentStack-owned configuration changes.
4. **Verifying** — checks component postconditions and records the transaction.
5. **Finished** - execution stopped and the terminal outcome is now known. Finished does not mean successful.

Phase and outcome are independent. While work runs, the tracker displays processed and total item counts, overall percentage, the active item, and individual item states. At terminal state, the progress track is replaced by one of `succeeded`, `partially_failed`, `failed`, or `cancelled`, plus requested, processed, succeeded, failed, skipped, and unchanged counts. Verified `keep`, inactive-preservation, and dominated-skip actions contribute only to unchanged; they are not counted as requested successes.

## Partial-failure recovery

A reviewed plan is consumed before the first mutation. The client marks it unavailable as soon as apply begins and never retries it.

When some actions fail:

- requested changes confirmed as succeeded remain recorded and are not rolled back merely to hide a partial result;
- existing verified items are described as unchanged rather than successful changes;
- the operation retains the public report, canonical outcome, and progress receipt;
- identical failures are grouped into a root-cause summary instead of repeated as twenty unrelated errors;
- the UI clears the consumed plan and approval control;
- **Retry failed items** refreshes inventory, selects failed items and their dependencies, and creates a fresh reviewed plan;
- **Review failed items** prepares the same selection without applying it;
- **Create fresh plan** refreshes current state without replaying authorization;
- the public error uses a stable code and recovery message, never the plan-store path.

Missing, expired, consumed, stale, mismatched, and unconfirmed plans have distinct path-free failure envelopes.

## Plain-language vocabulary

Primary UI copy uses terms such as:

- **Pending changes**
- **Create changes**
- **Approve and apply**
- **Needs attention**
- **Create fresh plan**
- **Sharing and sync**
- **Technical details**
- **Not connected**
- **Retry failed items**

Protocol and provenance terms such as sealed plan, record digest, capability plane, and transaction command stay in CLI documentation or technical disclosures when genuinely needed.


## Diagnostic and dependency contract

The browser receives sanitized diagnostics only. Each failed requested item may expose component label, action, result, safe category, installer method, exit code, retryability, and recommended action. Raw commands, arguments, private filesystem paths, stdout, and stderr remain server-side.

Provider and component requirements are resolved before plan creation. Choosing Playwright MCP or Chrome DevTools MCP automatically includes that provider and its recursive dependencies, such as Node.js. Unresolved requirements appear beside Installation preferences and disable **Create changes**; they are not deferred to a technical toast after submission.

Supported environments use contextual lifecycle labels. **Not connected** means the AI app or IDE is supported but no AgentStack-managed resources are linked yet. **Available** is reserved for something the user can add, and **Installed** is reserved for detected software.

## Accessibility and responsive behavior

- The five primary destinations remain visible and text-labelled.
- Every primary interactive target is at least 44px high.
- Explicit text never renders below 12px.
- Status regions use `aria-live` with `aria-atomic="true"`.
- Settings supports initial focus, bounded Tab movement, Escape, click-outside dismissal, and trigger-focus restoration.
- Component filtering preserves the search input and uses a bounded debounce.
- Desktop and compact layouts remain document-confined with no horizontal overflow.
- Light and dark modes use explicit semantic colors.
- Reduced-motion preferences disable nonessential movement.

## Verification

Static contracts in `internal/ui/web_contract_test.go` protect the five-area information architecture, plain-language vocabulary, module split, single authorization path, progress/recovery controls, typography floor, mobile targets, and safe theme tokens.

Go tests cover missing-plan mapping, path-free client failures, progress snapshots, partial-result retention, environment and transaction endpoints, public report sanitization, operation-store races, and callback panics.

`ui-tests/accessibility.mjs` is the protected browser gate. It exercises desktop and mobile navigation, environment inventory, change creation, approval locking, partial-failure recovery, installation progress, focus containment, theme behavior, touch targets, rendered text size, overflow, and axe checks.
