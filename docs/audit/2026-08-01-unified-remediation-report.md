# Unified Independent-Audit Remediation Report

## Decision

The source-level remediation is ready for repository review, but it is **not a certified public release**. All reproducible Linux-verifiable defects from the independent audit and the preceding technical due-diligence audit have concrete code changes and regression controls. Release identity, native Windows execution, Authenticode, protected-branch evidence, exact Go 1.26.5 execution, browser accessibility automation, vulnerability scanning, mutation testing, and publication attestations remain external fail-closed gates.

The supplied artifact was a source archive rather than a Git checkout. Its immutable historical base is `2356a0290239f3a7551a6db9dd7bb76f563fa96d`, but the modified candidate has no truthful commit SHA. `SOURCE_REVISION` and `SOURCE_PROVENANCE.json` therefore identify it as an unreleased remediation workspace instead of misrepresenting the base commit as the candidate.

## Unified closure ledger

| ID | Finding | Implemented control | Regression or gate | Status |
|---|---|---|---|---|
| U-01 | Long UI mutations could complete after the HTTP client lost the response | Mutation endpoints return `202 Accepted` with a token-protected operation receipt; work continues in a durable in-process operation store and the UI polls status | `TestLongMutationSurvivesHTTPWriteTimeoutViaOperationReceipt`, operation-store panic/failure tests, embedded UI contract | Addressed locally |
| U-02 | Bearer/JWT/JSON credentials could reach event persistence and diagnostics ZIPs | Central redaction package plus sanitization at child-error, persistence, legacy-read, export, and diagnostics boundaries | MCP redaction, event persistence, legacy event read, and diagnostics export regression tests | Addressed locally |
| U-03 | MCP timeout covered initialization but not ordinary requests or pool contention | One operation context bounds initialization, list/call, doctor, busy-session waits, and child teardown; pool cleanup is generation-safe | whole-operation timeout, busy-session timeout, replacement-generation, and stale-idle-timer race tests; race suite | Addressed locally |
| U-04 | Skill inventory validation allowed unexpected entries that were later installed | Exact expected inventory, portable-name validation, case-fold collision rejection, symlink/non-regular-node refusal, and copy allowlisting | unexpected-entry, unexpected-copy, portable-name, and symlink regression tests | Addressed locally |
| U-05 | Router failure after installation could return an error while the transaction remained `succeeded` | Post-install router failures persist a partial/failed transaction before returning | `TestApplyPlannedPersistsPartialTransactionWhenRouterConfigurationFails` | Addressed locally |
| U-06 | Package acquisition accepted floating tags, semver ranges, hidden package forms, or mismatched router specs | Exact npm, uv, and router package/version parsing, including explicit flag forms | catalog acquisition regression suite; named coverage floor | Addressed locally |
| U-07 | Repeated `Process.Wait` calls could lose the original exit error | Process terminal result is cached and returned consistently to every waiter | `TestWaitReturnsOriginalExitErrorToEveryCaller`, race suite | Addressed locally |
| U-08 | Planner silently accepted unknown exclusions and contradictory provider choices | Unknown exclusions, include/exclude conflicts, and excluded provider overrides fail closed | planner exclusion regression suite | Addressed locally |
| U-09 | CLI precondition failure could emit a misleading zero-value report | Error paths suppress success-shaped report output and version output includes source revision | CLI regression tests | Addressed locally |
| U-10 | Startup silently discarded recovery and retention failures | Store preparation propagates transaction-recovery and retention errors | `TestPrepareStoreSurfacesRecoveryAndRetentionErrors` | Addressed locally |
| U-11 | POSIX atomic replacement dropped setuid/setgid/sticky metadata | Replacement captures and reapplies full permission bits, including special bits | `TestReplacePreservesSpecialPOSIXModeBits` | Addressed locally |
| U-12 | Windows ACL audit used a localized-name substring allowlist | SDDL is parsed against an exact required-principal/rights allowlist; native test injects an Everyone ACE | SDDL unit tests; Windows integration test cross-compiles | Implemented; native Windows run required |
| U-13 | Source archive could not prove identity or build without `.git` | Explicit unreleased provenance, exact source manifest, unlisted-file rejection, Git-free builds, and parent-repository isolation | manifest verification and negative control; Git-free source build runs test, race, vet, and Windows cross-builds | Addressed locally; immutable candidate SHA pending |
| U-14 | Coverage was 62.3% with critical blind spots | Aggregate floor plus named floors for authorization, apply, MCP, state redaction, skill inventory, router acquisition, planner, process, session, UI, file replacement, and self-install | final measured aggregate 64.0%; critical coverage gate passes | Addressed as a regression gate; breadth debt remains visible |
| U-15 | Performance evidence did not detect regressions | Four benchmark families, five samples each, with deliberately broad maximum latency ceilings | benchmark gate and known-bad ceiling rejection | Addressed locally |
| U-16 | Release packaging could include staging residue or incomplete checksums | Exact release-output and bundle-member allowlists, recursive relative checksums, exact manifest validation, and staging cleanup | governance contract; PowerShell execution remains external | Implemented; protected release run required |
| U-17 | Go package and operational documentation debt obscured boundaries | Package docs, updated CLI/privacy/operations/release/supply-chain guidance, and this ledger | documentation contract | Addressed locally |

## Fresh verification on the final source tree

- `gofmt`: no changed-format files.
- `go mod verify`: pass.
- `go vet ./...`: pass.
- `go test ./... -count=1`: pass.
- `go test -race ./... -count=1`: pass.
- `go test -shuffle=on -count=20 ./...`: pass.
- Statement coverage: **64.0%**; every named critical-path floor passes.
- Six bounded fuzz campaigns: pass.
- Four benchmark families, five samples each: pass; every latency ceiling passes.
- Linux builds for all three commands: pass.
- Windows amd64 and arm64 command cross-builds: pass.
- Windows amd64 and arm64 security-test cross-compilation: pass.
- Documentation, governance, Bash syntax, and exact source-manifest checks: pass.
- Git-free source build: pass, including unit, race, vet, and Windows cross-build phases.
- Unlisted-file negative control: rejected as required.

## External gates and limitations

- The available runtime is Go 1.23.2. An exact Go 1.26.5 toolchain run was attempted but could not download because outbound DNS/network access was unavailable.
- No native Windows runtime was available; Windows ACL, process-job, PATH, setup, and UI E2E checks remain protected-CI gates. Cross-compilation is not runtime proof.
- PowerShell was unavailable, so `build.ps1`, `release.ps1`, governance application, Authenticode, and release publication were not executed here.
- Playwright was not cached and offline installation failed, so browser/axe execution did not run here.
- The Codex CLI was unavailable. **No Codex independent review occurred, and this was not treated as approval.**
- No commit, push, PR, tag, signature, release, publication, or repository-policy mutation was performed.

## Final verdict

**Source remediation: passes the locally executable review gate.**

**Production release: blocked until a real candidate commit is created and the protected Go 1.26.5, native Windows, signing, browser accessibility, vulnerability, mutation, governance, provenance, and attestation gates produce passing evidence.**
