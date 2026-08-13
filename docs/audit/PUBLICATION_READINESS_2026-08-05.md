# Publication Readiness Audit — 2026-08-05

## Verdict

**Source candidate ready for protected release CI.** The cumulative Phase 1-6 source and final hardening pass are coherent, testable, reversible, and prepared for the repository's signed-release workflow. Production publication still depends on the protected Go 1.26.5, native Windows, mutation, vulnerability, signing, attestation, and release-approval jobs. This workspace does not possess those hosted identities or credentials.

## Scope

The audit treated the Phase 6 archive as the cumulative baseline for all earlier Fabric phases and reviewed:

- canonical artifacts, CAS, migration receipts, SQLite shadow metadata, in-process adapters, embedded conformance, and constrained external adapter execution;
- reviewed mutation authority, recovery, path confinement, process containment, and secret handling;
- Go package layout, error contracts, tests, race coverage, critical coverage, fuzz targets, benchmarks, and release scripts;
- GitHub Actions, CircleCI, Dependabot, source manifests, source archives, SBOM/VEX/signing workflows, launch and rollback documents;
- the embedded loopback web UI's accessibility structure, responsive CSS, copy, CSP, token authorization, and explicit-confirmation workflow.

The unrelated `stata-workbench-2.3.1-impeccable-detect.json` file was excluded by user instruction and did not affect this audit.

## Confirmed fixes

### 1. Coverage-safe external adapter isolation

Repository-wide coverage instrumented helper binaries require `GOCOVERDIR`. The external adapter host deliberately constructs a private environment, so instrumented children previously wrote a warning to standard error and failed the host's strict successful-stderr rule.

The host now creates a private `0700` coverage directory inside the admitted sandbox and passes only that path to the child. It does not inherit the caller's coverage location or relax response validation. A regression test proves the redirected path is private and bounded.

### 2. Workspace persistence allocation reduction

`loadCollection` previously decoded every store twice: first into `map[string]json.RawMessage` to discover the schema, then into the versioned envelope. It now performs a lightweight schema-header admission and retains the strict full decode for duplicate-key, trailing-data, and structural validation.

On the audit host, `BenchmarkSearchMemory` improved from approximately 23.6-24.4 ms, 11.16 MiB, and 281k allocations per operation to approximately 14.8-16.6 ms, 7.30 MiB, and 148k allocations. Duplicate keys remain rejected in both versioned and legacy stores.

### 3. Bounded benchmark evidence

The previous benchmark gate launched every package through one `go test` process. The audit environment repeatedly stalled during package-build/cache transitions even though every benchmark completed independently. The gate now defines one bounded case per benchmark, fails each case independently, and retains an input-only validation mode for independently collected evidence. The ceiling checks and five-sample requirement remain unchanged.

### 4. Review-centered UI language

The loopback UI no longer claims "100% safe," "one click," or an unconditional safety guarantee. It describes the actual mechanism: detection, sealed-plan review, preservation of unrelated software, and explicit authorization. Decorative emoji-dependent status communication was replaced with text, and promotional card shadows/accent treatment were flattened to match the Conservation Ledger design system.

### 5. Dependency and release-document reconciliation

Dependabot now covers the `.codex` npm runtime as well as UI assurance and GitHub Actions. README, delivery status, provenance, changelog, SQLite-preview language, and the publication decision map now describe the same authority and release state.

## Audit checklist

| Area | Status | Evidence | Confidence |
| --- | --- | --- | --- |
| Tooling and CI drift | Healthy with protected gate | Workflows and CircleCI use `.go-version`; actions are SHA-pinned; release scripts are confirmation- and identity-gated | High |
| Documentation vs reality | Reconciled | Historical base, SQLite CGO behavior, authority boundaries, launch gates, and rollback state now align | High |
| Config and environment | Healthy | Loopback binding, random session token, strict CSP, bounded JSON, private state paths, explicit CGO fallback | High |
| Dependency health | Ready for hosted rescan | No Go module dependencies; offline npm audits report zero known findings; protected `govulncheck` remains mandatory | Medium |
| Test health | Healthy | Native and CGO-disabled suites, race packages, critical coverage, governance, documentation, and source-manifest gates are defined and exercised | High |
| Performance | Within budgets | All eight benchmarks provide five-sample evidence below existing ceilings; workspace search allocation pressure was reduced | High |
| Package and release | Ready for protected workflow | Deterministic source archive, exact manifest, Windows reproducibility, SBOM/VEX/signature/attestation jobs exist | High |
| Embedded UI | Good, hosted browser proof required | Semantic HTML, keyboard/focus tests, reduced motion, 320-1280 px matrix, CSP, no external runtime assets; local browser install unavailable | Medium |
| Compose | Not applicable | No Kotlin or Jetpack Compose source exists in the product | High |

## Security findings

| Severity | Finding | Disposition |
| --- | --- | --- |
| Critical | None confirmed in reachable product code | — |
| High | Local Go 1.23.2 lacks security fixes present in pinned Go 1.26.5 | Publication is restricted to protected CI using `.go-version`; local binaries are not release evidence |
| Watch | Phase 6 external execution is process-constrained, not a network/filesystem/syscall sandbox | External activation remains blocked; the CLI is differential conformance only |
| Watch | Public Windows binaries are CGO-disabled and therefore do not expose native SQLite commands | Documented fail-closed preview behavior; all established ASM features remain available |
| Info | Local `govulncheck`, signing, attestation, Windows native execution, and Playwright browser installation were unavailable | Protected workflows contain the required gates; publication must stop if any fail |

High-signal scans found no hardcoded secret-shaped production assignments, disabled TLS verification, permissive production file modes, shell-string command execution, or SQL built from untrusted strings. The only production `panic` is the reviewed built-in adapter registry's invariant-enforcing `MustRegistry` constructor.

## UI audit score

| Dimension | Score | Evidence boundary |
| --- | ---: | --- |
| Accessibility | 3/4 | Strong semantics, focus, keyboard, reduced-motion, and automated CI harness; local browser execution unavailable |
| Performance | 4/4 | Embedded local assets, no third-party runtime requests, bounded DOM, no scroll listeners or large media |
| Responsive design | 3/4 | Explicit 1120/800/520 topology changes and CI viewport matrix; local visual browser proof unavailable |
| Theming | 4/4 | Tokenized auto/light/dark themes with restrained semantic color |
| Implementation integrity | 4/4 | Product-specific Conservation Ledger system, reviewed authority language, and no generic marketing shell |
| **Total** | **18/20** | **Excellent; protected browser evidence remains a publication gate** |

## Architecture assessment

The release should not absorb a broad refactor. Four post-release deepening opportunities were identified:

1. Deepen Fabric CLI command parsing behind one registry-driven module interface.
2. Deepen workspace persistence and recovery behind one store interface.
3. Deepen reviewed Resource Hub/MCP mutation orchestration while preserving separate adapters and authorities.
4. Deepen loopback UI route registration and response/error handling behind a compact local-transport module.

The first recommended post-release candidate is workspace persistence because it offers the strongest locality and leverage while retaining a narrow interface and measurable benchmark/test surface.

## Release and rollback decision

The exact source candidate may proceed to protected CI. Publication requires every gate in `docs/LAUNCH_READINESS.md` to pass against one clean signed revision. The release workflow must promote the same verified bytes and retain the previous signed release as the rollback target. Any source identity, signature, native architecture, data integrity, orphan-process, vulnerability, or mutation-efficacy failure stops publication.
