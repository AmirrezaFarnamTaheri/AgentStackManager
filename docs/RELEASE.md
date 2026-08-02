# Release Process

AgentStack public releases target Windows x64 and Windows ARM64. Linux is used for source assurance but is not packaged as a supported installer product.

## Preconditions

- clean working tree;
- for tag-push releases, HEAD exactly at a GitHub-verified signed annotated `v<version>` tag whose commit equals fetched authoritative `origin/main`;
- for automatic releases, a successful Verify run on `main` or an approved manual dispatch against current `main`;
- Go 1.26.5, pinned by the repository root `.go-version`;
- valid Authenticode certificate and timestamp service;
- protected release environment approval;
- all required verification workflows green.

## Optional trusted tag release

To select a version explicitly, create and push a signed annotated tag from a clean, verified `main` checkout:

```powershell
git switch main
git pull --ff-only
git status --short
git tag -s v1.0.0 -m "Release v1.0.0"
git tag -v v1.0.0
git push origin v1.0.0
```

Replace `v1.0.0` with the intended version. `git status --short` must produce no output. A lightweight or unverified tag is rejected. Pushing the tag starts the release workflow automatically.

## GitHub Release workflow

`.github/workflows/release.yml` supports three controlled entry points:

- pushing a GitHub-verified signed annotated `v*` tag;
- completion of the Verify workflow on `main`;
- manual dispatch against current `main`, with an optional prerelease flag.

Before checkout, a tag-push release validates through the GitHub API that the ref is an annotated tag object with a verified signature. Automatic entry points check out current `main`, infer the next semantic version from Conventional Commit signals, and create a keyless Sigstore-signed annotated tag. The workflow recovers an interrupted workflow-owned unpublished tag and skips an already published commit. It then builds and signs both Windows architectures, creates GitHub provenance and SBOM attestations, executes the signed ARM64 artifacts on a native ARM64 runner, verifies all downloaded checksums and attestations, and only then publishes the GitHub Release with `gh release create`.

Publication uses the job-scoped `GITHUB_TOKEN` with `contents: write`; build jobs receive only the read, identity-token, attestation, and artifact-metadata permissions they require. Existing GitHub Releases are never overwritten.

## Local release command

```powershell
./scripts/release.ps1 -Version 0.2.0 -CertificateThumbprint <40-hex-thumbprint>
```

The script refuses a dirty tree, lightweight/unsigned tag, unsupported Go version, missing assurance tools, invalid governance/docs, vulnerability findings, non-reproducible unsigned binaries, VCS metadata drift, invalid signatures, or bad archive contents.

## Produced evidence

- signed console and graphical setup binaries for x64 and ARM64;
- deterministic architecture-specific ZIPs;
- source ZIP exported from the signed tag;
- internal and top-level SHA-256 manifests;
- CycloneDX catalog SBOM;
- CycloneDX binary SBOM for each raw signed executable, attested against that executable rather than a containing ZIP;
- component license inventory;
- OpenVEX vulnerability applicability output;
- provenance statement containing revision, tag, toolchain, catalog digest, and subject hashes;
- GitHub provenance attestations for release archives and SBOM attestations for the exact raw signed binaries.

## Required validation

```text
go test ./...
go test -race ./...
go vet ./...
bash scripts/check-critical-coverage.sh coverage.out
bash scripts/check-benchmarks.sh benchmark-results.txt
bash scripts/fuzz.sh 20s
bash scripts/check-governance.sh
bash scripts/check-docs.sh
```

The benchmark gate records five samples for one-shot and persistent MCP requests,
large-catalog planning, credential redaction, and operation-status lookup. It applies
deliberately tolerant latency ceilings that reject order-of-magnitude regressions while
remaining stable on shared CI hosts. Fuzz campaigns use one worker per target and a hard
per-target timeout so a pathological case fails visibly instead of stalling the workflow.

GitHub additionally runs native Windows setup/PATH/ACL/plan/apply/router/session smoke tests on both x64 and ARM64. The Go race detector runs on Linux and Windows x64; the Windows ARM64 runner executes the complete native non-race suite because upstream Go does not support race instrumentation on windows/arm64. The native suite includes explicit DACL continuity across atomic replacement and a runnable suspended-before-assignment Job Object with memory, CPU-rate, and active-process ceilings. Playwright/axe checks keyboard navigation, reduced motion, operation busy feedback, focus restoration, and authenticated shutdown on Windows.

## Reproducibility statement

AgentStack claims reproducibility only for the **unsigned** binary build performed twice from the same clean signed tag, toolchain, target, flags, and source tree. Authenticode timestamps and archive signing/attestation metadata are expected to differ. The release script compares unsigned outputs before signing and records the exact source revision and toolchain in provenance.

## Rollback

A release can be withdrawn by removing it from distribution and publishing a superseding signed release. Installed AgentStack-owned configuration can be restored from indexed backups. Third-party package rollback remains with the respective package manager and must be documented in the incident record.

## Source archive provenance

A source archive is buildable without a `.git` directory only after
`SOURCE_MANIFEST.sha256` verifies successfully and `SOURCE_REVISION` contains either
`git:<40-hex>` or the explicitly non-release `unreleased-base:<40-hex>` form. The protected
release workflow permits only the `git:` form, generated from the clean signed tag that is
identical to fetched `origin/main`, and records repository, workflow, run, tag, and commit
identity in `SOURCE_PROVENANCE.json`. The baseline commit must never be presented as the
identity of a modified, uncommitted source candidate.

## Source archive verification

Every source bundle carries `SOURCE_REVISION`, `SOURCE_PROVENANCE.json`, and
`SOURCE_MANIFEST.sha256`. A bundle without `.git` is accepted only after the
manifest verifies both every digest and the exact source file set, and the revision is either `git:<40-hex>` or the explicitly
unreleased `unreleased-base:<40-hex>` form. CI runs
`bash scripts/check-source-archive-build.sh` to create an ephemeral Git-free copy,
place it inside an unrelated parent Git repository, regenerate and verify its manifest,
prove an unlisted file is rejected, and execute the supported archive build path.
This proves archive buildability; it does not turn an unreleased workspace into
protected release evidence.
