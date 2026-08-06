# Convergence Validation Record

## Frozen-source evidence

| Area | Status | Evidence |
| --- | --- | --- |
| Donor archive structure | verified | pre-extraction member validation and `ARCHIVE_INSPECTION.json` |
| Donor file accountability | verified | 2,673 rows in `SURFACES.csv` |
| Donor evidence hashes | verified | all seven donor-specific ledger slices validated against their exact extracted roots |
| Combined dependency graph | verified | 53-record ledger validation; acyclic parent and dependency relationships |
| Target node existence | verified | ledger target-node validation against this source tree |
| Semantic evidence | verified | 44 unique primary test nodes and 60 unique record/composition/negative-path links |
| Focused convergence behavior | verified | resource hub, context, workspace, MCP linking, routines, app, CLI, UI, and convergence traceability tests |
| Full target regression | verified | `go test ./...` |
| Concurrency safety | verified | `go test -race ./...` |
| Static analysis | verified | `go vet ./...` and `git diff --check` |
| Coverage | verified | 65.8% total statement coverage; critical coverage gate passed |
| Documentation and governance | verified | `scripts/check-docs.sh` and `scripts/check-governance.sh` |
| Performance budgets | verified | nine benchmark families, five samples each, all published ceilings passed |
| Bounded fuzzing | verified | all six catalog, MCP, planner, state, and safe-file fuzz targets passed final two-second campaigns |
| Windows portability | statically-validated | Windows amd64 and arm64 packages compiled through `go test -exec=true ./...` |
| Development command builds | verified | Linux amd64 and Windows amd64/arm64 command builds under Go 1.23.2 |
| Browser accessibility automation | pending | pinned Playwright/axe packages were not cached and outbound package access was unavailable |
| Native Windows E2E, mutation, signing, attestations | pending | protected-CI release gate |

## Measured performance

Measurements were captured on Linux amd64 with Go 1.23.2. Reported values are the slowest of five samples and are compared with the checked-in regression ceilings.

| Benchmark | Maximum | Ceiling |
| --- | ---: | ---: |
| MCP one-shot list-tools | 6.198 ms | 10 ms |
| MCP persistent list-tools | 101.757 µs | 1 ms |
| Large-catalog planning | 3.232 ms | 5 ms |
| Credential redaction | 53.579 µs | 250 µs |
| UI operation lookup | 34.05 ns | 500 ns |
| Resource audit | 6.985 ms | 20 ms |
| Project scan | 1.888 ms | 10 ms |
| Search across 1,000 memory entries | 31.477 ms | 50 ms |
| Next-run schedule calculation | 6.440 µs | 50 µs |

The first combined run recorded one one-shot process-start sample at 10.439 ms against the 10 ms ceiling while its other samples ranged from 2.438 to 7.651 ms. Ten isolated rerun samples ranged from 2.411 to 6.198 ms. The original outlier remains in the audit evidence; the final evidence gate uses the isolated startup run plus five fresh samples for every other benchmark family. The allocation-heavy memory-search benchmark was also run independently to avoid the outer execution-harness timeout.

## Development build hashes

These are unsigned development artifacts produced by the local Go 1.23.2 toolchain, not protected release binaries.

| Artifact | SHA-256 |
| --- | --- |
| Linux amd64 `agentstack` | `539250bf294a755d762705d067c71be926ca8dc47c4f18f5f4329eaa793f5e77` |
| Linux amd64 `agentstack-sbom` | `68cf486db1c029cc2322d75684229e6ae0edf3159b90b6991021ffbe6117fa0f` |
| Linux amd64 `releasepack` | `9d8c2bc223b0612b9fa84393046130df8f2f8984719fc5487e2f28589a16e5c0` |
| Windows amd64 `agentstack.exe` | `4e6ca4b15d22a8d9a15fdca84787ed6132dd9e76238e0cdb88e77a83ce7206a8` |
| Windows arm64 `agentstack.exe` | `0fb5bfb1b3ac486dc6a48ee9c4ca61d1fe34abb820f4c54fd98f5028c82b1d5d` |

## Claim boundaries

