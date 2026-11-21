# ASM Fabric Phase 4: Versioned Target Adapters and Fidelity Evidence

## Evidence basis

Phase 4 implements the target-adapter slice described in the uploaded ASM research reports, especially `deep-research-report (9).md`:

- one versioned adapter contract for discovery, import, render, plan, verification, and capability negotiation;
- structural capability declarations instead of informal support booleans;
- machine-readable fidelity and loss reports for transformations, fallbacks, omissions, and unsupported fields;
- adapters that propose deterministic projections but never perform uncontrolled writes;
- continued use of ASM's existing reviewed Resource Hub and MCP client executors as the only mutation authorities.

The implementation is target-native Go. No donor runtime, plugin host, package manager, filesystem synchronizer, or target CLI wrapper was embedded.

## Converged decision

ASM now separates **target interpretation** from **mutation authority**:

```text
canonical artifact or MCP registration intent
  -> versioned adapter capability snapshot
  -> deterministic render and observed-state normalization
  -> machine-readable fidelity/loss report
  -> proposed operation
  -> existing reviewed Resource Hub or mcplink plan
  -> explicit-confirmation apply by the existing authority
```

Adapters are pure with respect to side effects. They cannot write files, execute target CLIs, change ownership state, create backups, approve a plan, or mutate SQLite/CAS. Resource Hub and `mcplink` retain live-state inspection, before-state revalidation, transactional apply, rollback, and recovery.

## Versioned contract

`internal/adapters` defines `fabric.asm.dev/adapter/v1alpha1` with these operations:

- `ID` and `SchemaVersion` identify the implementation and protocol;
- `Capabilities` returns a sealed structural capability snapshot;
- `Discover` normalizes observations supplied by an authoritative core module;
- `Import` validates a canonical candidate and reports representational losses;
- `Render` maps one canonical artifact to deterministic target projections;
- `Plan` classifies create, update, remove, no-op, and conflict transitions;
- `Verify` compares a reviewed postcondition with a core-supplied observation.

The contract is non-I/O by design. Discovery candidates and post-apply observations are supplied by core modules that already own the relevant filesystem or client-registration boundary.

## Capability model

Every capability snapshot is digest-bound and contains:

- canonical target and aliases;
- adapter and contract versions;
- target-version range;
- artifact-kind support level: `native`, `passthrough`, `fallback`, or `unsupported`;
- supported scopes, directories, formats, transports, and field mappings;
- deployment modes;
- MCP registration mode, location, root key, entry name, and transports.

Aliases such as `gemini`, `gemini-cli`, and `antigravity` resolve to the canonical `agy` adapter. Alias collisions with another canonical target or alias are rejected when the registry is constructed.

## Fidelity and loss model

A sealed loss report binds target, adapter version, capability digest, fidelity, and sorted loss records. Fidelity is derived rather than accepted from callers:

| Loss kind | Derived fidelity | Meaning |
| --- | --- | --- |
| none | `full` | represented without reported loss |
| transformation | `partial` | content is preserved but target-native semantics are not fully normalized |
| fallback | `lossy` | preserved in an ASM fallback location rather than a native target surface |
| omission | `lossy` | a field or behavior is omitted |
| unsupported | `blocked` | the required representation is unavailable |

Each loss includes artifact ID, field path, stable code, reason, and whether the field is required. Duplicate loss identities, mismatched capability identities, invalid digests, and altered reports fail closed.

Resource Hub exposes `--deny-loss`, which rejects planning whenever the aggregate report contains any transformation, fallback, omission, or unsupported record.

## Built-in target adapters

`internal/adapters/builtin` contains reviewed in-process adapters for:

| Target | Resource projections | MCP registration |
| --- | --- | --- |
| Codex | native skill directories; reviewed byte-preserving agents, rules, and prompts | reviewed `codex mcp` command path |
| Claude | native skill directories; reviewed byte-preserving agents, rules, commands, and prompts | project `.mcp.json` |
| Cursor | native skill directories; reviewed byte-preserving agents, rules, commands, and prompts | project `.cursor/mcp.json` |
| AGY/Gemini | resource projection intentionally not claimed in this slice | explicit AGY config or `~/.gemini/config/mcp_config.json` |
| OpenCode | native skill directories; reviewed byte-preserving agents, rules, commands, and prompts | project `opencode.json` |
| GitHub Copilot | instruction and prompt projections; other kinds use visible fallback storage | not supported by current MCP linker |
| Generic | visible ASM fallback storage | not supported |

