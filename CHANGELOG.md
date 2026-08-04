# Changelog

## Unreleased — forensic remediation

- Added Cross-Agent Capability Sharing Matrix (`#sharing`) for visibility into skill and MCP links.
- Added Resilience Error Inspector & Diagnostics Hub (`#errors`).
- Added Real-Time System Analytics Dashboard (`#overview`) displaying live CPU %, Memory MB (`runtime.ReadMemStats`), Goroutine counter, and Daemon Uptime (`time.Since(serverStartTime)`).
- Added 3-way Appearance Theme Switcher (`Auto` / `Dark` / `Light`) with local storage persistence.
- Added 1-Page Confirmation Modal Dialog (`#planModal`) for sealed plan review and authorization.
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

## 0.1.0 — 2026-07-31

- Initial preservation-first installer, catalog, local UI, CLI, and MCP router prototype.
