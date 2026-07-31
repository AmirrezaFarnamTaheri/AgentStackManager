# AgentStack Manager UI Design Contract

## Product intent

The manager is a local operational control plane, not a marketing site. Its visual system must help a technical user understand what is detected, what will change, what is preserved, and whether a mutation is currently running. Trust, legibility, and reversible decision-making take priority over decorative novelty.

## Visual principles

1. **Operational calm:** neutral surfaces, restrained elevation, and one accent color keep attention on state and actions.
2. **Preservation before installation:** existing and preserved states are visible alongside install, repair, and consent-required states.
3. **One icon language:** sidebar navigation uses coherent Lucide outline icons—dashboard, package, checklist, and router—with visible text labels.
4. **Strong hierarchy:** page purpose appears before controls; mutation controls are visually distinct from read-only controls.
5. **State is never color-only:** status copy, action labels, badges, and icons accompany color changes.
6. **Responsive density:** the desktop grid collapses without hiding controls or requiring horizontal page scrolling.

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
- The Playwright/axe workflow verifies serious and critical WCAG 2.2 AA violations, keyboard navigation, reduced motion, operation feedback, and authenticated shutdown.

## Design tokens

The embedded CSS custom properties are the source of truth for background, surface, text, muted text, border, accent, success, warning, error, and shadow values. Dark mode derives from `prefers-color-scheme`; reduced motion derives from `prefers-reduced-motion`. New components must consume these tokens instead of adding isolated color literals unless the literal belongs to a fixed brand asset.

## Verification

Static contracts live in `internal/ui/web_contract_test.go`. Browser-level behavior lives in `ui-tests/accessibility.mjs` and is a required Windows CI check. The navigation SVGs are embedded locally; the UI has no runtime icon-font or CDN dependency.
