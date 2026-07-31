# AgentStack Manager 0.2.0 remediation source

This is the complete uncommitted remediation candidate based on baseline commit
`d196b8d25de524d5659b5e1f82902ed8327f04ee`. All 40 audit findings have source-level
remediations. See `docs/audit/ASM-001-040-remediation-report.md` and the JSON export.

No production release is included. Protected signing, Windows-native, vulnerability,
accessibility, mutation, governance, provenance, and attestation gates must pass on a
clean signed tag before publication.

A complete `.github/workflows/release.yml` is included. It can run from a pushed signed tag or a manual dispatch referencing an existing signed tag, and it publishes only after signing, attestation, checksum, and native ARM64 gates pass.
