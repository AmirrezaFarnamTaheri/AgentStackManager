# Peer Project Convergence

AgentStack Manager now absorbs the strongest compatible semantics from seven peer projects into one Go control plane. The result keeps one authority model, one persistence boundary, one reviewed-mutation contract, one process supervisor, and one release pipeline. Donor package layouts and duplicate runtimes were decomposed before adoption.

## Scope

| Donor | Surfaces accounted | Primary value absorbed | Disposition boundary |
| --- | ---: | --- | --- |
| `ai-setup` | 349 | repository fingerprints, context scoring, multi-agent context refresh, explicit retained memory | hidden Git mutation became a guardrail; no auto-staging |
| `context-sync` | 59 | layered memory, safe project retrieval, Git context, schema migration, workspace discovery | product-specific Notion credentials were replaced by explicit routine adapters |
| `skillshare` | 1,188 | canonical resource registry, audit, copy/link sync, managed pruning, backups, source refresh, target adapters | donor server/TUI was superseded by ASM CLI and loopback UI |
| `skills-hub` | 186 | tagged resource store, content hashes, scheduled refresh workflows, typed target adapters | direct online install was rejected; local reviewed checkout is required |
| `mcp-linker` | 319 | multi-client linking, cross-client plans, MCP definition CRUD, Codex registration | saved provider credentials and parallel desktop authority were rejected |
| `AIaW` | 550 | hierarchical workspaces, prompt variables, artifacts, local versioned state, extension admission, MCP association | chat/billing and platform shells remain reference evidence outside ASM's control-plane role |
| `LifeSync-AI` | 22 | schedules, sequential automation, durable receipts, recovery | hard-coded weather/email providers became bounded command seams; secrets remain external |

The evidence set contains 53 semantic records, 44 primary record tests, and 60 unique record/composition/negative-path test links. Dispositions: 13 adapted, 11 recomposed, 9 hardened, 2 inspired-native, 2 guardrail-derived, 3 superseded, 4 rejected with a target substitute, and 9 context/reference records.

## Unified architecture

### 1. Resource hub

`internal/resourcehub` owns skills, agents, rules, commands, prompts, MCP server definitions, and context resources.

- canonical identity, content digest, tracked source, tags, scope, metadata, and target declarations;
- whole-resource ceilings of 10,000 files and 64 MiB, plus a 16 MiB per-file ceiling;
- local import only; no remote result can install itself;
- static admission audit for prompt injection, exfiltration, credentials, destructive behavior, fetch-pipe patterns, hidden HTML, and invisible Unicode;
- expiring digest-bound sync and refresh plans;
- target-native copy/link paths for Codex, Claude, Cursor, OpenCode, GitHub Copilot, and explicit generic roots;
- foreign-file preservation, AgentStack-only pruning, immutable backups, confirmed restore, and transactional rollback.

### 2. Project intelligence and context

`internal/contextengine` owns deterministic repository evidence and managed agent context.

- bounded stack, manifest, framework, command, and source fingerprinting;
- evidence-based context readiness scoring;
- safe file reads and search confined to a canonical project root;
- UTF-8-safe snippets and hard scan/result/byte ceilings;
- read-only Git branch, revision, status, and diff-stat collection through the supervised runner;
- managed blocks for `AGENTS.md`, `CLAUDE.md`, Cursor rules, and Copilot instructions while preserving human text;
- reviewed refresh with project-fingerprint and before/after digest validation.

### 3. Workspaces, memory, prompts, and artifacts

`internal/workspace` owns operator organization and local knowledge.

- acyclic folder/workspace hierarchy;
- canonical local roots plus resource and routine associations;
- user, project, workspace, and session memory with deterministic precedence, explicit scope, TTL, digest validation, search, and deletion;
- strict prompt-variable expansion with workspace and time built-ins;
- regular-file-only, size-bounded, content-addressed artifacts;
- schema-versioned atomic persistence with legacy migration and fail-closed future versions.
- durable multi-file transaction journals that fence live readers, restore stale interrupted writes, and separate committed authority from deferred cleanup.

### 4. MCP client linking

`internal/mcplink` links the existing bounded ASM router into Codex, Claude, Cursor, AGY/Gemini, and OpenCode.

- one AgentStack-owned entry per client;
- expiring reviewed link/unlink plans;
- before-state revalidation, minimal registration-only recovery records, and foreign-entry preservation;
- reviewed plans persist only client, action, path, and before/after digests; live configuration is reconstructed after revalidation at apply time;
- duplicate-key JSON and secret-bearing same-name entries fail closed, so unrelated client credentials never enter ASM plans or backups;
- existing Codex registration logic reused instead of duplicating a TOML authority;
- MCP definitions remain resource data; process authority stays in `mcp.ManagedChildRuntime`.

### 5. Scheduled routines

`internal/routines` generalizes donor-specific cron jobs and update schedulers.

- manual, daily, weekday, and interval schedules;
- timezone-aware deterministic next-run calculation;
- up to 32 sequential typed steps with explicit confirmation, stop-on-failure, a 24-hour total-run ceiling, and bounded command/argument/parameter/definition sizes;
- bounded direct-binary command steps—never shell strings;
- secret-redacted, schema-versioned run receipts, bounded history, and interrupted-persistence reconciliation;
- secret-bearing routine arguments are rejected at admission; explicit environment/file/reference keys remain allowed while provider credentials stay external.

## Cross-plane contracts

- **Authority:** import, inspection, scoring, recall, and planning are non-destructive. Sync, refresh, linking, deletion, restoration, and routine execution require explicit confirmation or exact plan identity plus digest.
- **Identity:** durable records use stable IDs; consequential plans bind ID, digest, source/registry/project state, expiry, and target.
- **Persistence:** each plane owns one versioned local store under ASM's private data root. Writes use staged replacement; unknown future schemas fail closed.
- **Process execution:** all external commands flow through `runner.CommandRunner` and the supervised process boundary with deadlines and output limits.
- **Secrets:** provider tokens are not fields in resource, workspace, MCP-link, or routine definitions. Diagnostic and routine receipt text crosses the shared redaction boundary.
- **Recovery:** target writes preserve foreign state, create backups where replacement occurs, and retain receipts sufficient to inspect or restore AgentStack-owned changes.

## Operator surfaces

All adopted capabilities are available through JSON CLI commands. The authenticated loopback dashboard exposes a read-only unified-fabric status surface. UI state never authorizes a mutation.

See [CLI Reference](CLI_REFERENCE.md), [Convergence Runbook](convergence/RUNBOOK.md), and [Trust and State Model](convergence/TRUST_AND_STATE.md).

## Evidence

- [Adoption ledger](convergence/ADOPTION.csv)
- [Surface accountability matrix](convergence/SURFACES.csv)
- [Donor analysis](convergence/DONOR_ANALYSIS.md)
- [Omission audit](convergence/OMISSION_AUDIT.md)
- [Validation record](convergence/VALIDATION.md)
- [Pre-mortem](convergence/PREMORTEM.md)
- [Archive inspection](convergence/ARCHIVE_INSPECTION.json)
- [Inventory summary](convergence/INVENTORY_SUMMARY.json)
