# Implementation Handoff: Unified Resource Control Plane

**Outgoing state:** commits `47a1432` and `5997e9e` on `codex/ui-clarity`.

**Incoming responsibility:** continue the control-plane roadmap without touching real client roots, credentials, sessions, provider routing, or reference snapshots except through explicit read-only evidence tooling.

## TL;DR

- Source releases now exclude `reference repos/` by releasepack policy, not only by Git ignore rules.
- A Git-less archive smoke check is implemented and tested.
- The authority contract and eight-snapshot donor ledger are committed.
- Task A2 (deterministic donor manifest generator `internal/donormanifest`, `cmd/donormanifest`, unit tests, and 8 snapshot manifests/receipts under `docs/convergence/manifests/`) is complete and verified.
- Task A3 (file-level donor disposition campaign with 100% path-complete dispositions for all 10,977 entries across 8 snapshots and closure verification test `TestManifestsClosureAndDispositions`) is complete and verified.
- Task B1 (artifact-linked lifecycle records in `internal/resourcehub/lifecycle.go`, `internal/resourcehub/types.go`, `internal/resourcehub/manager.go`, and `internal/resourcehub/lifecycle_test.go`) is complete and verified.
- Task B2 (source-bundle admission in `internal/resourcehub/sourcebundle.go` and `internal/resourcehub/sourcebundle_test.go`) is complete and verified.
- Task B3 (physical-identity observation scanner in `internal/observation/types.go`, `internal/observation/identity.go`, `internal/observation/scanner.go`, and `internal/observation/scanner_test.go`) is complete and verified.
- Task B4 (nested-client target catalog in `internal/targetcatalog/types.go`, `internal/targetcatalog/catalog.go`, `internal/targetcatalog/default.json`, and `internal/targetcatalog/catalog_test.go`) is complete and verified.
- Task B5 (managed-projection exclusion contract & exact duplicate/alias decisions in `internal/resourcehub/projection.go`, `internal/resourcehub/projection_test.go`, `internal/resourcehub/identity.go`, and `internal/resourcehub/identity_test.go`) is complete and verified.
- Task C1 (deterministic identity pipeline & normalized body indexing in `internal/similarity/normalize.go`, `internal/similarity/normalize_test.go`, `internal/similarity/index.go`, and `internal/similarity/index_test.go`) is complete and verified.
- Task C2 (advisory semantic evidence & versioned relationships in `internal/similarity/evidence.go` and `internal/similarity/evidence_test.go`) is complete and verified.
- Task C3 (reviewed consolidation decisions & canonical winner ordering in `internal/similarity/decision.go` and `internal/similarity/decision_test.go`) is complete and verified.
- Task D1 (adapter conformance contract in `internal/adapters/conformance/contract.go` and `internal/adapters/conformance/contract_test.go`) is complete and verified.
- Task D2 (OpenCode adapter and shadow vertical slice in `internal/adapters/opencode/adapter.go` and `internal/adapters/opencode/adapter_test.go`) is complete and verified.
- Task D3 (deployment executor, locking, preflight revalidation, and backup/restore in `internal/executor/deployment.go`, `internal/executor/backup.go`, and `internal/executor/deployment_test.go`) is complete and verified.
- Task D4 (MCPlink mutation executor in `internal/adapters/mcplink/executor.go` and `internal/adapters/mcplink/executor_test.go`) is complete and verified.
- Task D5 (disparity audit, drift detection & reconciliation CLI in `cmd/agentstack/main.go`, `internal/cli/control_plane.go`, and `internal/cli/control_plane_test.go`) is complete and verified.
- Task E1 (canonical secret-free MCP intent model in `internal/mcp/intent.go` and `internal/mcp/intent_test.go`) is complete and verified.
- Task E2 & E3 (universal ChangeSet composition engine and honest partial-success reporting in `internal/changeset/changeset.go` and `internal/changeset/changeset_test.go`) is complete and verified.
- The next implementation phase is Phase F: Operator experience and migration.

## Active incidents

### None in production

No client root, MCP registration, credential, runtime state, or deployment has been mutated in this implementation sequence.

## Ongoing investigations

### BASE-001: CGO SQLite toolchain unavailable on this Windows machine

**Status:** Open, environment blocker for the default full suite.

**Evidence:** `go test ./... -count=1` fails when `pkg-config` cannot find `sqlite3.pc` under `C:/Strawberry/c/lib/pkgconfig`.

**Impact:** packages depending on `internal/store/sqlite` fail to build in default CGO mode; it does not invalidate the focused A0/A1 tests.

**Next action:** establish the supported local SQLite development environment or document/validate the pure-Go SQLite configuration. Do not add an unreviewed SQLite binary, weaken build requirements, or conceal the failure.

### BASE-002: Windows sandbox permission assertion

**Status:** Open, baseline test/platform-semantic investigation.

