# Changelog

## 0.3.0-unreleased — 2026-08-06

- Added a unified Windows desktop launch path: the executable starts the loopback service, opens one dedicated address-bar-free application window, owns its lifetime, and hides all child consoles and raw package-manager output. Explicit `ui --browser` and `ui --no-open` modes remain available for development and diagnostics.
- Added simultaneous multi-target discovery and connection management for verified Codex, Claude Code, Gemini CLI, OpenCode, Cursor, and GitHub Copilot adapters, plus evidence-labelled read-only catalogue entries for additional IDEs, agents, and desktop clients.
- Added canonical Sharing & Sync inventory with managed, installed, contained, in-sync, drifted, duplicate, conflict, orphan, and unmanaged states. Canonical resources fan out to multiple target installations without being counted as unrelated copies.
- Added digest-bound multi-target sync plans, stale-state revalidation, bounded parallel target application, per-root serialization, independent failure isolation, deterministic receipts, and cancellation.
- Added dependency-aware parallel tool installation with global and per-installer concurrency limits while preserving deterministic transaction order and post-install verification.
- Added structured, privacy-safe root-cause diagnostics and official WinGet HRESULT decoding. Public endpoints no longer serialize commands, arguments, stdout, stderr, environment data, or private paths.
- Added deterministic Windows PE resources for the AgentStack icon and application manifest, generated without optional external resource compilers.
- Added responsive Sharing & Sync, connection batch actions, duplicate/conflict review, repair intelligence, searchable history, and five-area mobile navigation.

## Unreleased — forensic remediation

