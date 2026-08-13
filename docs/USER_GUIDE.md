# AgentStack Manager User Guide

## Installation

Extract the complete signed release ZIP. Keep `AgentStack-Setup.exe`, the architecture-specific console executable, and `SHA256SUMS.txt` together. Double-click setup or run the bundled PowerShell launcher.

The public setup path is fail-closed on the exact SHA-256 of its sibling console binary and the release checksum manifest. Artifacts are integrity-attested by GitHub Actions; Windows Authenticode is not required.


## Unified desktop launch

`agentstack ui` starts one desktop application window and owns the internal server for the lifetime of that window. Backend child processes run without visible console windows. Their raw commands, arguments, environment, stdout, and stderr are not displayed in the application. Use `agentstack ui --browser` only for explicit browser-based development, or `agentstack ui --no-open` when another local client should open the printed URL.

## Sharing, synchronization, and deduplication

Select several verified targets in **Environments** to connect or pause them together. Discovery requires executable, application-registration, configuration-schema, or existing managed-target evidence; a folder alone is not classified as a confirmed installation.

In **Sharing & Sync**, select canonical resources and target installations, build an immutable plan, review its digest-bound child operations, then approve it. Independent target roots may apply concurrently up to the selected bound. The manager serializes operations that share a canonical root, revalidates current state before mutation, and preserves successful independent targets when another target fails.

Duplicate classes are explicit: exact duplicates may be consolidated through a reviewed plan; equivalent target renderings remain installations of one canonical resource; version duplicates, shadows, or divergent name collisions require review. Foreign and unmanaged files are never automatically deleted.

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


## Desktop manager lifecycle

The manager has five primary areas:

1. **Home** summarizes readiness, attention items, recent work, and one recommended next action.
2. **Environments** shows evidence-backed discovery for AI applications, IDEs, CLIs, profiles, repositories, MCP servers, and workspaces. Multiple verified targets can be connected, paused, or reconnected in one reviewed batch.
3. **Sharing & Sync** separates canonical resources from their installations and shows Managed, Installed, Contained, In sync, Drifted, Duplicates, Conflicts, Plans, and History.
4. **Changes** keeps profile and tool selection, the inline estimate, exact pending changes, approval, and the only tool-install apply action in one place.
5. **Activity** shows the live parallel installation tracker, completed and partial transaction history, classified root causes, maintenance actions, system checks, and sanitized technical details.

### Apply and progress

Choose tools in Changes and select **Create changes**. Review every item, check the approval control, and select **Approve and apply**. The reviewed plan is single-use. The Activity tracker reports the server-provided stage—Preparing, Installing, Configuring, Verifying, or Complete—plus completed/total counts and each item's state. It does not estimate progress from a timer.

### Recover from a partial failure

If some installations fail, successful changes remain recorded. The consumed plan is cleared and cannot be retried. Activity offers **Review failed items** and **Create fresh plan**; both refresh inventory and build a new reviewed change set. Desktop messages use stable, plain-language failure codes and never expose the plan-store path, executable command, arguments, or raw subprocess output.

Every long-running manager action uses the same authenticated operation controller. Conflicting controls are locked while work is active, live regions announce complete state, and focus returns predictably. Client-side locking is not an authorization boundary: the server still enforces reviewed-plan identity, confirmation, the cross-process lease, mutation authority, and postcondition verification.

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

## Manage the unified fabric

Start with project evidence:

```powershell
agentstack context scan --root C:\src\project
agentstack context score --root C:\src\project --target codex
```

Import and audit a local resource, register a destination, then review and apply a sync plan:

```powershell
agentstack hub import --id my-skill --kind skill --path C:\src\my-skill --target codex
agentstack hub audit --id my-skill
agentstack hub target-add --id project --agent codex --root C:\src\project --mode copy
agentstack hub plan-sync --target project --resource my-skill
agentstack hub apply-sync --plan-id <ID> --digest <SHA> --yes
```

Use workspaces to group project roots, resources, routines, prompt templates, memory, and artifacts. Use routines for confirmed repeatable sequences; inspect redacted results with `agentstack routine history`.

The complete command set and safety rules are documented in [CLI Reference](CLI_REFERENCE.md) and [Convergence Runbook](convergence/RUNBOOK.md).

## Connect AI applications

Open **Environments** and select an AI application. AgentStack detects supported user configuration folders for Codex, Claude, AGY/Gemini, OpenCode, Cursor, and GitHub Copilot. Choose **Connect** to register an AgentStack-managed Resource Hub target, **Reconnect** to repair a paused target, or **Pause** to stop synchronization without deleting the application's own files.

Connection state is derived from registered targets and detected local configuration; it is not a decorative toggle. Credentialed resources still require their own provider sign-in.

## Understand a failed change

The Activity result names the root cause, installation method, normalized error code when available, observed evidence, affected tools, and the safest next action. Expand **Technical details** for sanitized diagnostic metadata. AgentStack does not expose raw commands, private paths, tokens, stdout, or stderr in the browser.

Use **Retry failed items** only after applying the listed fix. A fresh plan rechecks the current inventory and package catalog before any mutation.
