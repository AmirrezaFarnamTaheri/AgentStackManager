# AgentStack Manager remediation workspace

This source tree is an **unreleased remediation workspace**, not immutable pull-request,
protected-branch, signed-tag, or production-release evidence. Its historical base is
`2356a0290239f3a7551a6db9dd7bb76f563fa96d`; the current modifications have no Git commit
identity because this artifact was supplied as a source archive rather than a repository
checkout.

`SOURCE_REVISION` therefore uses the explicit `unreleased-base:<sha>` form, and
`SOURCE_PROVENANCE.json` carries null candidate/release fields. This prevents the baseline
commit from being misrepresented as the identity of the modified candidate.

Source archives are self-verifying through `SOURCE_MANIFEST.sha256`. Development builds now
work both from a clean Git checkout and from a verified source archive. A protected release
must run `scripts/release.ps1`, which requires a clean signed tag equal to fetched
`origin/main`, writes `git:<candidate-commit>` to `SOURCE_REVISION`, overwrites
`SOURCE_PROVENANCE.json` with repository/workflow/run evidence, regenerates the source
manifest, and produces signed binaries, checksums, SBOM/VEX, provenance, and attestations.

Until those protected gates pass, this tree must not be represented as a production release.
