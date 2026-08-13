---
version: 1
slug: "internal-ui-web"
primary_target: "internal/ui/web"
related_targets: []
---

# AgentStack Manager lifecycle workspace

## Scope and mode
- Target: `internal/ui/web`
- Visitor mode: Operate
- Scope: the complete embedded browser manager, preserving server authority, route IDs, authenticated API contracts, reviewed-plan semantics, and operation locking.

## Audience, job, and success
- Primary user: a developer or technical operator managing local AI applications, IDEs, command-line tools, MCP servers, workspaces, and AgentStack-managed resources.
- Job: understand what exists, choose desired tools, review exact changes, authorize once, follow installation, and recover from partial failure without repeating completed work.
- Success: one next action is obvious, environment state is discoverable, progress is truthful, errors are path-free, and advanced evidence is available without dominating the interface.

## Selected direction
- Direction: Lifecycle Ledger.
- World: a calm operational record that moves from observed environment state to selected intent, pending changes, active installation, and recorded outcomes.
- Structural thesis: exactly four primary destinations—Home, Environments, Changes, Activity—one compact masthead, one conditional installation tracker, and flat ledger rows rather than a dashboard-card wall.
- Memorable moment: a partial installation preserves completed items, identifies failed work, clears the consumed approval, and offers Review failed items or Create fresh plan without exposing internal paths.
- Palette: cold off-white and graphite with restrained teal for action/progress; green, amber, and red remain semantic only.
- Shape system: 14px work surfaces, 8px controls, pills only for compact state badges.
- Motion: bounded state transitions; no decorative choreography; reduced motion disables nonessential movement.

## Interaction and layout
- Desktop: 236px lifecycle rail; fluid workspace; Home readiness summary; Environments list/detail; Changes selection plus exact review; Activity progress/history/recovery.
- Mobile: four labelled navigation items remain visible; split regions and action rows stack; 44px targets and 12px text floor remain intact without horizontal overflow.
- Components: readiness summary, environment filters and detail, connection ledger, profile/tool selector, pending changes, approval block, installation tracker, transaction history, maintenance, system checks, technical disclosure.
- States: loading, ready, attention, waiting, running, completed, failed, skipped, partial recovery, empty, disabled, and locked.

## Constraints and anti-goals
- No frontend framework, remote fonts, external imagery, invented claims, fake progress, or unbounded logs.
- No subsystem-first navigation, duplicate review screen, confirmation modal, dashboard-card wall, or repeated error surfaces.
- No browser exposure of filesystem paths, commands, arguments, raw subprocess output, secrets, or credentials.
- Sharing and sync status is observational; connection mutation retains a separate reviewed Resource Hub or `mcplink` plan.
- Preserve skip navigation, focus visibility and containment, live regions, reduced motion, light/dark contrast, and server-side mutation authority.
