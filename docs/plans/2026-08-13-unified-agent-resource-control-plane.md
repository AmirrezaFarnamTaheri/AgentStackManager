# Unified Agent Resource Control Plane Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify ASM, the governed skill library, and locally installed agent resource trees into one preservation-first control plane for skills, agents, prompts, rules, commands, workflows, and MCP definitions.

**Architecture:** Resource Hub remains the canonical decision authority, `artifactgraph.Artifact` remains the sole canonical versioned envelope, and CAS remains immutable storage. Read-only importers turn the skill library and explicit local source paths into observations and candidate revisions; pure adapters render reviewed resources into operation sets. A deployment executor alone applies filesystem operations, while `mcplink` alone applies MCP-registration operations. `.agents` exposes disjoint authoring-inbox and managed-projection path sets, never a second canonical database.

**Tech Stack:** Go, embedded HTML/CSS/JavaScript, strict JSON, SQLite shadow index where available, Windows junction/reparse-point APIs, ASM Resource Hub, CAS, reviewed plans, adapter conformance corpus.

---

## 1. Scope lock

ASM owns only shareable agent resources:

- skills;
- agents and subagent definitions;
- prompts;
- rules and portable instruction files;
- commands;
- workflows and compositions;
- inert portable hook definitions (stored and projected with compatibility/loss reporting, never executed by ASM during ingestion or projection);
- MCP server definitions and client registrations;
- generated client projections.

ASM explicitly does not own credentials, provider switching, token routing, subscriptions, sessions, histories, caches, logs, agent execution, worktrees, IDE extensions, model settings, or arbitrary native client preferences.

## 2. Authority model

| Boundary | Authority |
|---|---|
| Resource Hub | Logical identity, canonical resource decisions, lifecycle, aliases, target bindings, reviewed plans |
| CAS | Immutable payload bytes and historical reconstruction |
| SQLite/index | Rebuildable search, similarity, and inventory acceleration only |
| Importers | Read-only observation and candidate creation |
| Skill library | Versioned upstream governance/taxonomy input |
| Adapters | Pure discovery normalization, import, rendering, loss reporting, planning, verification |
| Deployment executor | Sole target-filesystem mutation path |
| `mcplink` | MCP registration mutation authority, coordinated by a composed Change Set |
| `.agents` inbox paths | Explicit authoring input observed read-only; edits create candidate revisions |
| `.agents` managed paths | Compatibility projections carrying ASM ownership markers; never import sources |
| Client roots | Native client state, explicit overlays, and disposable managed projections |

Dependency direction:

```text
skill library + local source paths       reference repos
                    |                          |
                 observe              donor adoption ledger
                    v                          |
        Resource Hub canonical intent <--- catalog + fixtures
          |          |              |
         CAS       index       pure adapters
                                    |
                            planned operations
                                    v
                             sealed Change Set
                              |             |
                    deployment executor   mcplink
                              |             |
                     managed projections  MCP registrations
```

Every filesystem operation is bound to base observed state, desired state, ownership marker, and target identity. The executor acquires a per-target lock, revalidates immediately before mutation, journals each operation, backs up the full filesystem object type and relevant metadata, uses atomic replacement where the target supports it, verifies postconditions, and emits an independently recoverable receipt. `mcplink` owns MCP registration fields and follows the same Change Set lock ordering; the general executor never writes them.

## 3. Product model

The operator mental model is:

> One library, many targets, one change set, recoverable runs.

Primary flow:

```text
Inspect -> Choose intent -> Review exact changes -> Approve -> Verify -> Recover
```

The UI uses decision-oriented states: Ready, Needs review, Different versions, Drifted, Missing, Unsupported here, and Ignored. Internal lifecycle names, paths, digests, lineage, and fidelity details remain available under technical details.

`Changes` becomes the sole operator-facing authorization surface. It composes immutable child plans from Resource Hub, `mcplink`, and other existing mutation authorities without pretending that multi-target execution is globally atomic.

## 4. Resource lifecycle

```text
observed -> parsed -> classified -> candidate -> canonical -> projected -> verified
                         |              |
                         +-> ignored    +-> retired
                         +-> quarantine
                         +-> alias
```

`artifactgraph.Artifact` is the only canonical versioned envelope. Observations, candidate revisions, aliases, lifecycle transitions, and reviewed decisions are records that reference an artifact identity and digest. `resourcehub.Resource` remains a compatibility/read model during migration and must not evolve into a second authority. Only promoted canonical skills require the governed skill-library contract.

Required envelope fields:

- stable resource ID;
- kind and lifecycle state;
- display name and aliases;
- source observations and physical identity;
- content and envelope digests;
- schema findings;
- duplicate/overlap classification;
- family and capability candidates;
- target bindings;
- preservation and canonicalization decisions.

## 5. Donor adoption map

All donor snapshots are read-only evidence under `reference repos/`. They are excluded from builds and releases, never become runtime dependencies, and feed only the donor ledger, target catalog, conformance fixtures, and design decisions. Donor files enter normal observation only when deliberately registered as a source bundle.

| Donor | Adopt | Reject |
|---|---|---|
| `cc-switch-main` | MCP serializers, prompt/skill target knowledge, archive limits, atomic-write cases, sync fixtures | provider switching, proxying, credentials, usage accounting, independent database authority |
| `cc-switch-cli-main` | CLI/TUI discovery, review and diagnostic patterns | duplicate backend mechanisms already harvested from CC Switch |
| `skills-manager-main` | broad target-path catalog, global/project asymmetry, discovery-only roots, recursive scans, overlap guards, source-diff concepts | direct unreviewed deployment and independent canonical store |
| `skills-manage-main` | Central Library/Platform UX, frontmatter inspection, collections, preview patterns | direct install authority |
| `skill-zoo-main` | consistency, audit, maintenance, file-tree and update UX | second backend/store |
| `agent-of-empires-main` | typed merge, conflict fingerprints, managed markers, drift guards, project MCP/skill models | ACP, sessions, containers, terminals, worktrees, runtime orchestration |
| `skill_library` | families, capabilities, profiles, compositions, routing, risk, schemas, evaluations, manifests | direct writable authority or wholesale replacement of local skills |

Every adopted mechanism receives a donor-decision record containing donor snapshot, upstream identity, source path, retained behavior, rejected behavior, ASM owner, transplanted fixtures, and verification status.

## 6. Target waves

1. OpenCode vertical slice.
2. Codex and Cursor.
3. Claude, Gemini CLI, Antigravity CLI/IDE, and Windsurf.
4. Hermes, Copilot, OpenClaw, Kiro, Roo/Kilo, Continue, Goose, Qwen, and Kimi.
5. Nara, OpenHuman, and unknown clients through the generic filesystem adapter until verified native contracts exist.