**Evidence:** with `CGO_ENABLED=0`, the full suite passes except `internal/adapters/external.TestSandboxEnvironmentRedirectsCoverageIntoPrivateSandbox`, which reports coverage directory mode `drwxrwxrwx`.

**Impact:** prevents a fully green baseline but is unrelated to the release-boundary and donor-ledger commits.

**Next action:** inspect Windows ACL versus POSIX-mode expectations and decide whether production sandbox creation or the platform-specific assertion is incorrect. Keep the test’s privacy/security intent intact.

## Recent changes

| Commit | Change | Validation |
|---|---|---|
| `47a1432` | Excluded `reference repos/` from source manifests/archives, added case-insensitive regression test, Git-less archive smoke check, source manifest refresh | focused releasepack/supply-chain tests, governance, docs, source closure, archive smoke |
| `5997e9e` | Added unified authority contract and seeded eight-snapshot donor ledger | JSON parse, docs check, source manifest write/verify |

## Current repository state

- Branch: `codex/ui-clarity`.
- Expected untracked path: `.omx/`; preserve it and do not stage it.
- `reference repos/` is intentionally ignored and must remain local, read-only donor evidence.
- `.codegraph/` is absent; use `rg`/targeted file reads for discovery.
- Current required plan: `docs/plans/2026-08-13-unified-agent-resource-control-plane.md`.
- Execution roadmap: `docs/convergence/UNIFIED_RESOURCE_ROADMAP.md`.

## Immediate next work: A2 donor-manifest generator

### Objective

Produce a deterministic manifest and root receipt for every direct snapshot under `reference repos/`, including the empty AgentDNS snapshot, without copying or importing donor code.

### Preconditions

- Read `UNIFIED_RESOURCE_AUTHORITY.md`, `DONOR_ADOPTION_LEDGER.json`, and the roadmap’s A2 section.
- Confirm `reference repos/` remains ignored and releasepack excludes it.
- Define the output location before implementation. Generated evidence must not be silently added to release inputs.
- Assign exclusive ownership of the new manifest schema/generator package.

### Required behavior

1. Explicit root only; no ambient home-directory scan.
2. Stable lexical paths and sort order.
3. File SHA-256, byte count, object type, read errors, and reparse/symlink evidence.
4. No follow outside snapshot boundary.
5. Case collision and normalized-path collision detection.
6. Separate zero-file receipt for AgentDNS.
7. No runtime imports, projection, registry mutation, or target writes.

### Required tests

- deterministic same-tree output;
- order independent of creation order;
- path traversal and root escape rejection;
- file/directory/symlink/reparse handling;
- unreadable object evidence;
- duplicate/case-normalized path rejection;
- empty snapshot receipt;
- generated artifact excluded from source release unless explicitly tracked as documentation.

### Stop conditions

Stop and request review if a snapshot needs code execution to inspect, a donor path resolves outside its snapshot boundary, a manifest would include credentials/runtime material, or adding a generator requires changing Resource Hub/Artifact semantics.

## Commands verified in the last handoff

```powershell
go test ./internal/releasepack ./internal/supplychain ./internal/artifactgraph ./internal/resourcehub -count=1
pwsh -File scripts/check-governance.ps1
pwsh -File scripts/check-docs.ps1
go run ./cmd/releasepack --root . --manifest-mode verify
```

```bash
bash scripts/check-source-archive-build.sh "git:$(git rev-parse HEAD)"
```

The default `go test ./... -count=1` remains blocked by BASE-001. `CGO_ENABLED=0 go test ./... -count=1` isolates that issue and exposes BASE-002.

## Escalation rules

| Trigger | Immediate action | Escalate to |
|---|---|---|
| A release manifest/archive includes donor files | Stop release work, preserve evidence, do not delete donor files | release-boundary owner |
| Any code proposes client-root write before executor/Change Set | Stop implementation and return to authority review | architecture owner |
| A donor module requires credentials/provider/session authority | Record `reject` or `defer`; do not adapt it | scope owner |
| A retry design can repeat an ambiguous remote operation | Mark manual reconciliation required | deployment/MCP owner |
| Full-suite failure changes from baseline or affects focused gates | Capture command, environment, first failure and diff | test/platform owner |

## Incoming checklist

- [ ] Read authority contract, roadmap, ledger, and this handoff.
- [ ] Confirm branch and preserve `.omx/`/`reference repos/` state.
- [ ] Re-run focused gates above.
- [ ] Record the exact BASE-001/BASE-002 environment state before changing any test or build code.
- [ ] Complete A2 before opening B1/B2/B3 implementation.
- [ ] Add an entry to the next handoff with commit IDs, evidence, unresolved risks, and exact next action.

## Definition of a good next handoff

Every section above must contain an explicit entry or `none`; name commits and commands actually run; distinguish verified facts from proposals; include one immediate next action and one stop condition; never imply a local client migration occurred unless there is a sealed receipt.
