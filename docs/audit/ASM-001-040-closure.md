# ASM-001–ASM-040 Closure Ledger

This ledger maps every 0.1 forensic-audit finding to its remediation and verification gate. “Implemented” means the source control is present and locally testable. “Release-gated” means public delivery is blocked until the protected Windows/signing/CI environment produces the listed evidence; no unsupported fallback is permitted.

| ID | Status | Remediation evidence | Verification / release evidence |
|---|---|---|---|
| ASM-001 | Implemented | `internal/planner`, `internal/integrity`, `internal/state` saved plans, `Service.ApplyPlanned`, UI/CLI plan identity | altered, expired, catalog-drifted, inventory-drifted, pre-mutation consumption, and replay-rejection tests |
| ASM-002 | Release-gated | `scripts/release.ps1` requires clean HEAD exactly at a verified signed annotated tag, imports the protected verification key, requires the tag commit to equal fetched `origin/main`, and asserts `vcs.modified=false`/revision | release script build metadata checks; protected tag workflow |
| ASM-003 | Release-gated | release/verify workflows and release script require Go 1.26.5 and pinned `govulncheck`; source and binary scans | CI/release vulnerability artifacts; release refuses older Go |
| ASM-004 | Release-gated | setup embeds console SHA-256 and publisher thumbprint; `VerifyReleasePair`; PowerShell launcher and release require valid Authenticode | tamper tests plus signed Windows release verification |
| ASM-005 | Implemented | skill pack catalog pins full Git commit and expected entries; installer fetches and verifies resolved commit before copy | `internal/skills` and catalog lock tests |
| ASM-006 | Implemented | runner postcondition verifier rescans each successful install and records success only when expected state exists | runner/inventory postcondition tests |
| ASM-007 | Implemented with explicit boundary | high-entropy session path/token on every endpoint, origin/content/rate gates, current-user-only state ACL | UI authorization tests and Windows ACL E2E; docs disclose same-user malware residual |
| ASM-008 | Release-gated | `.github/workflows/verify.yml` native `windows-2025` x64 and `windows-11-arm` ARM64 jobs; `scripts/windows-e2e.ps1` | release is blocked unless both required checks pass |
| ASM-009 | Implemented | PID/instance-aware cross-process mutation lease in `internal/state/lease.go` used by apply/restore/lifecycle | lease contention/stale recovery tests |
| ASM-010 | Implemented | MCP doctor performs real `initialize` and `tools/list`, reports per-child failure, and caches briefly | fake healthy/failing MCP tests |
| ASM-011 | Implemented | catalog version policies and bounded probes; planner preserves incompatible runtime unless explicit exact upgrade consent | inventory/planner compatibility tests |
| ASM-012 | Implemented | Codex/AGY classification: absent/equivalent/owned-stale/foreign-conflict/read-failure; backup and owned repair only | registration conflict/error/repair tests |
| ASM-013 | Implemented | invocation-specific timeouts for inventory, install, warm, doctor, session, and lifecycle operations | timeout/dependency-suppression tests |
| ASM-014 | Implemented; Windows release-gated | Unix process groups and Windows Job Objects with kill-on-close, timeout and graceful-close escalation | parent/grandchild process-tree tests on Linux and both Windows CI architectures |
| ASM-015 | Release-gated | signed release workflow builds and attests actual ZIPs, ARM64 consumes and verifies the produced archive, and publication is a separate dependent job that cannot run before both architectures pass | archive verification and protected release checks |
| ASM-016 | Implemented | structured WinGet JSON parsing/reconciliation plus minimized external inventory | inventory parser and adoption tests |
| ASM-017 | Implemented | `custom` has no defaults/providers; provider selection derives only from selected components/explicit override | planner/UI custom-profile tests |
| ASM-018 | Implemented | credential components are explicitly installable or `guidedSetup`; each has login hint, official HTTPS documentation URL, status and verification instructions | catalog validation plus `agentstack integrations` tests |
| ASM-019 | Implemented | transaction saved before execution and after every action, including skips and dependency suppression; journal failures fail the transaction; startup recovers interrupted running state | journal/recovery tests |
| ASM-020 | Implemented | persisted inventory and transaction journals are minimized; command arguments/stdout/stderr are omitted, nested secrets are redacted, and export/clear/policy commands plus age retention cover plans, transactions, diagnostics and events | state privacy/retention/export/delete tests |
| ASM-021 | Implemented | all UI reads and mutations require token; secret per-process session URL; request rate/body/content/origin gates | unauthorized GET/POST and security-header tests |
| ASM-022 | Implemented | router negotiates a supported protocol from client initialization and rejects invalid lifecycle/order | MCP negotiation/initialization tests |
| ASM-023 | Implemented | critical-path coverage script requires named authorization/install/restore/lifecycle/process/session/UI functions to be exercised; Windows E2E supplements unit coverage | `scripts/check-critical-coverage.sh`, CI coverage artifact |
| ASM-024 | Release-gated | catalog-wide schema-valid deterministic CycloneDX generator, per-architecture Syft SBOMs attested against the exact raw signed binaries they describe, license inventory, govulncheck OpenVEX, valid in-toto/SLSA provenance, immutable action/image pins, and GitHub attestations | supply-chain tests and signed-release artifacts |
| ASM-025 | Implemented | rewritten user/security/release/privacy/operations/governance docs plus static drift check in CI/release | `scripts/check-docs.sh/.ps1` |
| ASM-026 | Implemented | indexed backup list, digest/target/structure preview, confirmed restore, live post-restore MCP validation, and automatic rollback on failed validation | state/app/CLI restore tests |
| ASM-027 | Implemented | claim narrowed to two-build unsigned binary reproducibility; deterministic ZIP metadata; clean tag/source archive | release script compares binaries and releasepack deterministic tests |
| ASM-028 | Implemented | pooled persistent MCP child client with bounded graceful shutdown and invalid-child eviction | reuse/lifecycle benchmark and tests |
| ASM-029 | Implemented | local structured redacted events with correlation IDs, duration/status and MCP child observer; sanitized diagnostics | event redaction/rotation and diagnostic bundle tests |
| ASM-030 | Implemented | token/session RNG failure aborts UI startup; no predictable fallback | RNG fault tests |
| ASM-031 | Implemented | public release target is Windows only; catalog has platform declarations; release/build docs no longer ship a Linux product | catalog platform tests and docs contract |
| ASM-032 | Release-gated | semantic HTML, skip link, keyboard focus, live regions, reduced motion; Playwright + axe WCAG automation | required Windows accessibility CI check |
| ASM-033 | Implemented | fuzz targets for catalog, planner, state, safe files, MCP; pinned Gremlins mutation job and quality gate | fuzz seed campaign and mutation artifact |
| ASM-034 | Implemented; Windows release-gated | Windows DACL applies/audits current-user-only access; POSIX mode `0700` | native Windows x64/ARM64 ACL tests |
| ASM-035 | Implemented | exact WinGet ID/version/source/publisher; exact uv/npm versions/sources/publishers; catalog rejects floating acquisitions | catalog supply-chain validation and runner argument tests |
| ASM-036 | Implemented | bounded MCP message/stdout/stderr, synchronized request handling, deadlines, process-tree termination | oversized output/message and failure tests |
| ASM-037 | Implemented | every inventory refresh reconciles managed router/paths/skills and updates ownership health/version/last verification | ownership health and repair classification tests |
| ASM-038 | Implemented | ownership-scoped preview/deactivate/remove; provider uninstall commands; skill quarantine; manual/unowned refusal | lifecycle tests and explicit `--yes` CLI gates |
| ASM-039 | Implemented | minimal six-component `core` default; broad `essential` retained only as a labeled legacy compatibility profile | catalog profile tests and documentation |
| ASM-040 | Implemented | CLI help and docs enumerate all ten catalog-driven profiles dynamically | CLI options/help tests and docs contract |

## Release decision

A public release is **fail-closed** until all entries marked release-gated have observed evidence from the protected workflows: signed clean tag, Go 1.26.5/vulnerability scans, valid Authenticode signatures, x64 and ARM64 Windows runtime/ACL/process tests, accessibility automation, actual-archive verification, SBOM/VEX/license/provenance, and attestation. The implementation does not downgrade or bypass those conditions when credentials, hardware, or repository administration are unavailable.
