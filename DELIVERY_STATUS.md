# AgentStack Manager publication-candidate workspace

This source tree is an **unreleased publication candidate**, not immutable pull-request,
protected-branch, signed-tag, or production-release evidence. Its historical base is
`d91fbe2bce522cd7a4e4d8a8ede04f5b9606a8b6`; the cumulative Fabric phases and final
publication hardening have no upstream Git commit identity because the starting artifact
was supplied as a source archive rather than a repository checkout.

`SOURCE_REVISION` therefore uses the explicit `unreleased-base:<sha>` form, and
`SOURCE_PROVENANCE.json` carries null candidate/release fields. This prevents the baseline
commit from being misrepresented as the identity of the modified candidate.

Source archives are self-verifying through `SOURCE_MANIFEST.sha256`. Development builds now
work both from a clean Git checkout and from a verified source archive. A protected release
must run `scripts/release.ps1`, which requires a clean signed tag equal to fetched
`origin/main`, writes `git:<candidate-commit>` to `SOURCE_REVISION`, overwrites
`SOURCE_PROVENANCE.json` with repository/workflow/run evidence, regenerates the source
manifest, and produces signed binaries, checksums, SBOM/VEX, provenance, and attestations.

The source is prepared for protected release CI. Publication still requires the signed-tag,
Go 1.26.5, native Windows, signing, attestation, and release-approval gates described in
`docs/RELEASE.md` and `docs/LAUNCH_READINESS.md`; those credentials and hosted runners are
not available in this workspace.

- SQLite shadow metadata is verified on Linux amd64 with CGO. CGO-disabled builds still fail closed for database commands while retaining all existing ASM behavior. The reports’ proposed pure-Go migration is dependency-gated: the current source uses a narrow direct SQLite C API rather than `mattn/go-sqlite3`, and no replacement is admitted until its complete module graph, licenses, performance, migration compatibility, and protected Go 1.26.5 builds are verified.
- Target adapter capabilities, fidelity/loss evidence, the embedded 64-case corpus, and SHA-256-pinned out-of-process differential conformance are implemented without authority transfer. Resource Hub and `mcplink` remain the only target mutation authorities. External adapter invocations can request hard memory, CPU, and process ceilings through Windows Job Objects or delegated Linux cgroup v2 scopes; unsupported platforms fail closed. External activation, network/filesystem isolation, and a true WASI sandbox remain pending.

- The embedded manager now uses the Home / Environments / Sharing & Sync / Changes / Activity lifecycle workspace. Changes owns selection, exact review, and the sole client apply path; Activity owns server-reported installation progress, partial-failure recovery, transaction history, maintenance, and diagnostics. Environments exposes read-only AI/IDE/CLI/MCP/workspace and managed-connection state. Consumed plans cannot be replayed, browser failures are path-free, and typography, mobile targets, menu focus, live announcements, theme contrast, filtering, and bounded history are protected by static and Playwright/axe contracts.

- The 0.3.0 candidate adds a desktop-owned application window, simultaneous evidence-backed target connections, canonical resource/install inventory, duplicate and conflict classification, immutable multi-target sync plans, bounded parallel execution, hidden child process windows, sanitized public receipts, and deterministic icon/manifest resources. The desktop host uses the installed Edge or Chrome application engine in dedicated app-window mode; a future direct WebView2-control migration remains possible but is not falsely claimed in this archive.
