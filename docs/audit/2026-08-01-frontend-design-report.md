# AgentStack Manager Frontend Design and Optimization Report

## Scope

Executed the requested Impeccable sequence against `internal/ui/web`:

1. `init`
2. `critique`
3. `shape`
4. `extract`
5. `craft`
6. `optimize`

The redesign preserves backend routes, element IDs, workflow order, safety language, confirmation semantics, and the embedded Go delivery model.

## Product record

`PRODUCT.md` now captures the repository-grounded audience, purpose, preservation-first mechanism, operating context, constraints, evidence, principles, and accessibility requirements. Facts not explicitly supplied by the user are marked as inferred.

## Baseline critique

Method: degraded single-context because no subagent/Task tool was exposed. Browser navigation and overlay injection were blocked by the execution environment, so the deterministic source detector and local Chromium document rendering were used.

- Nielsen score: **25/40**, acceptable but requiring material improvement.
- Baseline detector finding: overused Inter font.
- Primary issues: marketing-style first viewport, repeated card shells, weak mobile topology, category-interchangeable purple styling, and incomplete composition of loading/empty/error states.
- Snapshot: `.impeccable/critique/2026-07-31T22-12-02Z__internal-ui-web.md`.

## Shaped direction

The selected direction is **Conservation Ledger**.

- Operate-mode provenance workbench, not a marketing dashboard.
- Persistent workspace rail, compact command masthead, live operation ribbon, metric ledger, registry rows, sealed plan review, and explicit authorization boundary.
- Cold off-white and graphite field with one muted teal accent.
- 12px surfaces, 8px controls, pills only for provenance seals.
- Motion limited to task feedback and short state transitions.
- Surface brief: `.impeccable/surfaces/internal-ui-web.md`.

The mandatory direction roll completed in degraded offline mode with seed `b47af99d`; no external challenger board or approved composition was available.

## Extracted design system

The implementation now exposes a small reusable system:

- semantic color, type, radius, shadow, motion, and breakpoint tokens;
- primary, secondary, quiet, and destructive button vocabulary;
- field and focus vocabulary;
- operation ribbon;
- metric ledger;
- registry row;
- navigation item;
- record seal;
- loading skeleton, empty state, error state, and durable operation state.

The built system is recorded in `DESIGN.md` and `.impeccable/design.json` with eight self-contained component specimens.

## Implemented redesign

- Replaced the marketing-style header with a compact operational masthead.
- Rebuilt the sidebar as a persistent workspace rail and mobile command strip.
- Converted the first viewport into a status ribbon, metric ledger, focused configuration surface, and flat protection register.
- Reworked component selection into registry rows with delegated event handling.
- Added shape-matched metric loading skeletons and instructive empty states.
- Added consistent record seals, operation statuses, focus behavior, dark mode, and reduced-motion behavior.
- Preserved all tested DOM IDs, embedded assets, API contracts, and JavaScript workflow behavior.
- Eliminated page-level mobile overflow caused by intrinsic horizontal-navigation width.

## Optimization

The interface remains dependency-free and network-independent. Its three embedded source assets total about 54.5 KB uncompressed and about 14.6 KB gzip-compressed.

The measured bottleneck was a duplicate component-registry render during initial inventory refresh. Removing that duplicate render produced the following median changes across three isolated Chromium runs of the same redesigned interface:

| Metric | Before | After | Change |
|---|---:|---:|---:|
| JS heap used | 929,192 B | 870,132 B | -6.36% |
| Blink node count | 1,671 | 1,125 | -32.68% |
| Layout duration | 147.559 ms | 124.380 ms | -15.71% |
| Style recalculation duration | 20.770 ms | 19.917 ms | -4.11% |
| Total task duration | 187.005 ms | 162.137 ms | -13.30% |

The richer redesign adds 26 DOM elements relative to the former interface and increases compressed source by about 3 KB, while remaining a very small local static application. The optimization target was runtime duplication rather than destructive minification or removal of accessibility/state markup.

## Visual verification

Rendered with headless Chromium using the current embedded HTML, CSS, JavaScript, and real catalog/inventory fixture data:

- Desktop: 1440 x 1000
- Mobile: 390 x 844

The mobile document width remains within the viewport. The navigation strip scrolls internally without expanding the page.

## Detector and finish review

The final deterministic detector found one layout-animation warning (`transition: padding`). It was corrected to transform-based hover feedback and verified absent by source inspection. Per the Impeccable workflow, the detector was not rerun a second time.

The degraded finish reviewer returned `disposition: ship`. Review record: `.impeccable/finish-review.md`.

## Limitations

- No dual-agent critique or independent finish-review subagent was available.
- Direct browser navigation, user-visible detector overlay injection, and live localhost capture were blocked by environment policy.
- Native Windows browser execution and the repository Playwright/axe job remain external verification gates.
- No external QUALITY BAR or approved composition was available after the offline direction roll.
