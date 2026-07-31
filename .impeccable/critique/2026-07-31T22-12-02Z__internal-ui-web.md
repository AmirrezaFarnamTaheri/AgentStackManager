---
target: internal/ui/web
total_score: 25
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 3
timestamp: 2026-07-31T22-12-02Z
slug: internal-ui-web
---
## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3 | Persistent operation status is strong, but inventory loading is not shaped as a meaningful workspace state. |
| 2 | Match System / Real World | 3 | Safety language is concrete, but the interface looks like a generic SaaS dashboard rather than a controlled workstation change review. |
| 3 | User Control and Freedom | 2 | Navigation is clear, but long-running flows expose no contextual cancel, return, or next-action guidance. |
| 4 | Consistency and Standards | 2 | Controls are mostly consistent, while icon styling, status symbols, cards, pills, and section framing use competing visual vocabularies. |
| 5 | Error Prevention | 4 | Explicit plan review and authorization are excellent. |
| 6 | Recognition Rather Than Recall | 3 | Core sections and controls are visible, but component states and plan consequences require scanning many repeated cards. |
| 7 | Flexibility and Efficiency | 2 | Search helps, but expert scanning, bulk understanding, and keyboard-oriented density are underdeveloped. |
| 8 | Aesthetic and Minimalist Design | 2 | The purple, Inter, rounded-card system is category-interchangeable and gives equal visual weight to too many containers. |
| 9 | Error Recovery | 2 | Errors are surfaced, but recovery guidance is not consistently embedded near the affected operation. |
| 10 | Help and Documentation | 2 | Descriptions are present, but contextual explanations and action sequencing are not consolidated. |
| **Total** | | **25/40** | **Acceptable, significant redesign warranted** |

## Design Specificity Verdict

**LLM assessment:** The current surface is careful and usable, but visually interchangeable with a generic admin starter. It does not make the preservation-first mechanism legible at a glance. Safety is expressed as copy inside cards rather than as the organizing grammar of the workspace.

**Deterministic scan:** One warning in `internal/ui/web/styles.css`: overused `Inter` font. Manual source inspection also found repeated eyebrow labels, a three-column equal card grid, decorative status dots, generic purple accenting, and hand-authored Lucide-style SVGs that dilute the product-specific identity.

**Visual evidence:** Headless Chromium was attempted against both the live loopback server and a local file preview. Enterprise browser policy blocked `127.0.0.1` and `file:` navigation, so no reliable browser screenshot or overlay was available for the baseline. Source structure and deterministic detector output are the fallback evidence.

## Overall Impression

The interface protects the user better than it communicates that protection. The largest opportunity is to turn plan provenance and preservation state into the primary visual structure, reducing decorative containers while increasing operational clarity.

## What's Working

- The plan authorization flow makes destructive scope explicit.
- The persistent operation-status region and control locking address long-running work honestly.
- Product copy is concrete and avoids invented metrics or marketing claims.

## Priority Issues

### [P1] The first viewport behaves like a marketing header, not an operational control plane
- **Why it matters:** Users arrive to inspect and act, but the largest typography and three top-right actions compete with current machine state and the next safe step.
- **Fix:** Replace the hero-like header with a compact workspace masthead, a single primary next action, and a persistent operation/status rail.
- **Suggested command:** `$impeccable shape`

### [P1] Repeated cards obscure comparison and make the preservation model harder to scan
- **Why it matters:** Metrics, profile controls, guarantees, and components all use similar rounded containers. Users must read each card instead of recognizing state by structure.
- **Fix:** Use ledger rows, grouped registry sections, sparse dividers, and state seals. Reserve elevation only for the currently actionable surface.
- **Suggested command:** `$impeccable extract`

### [P1] Mobile navigation and action placement are weak for interrupted work
- **Why it matters:** A horizontally scrolling top nav and header-level controls make the primary flow harder to resume one-handed.
- **Fix:** Introduce a compact mobile command bar, keep current section visible, and place the next safe action in a stable thumb-reachable region.
- **Suggested command:** `$impeccable adapt`

### [P2] The visual system is generic and weakly tied to product trust
- **Why it matters:** Purple gradients, Inter, rounded pills, and decorative dots signal an undifferentiated AI tool rather than an auditable local control plane.
- **Fix:** Adopt the Conservation Ledger direction with cold neutrals, graphite, one muted teal accent, consistent 12px radii, and typography that reads like a Windows engineering workspace.
- **Suggested command:** `$impeccable craft`

### [P2] Loading, empty, error, and completion states are present but not composed as a system
- **Why it matters:** Users can see status, but they do not always see what changed, what remains safe, and what to do next in the same region.
- **Fix:** Standardize state banners, skeletons, empty-state actions, recovery copy, and completed-operation summaries.
- **Suggested command:** `$impeccable harden`

## Persona Red Flags

**Alex (Power User):** Component selection is visually verbose, the current page offers no compact scan mode, and primary actions are split across the masthead and panels. Fast review requires too much pointer travel.

**Sam (Accessibility-Dependent User):** Existing semantics are promising, but decorative symbols and hand-authored icon paths risk inconsistent announcement. Mobile horizontal navigation can become difficult at high zoom.

**Riley (Stress Tester):** Empty and error states do not consistently explain recovery. Large component sets and long plan reasons need stronger overflow and wrapping rules.

## Minor Observations

- Multiple eyebrow labels create a templated rhythm.
- The local-only status dot is decorative rather than a verified semantic status.
- `Install AgentStack` and plan-building actions compete before the user understands current state.
- The table is the clearest part of the product and should influence the rest of the system.

## Questions to Consider

- What if the whole interface read as one controlled change record instead of a collection of widgets?
- Can every section expose one obvious next safe action and one explicit recovery path?
- Which information deserves elevation, and which should become a quiet ledger row?
