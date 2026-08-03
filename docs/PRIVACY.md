# Privacy and Data Handling

## Data processed

AgentStack processes local software inventory, package names/versions, managed configuration state, transaction outcomes, MCP health metadata, backups of AgentStack-managed files, and optional local MCP memory. It does not intentionally collect user documents, source code, browser content, provider credentials, or cloud data.

## Purpose and minimization

Inventory is used to preserve existing software, determine exact plan actions, and verify postconditions. Persisted inventory removes absolute executable paths and raw command output. Structured events contain correlation IDs, component/server digests, durations, and status; secret-like nested keys and common token patterns are redacted.

## Storage and recipients

Data remains under the current user’s local AgentStack data root. AgentStack has no built-in telemetry or remote analytics. External package managers, MCP servers, and credential providers process data only when the user explicitly invokes them; their own privacy policies apply.

## Retention

Run `agentstack data policy` for the authoritative policy:

- sealed plans: 24 hours or earlier expiry;
- completed transactions: 30 days;
- generated diagnostic bundles: 14 days;
- structured events: 30 days, with size rotation;
- backups, ownership, MCP memory: retained until explicit deletion.

Unreadable recovery evidence is preserved rather than silently deleted.

## User controls

```text
agentstack data export --out FILE
agentstack diagnostics --out FILE
agentstack data clear --scope operational|memory|all --yes
```

Export excludes locks and sealed plans. Diagnostics are sanitized for support. `clear all` removes all AgentStack state, including backups and ownership, but does not uninstall unrelated packages or erase provider-managed credentials.

## Security and residual risk

Windows data receives a current-user-only DACL; POSIX state uses mode `0700`. The browser UI is loopback-only and token-gated. Software already running as the same user may still access that user’s files or browser session; AgentStack is not a sandbox against a compromised local account.

## Credential redaction boundaries

Free-form event messages and nested fields are sanitized before persistence. The same
redaction is applied again when legacy events are read and when diagnostics or data exports
are created. Authorization bearer values, JWTs, JSON token fields, common key/value secret
forms, GitHub token prefixes, and OpenAI-style secret prefixes are removed. This layered
boundary prevents historical unredacted records from being copied into support archives.

## Unified fabric data

The fabric stores only operator-created local state under the private AgentStack data root: canonical resource metadata/content, project context plans/backups, workspace records, scoped memory, artifacts, MCP-link intent/digest plans and registration-only recovery records, routines, and run receipts. MCP-link state never duplicates complete third-party client configurations or unrelated environment values.

Memory entries expose source, scope, expiry, digest, explicit forget, and local search. Artifacts are copied locally and can be verified or removed. Routine receipts are bounded and recursively redact credential-bearing keys and common token forms before persistence. Project content is read only on operator request and is not uploaded by these planes.

Provider credentials remain in provider or operating-system credential mechanisms. The fabric does not add a general credential vault.