### Requested local-root coverage

The catalog is a statement of support; observation is a statement of current evidence. Folder existence never proves that a client is installed.

| Root family | Current evidence | Initial treatment |
|---|---|---|
| `.agents` | present | Split explicit inbox paths from ownership-marked managed projections; exclude the latter by physical identity |
| `.agent` | present reparse point | Resolve physical identity, detect cycles/aliases, observe read-only until its contract is classified |
| `.gemini` | present | Catalog Gemini and nested Antigravity resource paths independently |
| `.antigravity` | absent as standalone root | Catalog candidate only; do not report installed |
| `.codex` | present | Verified native adapter after OpenCode pilot |
| `.claude` | present | Verified native adapter in wave 3 |
| `.opencode` | present; un-dotted `opencode` absent | OpenCode pilot root; retain alias candidates without inventing an installation |
| `.hermes` | present | Observe now; project only after native contract evidence |
| `.nara` | absent | Generic/read-only catalog candidate until evidenced |
| `.openhuman` | present | Generic/read-only until native contract evidence |
| `.windsurf` | present | Verified native adapter in wave 3 |
| `.cursor` | present | Verified native adapter after OpenCode pilot |

Each catalog entry must enumerate global/project paths, file/subpath patterns, supported resource kinds, recursive/discovery-only/read-only/projectable status, confidence, and physical identity. Completion requires every requested root to be mapped or explicitly unresolved.

## 7. Consolidation policy

- Exact duplicates: one canonical payload, all prior names retained as aliases.
- Normalized-body duplicates: require metadata and asset comparison.
- Semantic relationships: equivalent, subset, superset, complementary, alternative, conflicting, or unrelated.
- Parameterization: allowed only when variation is data, such as venue or provider metadata.
- Absorption: must record preserved unique material, rejected material, lineage, regression cases, and reversal path.
- Projection output is never eligible as a new import source.
- Canonical winner order is explicit operator choice, governed-source preference, completeness, schema validity, then a stable deterministic tie-breaker; the automatic recommendation never becomes a decision without review.
- Never auto-merge materially different commands, URLs, code blocks, negations, executable assets, hooks, required tools, or conflicting target metadata.
- Alias uniqueness is scoped by resource kind and namespace, with Windows case-insensitive collision checks.
- Asset consolidation compares relative path, bytes, executable status, and references from the primary document; path collisions with different bytes require review.
- Parameterize only data-only variation. Behavioral steps remain separate. Families distinguish template, specialization, alternative, and composition.
- Editing an observed source after promotion creates a new candidate revision; it never mutates canonical content.
- Semantic scorer/version changes invalidate recommendations, never canonical decisions.
- Retirement requires an observation window and retained rollback receipts; unresolved material remains quarantined.

## 8. MCP policy

MCP definitions are Resource Hub resources. Secret values are never stored. Canonical definitions separate identity, transport, required environment-variable names, capabilities, target compatibility, activation profile, observed registrations, and health.

Profiles are capability-oriented: Core, Coding/Indexing, Browser, Research, Database, Cloud, Media, and Diagnostic. Multiple providers can remain available while target assignments select a minimal active subset.

## 9. Implementation tasks

### Task 1: Freeze contracts and add donor evidence guards

**Files:**
- Create: `docs/convergence/UNIFIED_RESOURCE_AUTHORITY.md`
- Create: `docs/convergence/DONOR_ADOPTION_LEDGER.json`
- Modify: `.gitignore`
- Modify: `scripts/check-source-archive-build.sh`
- Modify: `scripts/check-governance.ps1`
- Test: `internal/releasepack/source_test.go`

**Steps:**

1. Write a failing release-pack test proving `reference repos/` cannot enter a source or binary release.
2. Run `go test ./internal/releasepack -run ReferenceRepos -count=1` and verify failure.
3. Add explicit packaging/build exclusions without deleting the snapshots.
4. Document the authority table and non-goals from this plan.
5. Inventory every direct child of `reference repos/` (including future additions) and classify it Adopt, Fixture-only, Reject, or Unverified with exact retained/rejected scopes; no hard-coded donor count.
6. Run `go test ./internal/releasepack ./internal/supplychain -count=1`.
7. Run `pwsh -File scripts/check-governance.ps1`.
8. Review `git diff --check` and commit the task.

### Task 2: Extend the canonical artifact model with lifecycle and decision records

**Files:**
- Modify: `internal/resourcehub/types.go`
- Create: `internal/resourcehub/lifecycle.go`
- Create: `internal/resourcehub/lifecycle_test.go`
- Modify: `internal/artifactgraph/model.go`
- Modify: `internal/artifactgraph/model_test.go`
- Modify: `internal/resourcehub/manager.go`

**Steps:**

1. Write table-driven tests for valid and invalid lifecycle transitions referencing `artifactgraph.Artifact` identities.
2. Add failing tests for alias, observation, candidate-revision, schema-finding, and preservation-decision records.
3. Run the focused tests and verify failure.
4. Extend `artifactgraph.Artifact` only where canonical fields are missing; keep lifecycle/decision data in referencing records and prevent a third envelope.
5. Treat `resourcehub.Resource` as a compatibility/read model, preserve registry version 1 decoding, and do not silently promote imported resources.
6. Add deterministic sealing and validation.
7. Run `go test ./internal/resourcehub ./internal/artifactgraph -count=1`.
8. Commit the task.

### Task 3: Register immutable source bundles

**Files:**
- Create: `internal/resourcehub/sourcebundle.go`
- Create: `internal/resourcehub/sourcebundle_test.go`
- Modify: `internal/resourcehub/manager.go`
- Modify: `internal/cli/cli.go`
- Modify: `docs/CLI_REFERENCE.md`

**Steps:**

1. Write failing tests for ZIP traversal, absolute paths, symlinks, member limits, total-size limits, duplicate members, and digest mismatch.
2. Add a fixture representing `skill_library` without copying the full library into testdata.
3. Run the focused tests and verify failure.
4. Implement read-only source registration into CAS with a sealed source-bundle receipt.
5. Ensure registration never activates or projects resources.
6. Add `hub source add|list|inspect` CLI surfaces.
7. Run focused tests, CLI tests, fuzz tests, and `go test ./internal/resourcehub ./internal/cas ./internal/cli -count=1`.
8. Commit the task.

### Task 4: Build a junction-aware observation scanner

**Files:**
- Create: `internal/observation/types.go`
- Create: `internal/observation/scanner.go`
- Create: `internal/observation/scanner_windows.go`
- Create: `internal/observation/scanner_other.go`
- Create: `internal/observation/scanner_test.go`
- Modify: `internal/ui/target_discovery.go`
- Modify: `internal/ui/target_discovery_test.go`

**Steps:**