- Added cache-busted embedded UI assets and no-store responses so new builds cannot reuse stale result-table JavaScript or CSS; hardened the sidebar logo and added an application favicon.
- Added default Windows UAC elevation for UI/setup launches, with loop-safe relaunch handling and clear cancellation errors.
- Added real AI-app target discovery and connect/pause controls for Codex, Claude, AGY/Gemini, OpenCode, Cursor, and GitHub Copilot, backed by Resource Hub registration rather than cosmetic status toggles.
- Added method-aware failure intelligence with normalized WinGet error codes, direct evidence, affected-tool groups, concrete repair instructions, and privacy-safe unknown-error fallback copy.
- Added environment health scoring, a repair queue, searchable/filterable installation history, and responsive result widgets without one-character wrapping.
- Corrected current verified WinGet pins for yq, Trivy, and scc, including the `BenBoyter.scc` identifier that resolves WinGet package-not-found failures.
- Validated and deduplicated three external due-diligence reports into a 30-item evidence ledger; remediated confirmed UI legibility, navigation, focus, contrast, touch-target, live-region, activity-bound, and plan-continuity defects; added Linux cgroup v2 process ceilings and external-adapter limit flags while preserving release and mutation authority gates.
- Replaced the historical Setup / Tools / Review / Operate shell with a debloated lifecycle workspace—Home, Environments, Changes, and Activity. Added read-only AI/IDE/CLI/MCP/workspace and connection visibility, server-reported installation stages and per-item progress, retained partial transaction results, safe consumed-plan recovery, path-free browser failures, transaction history, and one continuous selection-to-approval flow without changing backend mutation authority.
- Added the constrained `fabric.asm.dev/external-adapter/v1alpha1` process protocol, SHA-256-pinned executable admission, fresh-process deadlines and output ceilings, synthetic environment isolation, reviewed capability intersection, Phase 5 differential execution, sealed external conformance reports, and a read-only CLI command without external adapter activation.
- Added the embedded `fabric.asm.dev/adapter-conformance/v1alpha1` target oracle, 64-case differential and candidate-preserving round-trip suite, sealed read-only conformance reports, omission-aware fidelity evidence, and corrected no-op/MCP capability postconditions for adapter implementation version `1.1.0`.
- Added the versioned `fabric.asm.dev/adapter/v1alpha1` target contract, digest-bound structural capability snapshots, deterministic fidelity/loss reports, built-in Codex/Claude/Cursor/AGY/OpenCode/Copilot/generic projections, Resource Hub `--deny-loss`, and capability-drift checks while retaining existing reviewed mutation authorities.
- Added the additive `fabric.asm.dev/v1alpha1` canonical artifact graph with stable IDs, deterministic extension JSON, content/envelope digests, target bindings, execution classes, and field provenance while retaining Resource Hub v1 as the sole mutation authority.
- Added an immutable blob/tree content-addressed shadow store, deterministic reachability marks, digest-bound Resource Hub migration receipts, stale-source verification, and explicit-confirmation restore to new paths with atomic no-replace publication and retained incomplete-tree evidence.
- Added a versioned SQLite shadow metadata index with WAL, strict schema migrations, immutable receipt history, canonical row verification, stale-authority checks, read-only inspection, unrelated-database rejection, and verified no-overwrite online backups while retaining Resource Hub v1 as the sole write authority.
- Added Cross-Agent Capability Sharing Matrix (`#sharing`) for visibility into skill and MCP links.
- Added Resilience Error Inspector & Diagnostics Hub (`#errors`).
- Added measured runtime details for memory, goroutine count, and daemon uptime inside the Setup disclosure; CPU remains unavailable because the local backend does not measure it.
- Added 3-way Appearance Theme Switcher (`Auto` / `Dark` / `Light`) with local storage persistence.
- Kept exact review and authorization inside Changes as the sole browser apply path; no duplicate confirmation modal or stale-plan retry path remains.
- Added non-blocking asynchronous Windows notification handlers in `internal/notify`.
- Converged seven peer projects into five target-native ASM planes: audited resource hub, project context, workspace/memory/artifacts, MCP client linking, and scheduled routines.
- Added 53-record donor adoption ledger, 2,673-row surface-accountability matrix, hash-verified donor slices, omission audit, trust/state map, runbook, validation record, and launch pre-mortem.
- Added deterministic project scanning/scoring, confined file retrieval/search, read-only Git evidence, and reviewed multi-agent context refresh preserving human-authored text.
- Added typed local resources with tracked sources, admission audit, target-native copy/link distribution, foreign-file protection, managed pruning, reviewed source refresh, backups, and confirmed restore.
- Added hierarchical workspaces, scoped layered memory, strict prompt variables, content-addressed artifacts, versioned local schemas, legacy migration, and future-version rejection.
- Added reviewed MCP linking for Codex, Claude, Cursor, AGY/Gemini, and OpenCode while retaining the existing managed router as the sole process authority.
- Added manual/daily/weekday/interval routines, bounded sequential steps, explicit confirmation, redacted versioned receipts, bounded history, and interrupted-persistence reconciliation.
- Hardened convergence state with bounded resource trees/audits, secret-rejecting routine admission, durable workspace transaction journals, digest-only MCP plans, registration-only MCP recovery records, duplicate-key rejection, and deferred non-authoritative artifact cleanup.
- Added bounded regular-file admission and structural validation for resource, workspace, routine, context, MCP, and recovery state; rejected symlink substitution, oversized stores, malformed identifiers, invalid digests, unconstrained commands, and unsupported persisted graphs.
- Enforced a 1 MiB pre-decode ceiling for strict workspace and routine JSON input, preserved UTF-8 boundaries during context truncation, and corrected public-release documentation to require the Authenticode signer checks already enforced by setup and release CI.
- Rejected hidden Git mutation, direct remote marketplace activation, product-specific credential storage/connectors, shell-string automation, duplicate donor daemons, and UI-owned authority.

- Closed review findings in archive path validation, MCP shutdown cancellation, reviewed-plan identity binding, releasepack argument handling, and full-range Windows PATH transport.
- Added an explicit launch pre-mortem, staged rollout, rollback triggers, owners, checkpoints, and protected ship gates.

