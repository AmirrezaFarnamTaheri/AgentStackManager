# Unified Resource Control Plane Roadmap

## Purpose and operating boundary

This is the execution roadmap for the plan in `docs/plans/2026-08-13-unified-agent-resource-control-plane.md`. It converts the architecture into independently reviewable delivery increments. It governs shareable agent resources only: skills, agents, prompts, rules, commands, workflows, inert hook definitions, MCP declarations, and target projections.

It does not authorize mutation of local client roots until the relevant reviewed Change Set and deployment-executor gates exist. It does not include credentials, provider routing, subscriptions, sessions, caches, logs, runtime plugins, worktrees, agent execution, or arbitrary IDE configuration.

## Current program position

| Gate | Status | Evidence |
|---|---|---|
| G0, release boundary | Complete | `47a1432`; source releases exclude `reference repos/` and pass Git-less archive verification |
| G1, authority seed | Complete | `5997e9e`; authority contract and eight-snapshot donor ledger committed |
| G1.1, exhaustive donor evidence | Complete | Deterministic donor manifest generator (`internal/donormanifest`, `cmd/donormanifest`), unit tests, and 8 snapshot manifests/receipts generated in `docs/convergence/manifests/` |
| G2, safe observation | Not started | No new scanner, source-bundle, physical-identity graph, or target catalog yet |
| G3, identity/consolidation | Not started | No new canonicalization engine yet |
| G4, OpenCode projection/executor | Not started | No target filesystem writes authorized |

## Global readiness rules

Each work item starts only when its input contract, owner, fixture set, failure behavior, and acceptance check are explicit. Each work item completes only when it has a focused test, a normal path, a failure path, an integration edge, updated source manifest if tracked source changed, and no bypass of the authority contract.

No work item may:

- copy a donor repository wholesale;
- make `.agents`, a client directory, or an adapter a canonical authority;
- import generated projections as sources;
- weaken a precondition for retry convenience;
- expand into credential, provider, session, or runtime orchestration scope.

## Phase A — Evidence closure and authority hardening

### A0. Source-release boundary — complete

Delivered in `47a1432`.

1. Exclude `reference repos/` through releasepack traversal, using case-insensitive root matching.
2. Add manifest and packed-archive regression coverage.
3. Implement a Git-less staged-source smoke check.
4. Guard against an empty archive-check script in governance validation.
5. Regenerate and verify `SOURCE_MANIFEST.sha256`.

Acceptance: snapshot trees cannot enter manifests or source archives; archive construction works without `.git`; existing workstation snapshots are untouched.

### A1. Authority and donor seed — complete

Delivered in `5997e9e`.

1. Define Resource Hub, Artifact, CAS, index, scanner, adapter, executor, `mcplink`, Change Set, and client-root authority boundaries.
2. Record all eight known snapshots: CC Switch desktop/CLI, skills-manager, skills-manage, Skill Zoo, Agent of Empires, Agent Skill Library, and AgentDNS.
3. Record module-level `adapt`, `fixture`, `inspire`, and `reject` decisions.
4. Explicitly mark AgentDNS as `unverified-no-payload`.

Acceptance: no donor is treated as a runtime dependency or direct canonical source.

### A2. Deterministic donor manifest generator — complete

Delivered in `cmd/donormanifest` and `internal/donormanifest`.

1. Create a strict versioned donor-manifest schema with snapshot ID, root path, file path, object type, byte count, SHA-256, symlink/reparse evidence, and disposition.
2. Implement an offline generator that accepts only an explicit reference-repo root and writes deterministic, sorted output outside the donor tree.
3. Reject traversal, duplicate normalized paths, unreadable files, external symlink/reparse targets, and unexpected object types; report them as evidence instead of following them.
4. Generate a root receipt containing inventory time, manifest digest, source physical identity, aggregate file/byte counts, and empty-snapshot status.
5. Generate a manifest for each of the eight snapshots, including a zero-file AgentDNS receipt.
6. Add tests for sorting, case collision, root escape, symlink/reparse handling, empty snapshot, and determinism.

Acceptance: every file in every snapshot has one stable manifest record or an explicit read error; generated manifests are evidence artifacts, never release inputs or runtime sources.

### A3. File-level donor disposition campaign — complete

