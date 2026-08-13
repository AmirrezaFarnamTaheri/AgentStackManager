# Publication Wayfinder

## Destination

Produce one cumulative AgentStack Manager source candidate containing Fabric phases 1-6 and the final hardening pass, ready for the repository's protected signed-release workflow without transferring authority to shadow stores, adapters, the UI, or local audit tooling.

## Notes

- The protected workflow remains the only publication authority.
- Resource Hub and `mcplink` remain the target mutation authorities.
- SQLite, CAS, adapter conformance, and external adapter execution remain evidence or shadow planes.
- Local verification may support the source candidate but cannot replace Go 1.26.5, native Windows, signing, attestation, and publisher checks in hosted CI.

## Decisions so far

- **Keep the existing release authority** — no local commit, tag, signature, upload, or release publication is manufactured from an archive-derived workspace.
- **Fix coverage at the sandbox seam** — coverage-instrumented external adapter helpers receive a private sandbox `GOCOVERDIR`; the parent path and environment remain isolated.
- **Measure before optimizing** — workspace schema admission was optimized only after `BenchmarkSearchMemory` identified double JSON materialization as the allocation hot path.
- **Make benchmark evidence composable** — each benchmark has an independently bounded command and the validator can consume separately captured evidence when a constrained runner cannot sustain the full collection command.
- **Preserve the operational UI** — copy and visual hierarchy were refined in place; no marketing redesign or new authority-bearing interaction was introduced.
- **Keep SQLite explicitly preview-only in public Windows binaries** — CGO-disabled releases fail closed for database commands and document the native requirement.
- **Defer external adapter activation** — Phase 6 remains conformance-only until publisher identity and enforceable OS/WASI sandboxing exist.
- **Defer deep refactors until after publication** — the release diff stays narrow; architecture candidates are recorded separately.

## Not yet specified

- Publisher-bound adapter package and lockfile format.
- Enforceable WASI or OS sandbox design, including network, filesystem, process-tree, CPU, memory, and Windows descendant controls.
- Whether native SQLite metadata should become an officially distributed Windows preview.

## Out of scope

- Publishing or signing from this workspace.
- Activating external adapters.
- Promoting CAS or SQLite to authoritative reads or writes.
- Replacing the current UI framework or adding a Jetpack Compose client.
