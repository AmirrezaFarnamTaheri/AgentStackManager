# AgentStack Manager UI Design Contract

The canonical visual system is recorded in [`DESIGN.md`](../DESIGN.md). Product truth and durable UX constraints are recorded in [`PRODUCT.md`](../PRODUCT.md). This document defines the behavioral contract for the embedded manager.

## Product intent

The manager is a local operational control plane, not a marketing site. Its interface must help a technical user understand what is detected, what is selected, what will change, what is preserved, and whether a mutation is running. Trust, legibility, and deliberate authorization take priority over decorative novelty.

## Visual principles

1. **Conservation ledger:** observed state, plan input, reviewed output, and authorized mutation remain visually distinct.
2. **Operational calm:** cold neutral surfaces, restrained elevation, and one muted teal accent keep attention on records and actions.
3. **Registry before cards:** metrics, guarantees, and capability choices use dividers and rows unless elevation communicates a real work-surface boundary.
4. **One icon language:** navigation uses coherent local outline icons with visible text labels and no runtime icon dependency.
5. **State is never color-only:** status copy, action labels, seals, and icons accompany semantic color.
6. **Responsive topology:** desktop splits collapse into one column, the navigation rail becomes a compact horizontal command strip, and the page never gains horizontal overflow.
7. **Evidence typography:** the workhorse UI sans carries interface copy; the mono stack is reserved for values, versions, seals, and command output.

## Operation-feedback contract

Every long-running action uses the same controller:

- the initiating button receives `aria-busy="true"`, a spinner, and a task-specific busy label;
- mutation and selection controls are temporarily locked to prevent conflicting requests;
- the main surface exposes `aria-busy="true"`;
- a persistent polite live region announces running, success, and failure states;
- errors retain structured API detail in the router output when available;
- focus returns to the initiating control unless the operation intentionally moved focus to a new section;
- reduced-motion preferences suppress the spinner animation and nonessential transitions;
- plan confirmation cannot be toggled while apply is running.

The server-side mutation gate remains authoritative. Client-side locking is feedback and conflict prevention, not an authorization boundary.

## Accessibility contract

- Semantic headings and labelled form controls remain mandatory.
- The first keyboard target is the skip link.
- Section navigation moves focus to the selected section heading.
- Focus-visible outlines use a three-pixel accent ring.
- Status surfaces use `role="status"`, `aria-live="polite"`, and `aria-atomic="true"`.
- Icons that duplicate visible text are hidden from assistive technology.
- Primary controls retain at least 44px touch height.
- The Playwright/axe workflow verifies serious and critical WCAG 2.2 AA violations, keyboard navigation, reduced motion, operation feedback, and authenticated shutdown.

## Performance contract

- The embedded interface has no frontend framework, remote font, CDN, or runtime package dependency.
- Component selection uses one delegated listener rather than rebuilding listeners after every registry render.
- Initial refresh performs one registry render, not two.
- Capability groups use `content-visibility: auto` and an intrinsic fallback size.
- Motion is limited to transforms and opacity; layout-driving properties are not animated.
- Horizontal navigation may scroll inside its own strip, but it must not expand the page viewport.

## Design tokens

The embedded CSS custom properties are the runtime source of truth. `DESIGN.md` and `.impeccable/design.json` record the same system for design-aware tooling. Dark mode derives from `prefers-color-scheme`; reduced motion derives from `prefers-reduced-motion`. New components consume semantic tokens instead of adding isolated color literals unless the literal belongs to a fixed local asset.

## Verification

Static contracts live in `internal/ui/web_contract_test.go`. Browser-level behavior lives in `ui-tests/accessibility.mjs` and is a required Windows CI check. The navigation SVGs are embedded locally; the UI has no runtime icon-font or CDN dependency.