1. Write failing fixtures for directory, file, junction, broken junction, cycle, discovery-only root, managed projection, backup root, and project/global asymmetry.
2. Require explicit scan roots and depth/resource budgets.
3. Ensure physical payloads are hashed once and projections are not recursively followed.
4. Add progress, cancellation, bounded diagnostics, and stable observation IDs.
5. Treat folder existence as evidence, not proof of an installed client.
6. Run Windows-specific and portable tests.
7. Benchmark against a synthetic 12,000-entry/10,000-skill tree.
8. Commit the task.

### Task 5: Introduce the donor-backed target catalog

**Files:**
- Create: `internal/targetcatalog/types.go`
- Create: `internal/targetcatalog/default.json`
- Create: `internal/targetcatalog/catalog.go`
- Create: `internal/targetcatalog/catalog_test.go`
- Modify: `internal/ui/target_discovery.go`
- Modify: `internal/adapters/builtin/builtin.go`

**Steps:**

1. Extract target facts from the donor ledger rather than importing donor code.
2. Write failing tests for canonical target IDs, aliases, global paths, project paths, discovery-only paths, recursive scanning, and custom roots.
3. Add every requested root family from the coverage matrix, including `.agent`, Nara, and OpenHuman, plus Codex, Claude, OpenCode, Cursor, Windsurf, Gemini, Antigravity, Hermes, Copilot, OpenClaw, Kiro, Roo, Continue, Goose, Qwen, and Kimi.
4. Mark unverified targets as generic/read-only.
5. Replace duplicated path constants incrementally while preserving behavior.
6. Run target discovery and adapter conformance tests.
7. Commit the task.

### Task 6: Add exact duplicate and alias decisions

**Files:**
- Create: `internal/resourcehub/identity.go`
- Create: `internal/resourcehub/identity_test.go`
- Modify: `internal/resourcehub/artifactgraph.go`
- Modify: `internal/resourcehub/artifactgraph_test.go`
- Modify: `internal/resourcehub/inspect.go`
- Modify: `internal/resourcehub/types.go`

**Steps:**

1. Write failing tests for exact content groups, case collisions, legacy aliases, divergent same-name resources, and projection shadows.
2. Implement stable identity independent of directory name.
3. Add reviewed alias decisions; never auto-delete an observed source.
4. Record absorption lineage and reversible retirement intent.
5. Extend inspection counts and machine-readable reports.
6. Run Resource Hub and artifact-graph tests.
7. Commit the task.

### Task 7: Add normalized and semantic overlap indexing

**Files:**
- Create: `internal/similarity/normalize.go`
- Create: `internal/similarity/normalize_test.go`
- Create: `internal/similarity/index.go`
- Create: `internal/similarity/index_test.go`
- Modify: `internal/store/sqlite/store.go`
- Modify: `internal/store/sqlite/store_test.go`

**Steps:**

1. Define deterministic normalized-body hashing that removes only recognized frontmatter and whitespace differences.
2. Test that code, URLs, commands, and negations remain meaningfully distinct.
3. Store semantic similarity as advisory evidence, never canonical truth.
4. Classify candidate relationships without auto-merging.
5. Make the index rebuildable from Resource Hub/CAS.
6. Add scale benchmarks and bounded-memory tests.
7. Commit the task.

### Task 8: Complete pure projection adapter behavior

**Files:**
- Modify: `internal/adapters/contract.go`
- Modify: `internal/adapters/capabilities.go`
- Modify: `internal/adapters/builtin/builtin.go`
- Modify: `internal/adapters/conformance/corpus.go`
- Modify: `internal/adapters/conformance/conformance_test.go`
- Add target fixtures under: `internal/adapters/conformance/testdata/targets/`

**Steps:**

1. Transplant sanitized native fixtures and expected behavior from the donor snapshots.
2. Add contract cases for discovery-only roots, project/global asymmetry, recursive formats, aliases, unsupported fields, and client-native metadata.
3. Require import, render, plan, and verify to remain mutation-free.
4. Require all transformations to emit fidelity/loss evidence.
5. Implement adapter lane 8a for OpenCode only.
6. After the OpenCode vertical slice passes, implement 8b Codex and Cursor, then 8c Claude, Gemini/Antigravity, and Windsurf; Task 8 builds and tests adapters only, while Task 13 authorizes workstation migration.
7. Run built-in and external adapter conformance suites.
8. Commit each target adapter independently.

### Task 9: Prove the OpenCode vertical slice

**Files:**
- Modify: `internal/resourcehub/sync.go`
- Modify: `internal/resourcehub/sync_test.go`
- Modify: `internal/resourcehub/adapters_test.go`
- Create: `internal/resourcehub/opencode_vertical_test.go`
- Modify: `docs/USER_GUIDE.md`

**Steps:**

1. Build isolated fixtures for one new skill, exact duplicate, foreign collision, drift, missing target, and rollback.
2. Run observe -> candidate -> promote -> project -> verify in a temporary target.
3. Prove projection output is excluded from re-import.
4. Prove stale plans and foreign collisions fail closed.
5. Prove restore reconstructs absence, file, directory, symlink/junction, relevant metadata, and exact bytes as applicable.
6. Run race-enabled Resource Hub tests.
7. Document the operator flow.
8. Commit the task.

### Task 10: Compose one operator-facing Change Set

**Files:**
- Create: `internal/changeset/types.go`
- Create: `internal/changeset/coordinator.go`
- Create: `internal/changeset/coordinator_test.go`
- Create: `internal/deployment/executor.go`
- Create: `internal/deployment/executor_test.go`
- Create: `internal/deployment/journal.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/progress.go`
- Modify: `internal/ui/operations.go`
- Modify: `internal/ui/web/changes.js`
- Modify: `internal/ui/web/index.html`

**Steps:**

1. Write failing tests that compose Resource Hub filesystem-operation plans and `mcplink` child plans without transferring adapter or MCP mutation authority.
2. Bind child identities, digests, expiry, registry state, capability evidence, and recovery descriptions into one sealed Change Set.
3. Reject any stale or altered child before execution.
4. Bind every filesystem operation to `{base digest/type/existence, desired digest/type, ownership marker, target identity}`; acquire a per-target lock and revalidate immediately before mutation.
5. Execute through the deployment executor using a journal, type-aware backup, atomic replacement where supported, postcondition verification, idempotent retry/reconcile states, and exact recovery receipts.
6. Execute independent targets with honest partial-success and per-child recovery/roll-forward reporting; never claim fleet-wide atomicity.
7. Retain operator selections when rebuilding an invalidated plan and explain the invalidation cause.
8. Show exact operations, before/after digests, fidelity, ownership, and recovery details.
9. Keep one approval surface while preserving the filesystem executor and `mcplink` as disjoint domain executors with deterministic lock ordering.
10. Run deployment, app, UI contract, accessibility, interruption/retry, and race tests.
11. Commit the task.

