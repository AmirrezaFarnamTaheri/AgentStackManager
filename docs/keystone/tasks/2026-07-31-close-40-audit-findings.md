# Task Creation: Close all 40 AgentStack forensic-audit findings

## Goal
Transform AgentStack Manager from an evaluation prototype into a release candidate whose authorization, recovery, process, supply-chain, privacy, accessibility, and release claims are backed by executable controls and verifiable evidence.

## Context inspected
- `AgentStackManager-0.1.0-findings.json` (40 findings)
- `internal/app`, `catalog`, `inventory`, `mcp`, `planner`, `runner`, `selfinstall`, `state`, `ui`
- release scripts, CircleCI config, user/security/release documentation
- baseline: `go test ./...`, `go test -race ./...`, and `go vet ./...` pass at commit `d196b8d`

## Requirements inventory
### Critical requirements
- Bind apply to the exact reviewed plan and current inventory/catalog revision.
- Preserve unrelated/adopted software and configuration.
- Provide recoverable, cross-process-safe mutations and restoration.
- Verify installations and MCP children behaviorally, not by exit/path presence alone.
- Pin and attest every automatically acquired artifact.
- Build distributable archives only from clean, tagged source with supported Go.
- Make the Windows setup verify its console sibling and support signing.
- Add Windows x64/ARM64 executable evidence or block unsupported targets.

### Non-functional requirements
- Local-first, telemetry off, bounded/redacted logs, user-isolated state.
- Timeouts, cancellation, process-tree containment, and bounded I/O.
- Protocol negotiation, compatibility/version policy, and deterministic diagnostics.
- Accessible keyboard/screen-reader workflow and current documentation.
- Reproducible archives, SBOM, licenses, VEX, provenance, and governance controls.

### Constraints
- Existing dirty edits in the original checkout are protected by an isolated worktree.
- No signing certificate or Windows ARM64 runner is available locally; automation and fail-closed release gates must be implemented, with evidence generated where environment permits.
- Go standard-library-only runtime should be retained unless a dependency has clear assurance value.

## Iteration layering
### Iteration 1: Authorization, state, and recovery core
- Immutable server-side plans with digest/expiry/revision binding.
- Cross-process mutation lease, incremental transaction journal, recovery, backup index/restore.
- Postcondition verification, version compatibility, ownership reconciliation.

### Iteration 2: Process, protocol, and platform reliability
- Bounded command execution/output, process-tree abstraction, MCP lifecycle negotiation and doctor probes.
- Safe registration conflict handling, persistent MCP workers with idle TTL, structured diagnostics.
- Structured WinGet inventory and platform-aware catalog behavior.

### Iteration 3: Supply-chain, release, privacy, and UX assurance
- Locked catalog metadata, pinned skill pack, sibling hash/signature verification, clean-tag release gates.
- SBOM/licenses/VEX/provenance generation, signed-release workflow, Windows E2E matrix.
- Data minimization/export/clear, user-bound UI transport, accessibility/fuzz/property coverage, dynamic help, lifecycle commands.

## Vertical slices
1. **Plan-bound mutation contract** — ASM-001, 009, 019, 021, 030
   - Acceptance: altered/stale plan is rejected; one mutation lease per user; journal survives interruption; all UI endpoints require capability; RNG failure aborts startup.
   - Verification: API contract, concurrency, expiry, restart-recovery, and fault-injection tests.

2. **Verified component lifecycle** — ASM-006, 011, 016, 018, 026, 037, 038, 039, 040
   - Acceptance: versions/postconditions determine state; WinGet packages reconcile; guided integrations are labeled; restore/deactivate/remove-owned workflows preserve adopted software; profiles/help are catalog-driven.
   - Verification: mock package-manager E2E, ownership/restore tests, profile snapshots.

3. **MCP and process reliability** — ASM-010, 012, 013, 014, 022, 028, 036
   - Acceptance: doctor performs initialize/tools-list; protocol is negotiated; stale owned registrations repair safely; subprocesses have deadlines, bounded output, graceful shutdown, process-tree cleanup, and optional idle workers.
   - Verification: fake MCP servers, large-output/failure/race tests, grandchild cancellation tests, latency benchmarks.

4. **Local security, privacy, observability, accessibility** — ASM-007, 020, 029, 032, 034
   - Acceptance: user-bound local transport or equivalent ACL gate; minimized persistence with clear/export; bounded redacted event log/diagnostic bundle; Windows ACL verification; automated and manual accessibility evidence.
   - Verification: token/ACL tests, export/delete tests, support-bundle redaction, axe/keyboard CI.

5. **Supply-chain and release integrity** — ASM-002, 003, 004, 005, 015, 024, 027, 031, 035
   - Acceptance: supported Go, clean tag, deterministic archives, exact package source/version/publisher/digest, pinned skill content, verified sibling manifest, SBOM/licenses/VEX/provenance, signing hooks, no unsupported Linux artifact.
   - Verification: two-build comparison, `go version -m`, `govulncheck`, catalog policy tests, tamper tests, schema validation, signature verification in release CI.

6. **Assurance and governance closure** — ASM-008, 023, 025, 033
   - Acceptance: Windows x64 E2E, ARM64 gated on evidence, release-critical behavior matrix, fuzz/property/mutation jobs, documentation drift controls, protected release workflow documentation.
   - Verification: CI artifacts/reports, fuzz seeds, coverage mapping, docs contract tests, release-gate summary.

## Risks and dependencies
- Authenticode completion requires a trusted certificate and timestamp service; unsigned artifacts must be blocked from public release.
- ARM64 support depends on a genuine Windows ARM64 runner or hardware; otherwise the target remains disabled.
- WinGet source/version metadata may vary by package; catalog entries that cannot be locked must be moved to guided/manual setup.
- MCP persistent workers increase lifecycle complexity; one-shot remains the default for untrusted servers.

## Handoff
Next module: `implementation` because the requirements and six vertical slices are defined and every slice has executable verification gates.
