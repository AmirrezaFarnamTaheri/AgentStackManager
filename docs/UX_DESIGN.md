# AgentStack Manager UI Design Contract

The canonical visual system is recorded in [`DESIGN.md`](../DESIGN.md). Product truth and durable UX constraints are recorded in [`PRODUCT.md`](../PRODUCT.md). The lifecycle behavior is documented in [`UI_LIFECYCLE_WORKSPACE.md`](UI_LIFECYCLE_WORKSPACE.md).

## Product intent

The manager is a local operational control plane, not a marketing site or subsystem dashboard. It must help a technical user understand what exists, what needs attention, what will change, what is currently running, and how to recover. Trust, legibility, and deliberate authorization take priority over visual density or exposure of internal architecture.

## Visual principles

1. **Lifecycle before subsystems:** navigation follows the user's work—Home, Environments, Changes, Activity.
2. **One clear next action:** each area has one dominant action and avoids competing panels.
3. **Operational calm:** neutral surfaces and one restrained teal action accent keep attention on state and recovery.
4. **Registry before cards:** inventories, changes, progress items, and transactions use flat rows and dividers.
5. **State is never color-only:** text and structure accompany semantic color.
6. **Responsive topology:** layouts stack or reorder before typography or touch targets shrink.
7. **Evidence on demand:** versions, raw diagnostics, and provenance appear only where useful, usually behind Technical details.

## Information architecture contract

The interface exposes exactly four primary destinations:

1. **Home** — concise readiness, counts, recent activity, and one recommended next action.
2. **Environments** — read-only AI app, IDE, CLI, MCP, workspace, and managed connection inventory.
3. **Changes** — profile/tool selection, inline estimate, exact pending changes, approval, and the only client apply action.
4. **Activity** — live installation tracker, transaction history, maintenance, system checks, and disclosed technical output.

There is no primary Setup, Tools, Review, Operate, Fabric, Diagnostics, or Sharing destination. Those labels describe historical layouts or backend domains, not current navigation.

Environments shows sharing and synchronization state but does not grant connection-write authority. Connection changes retain their separate reviewed Resource Hub or `mcplink` path.

## Change and authorization contract

Changes is one continuous workspace. Selection and preview remain visible while an exact plan is created. Apply stays disabled until the exact pending-change list is available and the operator checks the approval control.

A reviewed plan is single-use. The client marks it consumed before apply starts. Selection changes, inventory refresh, catalog changes, or a failed apply clear the current approval. No modal or secondary surface may offer another apply path.

## Operation-feedback contract

Every long-running action uses the same authenticated operation controller:

- the initiating control exposes a task-specific busy label and is disabled;
- conflicting selection and mutation controls are locked;
- progress reports the server-provided stage, completed/total counts, percentage, active item, and item states;
- partial results remain available when an operation fails;
- the persistent tracker lives in Activity and may surface globally while work is active;
- a concise notice states the outcome and one recovery action without duplicating the tracker;
- technical output is opt-in and sanitized at the server boundary;
- focus returns predictably unless the action deliberately opens Activity;
- reduced-motion preferences suppress nonessential motion.

Client locking is feedback and conflict prevention. Server-side reviewed-plan identity, confirmation, lease, mutation authority, and postcondition verification remain authoritative.

## Failure and recovery contract

Browser-facing failures use stable codes and plain recovery guidance. They never expose local plan paths, executable commands, arguments, or raw subprocess output.

For partial installation failure:

- successful actions remain recorded;
- failed items are identified from the public transaction result;
- the consumed plan and approval are cleared;
- the UI offers **Review failed items** and **Create fresh plan**;
- a retry always creates a new plan from refreshed inventory.

## Accessibility contract

- The skip link is the first keyboard target.
- Navigation labels remain visible at every breakpoint and only one has `aria-current="page"`.
- Section changes move focus to the workspace without creating abrupt redirects during editing.
- Primary controls retain at least 44px touch height.
- Explicit rendered text has a 12px floor; body and controls normally use 14px or larger.
- Status regions use `role="status"`, `aria-live="polite"`, and `aria-atomic="true"`.
- Settings traps focus while open, closes on Escape or outside click, and restores the trigger.
- State meaning is never conveyed by color alone.
- Light and dark themes use explicit high-contrast semantic pairs.

## Performance contract

- The interface has no frontend framework, remote font, CDN, or runtime package dependency.
- Browser code is split by responsibility: core, changes, environments, activity, and bootstrap.
- One core `api` function and one `runOperation` controller own authenticated requests and operation polling.
- Search uses a bounded debounce and preserves user focus.
- Activity history is capped at 50 client entries.
- Progress updates patch bounded lists and do not create unbounded timers or logs.
- Motion uses transform or opacity only and is disabled under reduced motion.

## Verification

Static contracts live in `internal/ui/web_contract_test.go`. Go behavior tests cover the operation store, failure envelope, progress tracker, transaction history, environment projection, and public-report redaction. Browser behavior lives in `ui-tests/accessibility.mjs` and is a required protected Windows CI check.
