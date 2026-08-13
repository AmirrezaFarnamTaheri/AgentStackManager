# Unified Agent Resource Control Plane Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify ASM, the governed skill library, and locally installed agent resource trees into one preservation-first control plane for skills, agents, prompts, rules, commands, workflows, and MCP definitions.

**Architecture:** Resource Hub remains the canonical decision authority and CAS remains immutable storage. Read-only importers turn the skill library and local client roots into observations and candidates; pure adapters render reviewed resources into client-specific projections. `.agents` becomes a managed compatibility facade and optional authoring inbox, never a second canonical database.

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
- portable hook definitions;
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
| `.agents` | Managed compatibility projection plus explicit authoring inbox |
| Client roots | Native client state, explicit overlays, and disposable managed projections |

Dependency direction:

```text
skill library + local roots + donor fixtures
                    |
                 observe
                    v
        Resource Hub canonical intent
          |          |           |
         CAS       index     Change Set
                                |
                   pure target adapters
                                |
              reviewed projections + MCP links
```

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

Observed and candidate resources use a lightweight envelope. Only promoted canonical skills require the governed skill-library contract.

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

All donor snapshots are read-only evidence under `reference repos/`. They are excluded from builds and releases and never become runtime dependencies.

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

## 7. Consolidation policy

- Exact duplicates: one canonical payload, all prior names retained as aliases.
- Normalized-body duplicates: require metadata and asset comparison.
- Semantic relationships: equivalent, subset, superset, complementary, alternative, conflicting, or unrelated.
- Parameterization: allowed only when variation is data, such as venue or provider metadata.
- Absorption: must record preserved unique material, rejected material, lineage, regression cases, and reversal path.
- Projection output is never eligible as a new import source.

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
5. Seed the donor ledger with the seven local snapshots and exact retained/rejected scopes.
6. Run `go test ./internal/releasepack ./internal/supplychain -count=1`.
7. Run `pwsh -File scripts/check-governance.ps1`.
8. Review `git diff --check` and commit the task.

### Task 2: Add lightweight observed/candidate resource envelopes

**Files:**
- Modify: `internal/resourcehub/types.go`
- Create: `internal/resourcehub/lifecycle.go`
- Create: `internal/resourcehub/lifecycle_test.go`
- Modify: `internal/artifactgraph/model.go`
- Modify: `internal/artifactgraph/model_test.go`
- Modify: `internal/resourcehub/manager.go`

**Steps:**

1. Write table-driven tests for valid and invalid lifecycle transitions.
2. Add failing tests for aliases, source observations, physical identity, schema findings, and preservation decisions.
3. Run the focused tests and verify failure.
4. Implement the smallest versioned envelope compatible with existing `Resource` records.
5. Preserve backward decoding of registry version 1; do not silently promote imported resources.
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
3. Add Codex, Claude, OpenCode, Cursor, Windsurf, Gemini, Antigravity, Hermes, Copilot, OpenClaw, Kiro, Roo, Continue, Goose, Qwen, and Kimi.
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
5. Implement OpenCode first, then Codex and Cursor.
6. Extend to Claude, Gemini/Antigravity, and Windsurf only after the first vertical slice passes.
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
5. Prove restore returns the exact pre-apply bytes.
6. Run race-enabled Resource Hub tests.
7. Document the operator flow.
8. Commit the task.

### Task 10: Compose one operator-facing Change Set

**Files:**
- Create: `internal/changeset/types.go`
- Create: `internal/changeset/coordinator.go`
- Create: `internal/changeset/coordinator_test.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/progress.go`
- Modify: `internal/ui/operations.go`
- Modify: `internal/ui/web/changes.js`
- Modify: `internal/ui/web/index.html`

**Steps:**

1. Write failing tests that compose Resource Hub and `mcplink` child plans without transferring their mutation authority.
2. Bind child identities, digests, expiry, registry state, capability evidence, and recovery descriptions into one sealed Change Set.
3. Reject any stale or altered child before execution.
4. Execute independent targets with honest partial-success reporting.
5. Retain operator selections when rebuilding an invalidated plan.
6. Show exact operations, before/after digests, fidelity, ownership, and recovery details.
7. Keep one approval surface while preserving per-domain executors.
8. Run app, UI contract, accessibility, and race tests.
9. Commit the task.

### Task 11: Canonicalize MCP definitions and profiles

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

## 11. Completion criteria

- Resource Hub is the only canonical decision authority.
- `.agents` is a managed facade and explicit inbox, not a competing store.
- Client roots contain native state and disposable projections.
- Every local resource is canonical, candidate, alias, ignored, retired, or quarantined.
- Exact duplicates have one canonical payload and working legacy aliases.
- No projection can be re-imported as a source.
- Every transformation reports fidelity.
- Every managed write originates from a reviewed Change Set.
- Each target is independently recoverable.
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

## 13. Implementation handoff

Start with Tasks 1–4 only. Do not begin adapter expansion or workstation mutation until the authority, source-bundle, lifecycle, and junction-aware observation contracts pass their focused tests and the OpenCode shadow target is available.