### Task 11: Canonicalize MCP definitions and profiles before finalizing Change Sets

**Files:**
- Modify: `internal/resourcehub/types.go`
- Create: `internal/resourcehub/mcp_profile.go`
- Create: `internal/resourcehub/mcp_profile_test.go`
- Modify: `internal/mcplink/adapters.go`
- Modify: `internal/mcplink/mcplink_test.go`
- Modify: `internal/mcp/config.go`
- Modify: `internal/mcp/config_test.go`

**Steps:**

1. Add canonical MCP identity, transport, environment-variable names, capabilities, compatibility, assignments, and observed registration state.
2. Import definitions without secret values.
3. Distinguish available definitions from active registrations.
4. Add Core, Coding, Browser, Research, Database, Cloud, Media, and Diagnostic profiles.
5. Emit target-native registrations through existing mutation authority.
6. Run live initialize/tools-list probes only after reviewed apply.
7. Test duplicate names, conflicting transports, missing environment requirements, and inactive alternatives.
8. Commit the task.

### Task 12: Add consolidation decision queues

**Files:**
- Modify: `internal/resourcehub/inspect.go`
- Modify: `internal/ui/web/sync.js`
- Modify: `internal/ui/web/changes.js`
- Modify: `internal/ui/web/styles.css`
- Modify: `internal/ui/web/accessibility.css`
- Test: `ui-tests/accessibility.mjs`

**Steps:**

1. Group recommendations into exact consolidation, aliases, content conflicts, unique preservation, target incompatibility, quarantine, and retirement.
2. Show counts and representative examples before individual rows.
3. Separate library state, target state, and health.
4. Add ownership labels: Managed by ASM, Client-owned, Imported copy, Ignored.
5. Keep hashes, paths, lineage, and adapters under technical details.
6. Test keyboard operation, focus restoration, live announcements, mobile target size, and large-list behavior.
7. Commit the task.

### Task 13: Migrate clients in reviewed waves

**Files:**
- Create: `docs/convergence/CLIENT_MIGRATION_RUNBOOK.md`
- Create fixtures/tests per adapter under `internal/adapters/conformance/testdata/targets/`
- Modify: `docs/OPERATIONS.md`
- Modify: `docs/USER_GUIDE.md`

**Steps:**

1. Complete OpenCode, Codex, and Cursor shadow projections and rollback drills.
2. Add Claude, Gemini/Antigravity, and Windsurf.
3. Add remaining verified targets.
4. Keep Nara/OpenHuman generic until their local contracts are proven.
5. Require a refreshed inventory, sealed plan, backup, syntax validation, smoke test, visibility check, and rollback receipt per client.
6. Do not remove current junctions or `skills_old` trees during this task.
7. Commit each migration wave independently.

### Task 14: Consolidate the local corpus in bounded batches

**Files:**
- Create: `docs/convergence/LOCAL_CONSOLIDATION_RUNBOOK.md`
- Generate reviewed reports under ASM data state, not the repository.
- Extend relevant Resource Hub tests for every new decision class.

**Steps:**

1. Process exact duplicates.
2. Process normalized-body groups.
3. Resolve same-name conflicts.
4. Review academic workflow families.
5. Review automation families.
6. Review research, writing, orchestration, security, frontend, and MCP groups.
7. Keep batches between 20 and 50 decisions.
8. Require preserved-material records and regression cases for every absorption.
9. Never delete originals until verified projections and recovery receipts exist.
10. Commit only code/runbook changes; local decisions remain operational state.

### Task 15: Retire active clutter after convergence

**Files:**
- Modify: `docs/OPERATIONS.md`
- Modify: `docs/USER_GUIDE.md`
- Add cleanup planning/tests to the existing ownership and reviewed-plan modules.

**Steps:**

1. Generate a preview-only retirement plan for `skills_old`, redundant projections, repeated MCP libraries, malformed roots, and the literal `%USERPROFILE%` directory.
2. Verify every target is healthy and independently restorable.
3. Move retained archives outside active client roots.
4. Quarantine malformed or unresolved resources.
5. Remove only ASM-owned, receipt-bound projections.
6. Re-run full inventory, drift, MCP doctor, client smoke, security, accessibility, race, and release checks.
7. Commit documentation or code changes; record workstation mutations in ASM receipts.

## 10. Verification gates

Before any migration wave is complete:

```powershell
go test ./... -count=1
go test -race ./... -count=1
bash scripts/check-critical-coverage.sh
pwsh -File scripts/check-governance.ps1
pwsh -File scripts/check-docs.ps1
npm --prefix ui-tests test
```

Also require:

- adapter conformance;
- shadow-root structural validation;
- stale-plan rejection;
- foreign-resource preservation;
- exact rollback reconstruction;
- junction-cycle tests;
- 12,000-entry inventory benchmark;
- MCP initialize/tools-list smoke tests;
- source-package exclusion of donor snapshots.
- concurrent-edit rejection between approval and apply;
- operation-journal interruption and idempotent retry/reconcile;
- type-aware restoration of absent/file/directory/symlink/junction states;
- `.agents` managed-projection exclusion through direct and aliased physical paths while inbox content remains observable;
- mixed MCP/filesystem partial-success recovery with disjoint mutation ownership.

Projection is blocked when required target semantics cannot be represented, fidelity loss affects executable commands/hooks/tool requirements, ownership is foreign or ambiguous, or round-trip verification changes protected content. Advisory semantic classification must be evaluated against a versioned labeled corpus with per-class precision/recall and false-merge rate reported; no score threshold may authorize a merge. The 12,000-entry benchmark must record a reproducible machine/profile, peak memory, completion time, cancellation latency, and diagnostic bound, with regression budgets fixed from the accepted OpenCode baseline rather than invented in advance.

## 10A. Delivery slices, dependencies, and parallel lanes

The fifteen granular tasks roll up into nine implementation-sized slices. This preserves file-level test instructions while avoiding duplicate delivery tracks.

