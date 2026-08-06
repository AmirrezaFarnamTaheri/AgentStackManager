# AgentStack Lifecycle Workspace Design

## Status

Approved on 2026-08-05.

## Problem

The current browser manager still exposes too many overlapping surfaces and internal terms. Setup, tool selection, review, operation status, fabric, MCP testing, sharing, workspaces, routines, and diagnostics compete for attention. Installation is presented as a single Apply action even though the backend already executes a multi-step transaction. When an apply attempt partially fails, the reviewed plan has correctly been consumed before mutation, but the browser keeps the stale plan active; retrying exposes a raw missing-plan filesystem error.

## Product Goal

Turn the embedded manager into a clear lifecycle workspace where users can:

1. Understand what AgentStack found.
2. See which AI apps, IDEs, CLI runtimes, MCP servers, and managed resources exist in each environment.
3. Prepare and review changes without jumping between disconnected screens.
4. Follow installation progress by stage and item.
5. Recover from partial failure without raw paths or stale-plan retries.
6. Review sharing and synchronization relationships without granting new mutation authority to the UI.

## Information Architecture

The primary navigation contains four destinations:

- **Home** — current readiness, the next recommended action, active installation progress, and concise recent activity.
- **Environments** — AI apps, IDEs, command-line runtimes, MCP servers, workspaces, and the resources available in each. Sharing and sync status are shown here because they describe relationships between environments.
- **Changes** — profile choice, tool selection, pending-change preview, exact review, confirmation, and apply in one continuous workspace.
- **Activity** — current and previous transactions, per-item installation state, diagnostics, routines, and technical details.

No separate Fabric, MCP Test, Workspaces, Sharing, or Diagnostics primary destinations remain. Their capabilities are consolidated into Environments or Activity.

## Copy Rules

User-facing copy uses operational terms:

- `Pending changes`, not `sealed plan`.
- `Create changes`, not `build plan`.
- `Approve and apply`, not `apply reviewed plan`.
- `Connections`, not `responsibility planes` or `fabric`.
- `Technical details`, not raw record seals, digests, or filesystem paths.
- `Installed`, `Not connected`, `Connected`, `Needs attention`, `Available to add`, and `Not supported here` for environment state.

Plan IDs, digests, adapter versions, transaction IDs, and paths remain available only in technical details where safe.

## Failure Recovery

Reviewed plans remain single-use. This security invariant is preserved.

When apply begins, the UI immediately marks the plan as consumed. It never offers a second Apply against the same plan.

A terminal operation returns one structured result that keeps execution phase independent from outcome:

```json
{
  "phase": "finished",
  "outcome": "failed",
  "requested": 20,
  "processed": 20,
  "succeeded": 0,
  "failed": 20,
  "skipped": 0,
  "unchanged": 12,
  "retryable": true,
  "summary": "No requested changes were applied.",
  "detail": "12 existing verified items were left unchanged. Resolve the cause, then retry failed items in a fresh reviewed plan."
}
```

The operation result retains a sanitized transaction report, grouped root causes, and per-item diagnostics. The UI renders counts directly from that canonical result rather than equating a finished phase with success. Interrupted work uses the separate `cancelled` outcome and offers recovery for unfinished items. It offers:

- **Retry failed items**
- **Review failed items**
- **Create fresh plan**
- **Technical details**

A missing, expired, stale, or previously consumed plan returns `plan_unavailable` or `plan_stale`; raw `os.PathError` text never reaches browser copy.

## Progress Model

Apply operations expose structured progress through the existing operation polling endpoint. No WebSocket or unauthenticated EventSource endpoint is introduced.

```go
type OperationProgress struct {
    Phase        string                  `json:"phase"`
    Completed    int                     `json:"completed"`
    Total        int                     `json:"total"`
    CurrentID    string                  `json:"currentId,omitempty"`
    CurrentLabel string                  `json:"currentLabel,omitempty"`
    Items        []OperationProgressItem `json:"items,omitempty"`
}

type OperationProgressItem struct {
    ID      string `json:"id"`
    Label   string `json:"label"`
    Action  string `json:"action"`
    Status  string `json:"status"`
    Message string `json:"message,omitempty"`
}
```

