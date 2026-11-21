# ASM Fabric Phase 1: Canonical Artifact Envelope

## Evidence basis

This phase was derived from a complete reading of the five uploaded ecosystem and ASM evolution reports. The source files were treated as design evidence, not as governing runtime input.

| Source | Lines | SHA-256 | Principal contribution |
| --- | ---: | --- | --- |
| `deep-research-report (6).md` | 331 | `469de0edc63fc2ab57daf878e0ebfe51d5930e71916e9c67d623564ede4316ea` | Cross-platform project inventory and the one-owner-per-native-file constraint |
| `deep-research-report (5).md` | 501 | `e1ebb6fa88db2c0c77d9b5cd828b9da4f907fea4fef3b63271b5b483ee016103` | Typed canonical graph, adapter contract, preservation-safe migration order, and loss reporting |
| `deep-research-report (3).md` | 615 | `ebc944d58cde07a772507de4be3a713a153c7eea69fac80a4aafd297f79d78c4` | Stable identity, package/runtime separation, semantic merge requirements, and initial vertical slice |
| `deep-research-report (1).md` | 699 | `cc097db3aa58c5b6427f3cf93f00259d9fa94dcbe914367ae94b415d47b612a9` | ASM baseline, canonical-envelope migration, adapter ownership, and target architecture |
| `deep-research-report (9).md` | 1,087 | `c64fff816a40d4a3e66db307d57ee83631317778f9a333373274e06af59d8c74` | Concrete module backlog, canonical artifact schema, conservative execution classes, and release sequence |

## Converged decision

ASM keeps one Go authority and preserves its current reviewed-plan, lease, journal, ownership, backup, recovery, and postcondition contracts. Donor tools and standards are compatibility inputs. They do not become additional mutation authorities.

The first implementation slice is an additive canonical envelope around the current Resource Hub rather than an immediate persistence rewrite. This ordering establishes the identity and digest contract required by later CAS, SQLite, package, adapter, lockfile, and reconciliation work without changing any existing target write behavior.

## Implemented contract

`internal/artifactgraph` now provides:

- versioned `fabric.asm.dev/v1alpha1` artifact and snapshot envelopes;
- stable canonical IDs independent of native target paths;
- typed artifact kinds covering current resources and planned package/runtime surfaces;
- separate content digest and canonical-envelope digest;
- deterministic JSON normalization with duplicate-key and trailing-content rejection;
- unknown extension preservation through canonical `json.RawMessage` values;
- source references, conservative execution classes, capabilities, target bindings, and field-level provenance;
- graph-level sorting, duplicate-ID rejection, digest binding, and verification.

`internal/resourcehub` now provides a read-only `CanonicalSnapshot` bridge:

- existing registry resources remain the authoritative payload and mutation source;
- existing content digests are preserved rather than recomputed through a new store;
- Resource Hub metadata is retained both as canonical labels and under the `asm.resourcehub.v1` extension;
- current target declarations become typed target bindings;
- potentially executable resource kinds are classified conservatively;
- no registry file, target file, managed state, plan, backup, or source content is rewritten.

The CLI exposes this view as:

```text
agentstack hub graph
```

## Authority and state boundary

| State | Authority in this phase | Mutation allowed by graph layer |
| --- | --- | --- |
| Resource payload and metadata | Resource Hub version-1 registry and `resources/<id>/content` | No |
| Resource sync and refresh plans | Existing Resource Hub reviewed-plan code | No |
| Canonical artifact envelope | Derived in memory from current authoritative state | No |
| Canonical graph digest | Derived in memory from sealed artifact envelopes | No |
| Target files and managed ownership | Existing Resource Hub sync engine | No |

## Verification

Focused tests prove:

- deterministic artifact digests across map, tag, capability, target, and extension ordering;
- preservation of large JSON numbers and unknown extension fields;
- rejection of duplicate JSON keys, malformed identities, invalid execution classes, and digest tampering;
- stable graph ordering and duplicate canonical-ID rejection;
- Resource Hub identity, content digest, source, metadata, targets, provenance, and extension preservation;
- successful CLI round-trip and graph verification.

Executed verification for this slice:

```text
go test ./...
go test -race ./internal/strictjson ./internal/artifactgraph ./internal/resourcehub ./internal/app ./internal/cli
go vet ./...
git diff --check
bash scripts/check-docs.sh
bash scripts/check-governance.sh
GOOS=windows GOARCH=amd64 go test -exec=true ./internal/strictjson ./internal/artifactgraph ./internal/resourcehub ./internal/app ./internal/cli
GOOS=windows GOARCH=arm64 go test -exec=true ./internal/strictjson ./internal/artifactgraph ./internal/resourcehub ./internal/app ./internal/cli
```

All commands passed in the implementation environment. Windows results are compile-time validation; native Windows execution remains outside this local evidence.

## Explicit non-goals

This slice does **not** claim to implement:

- SQLite metadata storage or a content-addressed blob store;
- package manifests, dependency resolution, or lockfiles;
- native adapter extraction or third-party adapter execution;
- bidirectional synchronization or three-way semantic merge;
- Agent Skills, APM, Ruler, MCP Registry, or MCPB importers;
- secret brokerage, signatures, sandboxing, or registry federation;
- migration of the Resource Hub registry to a new schema.

Those depend on the canonical contract introduced here and remain later reviewed slices.