| Slice | Priority | Contains | Blocked by | Exit evidence |
|---|---|---|---|---|
| E1 Authority and donor boundary | P0 | Task 1 | none | Authority contract and release exclusion pass |
| E2 Canonical model migration | P0 | Task 2 | E1 | One artifact envelope; version-1 compatibility; lifecycle tests |
| E3 Unified source discovery | P0 | Tasks 3–5 | E2 | ZIP and explicit-root sources produce deterministic observations under safety budgets |
| E4 Identity and consolidation | P0 | Tasks 6–7 | E2, E3 | Exact/normalized/advisory-semantic evidence and reviewed decisions share one pipeline |
| E5 OpenCode vertical slice | P0 | Tasks 8–9, OpenCode only | E3, E4 | Observe through type-aware rollback passes without projection re-import |
| E6 MCP intent and profiles | P1 | Task 11 | E2, E3 | Canonical intent produces sealed `mcplink` child plans without secrets |
| E7 Change Set, executor, and queues | P0 | Tasks 10, 12 | E5, E6 | One approval; pure adapters; journaled per-domain execution and recovery |
| E8 Target migration waves | P1 | Task 13 plus retirement gate from Task 15 | E7 | Each target independently verified, observed, and recoverable before cutover/cleanup |
| E9 Corpus consolidation campaign | P1 | Task 14 plus bounded retirement from Task 15 | E4, E7 | Decisions run in 20–50 item batches; only receipt-bound projections retire |

Parallel work is permitted only where authority is disjoint: E4 indexing may proceed alongside E5 adapter fixtures after the identity result contract freezes; E6 may proceed alongside the OpenCode adapter after the canonical artifact contract freezes; target-fixture research may proceed without target writes. E1 -> E2 -> E3 -> E4 -> E5 -> E7 is the critical path.

**Definition of Ready for a slice:** authority owner named; inputs/outputs and non-goals explicit; fixtures include normal, failure, and integration-edge cases; target paths are evidence-backed; mutation and recovery owner named; acceptance checks are executable.

**Definition of Done for a slice:** focused tests pass; mutation-free components are verified as such; stale/foreign/concurrent state fails closed; fidelity and ownership evidence is emitted; rollback/reconcile is tested where mutation exists; documentation and donor ledger are current; no later slice is required to make the completed slice safe.

## 11. Completion criteria

- Resource Hub is the only canonical decision authority.
- `.agents` inbox and managed-projection path sets are disjoint; neither is a competing store and projections cannot be re-imported through aliases.
- Client roots contain native state and disposable projections.
- Every local resource is canonical, candidate, alias, ignored, retired, or quarantined.
- Exact duplicates have one canonical payload and working legacy aliases.
- No projection can be re-imported as a source.
- Every transformation reports fidelity.
- Every managed write originates from a reviewed Change Set.
- Each target is independently recoverable.
- Pure adapters never mutate targets; only the deployment executor writes managed filesystem projections and only `mcplink` writes MCP registrations.
- Every requested local root is cataloged with evidence/confidence or explicitly unresolved.
- MCP definitions have canonical identities and minimal verified assignments.
- Large scans remain bounded, cancellable, and observable.
- Provider routing, credentials, runtime orchestration, and native client state remain outside ASM.

## 12. Decision log

| Decision | Alternatives | Reason |
|---|---|---|
| Manage shareable resources only | all declarative config; complete environments | Maintains a coherent, auditable boundary |
| Resource Hub remains authoritative | `.agents` as source of truth | Preserves reviewed plans, stale-state protection, CAS recovery, and avoids split brain |
| `.agents` is a facade/inbox | bulk per-client mirrors; writable canonical tree | Keeps compatibility without making external partial writes authoritative |
| One operator-facing Change Set | separate resource and MCP approval products | Gives one understandable safety story while retaining backend authority boundaries |
| Donors are decomposed and reimplemented | wholesale merge; runtime dependency | Avoids competing stores and unrelated scope while harvesting mature behavior |
| Skill library is a governance seed | wholesale replacement; ignore it | Its architecture is strong but it does not represent the local corpus |
| Lightweight candidates, heavyweight canonical skills | require full contract on import | Makes ingestion of thousands of legacy resources feasible |
| OpenCode first | migrate all clients together | Existing whole-root junction makes it the safest vertical proof |
| Independent target recovery | fleet-wide atomicity claim | Filesystems and client configurations cannot provide honest global atomicity |
| `artifactgraph.Artifact` is the sole canonical envelope | add another observed/candidate envelope | Prevents three models from drifting while preserving lightweight lifecycle records |
| Split `.agents` input and output paths | one bidirectional `.agents` tree | Prevents managed projections from becoming canonical candidates |
| Explicit deployment executor | adapters or UI write targets | Makes pure rendering, TOCTOU checks, journaling, and recovery enforceable |

## 13. Implementation handoff

Start with slices E1–E3 only. Do not begin adapter expansion or workstation mutation until the authority, canonical artifact, source-bundle, target-catalog, and junction-aware observation contracts pass focused tests and the OpenCode shadow target is available. Then execute the critical path in order; E6 may run in parallel with OpenCode only after the canonical artifact contract is frozen.

## 14. Workstation topology: physical roots, logical ecosystems, and nested clients

The workstation cannot be modeled as one client per top-level dotted directory. The scanner must construct a graph with `logical client`, `logical resource root`, `filesystem object`, `physical identity`, `projection owner`, and `source eligibility` as separate concepts.

### 14.1 Confirmed cross-root topology

| Root | Observed structure | Required interpretation |
|---|---|---|
| `.agent` | Reparse point targeting `.agents` | Alias of the same physical resource hub; never scan twice |
| `.agents` | `agents`, `commands`, `external-repos`, `platform-sync`, `plugins`, `prompts`, `rules`, `skills`, `tools`, `workflows` | Shared compatibility ecosystem. Classify each child as inbox, managed projection, donor checkout, or client-owned source; do not assign one policy to the whole root |
| `.gemini` | `agy-cli`, `antigravity-cli`, `antigravity-ide`, `cli`, `ide`, `extensions`, `skills`, `tools`, `context-mode`, config/history/state | Multi-client container, not a single Gemini target. Each nested client receives its own target ID, root contract, projection namespace, and capability matrix |
| `.codex` | `agents`, `hooks`, `mcp-configs`, `plugins`, `prompts`, `rules`, `skills`, `skills_old`, packages and extensive runtime state | Only declared shareable subpaths enter ASM. Auth, sessions, databases, logs, caches, browser/computer-use state and backups remain native |
| `.claude` | agents/commands as reparse points, plus hooks, MCP configs, plugins, rules, skills, plans and runtime/session trees | Resolve junction destinations. Manage portable resources only; preserve plugin/runtime/session ownership |
| `.cursor` | agents/commands reparse points, rules, skills, `skills-cursor`, hooks/MCP config and IDE state | Distinguish shared projections, Cursor-native rules, and IDE-owned configuration |
| `.opencode` | skills reparse point, commands, hooks, instructions, MCP configs, plugins, prompts, tools, package/runtime content | Pilot target. Never absorb `node_modules`, auth, memory/runtime data or built output as resources |
| `.hermes` | nested `.agents`, agents/commands reparse points, rules, scripts, skills and MCP configs | Treat `.agents` compatibility view and Hermes-native paths separately; verify whether scripts are inert resources or executable runtime extensions |
| `.openhuman` | skill registry and skills beside cache/log/user/keychain/window state | Only registry metadata proven portable and skills are candidates; all user/keychain/runtime state is excluded |
| `.windsurf` | skills reparse point, `skills_old`, extensions and IDE arguments | Skills are projections; extensions and IDE settings remain native |
| `.agentsys` | adapters, plugins, templates, CLI/bin/lib/meta plus Claude/Codex/Kiro plugin surfaces | Treat as a donor implementation installed in the user profile. Inventory its target facts and fixtures, but do not make it a competing authority or execute its code inside ASM |
| `.antigravity_tools` | accounts, tokens, logs and databases | Explicitly excluded native credential/runtime system; it is not the Antigravity resource root |
| `.cc-switch` | SQLite database and settings | Evidence of an installed manager, not a shareable-resource source. Observe registrations through client-native files, never ingest its database as canonical state |