Supported phases are `preparing`, `installing`, `configuring`, `verifying`, and `complete`. Item statuses are `waiting`, `running`, `succeeded`, `failed`, `skipped`, and `not-needed`.

The runner emits an action-start callback before external execution and continues to publish transaction checkpoints after each completed action. The service maps those events into progress without weakening transaction journaling.

## Environment Inventory

A read-only `/api/environments` endpoint returns a normalized view over existing catalog, inventory, adapter, workspace, resource-hub, and MCP-link data.

```go
type EnvironmentOverview struct {
    GeneratedAt  time.Time         `json:"generatedAt"`
    Environments []Environment     `json:"environments"`
    Connections  []ConnectionState `json:"connections"`
}

type Environment struct {
    ID        string                `json:"id"`
    Name      string                `json:"name"`
    Kind      string                `json:"kind"`
    State     string                `json:"state"`
    Version   string                `json:"version,omitempty"`
    Location  string                `json:"location,omitempty"`
    Resources []EnvironmentResource `json:"resources,omitempty"`
    Message   string                `json:"message,omitempty"`
}
```

Kinds are `ai-app`, `ide`, `cli`, `mcp`, and `workspace`. The endpoint is observational only. Sync and link actions continue to use existing reviewed Resource Hub and `mcplink` plans.

## Frontend Structure

The embedded web application remains dependency-free. The 1,100-line `app.js` is split by responsibility:

- `core.js` — API client, shared state, navigation, operation polling, error presentation.
- `changes.js` — selection, plan creation, inline review, apply, and fresh-plan recovery.
- `environments.js` — environment inventory, detail panels, connections, sharing, and sync-plan entry points.
- `activity.js` — active tracker, transaction history, diagnostics, routines, and technical details.
- `app.js` — startup wiring only.

All scripts share one explicit `window.AgentStack` namespace. No bundler, package manager, external asset, or framework is added.

## Visual Direction

The existing Conservation Ledger design system remains. The redesign is a distillation, not a new visual identity:

- One quiet page title and one contextual primary action.
- A four-item navigation rail.
- Flat lists and tables instead of nested cards.
- A persistent compact active-operation tracker when work is running.
- Technical details behind disclosures.
- Semantic color only for real state.
- Minimum 12px metadata, 14px body/control text, and 44px mobile targets.

## Security and Authority Invariants

- The browser remains intent-only.
- Resource Hub and `mcplink` remain the only mutation authorities.
- A reviewed plan is consumed before the first external mutation.
- Failed work requires a fresh inventory and fresh review.
- The loopback bearer-token contract remains unchanged.
- Operation polling remains authenticated with the existing request header.
- User-facing errors never include private paths, command arguments, stderr, tokens, or credentials.
- Existing preservation, backup, no-silent-upgrade, and no-auto-uninstall rules remain unchanged.

## Testing

The implementation must add:

- Unit tests for structured failure mapping and consumed-plan recovery.
- Operation-store tests for progress snapshots and concurrent polling.
- Runner tests for action-start callbacks.
- Service tests for phase/item progress and partial failure reports.
- Environment-overview tests for installed, available, unsupported, and connected states.
- Web contract tests for the four-destination IA and banned jargon.
- Playwright tests for plan consumption, progress updates, partial failure recovery, environment drill-down, sharing/sync visibility, mobile layout, focus, and accessibility.
- Full native, `CGO_ENABLED=0`, race, vet, coverage, benchmark, docs, governance, manifest, and cross-platform build gates.

## Deferred Work

The following are not part of this change:

- WASI activation.
- Remote environment discovery.
- Peer-to-peer synchronization.
- New package acquisition during startup.
- Network egress filtering.
- Replacing operation polling with SSE.
- Pure-Go SQLite migration.
