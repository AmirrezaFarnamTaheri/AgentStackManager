# Due-diligence validation and remediation ledger — 2026-08-05

## Scope

This ledger validates the two forensic assessments and the Gemini technical due-diligence report against the authoritative 337-file UI-clarity source tree. Repeated findings were deduplicated. Suggested code was treated as untrusted prose and was adopted only when it matched the existing authority, portability, and test contracts.

## Disposition definitions

- **Implemented** — confirmed defect fixed in this source tree with executable verification.
- **Already satisfied** — report describes a gap that the current source already closes.
- **Release-gated** — source support exists, but hosted identity, signing, or physical-platform evidence cannot be manufactured locally.
- **Dependency-gated** — valid direction, but safe implementation requires a reviewed dependency and protected toolchain evidence unavailable in this workspace.
- **Deferred design** — strategic initiative, not a present defect; requires a separate specification and threat model.
- **Rejected default** — recommendation conflicts with an established invariant or lacks evidence.

## Consolidated finding ledger

| ID | Report item | Validation | Disposition | Result / required gate |
| --- | --- | --- | --- | --- |
| DD-01 | Unsigned local convenience binaries | Confirmed for locally packaged executables | Release-gated | Protected release workflow already requires Go 1.26.5, signed-tag identity, native Windows verification, SBOM, OpenVEX, provenance, attestations, and approval. Local outputs remain explicitly unsigned. |
| DD-02 | Go 1.23.2 local build vs Go 1.26.5 release | Confirmed for the audit host; repository pin is already 1.26.5 | Already satisfied / release-gated | `.go-version`, governance scripts, CircleCI, and release scripts enforce 1.26.5. Public artifacts must come from protected CI. |
| DD-03 | CGO-disabled SQLite feature divergence | Confirmed | Dependency-gated | The report incorrectly attributes the implementation to `mattn/go-sqlite3`; ASM uses a narrow direct SQLite C API. A pure-Go replacement remains desirable, but admission is blocked until the exact module graph, license set, performance, migration compatibility, and protected Go 1.26.5 builds are verified. |
| DD-04 | Non-Windows process resource ceilings | Partially confirmed | Implemented | Linux now atomically places bounded children in delegated cgroup v2 scopes using memory, CPU, and process ceilings. Windows retains Job Objects. Unsupported Unix platforms reject requested hard limits instead of claiming containment. |
| DD-05 | External adapters lack full OS/WASI isolation | Confirmed as a documented limitation | Deferred design | Activation remains blocked. A WASI transition requires a guest ABI, capability model, filesystem/network policy, package identity, revocation, performance proof, and differential compatibility plan. |
| DD-06 | 8–10 px micro-typography | Confirmed | Implemented | All explicit UI text sizes now meet a 12 px floor; body and interactive controls use a 14 px baseline or larger. Browser assertions enforce the floor. |
| DD-07 | Internal protocol jargon in primary UI | Confirmed | Implemented | Primary labels now use “Pending changes,” “System capabilities,” “Generate change plan,” “Approve,” and “Apply changes.” Protocol terminology remains only where technically necessary. |
| DD-08 | Abrupt redirect from Setup/Tools to Review | Confirmed | Implemented | Plan generation remains in context and renders an inline pending-change summary. Review opens only through an explicit action. The Review destination remains the sole authorization surface. |
| DD-09 | Core capabilities hidden in Advanced accordion | Confirmed | Implemented | Advanced destinations are visible under static categorized navigation rather than a collapsed `<details>` control. |
| DD-10 | Catalog filter destroys DOM and focus | Confirmed | Implemented | Catalog nodes are rendered once per catalog/tier structure and filtered in place with a bounded debounce, preserving search focus and node identity. |
| DD-11 | Settings popover lacks keyboard management | Confirmed | Implemented | Escape dismissal, click-outside close, focus entry, bounded Tab cycling, `aria-expanded`, and focus restoration are implemented. |
| DD-12 | Dynamic `color-mix()` contrast uncertainty | Confirmed as an avoidable risk | Implemented | Dynamic mixes were replaced with explicit light/dark semantic tokens. Browser and axe checks cover both themes. |
| DD-13 | Mobile controls below 44 px | Confirmed | Implemented | Mobile interactive controls enforce a 44 px minimum target and wrap instead of compressing. Runtime assertions cover the compact viewport. |
| DD-14 | Live regions omit `aria-atomic` | Confirmed | Implemented | Every status live region now announces complete atomic state. Static and runtime checks enforce the contract. |
| DD-15 | Activity history grows without bound | Confirmed | Implemented | Browser-session activity is capped at 50 entries. Runtime checks exercise overflow and prove the cap. |
| DD-16 | Accessibility/runtime tests are mostly string checks | Confirmed | Implemented | Playwright/axe coverage now exercises typography, touch targets, focus preservation, menu dismissal, atomic status, dark theme, inline plan continuity, and bounded activity. |
| DD-17 | Protected Playwright/axe CI absent | Stale claim | Already satisfied | The existing UI assurance package and protected workflows already run browser/accessibility gates; this remediation expands their assertions. |
| DD-18 | SBOM/VEX/provenance automation absent | Stale claim | Already satisfied | Release scripts already generate CycloneDX, OpenVEX, provenance, checksums, and GitHub attestations. |
| DD-19 | Native Windows x64/ARM64 proof absent | Stale as a source capability claim | Already satisfied / release-gated | Native jobs exist. Only a protected run against the exact candidate can create publication evidence. |
| DD-20 | Source integrity and 337-file manifest | Confirmed positive control | Preserve | Manifest verification remains mandatory and will be regenerated after this remediation. |
| DD-21 | Single Review authorization surface | Confirmed positive control | Preserve | No alternate client-side apply path was introduced. Server-side plan identity, confirmation, and mutation authorities remain unchanged. |
| DD-22 | Loopback binding and bearer-token authorization | Confirmed positive control | Preserve | No non-loopback listener, cookie downgrade, or unauthenticated mutation path was introduced. |
| DD-23 | Zero-allocation UI operation store | Confirmed positive control | Preserve | Existing benchmark gate remains unchanged; UI DOM remediation does not alter the Go operation store. |
| DD-24 | Prewarm `npx`/`uvx` MCP servers by default | Not established as a defect; conflicts with lazy execution and no-silent-acquisition | Rejected default | Any future prewarming must be explicit opt-in, offline-capable, plan-visible, and unable to fetch packages or retain unapproved processes. |
| DD-25 | Replace polling with unauthenticated EventSource/SSE | Opportunity, not current defect | Deferred design | Native `EventSource` cannot attach the current bearer header. A secure stream requires a scoped stream token or same-origin credential design, bounded replay, backpressure, and shutdown semantics. |
| DD-26 | Kernel-level network egress enforcement | Valid sandbox horizon | Deferred design | Must be solved with the external-adapter/child-process sandbox threat model; no source-only claim of egress containment is made. |
| DD-27 | P2P context/skill synchronization mesh | Unsupported product expansion | Rejected / separate project | Would create new distributed authority, identity, conflict, privacy, and revocation surfaces. It is not a remediation for the local control plane. |
| DD-28 | OCI catalog pulling, dynamic runtime reload, signed diff receipts, client-side WASM validation | Speculative enhancements without defect evidence | Deferred design | Each requires its own requirements, threat model, compatibility contract, and authority review. None is silently added to this release. |
| DD-29 | Physical hardware/GPU/display matrix | Valid evidence gap | Release-gated | Protected Windows/macOS hardware testing must validate the exact signed candidate. Cross-compilation is not treated as runtime proof. |
| DD-30 | WASI promises of sub-5 ms starts and 10× memory improvement | Unsupported numerical claims | Rejected as a guarantee | No benchmark evidence supports these targets. Future prototypes must establish baselines and report distributions before targets become release criteria. |

## Implemented source changes

1. UI legibility, vocabulary, navigation, in-context change preview, focus management, deterministic themes, mobile target sizing, atomic live regions, bounded activity, and stable filtering.
2. Browser assurance expanded from structural checks to runtime interaction and accessibility assertions.
3. Linux cgroup v2 hard ceilings added to the shared managed-process boundary; external adapter conformance accepts optional memory, CPU, and process limits.
4. Convergence evidence updated so the process boundary remains traceable after deleting duplicate external platform launchers.
5. Documentation and CLI reference aligned with actual containment and release limits.

## Publication boundary

This remediation produces source and unsigned convenience builds only. It does not claim protected-host compilation, code signing, notarization, transparency-log inclusion, physical hardware execution, or public publication. Those remain hard release gates.
