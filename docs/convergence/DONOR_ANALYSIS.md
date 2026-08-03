# Donor Mechanism Analysis

This document records what each donor contributed, how its mechanics changed, and where its semantics now live. Repository popularity and packaging did not decide adoption. Correctness, bounded authority, recovery, operator clarity, and fit with ASM's existing control plane did.

## `ai-setup`

### Absorbed

- Repository fingerprinting became deterministic local scanning in `contextengine.Manager.Scan`.
- Context scoring became an evidence-based native score whose findings identify concrete missing or stale surfaces.
- Context regeneration became managed-block refresh with before/after hashes, project fingerprints, expiry, backups, and preservation of human-authored text.
- Per-agent setup files became one target model shared by context refresh and resource distribution.
- Session learning became explicit scoped memory with TTL, digest, recall precedence, search, and deletion.
- Source selection became tracked local source metadata and reviewed resource refresh.

### Rejected mechanism

Automatic Git-hook installation and auto-staging expanded hidden authority. ASM preserves the freshness goal through read-only Git context and reviewed plans. Tests prove the inspection path executes only bounded read commands.

## `context-sync`

### Absorbed

- User/project/workspace/session layers became typed memory scopes.
- Remember, recall, search, and forget became bounded local operations.
- File retrieval became canonical-root, symlink-safe, byte-limited reading and searching.
- Git context runs through the supervised command boundary.
- Workspace detection and project profiling were recomposed with hierarchy, prompt, artifact, memory, resource, and routine ownership.
- Schema migration became explicit versioned envelopes with legacy readers and future-version rejection.

### Rejected mechanism

The built-in Notion connector stored product-specific credential concerns inside the context system. ASM uses an explicit direct-command routine seam, leaving credentials with the provider or OS store.

## `skillshare`

### Absorbed

- The registry became a generalized resource hub covering seven resource kinds.
- Copy/link synchronization became digest-bound reviewed planning, foreign-file conflict detection, and AgentStack-only pruning.
- Static skill audit became a broader resource admission boundary.
- Backups became immutable, digest-verified replacement/removal snapshots with confirmed restore.
- Source lifecycle became local tracked-source refresh; source and canonical digests are both revalidated at apply.
- Target adapters became target-native paths for supported agent clients.

### Superseded mechanism

The donor daemon, TUI, and web server would create a second authority. ASM exposes the same operator outcomes through its existing CLI and authenticated loopback dashboard.

## `skills-hub`

### Absorbed

- Managed store fields—tags, scope, source, targets, metadata—became canonical resource attributes.
- Content hashes and bulk sync strengthened reviewed resource plans.
- Auto-update scheduling became explicit routines and source-refresh plans with receipts.
- Tool adapters became typed target roots and target-kind path mappings.

### Rejected mechanism

Featured online search and direct GitHub download allowed remote discovery to approach installation authority. ASM accepts a local reviewed checkout only. A failed URL-shaped import cannot mutate the registry.

### Superseded mechanism

Managed/explore/update/settings views became CLI operations plus unified status instead of a second desktop product.

## `mcp-linker`

### Absorbed

- File adapters for Claude, Cursor, AGY/Gemini, and OpenCode became strict client operations.
- Cross-client sync became expiring reviewed link/unlink plans with state revalidation and backups.
- MCP server CRUD became `mcp-server` resources, separating definition storage from runtime authority.
- Codex linking reuses ASM's existing registration implementation.

### Rejected mechanism

Encrypted credential storage remained a new secret authority even when encryption was sound. ASM stores no provider credentials in general fabric state.

### Superseded mechanism

Desktop MCP management became CLI linking plus read-only dashboard status. The router and configuration files remain authoritative.

## `AIaW`

### Absorbed

- Multiple workspaces became an acyclic local folder/workspace model.
- Dynamic prompt variables became strict deterministic expansion.
- Artifacts became bounded content-addressed local records.
- Browser-reactive persistence became versioned atomic local stores.
- Plugins, prompts, agents, commands, rules, and MCP definitions became audited resource kinds.
- Workspace MCP configurability combined with ASM's existing managed child runtime and MCP client linker.

### Reference-only surfaces

Multi-provider chat, billing, PWA, Android, and Tauri shells solve a different product problem. They remain provenance and future product evidence. Importing them would create provider, billing, message-history, and platform authorities unrelated to managing local agent infrastructure.

## `LifeSync-AI`

### Absorbed

- Morning/night jobs became generic manual/daily/weekday/interval schedules.
- Fetch/summarize/deliver scripts became bounded sequential typed steps and explicit direct-command adapters.
- Ad-hoc result inspection inspired durable run receipts, history, bounded retention, and restart reconciliation.

### Rejected mechanisms

- Hard-coded weather API calls became an operator-owned bounded command adapter.
- Email/provider secrets remain outside ASM state.
- Routine receipt outputs and errors are recursively redacted before persistence.
