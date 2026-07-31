# AgentStack Manager User Guide

## Installation

Extract the complete signed release ZIP. Keep `AgentStack-Setup.exe`, the architecture-specific console executable, and `SHA256SUMS.txt` together. Double-click setup or run the bundled PowerShell launcher.

The public setup path is fail-closed: it requires a valid Authenticode signature, the expected publisher thumbprint embedded at build time, and the exact embedded SHA-256 of its sibling console binary.

## Preservation-first workflow

1. Run `agentstack inventory`.
2. Select a focused profile. New installations should start with `core`.
3. Run `agentstack plan` and inspect every action.
4. Retain the emitted plan ID and digest.
5. Apply only that identity with `--yes`.
6. Run `agentstack mcp doctor` and a client session smoke test.

```powershell
$plan = agentstack plan --profile core | ConvertFrom-Json
$plan.actions | Format-Table kind, componentId, reason
agentstack apply --plan-id $plan.id --digest $plan.digest --yes
```

A plan expires and is invalidated by relevant machine, catalog, or selection changes. AgentStack does not automatically upgrade, uninstall, or claim ownership of pre-existing software.


## Browser manager feedback

Every long-running manager action uses the same operation surface. The initiating button shows a task-specific spinner and busy label, mutation-related controls are locked, and a polite live region reports running, successful, or failed state. Focus returns to the initiating control unless the operation intentionally navigates to a result section. Navigation uses one locally embedded Lucide outline icon family; no icon CDN or runtime font is loaded.

Client-side locking is not an authorization boundary. The server still enforces the mutation gate, sealed-plan identity, and cross-process lease.

## Install and repair semantics

- `preserve`: existing compatible software remains untouched.
- `install`: an exact catalog package/source/version is missing.
- `repair`: AgentStack previously owned the component, but its postcondition is no longer satisfied.
- `upgrade`: allowed only when an incompatible runtime blocks the requested profile and `--allow-upgrades` is explicit.
- `preserve-inactive`: a duplicate provider remains installed but is not exposed by the managed profile.
- `manual`: the catalog gives an explicit provider setup/login action; AgentStack does not fabricate or store credentials.

Successful package-manager exit is not sufficient. AgentStack rescans and verifies the catalog postcondition before recording an action as successful.

## Credential integrations

Run `agentstack integrations` to see exact installation status, login mechanism, required environment variables, official next step, and whether AgentStack can install the integration without authenticating. Use `--allow-credentials` only when intentionally selecting such a component.

AgentStack never stores provider passwords, OAuth tokens, API keys, or cloud credentials. Credential storage and revocation remain with the provider’s official CLI or OS-backed credential mechanism.

## MCP and sessions

```powershell
agentstack mcp init --profile core --yes
agentstack mcp doctor
agentstack codex --profile core --
agentstack agy --profile core --
```

The router negotiates the client protocol version, bounds messages and stderr, performs live child probes, and reuses healthy child processes during one router session. Child and session process trees are terminated through Unix process groups or Windows Job Objects when a timeout, cancellation, or shutdown occurs. Catalog-managed MCP children also declare Windows Job Object ceilings for aggregate job memory, CPU hard-cap percentage, and active process count. Non-Windows builds reject nonzero hard-limit requests rather than claiming unsupported enforcement; the supported distributable remains Windows x64 and ARM64.

Registration rules:

- absent entry: add the AgentStack entry;
- equivalent entry: preserve it;
- stale AgentStack-owned entry: back up and repair it;
- foreign conflict: stop and report it;
- lookup/read failure: stop instead of assuming absence.

## Recovery

List and preview backups before restore:

```powershell
agentstack backup
agentstack backup restore --id <id> --preview
agentstack backup restore --id <id> --yes
```

The preview verifies digest, target, and structure. Restore is limited to the indexed original target. Router restores receive a live MCP validation after writing.

Transactions are journaled before and after each action. Interrupted transactions are marked recoverable on the next start. Package-manager side effects already completed before a later failure are reported rather than silently removed; third-party package managers remain the source of truth for their own rollback.

## AgentStack-owned removal

```powershell
agentstack owned preview --component superpowers
agentstack owned deactivate --component memory-mcp --yes
agentstack owned remove --component superpowers --yes
```

Removal is limited to ownership records created by AgentStack. Skill content is quarantined before removal. Unclassified/manual and unrelated software is refused.

## Privacy controls

```powershell
agentstack data policy
agentstack data export --out agentstack-data.zip
agentstack diagnostics --out agentstack-diagnostics.zip
agentstack data clear --scope operational --yes
```

Default retention is 24 hours for sealed plans, 30 days for completed transactions, 14 days for generated diagnostics, and 30 days for structured event records. Backups, ownership, and MCP memory remain until explicit deletion because automatic expiry could destroy recovery or user state.

## Data locations

```text
%LOCALAPPDATA%\AgentStack
%LOCALAPPDATA%\Programs\AgentStack\bin\agentstack.exe
```

AgentStack applies a current-user-only DACL to its Windows data directory and audits it in native Windows tests. The loopback UI token protects the browser session but does not defend against malicious software already running as the same user.
