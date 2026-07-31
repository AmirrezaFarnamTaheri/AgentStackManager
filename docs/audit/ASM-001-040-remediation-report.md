# AgentStack Manager 0.2.0 Remediation Report

## Result

All 40 findings from the AgentStack Manager 0.1.0 forensic audit have a concrete source-level remediation and a named verification control. The product now fails closed when protected release evidence is unavailable.

This document distinguishes two meanings of completion:

- **Implementation addressed:** the remediation exists in source and is covered by local tests or static policy checks.
- **Release certified:** protected Windows, signing, vulnerability, accessibility, mutation, provenance, and repository-governance evidence has executed successfully for a clean signed tag.

No public release was created in this environment. Authenticode credentials, a trusted tag-signing key, GitHub protected-environment reviewers, the configured repository ruleset, Go 1.26.5 network tooling, and native Windows x64/ARM64 runners are external release prerequisites. The release workflow refuses to bypass them.

## Scope and isolation

- Authoritative baseline: commit `d196b8d25de524d5659b5e1f82902ed8327f04ee`.
- Remediation worktree: `remediation/close-40-findings`.
- Original workspace changes were not overwritten.
- No commit, push, tag, deployment, publication, package-registry upload, or external infrastructure mutation was performed.

## Closure summary

- Findings addressed in implementation: **40 / 40**.
- Locally verifiable closures: see `ASM-001-040-remediation.json`.
- Protected-release evidence gates: intentionally remain fail-closed until executed in the authorized release environment.
- Detailed one-to-one mapping: `ASM-001-040-closure.md`.

## Principal remediations

1. Content-addressed, expiring, single-use plan approval bound to catalog and machine inventory.
2. Cross-process leases, incremental minimized journals, interruption recovery, postcondition verification, and dependency suppression.
3. Current-user-only state ACLs, authenticated loopback UI endpoints, secure random session paths, request limits, and explicit mutation confirmation.
4. Exact package/source/publisher/version policy, compatibility probes, pinned skill-pack commit and inventory, and atomic skill publication.
5. Live MCP initialize/tools-list doctor probes, negotiated protocol version, persistent bounded child sessions, and Windows Job Object containment.
6. Conservative Codex/AGY registration classification, conflict refusal, owned repair, backup, and rollback.
7. Backup preview, digest and structure validation, live post-restore validation, and automatic rollback after failed validation.
8. Minimized persisted inventory, recursive redaction, age retention, privacy export/deletion, and sanitized diagnostics.
9. Ownership-scoped preview/deactivation/removal with path-root validation, symlink refusal, and skill quarantine.
10. Clean signed-tag release policy, canonical Go toolchain pin, current pinned scanners, reproducibility checks, Authenticode, deterministic archives, SBOM/license/VEX/provenance, attestation, and delayed publication until both Windows architectures pass.

## Local proof observed

- `go test ./...`: pass.
- `go test -race ./...`: pass.
- `go vet ./...`: pass.
- `gofmt -l .`: empty.
- Statement coverage: **61.4%**.
- Named critical-path coverage gate: pass.
- Six fuzz campaigns: pass.
- Governance contract: pass.
- Documentation contract: pass.
- JSON and workflow YAML parsing: pass.
- Bash syntax validation: pass.
- Secret-pattern scan: pass.
- Linux executable build: pass.
- Windows amd64 and arm64 console builds: pass.
- Windows amd64 and arm64 GUI-subsystem setup builds: pass.
- Every test package cross-compiles for Windows amd64 and arm64: pass.
- Compiled CLI smoke: catalog/profile counts, sealed plan, confirmation refusal, single-use plan replay rejection, data policy, and sanitized diagnostics: pass.

## Protected release gates still required

A production release is blocked until all of the following are observed on the clean tagged source:

1. The tag is annotated, signed by the imported trusted public key, and points exactly to protected `origin/main`.
2. Go 1.26.5 builds and `govulncheck v1.1.4` source/binary scans pass.
3. Syft v1.50.0 generates per-binary SBOMs.
4. Authenticode signing and timestamp verification pass for setup, console, and PowerShell launcher.
5. Native Windows x64 and ARM64 E2E tests pass, including ACLs, PATH, process trees, setup-pair verification, MCP, and client registration.
6. Playwright/axe accessibility and pinned mutation testing pass.
7. Deterministic archives, internal and top-level hashes, OpenVEX, license inventory, in-toto/SLSA provenance, and GitHub attestations validate.
8. The declared GitHub ruleset and protected release environment are applied and verified against the actual repository.

## Verdict

**Implementation complete; release certification pending protected external gates.**

The 0.1 release should not be reused. The remediated source is the correct basis for the next clean, signed, evidence-producing release candidate.