### 14.2 Additional observed client families

The target catalog must also represent the observed `.copilot`, `.kiro`, `.openclaw`, `.continue`, `.roo`, `.qwen`, `.kimi`, `.kimi-code`, `.amazonq`, `.augment`, `.codeium`, `.factory`, `.junie`, `.kilocode`, `.qoder`, `.trae`, `.zencoder`, `.openhands`, `.pi`, `.serena`, and other AI-tool roots discovered by the bounded top-level inventory. Presence creates an observation with confidence, not an automatic support claim.

For every root, capture:

1. lexical path and all aliases;
2. Windows volume/file identity and reparse target chain;
3. logical client and nested-client ID;
4. global versus project scope;
5. resource-kind subpaths and filename contracts;
6. read-only, inbox, projectable, discovery-only, ignored, or prohibited classification;
7. ownership marker and foreign-state rules;
8. recursion, depth, file-count and byte budgets;
9. excluded native-state subpaths;
10. confidence, evidence source, last verified version, and fixture revision.

### 14.3 Physical-identity graph invariants

- A physical directory is hashed once even when reachable through `.agent`, `.agents`, nested `.agents`, client junctions, or project links.
- Reparse targets are resolved one hop at a time with a visited identity set, maximum hop count, volume-boundary evidence, and broken-target diagnostics.
- Managed projections are excluded by ownership manifest plus physical identity, not by path prefix alone.
- A source path resolving inside a managed projection is rejected even when reached through a different junction or case spelling.
- A real client-owned source that several targets link to remains one observation with several visibility edges.
- `skills_old`, backups, generated distributions, dependency trees, caches, and archives are inventory evidence but default to non-source status.

## 15. Exhaustive reference-repository decomposition

Every direct child of `reference repos/` receives a machine-generated file manifest and a human donor decision ledger. The ledger is exhaustive at file-path level: each file is assigned `adopt`, `adapt`, `fixture`, `inspire`, `defer`, `reject`, `duplicate-of`, or `not-applicable`, with the ASM destination and verification case. A module cannot be marked reviewed solely from its README.

### 15.1 `cc-switch-main` — desktop integration and native serializer donor

Decompose the 1,174-file snapshot into these planes:

- **Target-native configuration:** `claude_*`, `codex_config`, `gemini_*`, `hermes_config`, `opencode_config`, `openclaw_config`, `prompt_files`, `mcp/*`. Adopt formats, path rules, import normalization, validation, and round-trip fixtures into target adapters.
- **Shareable-resource services:** MCP, prompt, skill, profile, import/export, deep-link parsers and sync protocol. Recompose validation and preview behavior behind Resource Hub; do not port its database authority.
- **Safety primitives:** database backup rotation, sync archive handling, JSON canonicalization, switch locks, configuration validation, post-change synchronization, error mapping, and sequential bulk action behavior. Compare each primitive against ASM equivalents and retain the stronger invariant/test.
- **UI patterns:** unified MCP/skills panels, MCP wizard and validation, prompt/skill preview, backup list, directory settings, import confirmation and risk preview. Adapt into Changes and decision queues, not separate product areas.
- **Explicit exclusions:** providers, OAuth/accounts, proxy/router, failover, subscriptions, pricing/usage, session manager, terminal, quota and credential features.

Supersession rule: when desktop and CLI contain peer implementations, desktop is the format/feature reference unless CLI has stricter isolation, command ergonomics, or a newer test. Record the winner per module; never blindly port both.

### 15.2 `cc-switch-cli-main` — headless control-surface and isolation donor

Decompose the 521-file snapshot into:

- Clap command shapes for `mcp`, `prompts`, `skills`, config backup/restore/import/export, environment diagnostics and completions;
- interactive/TUI routes, forms, help, tables, clipboard, keymaps, runtime skill display and terminal-safe formatting;
- target selection and application-specific config adapters;
- sandboxed test-home helpers, startup recovery, database migration/backup, daemon IPC/pidfile/restart patterns;
- WebDAV/S3 sync archive validation only as evidence for portable bundle handling.

Adopt CLI/TUI review and diagnostics as a thin ASM client. Reuse no independent state engine. Reject providers, proxy, usage, accounts, sessions and daemonized mutation authority. Where code duplicates CC Switch desktop, retain only CLI-specific presentation, isolation, or test coverage.

### 15.3 `skills-manager-main` — merge, deployment, audit and recovery donor

This repository contains the richest directly relevant backend. Review every module under `core/` and `commands/`:

- `merge/{protocol,snapshot,treebuild,validate,decision,pending,resolve,apply}` becomes primary evidence for three-way merge, conflict fingerprints, pending decisions, tree validation and snapshot-backed recovery;
- `content_hash`, `sync_metadata`, `tool_adapters`, `scanner`, `project_scanner`, `path_guard`, `repo_lock`, `file_watcher`, `audit_log`, `auto_backup`, `migrations`, and `install_cancel` become conformance inputs for identity, locking, observation, cancellation and recovery;
- `central_repo`, `skill_store`, `skill_metadata`, `installer`, `skill_auto_updater`, `git_fetcher`, and `github_api` inform source registration and update detection, but Resource Hub/CAS retain authority;
- presets, project/workspace views, source diffs, recovery dialogs and sync-status visualizations inform capability sets, target assignments, Changes and Activity.

Do not adopt its central repo, Git backup, crypto/credentials, or sync engine as parallel authorities. Transplant the strongest algorithms and fixtures into ASM-owned contracts.

### 15.4 `skills-manage-main` — broad discovery, collection and marketplace UX donor

Review backend commands for agents, collections, discovery, GitHub import, linker, marketplace, scanner, settings and skills; review frontend stores/pages for central library, platform views, project discovery, collections, marketplace, Obsidian vaults, global search, virtualized lists, frontmatter and markdown previews.

Adopt:

