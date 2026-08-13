# Unified Resource Authority Contract

This contract governs the shareable-resource convergence program. It complements the existing Fabric trust/state documents and takes precedence for the new resource-control-plane work.

## Scope

ASM manages only portable skills, agents, prompts, rules, commands, workflows, inert hook definitions, MCP definitions, and target projections. Credentials, accounts, provider routing, subscriptions, sessions, caches, logs, plugins with executable runtime behavior, IDE settings, and agent execution remain client-owned.

## Authorities

| Boundary | Sole authority | May not do |
|---|---|---|
| Resource Hub | Canonical identity, lifecycle, admission/alias/retirement decisions, target bindings, reviewed child plans | Treat client files, donor snapshots, or generated projections as canonical state |
| `artifactgraph.Artifact` | Versioned canonical artifact envelope | Carry an independent target runtime or workstation state model |
| CAS | Immutable bytes and historical reconstruction | Decide identity, lifecycle, or deployment |
| Indexes | Rebuildable inventory, search, duplicate and advisory similarity evidence | Make canonicalization or merge decisions |
| Importers/scanners | Read-only observations and candidate revisions | Write a target, promote a resource, or follow managed projections |
| Target adapters | Deterministic normalization, capability declaration, rendering, loss reporting, and verification | Mutate a target or retain hidden state |
| Deployment executor | Filesystem mutation after sealed review, preflight, journaling, verification and recovery | Change MCP registration fields |
| `mcplink` | MCP registration mutation after sealed review | Write generic resource projections |
| Change Set | Operator-facing composition, approval digest, child-plan ordering and receipts | Bypass domain executor preconditions |
| Client roots | Native state and explicit local source/inbox material | Become a second canonical store |

## Direction of data flow

```text
registered source + local source path -> observation -> candidate revision
-> Resource Hub decision -> adapter operation plan -> sealed Change Set
-> deployment executor or mcplink -> verified receipt
```

Reference repositories feed the donor ledger, target-catalog evidence, fixture corpus, and peer comparison records. They do not enter the observation path unless a user deliberately registers a bounded source bundle.

## Non-negotiable invariants

1. A managed projection cannot be re-ingested through another path, junction, case spelling, or alias.
2. A physical object has one observed identity even if exposed by several logical client paths.
3. Every mutation binds base object descriptor, desired descriptor, target identity, ownership evidence, sealed plan digest, journal operation ID, and recovery record.
4. Foreign, divergent, stale, ambiguous, or unsupported target state fails closed without mutation.
5. Automatic retry never changes the plan, relaxes a precondition, or repeats an ambiguous non-idempotent operation.
6. A resource source edited after admission produces a new candidate revision; it does not mutate the canonical artifact.
7. Similarity is advisory evidence only. It cannot automatically merge, delete, parameterize, or retire resources.
8. Every target is independently recoverable. Multi-target execution may be partially successful but is never described as globally atomic.

## State ownership labels

Every observed resource and target path exposes one of: `managed-by-asm`, `client-owned`, `imported-copy`, `candidate-source`, `ignored`, `quarantined`, or `unknown`. `unknown` is read-only until reviewed.

## Change control

Changes to these boundaries require a decision-log entry, a focused invariant test, a normal/failure/integration case, and a migration/recovery statement. No donor abstraction is introduced until the peer comparison ledger names its destination and retirement decision.
