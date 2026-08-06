# ASM Fabric Phase 5: Target Adapter Conformance Corpus

## Evidence basis

Phase 5 implements the next dependency-ordered slice identified after the versioned adapter contract in Phase 4:

- a checked-in, machine-readable oracle for every reviewed built-in adapter;
- differential comparison of structural capability snapshots against explicit target fixtures;
- candidate-preserving render/import round trips for every declared artifact projection;
- target-by-target plan-state and postcondition verification;
- a sealed read-only report that operators and CI can verify independently;
- no external adapter process, plugin loading, target writes, or authority transfer.

The implementation remains target-native Go. The fixture corpus records reviewed ASM expectations; it does not embed donor runtimes or treat target documentation as executable instructions.

## Converged decision

The adapter contract is now guarded by a stable evidence loop:

```text
embedded reviewed corpus
  -> strict JSON admission and structural sealing
  -> built-in adapter capability discovery
  -> differential capability comparison
  -> canonical artifact render
  -> deterministic observation normalization
  -> candidate-preserving import
  -> loss-report comparison
  -> create/update/remove/no-op/conflict matrix
  -> postcondition verification
  -> sealed conformance report
```

This is a read-only test and inspection path. Resource Hub and `mcplink` remain the only target mutation authorities.

## Corpus contract

`internal/adapters/conformance/testdata/corpus.json` uses:

- corpus API `fabric.asm.dev/adapter-conformance/v1alpha1`;
- adapter contract `fabric.asm.dev/adapter/v1alpha1`;
- exact canonical target, adapter ID, adapter version, and target-version range;
- canonical aliases and deployment modes;
- MCP support, registration mode, location class, keys, entry name, and transports;
- one fixture for every declared artifact capability;
- explicit unsupported fixtures where absence is intentional;
- expected directory, format, field support, relative destination, fidelity, and stable loss identities.

The loader rejects unknown fields, duplicate JSON keys, duplicate targets, alias collisions, duplicate artifact kinds, non-canonical or traversing paths, invalid support values, malformed loss identities, and fidelity values that do not follow from the declared losses. The in-memory sealed corpus has a deterministic SHA-256 digest.

## Coverage

The embedded corpus covers all seven built-in targets:

| Target | Artifact fixtures | MCP fixture | Alias evidence |
| --- | ---: | --- | --- |
| Codex | 7 | command registration | `openai-codex` |
| Claude | 7 | project `.mcp.json` | `claude-code` |
| Cursor | 7 | project `.cursor/mcp.json` | none |
| AGY/Gemini | 1 explicit unsupported resource projection | explicit AGY config | `antigravity`, `gemini`, `gemini-cli` |
| OpenCode | 7 | project `opencode.json` | none |
| GitHub Copilot | 7 | unsupported | `copilot` |
| Generic | 7 | unsupported | none |

The complete suite contains 64 cases:

- 7 structural capability differentials;
- 7 canonical/alias resolution checks;
- 43 artifact projection and candidate-preserving import cases;
- 7 complete plan-state and postcondition matrices.

Every capability map entry must have a fixture. A fixture marked supported must exist in the live capability map. Unsupported fixtures must not advertise a projection.

## Round-trip claim boundary

The current adapter `Import` request accepts an authoritative core observation plus a canonical candidate; it does not receive or parse arbitrary target bytes. Therefore Phase 5 verifies a **candidate-preserving contract round trip**:

1. seal a canonical fixture artifact;
2. render its reviewed target projection;
3. normalize a core-supplied observation;
4. import the canonical candidate under the same capability;
5. require the imported envelope digest and loss-report digest to remain unchanged.

This proves deterministic projection, observation ordering, candidate preservation, and evidence consistency. It does **not** claim target-native parsing or arbitrary target-to-canonical reconstruction.

## Differential and postcondition evidence

For every built-in adapter, the runner compares:

- adapter ID and implementation version;
- target and target-version range;
- aliases and deployment modes;
- complete declared artifact-capability map;
- MCP structural capability and resolved location;
- exact relative and absolute target projection paths;
- support level, content digest, fidelity, and stable loss records.

Each adapter also runs the complete present/absent state matrix:

- create when absent;
- no-op when semantically equivalent;
- update when owned and unchanged from its reviewed base;
- conflict when owned state diverges;
- conflict for foreign state;
- remove owned state;
- conflict for diverged removal;
- no-op when already absent.

Postcondition verification distinguishes byte equality, declared semantic equivalence, and absence.

## Defects converted into guardrails

The corpus exposed and Phase 5 corrected three contract defects:

1. Populated canonical description and labels were declared omitted by passthrough capabilities but were not emitted as loss records. They now produce stable omission losses, making fidelity `lossy` when those fields are populated.
2. A no-op for an already absent projection could not verify absence. The verifier now handles absent no-op postconditions explicitly.
3. A semantically equivalent no-op was previously verified only by desired-byte digest, which implied a write that no-op never performs. Equivalent unchanged observations now verify through the explicit `Equivalent` signal.
4. MCP-unsupported targets advertised inert root and entry fields. Inactive MCP capabilities must now be structurally empty and fail admission otherwise.

The built-in adapter implementation version is now `1.1.0`; reviewed plans carrying older capability snapshots must be regenerated.

## Operator surface

```text
agentstack hub adapter-conformance \
  [--project-root PATH] \
  [--target-root PATH] \
  [--target TARGET]...
```

The command is read-only. It accepts canonical targets or aliases, deduplicates aliases to the canonical target, runs the embedded corpus, and emits `fabric.asm.dev/adapter-conformance-report/v1alpha1`.

A report contains:

- adapter contract and corpus digest;
- target adapter ID and capability digest;
- deterministic case IDs and categories;
- a digest for every successful evidence item;
- explicit failure reasons;
- derived target and aggregate pass/fail counts;
- a report-level digest.

A failed report exits non-zero after printing the complete machine-readable evidence. Report tampering is rejected by independent resealing.

## Authority and security boundary

Phase 5 does not add adapter filesystem or process access. The service supplies synthetic path context only so built-in path projections can be compared. The conformance package:

- does not scan target directories;
- does not read target configuration;
- does not write files or databases;
- does not invoke target CLIs;
- does not approve or apply reviewed plans;
- does not load external adapter code;
- does not grant trust based on a passing score alone.

A passing report is compatibility evidence for the checked-in contract and corpus, not production authorization.

## Validation coverage

Tests cover:

- strict corpus sealing and complete registry coverage;
- all 64 embedded built-in cases;
- canonical/alias filter deduplication;
- fixture traversal rejection;
- intentional differential drift detection;
- report tamper rejection;
- omission-aware passthrough losses;
- absent and equivalent no-op verification;
- inactive MCP capability field rejection;
- read-only CLI report generation and verification;
- the existing Resource Hub and MCP capability-drift suites.

## Explicit non-goals

Phase 5 does not:

- implement a sandboxed external-adapter process protocol;
- load untrusted plugins or remote adapter packages;
- claim arbitrary target-byte parsing or semantic compilation;
- promote adapters, the corpus, SQLite, or CAS to mutation authority;
- mutate Resource Hub, MCP client, target, backup, or ownership state;
- replace protected native CI or signed release provenance.

The constrained external-adapter process protocol and differential execution are implemented in [Fabric Phase 6](FABRIC_PHASE6.md). External activation remains blocked until a publisher-bound package/lockfile and a true WASI or stronger OS sandbox provide enforceable network and filesystem controls in addition to the implemented Windows Job Object and optional Linux cgroup v2 process ceilings.