- broad platform catalog facts and custom-directory flows;
- collection semantics as named Resource Hub selection sets;
- virtualized inventory/search and progressively disclosed previews;
- repository-import preflight and frontmatter diagnostics;
- platform-specific installation preview concepts.

Reject its direct linker/install authority and database as canonical state. Obsidian support becomes a generic source plugin, not a privileged architecture branch.

### 15.5 `skill-zoo-main` — filesystem truth, local protocol and maintenance donor

Review persistence archive/external-import/metadata/update-history, services for CLI/GitHub/lock/skill/watcher/usage, desktop/CLI local protocol fixtures, agent path settings, consistency panels, maintenance/update manager, audit cards, file-tree/content panes, install/configure dialogs and pending-mutation hook.

Adopt or adapt:

- origin-aware discovery and real-path detection;
- visibility-scoped duplicate/conflict concepts;
- local protocol fixtures and desktop/CLI parity tests;
- archive limits, lock semantics, external-import identity and update history;
- consistency, maintenance, issue and audit presentation.

Do not adopt filesystem-as-global-authority or symlink-only deployment. Preserve its user-owned-source insight inside ASM observations while keeping Resource Hub canonical decisions and reviewed executor writes.

### 15.6 `agent-of-empires-main` — schema, plugin, migration and durable-operation donor

Most runtime orchestration is outside scope, but several higher-order mechanisms are valuable:

- settings-schema derive/registry/merge/resolve/validate provides a model for single-source target capability declarations and generated UI/API validation;
- plugin manifest, capability, integrity, lockfile, source, update-check, automation policy and sandbox modules inform a future declarative importer/adapter extension boundary;
- migrations provide sequential, idempotent schema/data migration discipline;
- durable event storage, worker registry and recovery patterns inform mutation journal durability without importing agent execution;
- project MCP, MCP overrides/state/model and skills model provide typed merge/conflict and project/global precedence fixtures;
- file provenance API and artifact models inform evidence exposure;
- directory guards, hooks targets and status files inform safe inert-hook projection;
- coverage-matrix and isolated-home E2E practices inform test planning.

Explicitly exclude ACP execution, tmux, terminals, worktrees, containers, sessions, remote dashboard, push, telemetry, sound and agent lifecycle. Plugin adoption is declarative and capability-restricted only; ASM must not inherit a general runtime plugin host in the first convergence.

### 15.7 `skill_library` — governance, semantics and evaluation donor

Treat all major planes as independent adoption candidates:

- 179 canonical skills and their file manifests, provenance, contracts, OpenAI metadata, assets, helpers and ten declared evaluation suites;
- 29 families, 537 capabilities, 327 routing edges, 37 profiles and 28 compositions;
- compatibility runtime matrix, artifact ownership, eight policies, release/toolchain contracts and 42 schemas;
- audit and build tools for architecture, schema, semantic fit, specificity, content quality, boilerplate, debris, routing, composition, ownership, receipts, performance, security, provenance, documentation, distribution parity and workflow quality;
- release-envelope, proof/verification receipt, result-contract, handoff-packet and suite-context schemas.

Adopt the ontology and contracts selectively into ASM-native types. Do not require legacy candidates to satisfy the full canonical contract before review. Convert Python tools into one of three outcomes: transplant invariant/tests into Go, retain as offline cross-check during transition, or reject as redundant. Declarative evaluation status must remain explicit until cases actually run; generated `dist` trees never become sources.

### 15.8 `AgentDNS-main` — empty/incomplete snapshot

The observed snapshot contains only empty directory structure and no reviewable files. Classify it `Unverified/No payload`, produce no adopted behavior, and keep a checksum/path receipt so later replacement with a populated snapshot triggers a fresh donor review rather than silently appearing complete.

## 16. Peer supersession and transplantation protocol

For every overlapping primitive across ASM and donors, create a peer comparison record:

| Field | Meaning |
|---|---|
| Capability | The behavior being compared, not the donor filename |
| Candidates | ASM and every donor module implementing it |
| Invariants | Safety, fidelity, determinism, performance and UX guarantees |
| Evidence | Tests, fixtures, schemas, failure handling and production constraints |
| Winner | Existing ASM, donor transplant, recomposed hybrid, or intentional coexistence |
| Delta | Exact missing behavior to port |
| Destination | ASM package/type/UI surface |
| Regression corpus | Normal, failure and integration-edge cases |
| Retirement | Which duplicate implementation becomes fixture-only or rejected |

Required comparison families include scanning, real-path identity, target catalogs, skill parsing, frontmatter, content hashing, archives, locks, backup/restore, three-way merge, conflict fingerprints, atomic writes, retries, cancellation, migrations, MCP serialization, source import, collections/profiles, audit logs, file watchers, update detection, diff presentation, virtualized inventory, and CLI/UI parity.

No donor module is copied wholesale. Apply this sequence:

```text
inventory -> extract invariant -> compare peers -> choose winner -> transplant fixture
-> implement ASM-native primitive -> differential test -> retire duplicate path
```

## 17. Automatic mutation retry and recovery protocol

Automatic retry belongs inside the deployment executor, below Change Set approval and above filesystem primitives. It must never re-plan, relax a precondition, overwrite foreign drift, or replay an MCP operation through the filesystem executor.

### 17.1 Durable operation state machine

```text
planned -> approved -> lock-wait -> preflight -> backed-up -> applying -> verifying -> committed
                         |             |             |           |
                         |             |             |           +-> verify-retry
                         |             |             +-> apply-retry
                         |             +-> blocked-drift
                         +-> lock-retry

any nonterminal -> interrupted -> reconcile -> resume | roll-forward | rollback | manual-review
```

Each journal entry includes Change Set/child/operation IDs, target identity, attempt number, timestamps, base/desired/current object descriptors, ownership evidence, backup locator/digest, mutation token, error class, retry decision, next-attempt time, verification result and recovery disposition.

### 17.2 Retry classification

- **Automatically retryable:** sharing violations, antivirus/indexer locks, transient rename/access errors, temporarily unavailable network-mounted target, bounded MCP transport timeout when the remote operation has an idempotency/read-back contract, and verification reads temporarily blocked by the same OS condition.
- **Retry only after reconciliation:** process interruption after backup or ambiguous write result. Inspect current/base/desired state; declare committed if desired is already present, safely resume only from a proved journal boundary, otherwise restore or require review.
- **Never automatically retry:** base digest/type/existence mismatch, ownership ambiguity, foreign collision, changed reparse target, path escape, permission-policy denial, schema/fidelity failure, missing backup, non-idempotent MCP ambiguity, or altered Change Set digest.

### 17.3 Retry policy

