# Convergence Omission and Contradiction Audit

## Result

The donor corpus contains 2,673 files across seven roots. Every file and symlink has a SHA-256 surface row and at least one semantic-record link. The combined adoption graph contains 53 records and resolves every implemented target node back to donor evidence.

## Audit ladder

| Level | Method | Result |
| --- | --- | --- |
| Files and symlinks | archive preflight, bounded extraction, deterministic hash inventory | 2,673 files accounted; one relative symlink recorded; no unsafe archive member accepted |
| Roots and deliverables | manifests, READMEs, entry points, package/build files, CI, platform roots | all seven donor roots mapped to context records and capability groups |
| Exported/runtime surfaces | command handlers, stores, services, adapters, UI routes, schedulers, MCP/config code | material surfaces assigned to target planes or explicit non-adoption records |
| Semantic contracts | memory, context, registry, sync, backup, linking, workspace, artifacts, schedules, receipts | 44 primary tests plus 16 composition/negative-path tests; 60 unique links across 53 records |
| State machines | plan/apply, refresh, memory, linking, routine run/recovery | allowed, forbidden, expiry, replay, drift, confirmation, and restart paths documented |
| Negative paths | traversal, symlinks, foreign files, source drift, digest mismatch, secret output, remote import, shell execution | target tests fail closed at each material boundary |
| Operator surfaces | donor CLIs/TUIs/desktop apps/web views | outcomes consolidated into one JSON CLI and authenticated loopback status UI |
| Scripts, CI, release | donor setup/update schedules plus ASM existing release pipeline | schedules became routines; release stays under ASM's signed reproducible pipeline |
| Deep leaves | presets, examples, fixtures, schemas, UI assets, platform wrappers | useful primitives captured; remaining leaves inherit explicit root/reference or supersession disposition |
| Cross-mechanism contradictions | duplicate registries, duplicate MCP authorities, remote install, credential storage, UI authority | resolved in favor of one canonical ASM owner per responsibility |

## Explicit non-adoptions

These are complete decisions, not unexamined omissions:

- **AIaW chat and billing:** retained as reference because ASM manages local agent infrastructure rather than provider conversations, subscriptions, or billing state.
- **AIaW PWA/mobile/Tauri shells:** retained for future platform evidence; current release contract is Windows-native.
- **Context-sync Notion connector:** replaced by an explicit bounded adapter seam; no provider token enters fabric state.
- **Skills-hub online marketplace:** remote results cannot become executable state; reviewed local import is the admission path.
- **MCP-linker credential vault:** provider/OS credential stores remain the secret authority.
- **LifeSync hard-coded weather/email integration:** generalized into direct command routine steps.
- **Donor-specific UIs and daemons:** superseded by ASM service, CLI, and authenticated loopback UI.

## Composition checks

- Resource registries from Skillshare, Skills Hub, AIaW, and MCP Linker fuse into one typed resource hub.
- Context generation from ai-setup and context-sync shares the same workspace and resource target vocabulary.
- Update schedulers from Skills Hub and LifeSync use one routine engine and source-refresh plan.
- MCP configuration from MCP Linker and AIaW points to the existing managed router; no second process runtime was introduced.
- Local storage from AIaW and context-sync uses one schema-versioned persistence pattern.

## Residual boundaries

- Direct remote catalog search/download remains outside the trusted core. A future implementation requires a quarantined download/admission protocol with signature/hash policy and no automatic activation.
- Chat/provider/billing features remain outside the product boundary.
- Non-Windows release claims remain reference-only until ASM deliberately expands its support contract.
- Native Windows behavior, signing, vulnerability scanning, and attestations remain protected-CI release gates.