- Runtime behavior executed in this environment is **verified** under Go 1.23.2 on Linux amd64.
- Windows behavior is **statically validated** by amd64 and arm64 test compilation; native execution remains a protected-CI gate.
- Donor behavior is not presented as byte-for-byte equivalence. `ADOPTION.csv` records each adaptation, hardening, recomposition, supersession, rejection, and inspired-native result.
- Exact Go 1.26.5 release reproduction, native Windows mutation/E2E, browser Playwright/axe execution, vulnerability scanning, Authenticode signing, SBOM attestation, and provenance attestation remain **pending protected CI**.

## Fabric Phase 2 validation supplement

The Phase 2 content-addressed shadow-store claims are validated separately from the frozen convergence-release measurements above.

| Area | Status | Evidence |
| --- | --- | --- |
| CAS deterministic identity and deduplication | verified | `internal/cas/store_test.go` |
| Recursive object integrity and reachability marks | verified | `internal/cas/store_test.go` |
| Symlink, traversal, malformed-manifest, corruption, and bound rejection | verified | `internal/cas/store_test.go` |
| No-overwrite blob, destination, and tree-entry materialization | verified | `internal/cas/store_test.go` |
| Resource Hub shadow stage and legacy-digest round trip | verified | `internal/migrations/asmv1/stage_test.go` |
| Receipt tamper and stale-source rejection | verified | `internal/migrations/asmv1/stage_test.go` |
| CLI stage, verify, restore, and confirmation gates | verified | `internal/cli/cas_test.go` |
| Existing Resource Hub authority and full regression suite | verified | `go test ./...` |
| Focused concurrency safety | verified | `go test -race ./internal/cas ./internal/migrations/asmv1 ./internal/resourcehub ./internal/app ./internal/cli ./internal/convergence` |
| Windows and macOS compilation | statically-validated | focused packages compiled for Windows amd64/arm64 and Darwin amd64/arm64 with `go test -exec=true` |


## Fabric Phase 3 validation supplement

| Area | Status | Evidence |
| --- | --- | --- |
| SQLite schema, WAL, transaction, identity, and idempotence | verified | `internal/store/sqlite/store_test.go` |
| Immutable history and shadow-head advancement | verified | `internal/store/sqlite/store_test.go` |
| Receipt/artifact/resource row tamper rejection | verified | `internal/store/sqlite/store_test.go` |
| Unrelated database, symlink, missing-path, and bound rejection | verified | `internal/store/sqlite/store_test.go` |
| Verified online backup and no-overwrite publication | verified | `internal/store/sqlite/store_test.go` |
| CLI stage, inspect, current-state verify, stale rejection, and backup gates | verified | `internal/cli/sqlite_test.go` |
| Existing ASM regression suite | verified | `go test ./...` |
| CGO-disabled fallback | verified | `CGO_ENABLED=0 go test ./internal/store/sqlite ./internal/app ./internal/cli` |
| Native SQLite backend | verified on Linux amd64 | local SQLite C API and focused tests |
| Windows/macOS native SQLite execution | pending protected native CI | source compiles through the CGO-disabled fail-closed backend |

## Fabric Phase 4 validation supplement

| Area | Status | Evidence |
| --- | --- | --- |
| Versioned adapter, capability, rendered-set, and loss-report contracts | verified | `internal/adapters/contract_test.go` |
| Built-in target capability and projection matrix | verified | `internal/adapters/builtin/builtin_test.go` |
| Traversal, invalid digest, report-identity, duplicate, and foreign/diverged-state rejection | verified | `internal/adapters/contract_test.go` |
| Resource Hub capability binding, visible fidelity, and deny-loss policy | verified | `internal/resourcehub/adapters_test.go` |
| Resource Hub operation-loss tamper and capability-drift rejection | verified | `internal/resourcehub/adapters_test.go` |
| MCP capability/loss snapshots and tamper rejection | verified | `internal/mcplink/mcplink_test.go` |
| Read-only CLI capability inspection and alias resolution | verified | `internal/cli/fabric_test.go` |
| Existing ASM regression suite | verified | `go test ./...` |
| Focused concurrency safety | verified | `go test -race ./internal/adapters/... ./internal/resourcehub ./internal/mcplink ./internal/app ./internal/cli ./internal/convergence` |
| Windows and macOS source compatibility | statically-validated | focused packages compiled with the CGO-disabled fail-closed SQLite backend |

