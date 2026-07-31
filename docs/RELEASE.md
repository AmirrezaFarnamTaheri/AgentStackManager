# Release Process

AgentStack public releases target Windows x64 and Windows ARM64. Linux is used for source assurance but is not packaged as a supported installer product.

## Preconditions

- clean working tree;
- HEAD exactly at a signed annotated `v<version>` tag;
- the trusted release-tag public key is imported from the protected release environment and tag signature verification succeeds;
- the tag commit equals the fetched authoritative `origin/main` commit;
- Go 1.26.5, pinned by the repository root `.go-version`;
- valid Authenticode certificate and timestamp service;
- protected release environment approval;
- all required verification workflows green.

## GitHub Release workflow

`.github/workflows/release.yml` supports two controlled entry points:

- pushing an existing signed annotated `v*` tag;
- manually dispatching the workflow with an existing signed annotated tag and an optional prerelease flag.

The workflow serializes runs per tag, checks out the exact tag, verifies that the tag commit equals protected `origin/main`, builds and signs both Windows architectures, creates GitHub provenance and SBOM attestations, executes the signed ARM64 artifacts on a native ARM64 runner, verifies all downloaded checksums and attestations, and only then publishes the GitHub Release with `gh release create`.

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
./scripts/check-critical-coverage.sh coverage.out
./scripts/fuzz.sh 20s
./scripts/check-governance.sh
./scripts/check-docs.sh
```

GitHub additionally runs native Windows setup/PATH/ACL/plan/apply/router/session smoke tests on both x64 and ARM64, and Playwright/axe keyboard-accessibility checks on Windows.

## Reproducibility statement

AgentStack claims reproducibility only for the **unsigned** binary build performed twice from the same clean signed tag, toolchain, target, flags, and source tree. Authenticode timestamps and archive signing/attestation metadata are expected to differ. The release script compares unsigned outputs before signing and records the exact source revision and toolchain in provenance.

## Rollback

A release can be withdrawn by removing it from distribution and publishing a superseding signed release. Installed AgentStack-owned configuration can be restored from indexed backups. Third-party package rollback remains with the respective package manager and must be documented in the incident record.
