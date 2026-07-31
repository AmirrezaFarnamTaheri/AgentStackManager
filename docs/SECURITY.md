# Security Model

## Trust boundary

AgentStack is a local single-user control plane. Its privileged operations run with the current user’s permissions. It does not create an administrator service, remote API, multi-user tenancy boundary, or credential vault.

The browser manager binds only to loopback and uses both a high-entropy session path and a per-process request token. All API endpoints—including reads—require the token; cross-origin requests, non-JSON mutations, oversized bodies, and concurrent UI mutations are rejected. This protects against accidental access and browser-origin attacks. It is **not** a boundary against malicious code already executing as the same OS user.

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
- Public releases require a clean signed annotated tag, Go 1.26.5, source and binary vulnerability scans, reproducibility comparison, Authenticode signing, SBOMs, license inventory, OpenVEX, deterministic archives, and artifact attestations.
- Setup verifies both its own Authenticode signer and the signed sibling console binary’s embedded digest and publisher thumbprint.

A catalog change remains a privileged supply-chain decision and is protected by code-owner/review policy.

## Credentialed integrations

AgentStack records no provider secret. It shows the official login mechanism and next action, then delegates authentication and revocation to the provider’s CLI or OS-backed credential store. Diagnostic/event redaction removes secret-like keys and common bearer/token patterns.

## Recovery limitations

AgentStack can restore indexed managed-file backups and ownership-scoped skill quarantine. It cannot promise atomic rollback of a third-party package manager after that package manager has completed an install. Such side effects are journaled and surfaced for operator recovery.

## Reporting

Use `agentstack diagnostics` for a path-sanitized, secret-redacted support bundle. Never attach credentials, provider tokens, private keys, raw home-directory exports, or unreviewed machine inventory to an issue.
