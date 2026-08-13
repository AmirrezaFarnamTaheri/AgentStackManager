# Security Model

## Trust boundary

AgentStack is a local single-user control plane. Its privileged operations run with the current user’s permissions. It does not create an administrator service, remote API, multi-user tenancy boundary, or credential vault.

The desktop manager’s internal service binds only to loopback and uses both a high-entropy session path and a per-process request token. All API endpoints—including reads—require the token; cross-origin requests, non-JSON mutations, oversized bodies, and concurrent UI mutations are rejected. This protects against accidental access and browser-origin attacks. It is **not** a boundary against malicious code already executing as the same OS user.

Windows state directories receive a current-user-only DACL and are audited in Windows-native CI. Atomic managed-file replacement carries the destination DACL onto the replacement before the swap. POSIX state directories are forced to mode `0700`, and replacement preserves the destination mode.

## Mutation authorization

- Plans are immutable, expiring, content-addressed records.
- Apply requires the exact plan ID and digest.
- The service rechecks catalog and minimized inventory digests immediately before mutation.
- A cross-process lease prevents concurrent state mutation.
- Transactions are journaled incrementally and recovered after interruption.
- Existing unrelated packages, skills, paths, and MCP entries are outside the ownership boundary.

## Command and process safety

Catalog commands are static structured arrays—not shell strings. Component identifiers, versions, package sources, publishers, platform compatibility, and MCP resource ceilings are validated. Every external invocation has a timeout and bounded stdout/stderr. Unix process groups are PGID-checked before signaling and remain reachable after a leader exits; Linux additionally records the kernel process start time when available to reject same-PID reuse. Windows MCP trees start suspended, enter a Job Object before executing, and then resume under kill-on-close plus catalog-defined job-memory, CPU-rate, and active-process limits.

MCP messages are size-bounded, protocol versions are negotiated, and `doctor` performs live `initialize` and `tools/list` probes rather than testing only executable presence.

## Supply chain

- Automatic npm, uv, and WinGet actions are exact-version and approved-source entries.
- The essential skill pack is fetched at an exact audited Git commit, and the resolved commit plus expected skill inventory is verified before copying.
- Public releases require a clean signed annotated tag, Go 1.26.5, source and binary vulnerability scans, reproducibility comparison, SBOMs, license inventory, OpenVEX, deterministic archives, checksum manifests, and artifact attestations.
- Setup verifies the sibling console binary’s embedded digest and release checksum manifest. Public Windows console and setup artifacts must also carry a valid Authenticode signature from the configured publisher; setup rejects an invalid or unexpected signer.

A catalog change remains a privileged supply-chain decision and is protected by code-owner/review policy.

## Credentialed integrations

AgentStack records no provider secret. It shows the official login mechanism and next action, then delegates authentication and revocation to the provider’s CLI or OS-backed credential store. Diagnostic/event redaction removes secret-like keys and common bearer/token patterns.

## Recovery limitations

AgentStack can restore indexed managed-file backups and ownership-scoped skill quarantine. It cannot promise atomic rollback of a third-party package manager after that package manager has completed an install. Such side effects are journaled and surfaced for operator recovery.

## Reporting

Use `agentstack diagnostics` for a path-sanitized, secret-redacted support bundle. Never attach credentials, provider tokens, private keys, raw home-directory exports, or unreviewed machine inventory to an issue.

## Unified fabric boundaries

- Remote marketplace results and URL-shaped imports cannot install or activate resources. The trusted path begins with a local reviewed file or checkout.
- Resource audit precedes normal sync planning. Blocking results require explicit `--allow-risk` in the plan and still cannot override foreign destination conflicts.
- Project reads/searches canonicalize roots, reject traversal and symlink escape, and enforce file, scan, and result limits.
- Git context runs only bounded read commands; context management never installs hooks or stages files.
- MCP client linking preserves unrelated entries, rejects foreign or secret-bearing same-name entries, rejects duplicate-key JSON, and revalidates configuration immediately before apply. Plans retain only intent/digests and recovery records contain only the prior ASM registration, never the full client file.
- Routine command steps invoke direct binaries through the supervised runner. They do not execute shell strings or persist environment variables.
- Routine outputs and errors pass through recursive credential redaction before durable receipt storage.
- Provider tokens, SMTP passwords, API keys, and private keys are not fields in unified fabric schemas. Routine admission rejects secret-bearing arguments; explicit environment/file/reference keys remain the supported indirection.
- Resource admission is bounded by per-file, aggregate-byte, and file-count ceilings before canonical state changes.
- Durable resource, workspace, routine, context, and MCP state is read through bounded regular-file admission. Symlinks, non-regular files, oversized payloads, duplicate JSON keys, unsupported schema versions, invalid identifiers, unconstrained commands, and path-confined record violations fail closed before use.

## SQLite shadow metadata security

The Fabric SQLite database is a non-authoritative index of sealed migration evidence. Database paths must be regular non-symlink files beneath non-symlink directories, are bounded to 256 MiB, and are restricted to mode `0600` where POSIX permissions apply. ASM refuses databases carrying another application ID or pre-existing foreign schema instead of adopting them. Persistent journal changes occur only after that admission check. Every read validates the ASM application ID, schema migration ledger, quick check, foreign keys, sealed receipt, and canonical artifact/resource rows. No payload bytes or secret values are stored in SQLite.

The native backend is available only in CGO builds linked with SQLite 3.37 or newer. CGO-disabled builds return a fail-closed unavailable error for database commands and do not silently substitute a weaker storage format.
