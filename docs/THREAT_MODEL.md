# Threat Model

## Assets

- current-user filesystem and PATH;
- AgentStack ownership, backups, plans, and transaction journal;
- Codex/AGY MCP configuration;
- local MCP memory;
- release binaries and catalog policy;
- provider credentials managed outside AgentStack.

## Actors and entry points

- legitimate local user through CLI/setup/browser UI;
- same-user local malware;
- another local OS user;
- malicious package or compromised registry/source;
- malicious MCP child or tool output;
- compromised CI runner, workflow, tag, signing key, or release archive;
- malformed configuration, inventory output, or interrupted process.

Entry points include CLI arguments, loopback HTTP, catalog data, package-manager output, Git skill content, MCP stdio messages, Codex/AGY config, release scripts, and downloaded ZIPs.

## Key mitigations

| Threat | Mitigation | Residual risk |
|---|---|---|
| Apply differs from reviewed plan | sealed ID/digest plus catalog/inventory revalidation | a trusted user can still approve a harmful exact plan |
| Cross-origin browser attack | secret session path/token, origin checks, all-endpoint auth, JSON/content/size/rate gates | same-user malware is not isolated |
| Concurrent/corrupt state | mutation lease, staged writes, incremental journal, digest-indexed backups | third-party package managers are not transactional with AgentStack |
| Command injection | structured static command arrays, catalog validation, no shell concatenation | invoked third-party tools retain their own attack surface |
| Malicious MCP output | bounded message/stderr, protocol validation, process-tree limits | selected MCP tools may intentionally perform privileged local actions |
| Supply-chain substitution | exact versions/sources/commit, SBOM/license/VEX, clean signed tag, reproducibility, signatures, attestations | trust remains in approved publishers, CI, and signing key custody |
| Stale client configuration | conservative classification, backup, owned repair, conflict stop | external client schema changes require catalog/code updates |
| Sensitive local metadata | minimized inventory, redacted events, DACL/0700, retention/export/delete controls | same-user compromise can read user-owned state |

## Abuse cases

- trick the user into applying a different selection after review;
- replace the setup sibling binary;
- publish a moving skill branch with malicious agent instructions;
- flood MCP stdout/stderr or hang a package manager;
- create a foreign router entry with the AgentStack name;
- race two managers against ownership/configuration state;
- exfiltrate raw home paths or secrets through diagnostics;
- bypass release checks with a dirty tree, lightweight tag, unsupported toolchain, or unsigned artifact.

Each case is covered by automated checks or a fail-closed release/runtime control. Windows end-to-end and accessibility evidence are required release gates rather than assumed from cross-compilation.

## Convergence-specific threats

| Threat | Control |
| --- | --- |
| Malicious donor prompt or extension content | inert local import, static audit, reviewed sync, no execution from registry state |
| Remote marketplace supply-chain substitution | no direct remote install; operator supplies local reviewed checkout with tracked digest |
| Workspace traversal or symlink escape | canonical-root confinement and bounded file/search operations |
| Hidden Git mutation | read-only bounded Git commands; no hook installation or staging |
| Cross-client MCP config destruction | foreign-entry preservation, same-name conflict, before-state digest, backup |
| Routine shell injection | direct executable and argument array; no shell string |
| Credential leakage in automation evidence | recursive structured/text redaction before receipt persistence |
| State-version confusion | explicit schemas, legacy migration, unknown-version rejection |
| Duplicate product authority | one service and one durable owner per plane; UI status is non-authoritative |

### SQLite shadow metadata threats

| Threat | Control |
| --- | --- |
| Wrong database path damages another application | application-ID and empty-schema admission before persistent pragmas or migrations |
| Database symlink/path substitution | regular-file and no-symlink path walk plus pre/post-open file identity checks |
| Logical row tampering | sealed receipt validation and byte-for-byte artifact/resource row comparison |
| SQLite structural corruption | quick check, foreign-key check, supported schema and migration-ledger validation |
| Backup overwrites operator data | explicit confirmation, online backup to incomplete file, verification, hard-link no-replace publication |
| SQLite becomes a second authority | database is shadow-only; current verification resolves back to Resource Hub and CAS |
| Platform silently falls back to another format | CGO-disabled backend fails closed with `ErrUnavailable` |
