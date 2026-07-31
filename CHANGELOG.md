# Changelog

## Unreleased — forensic remediation

- Hardened release dispatch with pre-checkout signed-tag validation, upgraded GitHub Actions to Node 24-compatible immutable pins, and refreshed Playwright to 1.62.0.
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