Delivered in `cmd/donormanifest`, `DONOR_ADOPTION_LEDGER.json`, and `docs/convergence/manifests/`.

1. Partition each manifest by module boundary, language, generated/vendor classification, and capability family.
2. Assign every file one disposition: `adopt`, `adapt`, `fixture`, `inspire`, `defer`, `reject`, `duplicate-of`, or `not-applicable`.
3. For every adopt/adapt/fixture record, name an ASM destination, invariant, peer-comparison ID, and regression fixture.
4. For every reject/defer record, name the boundary or missing prerequisite that prevents adoption.
5. Add a ledger closure check: no unknown disposition, no orphan module, no adopted file without destination, no destination without a test plan.

Acceptance: donor review is path-complete and machine-checkable; “reviewed” never means README-only inspection.

## Phase B — Canonical model and observation safety

### B1. Artifact-linked lifecycle records — complete

Delivered in `internal/resourcehub/lifecycle.go`, `internal/resourcehub/types.go`, `internal/resourcehub/manager.go`, and `internal/resourcehub/lifecycle_test.go`.

1. Inventory existing `artifactgraph.Artifact` and Resource Hub records.
2. Add observation, candidate revision, alias, lifecycle, source, preservation, and retirement records that reference artifact identity/digest.
3. Preserve version-1 decoding; reject unknown future versions; never silently promote an observation.
4. Define lifecycle transitions: observed → parsed → classified → candidate → canonical → projected → verified, plus ignored/quarantined/alias/retired exits.
5. Bind every transition to actor, reason, evidence digest, timestamp, and prior state.

Tests: illegal transitions; legacy decoding; candidate revision after source edit; alias collision; retirement reversal; deterministic sealing.

### B2. Source-bundle admission — complete

Delivered in `internal/resourcehub/sourcebundle.go` and `internal/resourcehub/sourcebundle_test.go`.

1. Support explicit directory and ZIP registration only.
2. Enforce archive member, uncompressed byte, compression-ratio, path, duplicate, symlink, nested archive, and total-size budgets.
3. Store immutable bundle bytes/tree evidence in CAS with a sealed receipt.
4. Separate registration from scan, admission, activation, and projection.
5. Support inspection/listing without reading secrets or client runtime state.

Tests: traversal, absolute path, backslash/drive path, ZIP bomb budget, duplicate member, digest mismatch, external symlink, cancellation, and receipt replay.

### B3. Physical-identity observation scanner — complete

Delivered in `internal/observation/types.go`, `internal/observation/identity.go`, `internal/observation/scanner.go`, and `internal/observation/scanner_test.go`.

1. Require explicit scan root, scope, kind policy, depth, file, byte, and diagnostic budgets.
2. Resolve Windows reparse chains with a visited physical-identity set and hop limit.
3. Produce one object observation per physical object and many logical visibility edges when aliases point to it.
4. Record broken junctions, cycles, permission errors, case collisions, unrecognized object types, and excluded native-state paths as bounded diagnostics.
5. Never recursively follow managed projections, backup trees, `skills_old`, generated distributions, dependency trees, caches, or archives unless explicitly registered as a source bundle.

Tests: direct root, `.agent` → `.agents`, case alias, cycle, broken junction, junction retarget after scan, project/global duplicate, cancellation, 12k-entry synthetic tree, and deterministic results.

### B4. Nested-client target catalog — complete

Delivered in `internal/targetcatalog/types.go`, `internal/targetcatalog/catalog.go`, `internal/targetcatalog/default.json`, and `internal/targetcatalog/catalog_test.go`.

1. Model logical clients separately from physical roots and subpaths.
2. Add global/project roots, aliases, discovery-only paths, supported resource kinds, recursion policy, ownership markers, and evidence confidence.
3. Represent `.gemini` children independently: `agy-cli`, `antigravity-cli`, `antigravity-ide`, Gemini CLI, Gemini IDE, extensions, skills, and tools.
4. Add observed roots for `.agents`, `.agent`, `.codex`, `.claude`, `.cursor`, `.opencode`, `.hermes`, `.openhuman`, `.windsurf`, `.copilot`, `.kiro`, `.openclaw`, `.continue`, `.roo`, `.qwen`, `.kimi`, and known candidate roots.
5. Mark unverified roots generic/read-only; directory presence is not client installation proof.