- Default attempts: initial try plus three retries.
- Backoff: decorrelated bounded exponential delay with persisted next-attempt time; suggested baseline 100 ms, 500 ms, 2 s, bounded at 5 s for interactive local files. Target contracts may narrow but not broaden retryable classes.
- Reacquire the per-target lock and rerun full preflight before every attempt.
- Never hold a process lock while sleeping; journal intent, release, wait, reacquire and revalidate.
- A successful atomic rename followed by a lost acknowledgement is reconciled by desired digest and ownership marker, not repeated blindly.
- Cancellation stops before the next mutation boundary and leaves an explicit recoverable journal state.
- Retry exhaustion emits a blocked receipt with exact recovery options and preserves operator selections for rebuilding.
- MCP retries require an operation-specific idempotency key or deterministic read-after-write comparison. Otherwise ambiguous timeout becomes manual reconciliation.

### 17.4 Retry tests

Use deterministic fault injection at open, lock, backup, temp-write, fsync, rename, directory-entry update, metadata restore, verify and receipt-seal boundaries. Cover process termination between every adjacent journal state; restart recovery; two ASM processes contending for one target; target edit during backoff; junction retargeting; disk-full; read-only media; partial MCP success; and rollback failure. The invariant is exactly one of: verified desired state with committed receipt, verified original state with rollback receipt, or untouched/blocked state with sufficient evidence for manual recovery.

## 18. Second-order convergence pass

After first-order primitives are proven by OpenCode and one MCP profile, perform a second-order pass over larger planes. A plane is admitted only when at least two proven primitives compose without bypassing Resource Hub or the Change Set.

1. **Target Contract Plane:** single-source declarations generate discovery rules, capability matrices, adapter validation, UI fields and conformance cases, inspired by AoE settings schemas and donor target catalogs.
2. **Governance Plane:** skill-library families, capabilities, policies, ownership and compatibility become versioned Resource Hub metadata with explicit provenance and migration.
3. **Composition and Routing Plane:** named capability sets and compositions recommend resources based on evidence; routing never silently activates or mutates targets.
4. **Evaluation Plane:** static, behavioral, adversarial, failure-injection and receipt-integrity suites execute through a versioned harness; claims distinguish declared, tested and verified.
5. **Provenance Plane:** every canonical resource and projection exposes source chain, transformations, donor decisions, fidelity loss, ownership and receipts.
6. **Extension Plane:** declarative source/importer/adapter packages may be registered through signed manifests and constrained capabilities; no general code plugin execution in the initial release.
7. **Migration Plane:** sequential idempotent migrations cover registry, artifact, target-catalog, ownership-marker and journal schemas, with downgrade/recovery evidence.
8. **Maintenance Plane:** update detection, consistency audits, stale projection detection, quarantine, retirement windows and recovery retention become scheduled read-only checks that produce proposed Change Sets.
9. **Operator Plane:** Home, Library/Targets, Changes and Activity share one state model; CLI/TUI/Web are clients of the same application service and receipts.
10. **Observability Plane:** bounded progress, audit events, retry state, latency, scan budgets and error taxonomy are exposed without collecting credentials or session content.

Second-order convergence explicitly excludes provider routing, agent execution, session/worktree management, subscription/quota management, credentials and general-purpose runtime plugins.

## 19. Revised granular execution backlog

### Epic A — Evidence and authority

- A1 generate workstation-root manifest with physical identities and nested-client graph;
- A2 generate exhaustive donor file manifests for all eight snapshots;
- A3 create peer comparison ledger and donor decision schema;
- A4 freeze canonical envelope, CAS, index, importer, adapter, executor and `mcplink` authority contracts;
- A5 add release/build guards for donor/reference and workstation artifacts.

### Epic B — Canonical model and safe observation

- B1 artifact-linked observation/candidate/decision records;
- B2 source bundle registration for directory and ZIP inputs;
- B3 junction/reparse-aware scanner with budgets and cancellation;
- B4 nested-client target catalog and confidence model;
- B5 managed-projection manifests and alias-proof source exclusion;
- B6 deterministic inventory and differential scan benchmarks.

### Epic C — Identity and convergence intelligence

- C1 physical/content/normalized identity pipeline;
- C2 namespace-aware aliases and Windows collision rules;
- C3 asset/reference-aware comparison;
- C4 semantic relationship evidence and versioned evaluation corpus;
- C5 canonical-winner recommendations and reviewed decisions;
- C6 absorption/split/parameterization/composition records with reversal.

### Epic D — Projection and execution

- D1 pure adapter contract and donor-derived fixture corpus;
- D2 OpenCode render/plan/verify adapter;
- D3 deployment executor with locks, backups, journals and atomic replacement;
- D4 automatic retry, interruption reconciliation and idempotent recovery;
- D5 OpenCode end-to-end shadow, apply, verify, rollback and visibility proof;
- D6 Codex/Cursor, then Claude/Gemini/Antigravity/Windsurf adapter waves.

### Epic E — MCP convergence

- E1 canonical MCP identity and secret-free intent;
- E2 target-native serializers and observed-registration comparison;
- E3 capability sets and minimal assignments;
- E4 `mcplink` idempotency/read-back and retry classification;
- E5 mixed resource/MCP Change Set partial-success recovery.

### Epic F — Operator control plane

- F1 universal Change Set composition and one digest-bound approval;
- F2 exact diff, fidelity, ownership, retry and recovery detail;
- F3 grouped consolidation and conflict decision queues;
- F4 Activity journal, receipts, retry progress and recovery actions;
- F5 CLI/TUI/Web parity contracts and accessibility/large-list testing.

### Epic G — Second-order governance and migration

- G1 target-contract generator;
- G2 governance/capability/family/profile import from the skill library;
- G3 composition/routing and evaluation harnesses;
- G4 provenance and evidence APIs;
- G5 sequential schema/data migrations;
- G6 scheduled read-only maintenance proposals;
- G7 bounded corpus campaigns and independently recoverable client migrations;
- G8 retirement only after observation windows and receipt retention gates.

## 20. Final acceptance gates

The program is complete only when:

- all observed AI-related dotted roots have a disposition and every nested client is represented separately;
- all eight reference snapshots have exhaustive file manifests and no unclassified module/file remains;
- every overlapping primitive has a peer comparison winner and differential regression evidence;
- one physical source cannot appear as multiple canonical candidates through junctions or aliases;
- adapters, importers, indexes and UI remain mutation-free;
- every write is approved, precondition-bound, journaled, retry-classified, verified and independently recoverable;
- automatic retry cannot overwrite drift or repeat an ambiguous non-idempotent operation;
- OpenCode plus Codex/Cursor and one mixed MCP/resource scenario pass normal, injected-failure, interruption and rollback suites;
- skill-library governance layers are imported with declared/tested/verified status distinctions;
- donor snapshots and generated projections remain outside source ingestion and release artifacts;
- client-native credentials, sessions, caches, histories, provider routing and runtime state remain untouched.