- Added four deep module boundaries: closed source-provenance packaging, reviewed-plan transaction execution, managed MCP child lifecycle, and supervised process execution.
- Replaced duplicated process launching in runner, inventory, session, MCP, self-install, and Windows PATH helpers with one bounded, resource-aware supervisor.
- Split MCP child handling into focused client, session, pool, protocol, and buffer units while preserving the single `ManagedChildRuntime` interface.
- Removed blanket `cmd/` and `_windows.go` mutation exclusions and added a native Windows mutation gate that exclusively targets `_windows.go` production paths.
- Source archives now contain only manifest-listed files and are reopened for exact-set and digest verification before publication.
- Automated releases from `main`: semantic versions are inferred from Conventional Commit signals, tags are keyless-signed through GitHub OIDC/Sigstore, interrupted unpublished tags are recovered safely, and trusted GPG tag pushes remain supported.
- Hardened release dispatch with pre-checkout signed-tag validation, upgraded GitHub Actions to Node 24-compatible immutable pins, refreshed Playwright to 1.62.0, and cancelled superseded Verify runs.
- Made startup recovery mutation-lease-aware so a second process cannot reclassify live work, and made multi-root quarantine collision-free with preflight and rollback.
- Enforced single-value strict JSON decoding for catalogs and MCP configuration, and made dry-run planning surface invocation-resolution failures.
- Added direct semantic-version, integrity, session-environment, and strict-decoder coverage; established LF policy and a project-local seven-server Codex MCP/agent baseline.
- Removed Git executable-bit assumptions from CI and source-archive verification by invoking repository shell scripts explicitly through Bash.
- Replaced PowerShell-dependent Windows ACL handling with native Win32 security-descriptor capture, canonical SDDL comparison, post-rename verification, and rollback on metadata drift.
- Strengthened mutation coverage at resource-limit and preferred-provider boundaries without lowering the 75% efficacy threshold.
- Consolidated OS process liveness under `processctl`, hardened Unix process-group checks, added Linux process-start identity validation when available, and retained descendant cleanup after a group leader exits.
- Added catalog-defined Windows Job Object memory, CPU-rate, and active-process ceilings for managed MCP children; each child starts suspended and resumes only after successful Job Object assignment.
- Preserved destination POSIX modes and Windows DACLs during atomic managed-file replacement.
- Centralized Windows PATH comparison, added UTF-16LE/Base64 PowerShell transport, and preserved unrelated persistent user PATH entries byte-for-byte, including non-ASCII segments.
- Added a coherent Lucide navigation icon system and shared accessible busy-state feedback for every long-running manager operation.
- Added a validated external-report acceptance ledger separating accepted, revised, rejected, and release-gated claims.

- Bound apply authorization to expiring content-addressed plans and current catalog/inventory state.
- Added cross-process mutation leasing, incremental journals, interruption recovery, postcondition checks, indexed restore preview, and live post-restore MCP validation.
- Added exact source/version/publisher/platform catalog policy, compatibility probes, a minimal `core` default, and pinned skill-pack commit/inventory verification.
- Added bounded command/MCP output, timeouts, Windows Job Object containment, persistent MCP children, protocol negotiation, real MCP doctor probes, and conservative Codex/AGY registration repair.
- Added privacy retention/export/deletion, minimized inventory, recursive secret redaction, current-user data ACLs, sanitized diagnostics, correlation-aware events, and AgentStack-owned deactivate/remove/quarantine workflows.
- Added Windows x64/ARM64 end-to-end, accessibility, fuzz, mutation, critical-coverage, governance, documentation, SBOM, license, VEX, provenance, reproducibility, signing, and archive verification gates.
- Added one-time plan consumption, atomic skill publication, signed setup-pair runtime verification, canonical `.go-version` pinning, and schema-valid deterministic CycloneDX identities.
- Corrected assurance-tool pins to official `govulncheck v1.1.4` and source-built `Syft v1.50.0`.
- Added a gated GitHub Release workflow with signed-tag/manual dispatch, per-tag concurrency, job-scoped permissions, provenance/SBOM attestations, native ARM64 proof, checksum verification, and `gh release create` publication. CI now lints all workflows with pinned `actionlint v1.7.12`.

- Hardened the publication candidate: coverage-instrumented external adapters now redirect Go coverage into their private sandbox without inheriting the caller path or weakening successful-stderr rejection.
- Replaced the monolithic benchmark invocation with bounded, evidence-preserving per-case execution; optimized workspace schema admission to reduce `BenchmarkSearchMemory` from roughly 24 ms, 11.2 MiB, and 281k allocations to roughly 15-18 ms, 7.3 MiB, and 148k allocations on the audit host.
- Replaced absolute safety and one-click claims in the loopback UI with precise reviewed-plan, preservation, and explicit-authorization language; removed decorative emoji-dependent state communication and flattened promotional surfaces.
- Added weekly dependency automation for the `.codex` MCP runtime and reconciled publication, provenance, SQLite-preview, launch, and rollback documentation.

## 0.1.0 — 2026-07-31

- Initial preservation-first installer, catalog, local UI, CLI, and MCP router prototype.