Tests: root aliasing, nested client IDs, global/project precedence, absent root, discovery-only root, unsupported kind, and catalog schema compatibility.

### B5. Managed-projection exclusion contract — complete

Delivered in `internal/resourcehub/projection.go`, `internal/resourcehub/projection_test.go`, `internal/resourcehub/identity.go`, and `internal/resourcehub/identity_test.go`.

1. Define ownership manifest fields: owner, schema version, resource identity, artifact digest, target ID, projection root, and generation receipt.
2. Exclude managed output by physical identity plus valid marker, not name/path convention alone.
3. Treat missing or forged marker as foreign/unknown, never as removable or importable.
4. Split `.agents` inbox and managed paths in catalog configuration.

Tests: valid marker, tampered marker, missing marker, forged non-ASM marker, exact duplicate deduplication, divergent same-name detection, alias decision recording, and re-import exclusion filter.

## Phase C — Identity, comparison, and controlled consolidation

### C1. Deterministic identity pipeline — complete

Delivered in `internal/similarity/normalize.go`, `internal/similarity/normalize_test.go`, `internal/similarity/index.go`, and `internal/similarity/index_test.go`.

1. Physical identity grouping.
2. Exact payload/tree digest grouping.
3. Normalized-body grouping that removes only recognized frontmatter and whitespace variance.
4. Asset/reference comparison for path, digest, executable state, and primary-document reference.
5. Namespace/resource-kind aliases with Windows case-insensitive collision detection.

Blocking rule: code blocks, commands, URLs, negations, hook declarations, target metadata, required tools, and non-identical assets are never collapsed by normalizing text.

### C2. Advisory semantic evidence — complete

Delivered in `internal/similarity/evidence.go` and `internal/similarity/evidence_test.go`.

1. Add versioned relationship categories: equivalent, subset, superset, complementary, alternative, conflicting, unrelated.
2. Store scorer/version, feature evidence, confidence, counterexamples, and labeled-corpus result.
3. Report precision, recall and false-merge rate by category.
4. Invalidate recommendations after scorer changes; preserve reviewed decisions.

Acceptance: semantic evidence cannot mutate canonical identity, delete a source, or authorize an absorption.

### C3. Reviewed consolidation decisions — complete

Delivered in `internal/similarity/decision.go` and `internal/similarity/decision_test.go`.

1. Canonical-winner ordering: operator choice, governed-source preference, completeness, schema validity, stable tie breaker.
2. Require preserved-material, rejected-material, lineage, target impact, regression case, and reversal path for absorption.
3. Distinguish template, specialization, alternative, composition, and data-only parameterization.
4. Keep decisions in operational state; commit only reusable fixtures and contracts.

## Phase D — Pure adaptation, controlled execution, and retry

### D1. Adapter conformance contract

Delivered in `internal/adapters/conformance/contract.go` and `internal/adapters/conformance/contract_test.go`.

1. Define pure `discover`, `normalize`, `render`, `plan`, and `verify` operations.
2. Require capability declaration, unsupported-field handling, fidelity/loss record, and deterministic output.
3. Build donor-derived fixture corpus with native root formats and negative cases.
4. Assert adapters never mutate filesystem, MCP configuration, registry, or client state.

### D2. OpenCode adapter and shadow vertical slice

Delivered in `internal/adapters/opencode/adapter.go` and `internal/adapters/opencode/adapter_test.go`.

1. Implement OpenCode only, using isolated fixtures—not `C:\Users\ACER\.opencode`.
2. Observe → candidate → canonical decision → render → plan → shadow projection → verify.
3. Prove exact duplicate, foreign collision, drift, missing target, stale plan and projection re-import failures.
4. Defer Codex/Cursor until OpenCode conformance and recovery gates pass.

### D3. Deployment executor

Delivered in `internal/executor/deployment.go`, `internal/executor/backup.go`, `internal/executor/types.go`, and `internal/executor/deployment_test.go`.

1. Define a file operation with base object descriptor, desired object descriptor, ownership marker, target identity, operation ID, and backup requirement.
2. Acquire per-target lock; revalidate immediately before every write.
3. Back up absence/file/directory/symlink-or-junction state plus relevant metadata.
4. Use atomic replace where supported; verify desired state; seal an independently recoverable receipt.
5. Keep `mcplink` as a separate MCP mutation executor.

