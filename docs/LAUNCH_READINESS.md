# Launch Readiness and Pre-Mortem

## Decision

**Conditional go.** The implementation may enter protected release CI. Publication remains blocked until every ship gate below passes on the exact release revision.

## Ship gates

| Gate | Required evidence | Blocks publication when |
| --- | --- | --- |
| Source identity | Clean signed revision, closed source manifest, deterministic source archive, exact member/digest verification | Any unlisted member, digest drift, unsafe archive path, or revision mismatch appears |
| Toolchain | Go 1.26.5 from `.go-version`; tests, race detection where supported, vet, coverage, fuzz, and benchmark budgets | Toolchain differs or any verification command fails |
| Windows runtime | Native x64 and ARM64 setup, PATH, ACL, plan/apply, router, session, Job Object, and accessibility checks | Either architecture lacks native execution evidence |
| Mutation assurance | Default mutation suite with no blanket package/platform exclusions plus native Windows mutation of `_windows.go` production paths | Efficacy or mutant-coverage threshold fails |
| Supply chain | Vulnerability scan, SBOM, license inventory, VEX, checksums, signatures, and GitHub attestations | High-risk finding is unresolved or artifact identity cannot be proven |
| Recovery | Prior signed release available, restore preview works, release withdrawal procedure assigned | Rollback cannot start immediately or recovery evidence is missing |

## Failure scenario

It is the day after publication. The release was withdrawn because one architecture leaked a child process during shutdown, a source ZIP contained an unsafe cross-platform member, or a valid Windows PATH was truncated during setup. Operators could not trust the artifact boundary, and rollout stopped.

## Prioritized failure paths

### 1. Source archive escaped its intended extraction root

**Necessary conditions that enabled failure**

- Archive names were validated with host-native path semantics.
- Windows drive, UNC-style, backslash, or traversal forms reached the ZIP writer.
- The published ZIP was not reopened and compared with the signed manifest.

**Prevention requirements**

- Validate every prefix, source-manifest path, and member with platform-independent archive rules.
- Reject absolute, drive-qualified, UNC-style, backslash, NUL, colon, and traversal forms.
- Reopen every source ZIP and verify exact membership plus every SHA-256 digest.

**Owner:** Release engineering
**Checkpoint:** `internal/releasepack` negative-path tests and protected source-package job
**Ship gate:** Source identity

### 2. MCP shutdown left a child process alive

**Necessary conditions that enabled failure**

- Shutdown canceled only registered persistent sessions.
- A child still initializing was not registered for closure.
- One-shot operations were outside the runtime cancellation tree.

**Prevention requirements**

- `ManagedChildRuntime.Close` cancels the runtime root context.
- Persistent starts expose a start-cancel handle before initialization completes.
- One-shot and persistent initialization regression tests prove process termination and request release.
- Race detection covers close-versus-start and close-versus-request paths.

**Owner:** MCP runtime maintainer
**Checkpoint:** MCP lifecycle tests under `go test -race ./...`
**Ship gate:** Toolchain and Windows runtime

### 3. Plan identity drifted from the reviewed authorization

**Necessary conditions that enabled failure**

- Storage key, internal plan ID, digest, and correlation ID were treated as independent values.
- A renamed or substituted plan file remained executable when its digest was known.

**Prevention requirements**

- Requested plan ID must equal the loaded plan ID before inventory scan or mutation.
- Digest, expiry, catalog hash, inventory hash, lease, and single-use consumption remain mandatory.
- Identity mismatch leaves the stored plan untouched and produces `ErrPlanMismatch`.

**Owner:** Transaction boundary maintainer
**Checkpoint:** `internal/reviewedplan` identity-mismatch regression test
**Ship gate:** Toolchain

### 4. Windows PATH transport truncated a valid value

**Necessary conditions that enabled failure**

- UTF-16 PATH content was Base64 encoded across PowerShell.
- Output capture remained at 64 KiB, below the encoded maximum practical Windows value.
- Truncation reached the decoder as malformed state.

**Prevention requirements**

- Read-side transport uses `pathenv.MaxWindowsStringTransportBytes` at 128 KiB.
- A boundary test proves the encoded maximum value fits the capture budget.
- Native Windows setup/PATH tests run on x64 and ARM64.

**Owner:** Windows installation maintainer
**Checkpoint:** path transport unit test and native setup workflow
**Ship gate:** Windows runtime

### 5. Local green checks diverged from the protected release environment

**Necessary conditions that enabled failure**

- Development verification used a different Go version or operating system.
- Signing, attestation, native mutation, or publisher-certificate checks were inferred from local builds.
- Publication proceeded without the protected environment.

**Prevention requirements**

- Local results are development evidence only when the toolchain differs from `.go-version`.
- Release publication runs only through the protected GitHub workflow.
- Exact revision, toolchain, checksums, signatures, SBOMs, VEX, and attestations are retained with the release.

**Owner:** Release manager
**Checkpoint:** Protected environment approval and release-job summary
**Ship gate:** All gates

## Staged rollout

1. Build a release candidate from the exact protected revision.
2. Run native x64 and ARM64 smoke suites against the signed candidate.
3. Install on internal Windows hosts; exercise inventory, plan, apply, MCP doctor, diagnostics, backup preview, and shutdown.
4. Publish as a prerelease and retain the previous signed release as the immediate fallback.
5. Promote only after the prerelease evidence remains clean and no new diagnostic signature appears.
6. Publish the stable release, preserve all provenance, and keep the rollback owner active through the release window.

## Rollback

**Triggers**

- Source identity or signature failure
- Data/state integrity failure
- Orphaned process or shutdown failure
- Error rate or support incidents materially above the prior release
- Native architecture-specific regression

**Actions**

1. Withdraw the affected release from distribution.
2. Restore the prior signed release and its published checksums.
3. Preserve diagnostics, transaction IDs, backup IDs, and provenance evidence.
4. Preview and apply indexed configuration restore where AgentStack-owned files changed.
5. Use provider-native recovery for completed third-party package mutations.
6. Publish a superseding signed release; never overwrite an existing release artifact.

The rollback target is operational within five minutes of the decision to withdraw, excluding third-party package-manager recovery.

## Convergence ship gates

| Gate | Required evidence | Rollback trigger |
| --- | --- | --- |
| Authority coherence | one durable owner per fabric plane; no donor runtime or UI write authority | duplicate registry/config/store mutates live state |
| Evidence closure | 2,673 donor surfaces linked; 53 semantic records; unique test evidence; all high/critical records resolved | orphan material surface or umbrella test discovered |
| State migration | legacy workspace/routine fixtures upgrade; future schemas fail closed; receipt reconciliation passes | existing local state cannot load or next-run state duplicates |
| Secret minimization | no credential fields; routine admission rejects secret arguments; receipts redact secrets; MCP plans/backups contain no full client configs | token/password appears in persistent fabric state or export |
| Resource bounds | scan/audit/memory/schedule benchmarks stay below regression ceilings | latency or memory exceeds gate, queue/output becomes unbounded |
| Recovery | resource backups restore; workspace journals recover; MCP recovery records are registration-only; foreign state preserved; routine receipts survive restart | AgentStack-owned mutation cannot be inspected or rolled back |

Convergence capabilities should first run on internal project roots and disposable client configurations. Promote only after resource sync, context refresh, MCP link/unlink, memory migration, artifact verification, routine failure, receipt recovery, and rollback paths are exercised on the signed release candidate.