Resource projections are confined beneath the registered target root. Rendered relative paths reject absolute paths and traversal. Desired content digests, capability snapshots, rendered sets, and loss reports are deterministic and independently verifiable.

## Resource Hub integration

Resource Hub sync planning now obtains destinations and support evidence from the target adapter instead of a hard-coded target switch. Every plan and operation binds:

- adapter ID and version;
- capability digest;
- fidelity and loss-report digest;
- per-operation losses;
- existing registry, source, destination, and expiry evidence.

Apply re-resolves the live adapter capability and rejects drift after review. It also reconstructs and verifies every operation-level loss report before the first mutation. Existing security audit, ownership, conflict, before-state, backup, rollback, and explicit-confirmation behavior remains in Resource Hub.

Existing short-lived Resource Hub or MCP plans created before this contract contain no capability evidence and are intentionally invalid after upgrade. Operators regenerate them; no durable resource, target, backup, or client state is migrated.

## MCP client integration

`mcplink` now resolves Codex, Claude, Cursor, AGY/Gemini, and OpenCode registration locations and registration modes from adapter capabilities. Reviewed link/unlink plans include sorted capability snapshots and loss reports. Apply rejects:

- unsupported contract versions;
- invalid or duplicate capability/loss snapshots;
- operation-to-snapshot identity mismatch;
- altered per-operation fidelity evidence;
- live capability drift after review;
- existing foreign same-name registrations or changed client state.

The existing codecs still preserve unrelated client configuration. Existing `mcplink` code remains the only file/CLI mutation and rollback authority.

## Operator surface

```text
agentstack hub adapters [--project-root PATH] [--target-root PATH] [--target TARGET]...
agentstack hub plan-sync --target ID [--resource ID]... [--deny-loss]
```

`hub adapters` is read-only and emits the contract version plus sealed capability snapshots. `--target` accepts canonical IDs and registered aliases.

Normal `hub plan-sync` output now includes the aggregate fidelity report and operation-level losses. `--deny-loss` is a planning policy gate; it does not modify target state.

MCP client plans include capability snapshots and loss reports automatically. Current built-in MCP registration paths are full fidelity; unsupported MCP targets are rejected before a plan is issued.

## Validation coverage

Tests cover:

- deterministic capability, rendered-set, and loss-report digests;
- alias resolution and collision-safe registry construction;
- invalid digests, mismatched report identities, traversal, and duplicate-loss rejection;
- closed create/update/remove/no-op/conflict transition behavior;
- built-in target capability snapshots and confined destinations;
- visible transformation and fallback reporting;
- Resource Hub aggregate reports, `--deny-loss`, operation tampering, and capability drift;
- MCP capability/loss snapshots and tampered operation-loss rejection;
- CLI alias resolution and machine-verifiable capability output;
- existing Resource Hub and MCP mutation, conflict, rollback, secret-preservation, and full regression behavior.

## Explicit non-goals

Phase 4 does not:

- allow adapters to access the filesystem or execute target commands directly;
- promote SQLite, CAS, or adapters to mutation authority;
- implement an external adapter process protocol or untrusted plugin loading;
- claim target-native semantic compilation for byte-preserving Markdown projections;
- support AGY/Gemini Resource Hub projection without a reviewed target contract;
- enable MCP registration for Copilot or generic targets;
- install packages, fetch remote registries, or change release/deployment state;
- claim round-trip equivalence for fields explicitly reported as transformed, omitted, fallback, or unsupported.

The target conformance fixture corpus is implemented in [Fabric Phase 5](FABRIC_PHASE5.md), and the constrained SHA-256-pinned out-of-process differential host is implemented in [Fabric Phase 6](FABRIC_PHASE6.md). External activation remains gated on a publisher-bound package/lockfile and enforceable WASI or OS sandboxing.