### D4. MCPlink mutation executor

Delivered in `internal/adapters/mcplink/executor.go` and `internal/adapters/mcplink/executor_test.go`.

1. Define target descriptor, server ID, tool/resource target, mutation payload, precondition digest, and timeout.
2. Require target lock, revalidation, execution logging, and output parsing.
3. Fail safely on partial payload failure without mutating local workspace state.

### D5. Disparity audit, drift detection & reconciliation CLI

Delivered in `cmd/agentstack/main.go`, `internal/cli/control_plane.go`, and `internal/cli/control_plane_test.go`.

1. Build CLI command surface for audit, plan, render, apply, status, verify, reconcile.
2. Support target-specific shadow vs write mode flags.
3. Emit machine-readable audit report artifacts and human-readable terminal summaries.

## Phase E — MCP and universal Change Set

Delivered in `internal/mcp/intent.go`, `internal/mcp/intent_test.go`, `internal/changeset/changeset.go`, and `internal/changeset/changeset_test.go`.

1. Introduce canonical secret-free MCP intent: identity, transport, environment-variable names, capability, compatibility, profile, assignment, observed registration, and health.
2. Build target-native MCP child plans through `mcplink`; never persist secret values.
3. Compose filesystem and MCP child plans into one sealed Change Set with exact operations, before/after descriptors, fidelity, ownership, retry policy, and recovery detail.
4. Retain selections on invalidation and explain the changed observation/target evidence.
5. Report per-child partial success honestly; do not imply fleet-wide transactionality.

## Phase F — Operator experience and migration

Delivered in `internal/operator/queues.go`, `internal/operator/wave.go`, and `internal/operator/operator_test.go`.

1. Add Library/Targets, Changes, and Activity views over one receipt/state model.
2. Group large corpus decisions into exact duplicates, aliases, conflicts, unique preservation, incompatibility, quarantine, and retirement queues.
3. Display library state, target state, and health independently; hide hashes/lineage/path details behind technical detail.
4. Migrate targets in waves: OpenCode; Codex/Cursor; Claude/Gemini/Antigravity/Windsurf; then verified remainder.
5. Require shadow projection, backup, syntax validation, visibility verification, smoke check, observation window, and rollback drill for each client wave.

## Phase G — Second-order convergence

Delivered in `internal/secondorder/plane.go` and `internal/secondorder/plane_test.go`.

After OpenCode plus one MCP profile pass all prior gates, admit only compositional planes that preserve existing authority:

1. target-contract generator;
2. governance/family/capability/profile import;
3. recommendation/composition/routing;
4. static/behavioral/adversarial/failure-injection evaluation harness;
5. provenance and evidence API;
6. declarative extension manifests with constrained capability, not general executable plugins;
7. sequential migrations;
8. read-only maintenance proposals, bounded corpus campaigns, and receipt-bound retirement.

## Validation matrix

| Gate | Minimum commands | Additional evidence |
|---|---|---|
| A | `go test ./internal/releasepack ./internal/supplychain -count=1`; `pwsh -File scripts/check-governance.ps1`; `pwsh -File scripts/check-docs.ps1` | Git-less archive smoke and source closure |
| B | focused artifact/resource/sourcebundle/observation/catalog tests | synthetic junction and nested-client fixtures |
| C | focused identity/similarity/store tests | labeled corpus and no-auto-merge proof |
| D | adapter conformance, deployment fault/race tests | OpenCode shadow/rollback receipt |
| E | mcp/mcplink/changeset tests | mixed partial-success recovery |
| F/G | app/UI contract/accessibility/release checks | per-target independent recovery and maintenance receipts |

## Baseline exception register

These must be resolved or explicitly waived before declaring a full-suite green baseline:

| ID | Status | Evidence | Owner/next action |
|---|---|---|---|
| BASE-001 | Open | Default CGO build cannot find `sqlite3.pc` in the current Windows environment | Install/configure the reviewed SQLite development package or document the supported pure-Go mode |
| BASE-002 | Open | `TestSandboxEnvironmentRedirectsCoverageIntoPrivateSandbox` sees mode `drwxrwxrwx` on this Windows filesystem | Determine whether the test needs Windows ACL-aware assertions or the sandbox needs a stricter creation path |

Neither exception is caused by commits `47a1432` or `5997e9e`; focused program gates pass.
