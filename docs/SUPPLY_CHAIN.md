# Supply-Chain Assurance

## Catalog controls

Every automatic component declares an exact identity appropriate to its installer: WinGet package ID/version/source, npm package version, uv package version/source, or Git commit. Catalog validation rejects floating `latest`, missing versions, unapproved sources, duplicate component IDs, invalid dependency graphs, unsupported platforms, and incomplete license coverage.

The Superpowers skill pack is fetched by exact commit and accepted only when Git resolves that commit and the expected skill inventory is present. Existing skills are preserved.

## Release inputs

A public build requires:

- a clean tree at a verified signed annotated `v*` tag;
- Go 1.26.5;
- exact pinned assurance tools;
- `go test`, race, vet, fuzz seed campaigns, Linux/common mutation testing, native Windows-specific mutation testing, critical-path coverage, governance/docs checks;
- source and Windows-binary `govulncheck`;
- two-build unsigned reproducibility comparison per binary;
- valid Authenticode signatures and expected publisher thumbprint;
- CycloneDX catalog and binary SBOMs;
- component license inventory;
- OpenVEX output;
- deterministic ZIPs and SHA-256 manifests;
- GitHub artifact attestations.

A development build is explicitly unsigned and must not be presented as a public release.

## Installed-stack assurance

The catalog SBOM describes all selectable components, not only Go module dependencies. License metadata and approved source/publisher fields are checked in tests. Credentialed/manual integrations remain explicit and are not represented as automatically installed capabilities.

## Vulnerability response

Catalog updates must preserve exact versions and sources, update SBOM/license/VEX evidence, pass code-owner review, and produce a new signed release. VEX expresses applicability decisions; it does not suppress uninvestigated vulnerabilities.

## Exact acquisition and inventory closure

NPM and PyPI entries must use exact versions that agree with their catalog `version`
metadata; ranges, tags, wildcards, URLs, and unversioned router acquisition commands are
rejected. Skill-pack validation compares the complete discovered skill set to the audited
`expectedEntries` set and fails on both missing and unexpected entries before any target is
modified. Expected names use the portable lowercase alphanumeric/hyphen form, reject Windows
device names and case-fold collisions, and every accepted skill tree must contain only regular
files/directories with no symbolic links. Copying iterates only the audited allowlist.

Source bundles include `SOURCE_REVISION`, `SOURCE_PROVENANCE.json`, and a full
`SOURCE_MANIFEST.sha256`. An unreleased archive explicitly identifies only its historical
base and carries no candidate commit. Protected release automation overwrites those fields
with the signed-tag commit and CI run evidence before packaging. The provenance capsule packages only manifest-listed regular files and reopens the ZIP to verify exact membership and content digests, so caches, ignored directories, development databases, and compiled executables cannot enter a source bundle through working-tree contamination.

## Donor convergence provenance

Seven donor archives were validated before extraction, hashed file-by-file, and recorded in `docs/convergence/SURFACES.csv`. `docs/convergence/ADOPTION.csv` connects independently meaningful donor units to target symbols, invariants, dispositions, and unique tests.

No donor package manager, build output, vendored dependency tree, browser database, or binary is shipped as an authoritative runtime. Adopted behavior was implemented in the existing Go target and passes the target's source-manifest and release-pack boundaries.

## SQLite native backend

The Phase 3 metadata preview calls the public SQLite C API through a narrow internal CGO boundary and records SQLite's public-domain status in the component license inventory. No downloaded donor database or generated third-party Go binding is vendored. Protected release builders that enable metadata support must pin and inventory the SQLite library, run native tests, include it in SBOM/provenance evidence where applicable, and verify the minimum supported SQLite version. CGO-disabled builds retain a fail-closed stub rather than invoking an untracked external executable.