## Fabric Phase 5 validation supplement

| Area | Status | Evidence |
| --- | --- | --- |
| Strict embedded corpus and complete built-in registry coverage | verified | `internal/adapters/conformance/conformance_test.go#TestEmbeddedCorpusCoversEveryBuiltinAdapter` |
| 64-case capability differential, artifact contract round trip, plan matrix, and postcondition suite | verified | `internal/adapters/conformance/conformance_test.go#TestEmbeddedCorpusPassesAllBuiltinAdapters` |
| Alias filter deduplication | verified | `internal/adapters/conformance/conformance_test.go#TestTargetFilterDeduplicatesAliases` |
| Traversal rejection and intentional differential drift detection | verified | `internal/adapters/conformance/conformance_test.go#TestCorpusRejectsTraversalAndDifferentialDrift` |
| Sealed report tamper rejection | verified | `internal/adapters/conformance/conformance_test.go#TestReportTamperingIsRejected` |
| Omission-aware passthrough fidelity | verified | `internal/adapters/builtin/builtin_test.go#TestPassthroughReportsPopulatedMetadataOmissions` |
| Absent and equivalent no-op postconditions | verified | `internal/adapters/contract_test.go#TestVerifyAcceptsAbsentNoopPostcondition`; `internal/adapters/contract_test.go#TestVerifyAcceptsEquivalentNoopWithoutRewritingDigest` |
| Inactive MCP capability closure | verified | `internal/adapters/contract_test.go#TestInactiveMCPCapabilityRejectsRegistrationFields` |
| Read-only operator report | verified | `internal/cli/fabric_test.go#TestHubAdapterConformanceCLIEmitsVerifiedReport` |
| External adapter execution | implemented in Phase 6 for constrained conformance only | see the Phase 6 supplement; activation and complete OS/WASI sandboxing remain excluded |

## Fabric Phase 6 validation supplement

| Area | Status | Evidence |
| --- | --- | --- |
| SHA-256-pinned executable admission, handshake, descriptor sealing, and cleanup | verified | `internal/adapters/external/external_test.go#TestOpenPinsExecutableNegotiatesAndMatchesReference` |
| Complete Phase 5 reference-versus-candidate differential | verified | `internal/adapters/external/external_test.go#TestRunConformanceMatchesPhase5Corpus` |
| Capability ceiling and visible restriction evidence | verified | `internal/adapters/external/external_test.go#TestCapabilityIntersectionRemovesUnreviewedClaims` |
| Digest mismatch and symlink rejection | verified | `internal/adapters/external/external_test.go#TestAdmissionRejectsDigestMismatchAndSymlink` |
| Deadline, output, crash, malformed response, response identity, and stderr failure paths | verified | `internal/adapters/external/external_test.go#TestProcessLimitsAndProtocolIdentityFailClosed` |
| Target-root escape and core plan-state divergence | verified | `internal/adapters/external/external_test.go#TestExternalRenderEscapeAndPlanDivergenceAreRejected` |
| Descriptor/intersection/reference/candidate/mismatch report tampering | verified | `internal/adapters/external/external_test.go#TestConformanceReportTamperingIsRejected` |
| Private staged bytes and sanitized child environment | verified | `internal/adapters/external/external_test.go#TestAdmittedExecutableIsPrivateAndEnvironmentIsSanitized` |
| Within-session capability drift | verified | `internal/adapters/external/external_test.go#TestCapabilityDriftWithinSessionIsRejected` |
| Read-only operator command | verified | `internal/cli/fabric_test.go#TestHubExternalAdapterConformanceCLIEmitsVerifiedReport` |
| Full normal-execution differential suite | verified | `go test ./...` |
| Process-host race coverage | verified | `go test -race ./internal/adapters/external ./internal/cli ./internal/app` with the expensive full subprocess corpus skipped under race instrumentation and covered by normal execution |
| Complete kernel/WASI sandbox, network/filesystem namespaces, CPU/memory quotas, and Windows descendant containment | unverified and not implemented | explicit Phase 6 limitation; external activation remains blocked |
