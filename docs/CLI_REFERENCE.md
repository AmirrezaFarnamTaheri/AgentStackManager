# CLI Reference

The CLI emits JSON unless a command is explicitly interactive. Mutating commands either consume a sealed plan or require an explicit operation-specific confirmation.

## Application and discovery

```text
agentstack ui [--no-open] [--listen 127.0.0.1:PORT]
agentstack setup [--no-launch]
agentstack install-self
agentstack version
agentstack status
agentstack inventory
agentstack catalog
agentstack profiles
agentstack integrations
agentstack sbom [--version VERSION] [--licenses PATH] [--out FILE]
agentstack releasepack --root DIR --out ZIP [--prefix NAME]
```

`ui` refuses non-loopback addresses. Its random session path and request token are per-process secrets. They protect against cross-origin and accidental access; they are not a security boundary against malicious software already running as the same OS user.

## Sealed planning and apply

```text
agentstack plan [selection options]
agentstack apply --plan-id ID --digest SHA256 --yes
```

A plan includes `id`, `digest`, `catalogDigest`, `inventoryDigest`, `createdAt`, and `expiresAt`. Apply rejects missing, altered, expired, catalog-drifted, or inventory-drifted plans. It never accepts profile or component selection flags.

PowerShell example:

```powershell
$plan = agentstack plan --profile core --include fd | ConvertFrom-Json
$plan.actions | Format-Table kind, componentId, reason
agentstack apply --plan-id $plan.id --digest $plan.digest --yes
```

### Selection options

```text
--profile ID
--include id,id
--exclude id,id
--provider capability=component-id
--allow-credentials
--allow-upgrades
```

Dependencies are expanded and topologically ordered. Excluding a required dependency fails planning. An incompatible existing runtime is preserved unless the user explicitly supplies `--allow-upgrades` and the catalog contains an exact approved upgrade.

## MCP

```text
agentstack mcp init [selection options] --yes [--no-warm] [--no-register]
agentstack mcp doctor
agentstack mcp list
agentstack mcp-router [--config PATH]
```

`mcp doctor` performs real MCP `initialize` and `tools/list` probes. Results are briefly cached to avoid repeated package startup. The router exposes four stable tools:

- `agentstack_router_list_servers`
- `agentstack_router_list_tools`
- `agentstack_router_call_tool`
- `agentstack_router_doctor`

Healthy child processes are pooled for the router lifetime and terminated as a bounded process tree when the router exits or a child becomes unusable.

## Session wrappers

```text
agentstack codex [selection options] -- [Codex arguments]
agentstack agy [selection options] -- [AGY arguments]
```

The wrapper validates/initializes the selected MCP profile and repairs only an AgentStack-owned stale registration before starting the requested client. A foreign name conflict or an unreadable client configuration is a hard error.

## Backups and restore

```text
agentstack backup [list]
agentstack backup restore --id ID [--target EXACT_ORIGINAL_PATH] --preview
agentstack backup restore --id ID [--target EXACT_ORIGINAL_PATH] --yes
```

Preview verifies the indexed digest, target ownership, and structural validity without writing. Restore re-verifies the digest, restores only the indexed original target, and performs live MCP validation when a router configuration is restored.

## Privacy and diagnostics

```text
agentstack data policy
agentstack data export [--out FILE]
agentstack data clear --scope operational|memory|all --yes
agentstack diagnostics [--out FILE]
```

`data policy` reports effective retention. Export excludes active locks and unexpired sealed plans. Diagnostics redact secret-like fields and user-specific paths. `clear all` removes AgentStack state, backups, ownership records, and memory; it does not uninstall unrelated third-party software.

## AgentStack-owned lifecycle

```text
agentstack owned preview [--component ID]
agentstack owned deactivate [--component ID] --yes
agentstack owned remove [--component ID] --yes
agentstack cleanup --preview
```

Lifecycle commands operate only on recorded AgentStack-owned resources. Skill files are moved to recoverable quarantine before ownership is removed. Manual/unclassified installations are refused. `cleanup --preview` never removes software.

## Profiles

- `core` — minimal runtime, search, JSON, and memory router foundation.
- `essential` — legacy broad 0.1 baseline; retained for compatibility.
- `recommended` — broad quality/security/documentation enrichment.
- `web-development` — browser and web engineering stack.
- `security` — scanning and security-review tools.
- `architecture` — repository mapping, diagrams, metrics, and dependency analysis.
- `documentation` — conversion, prose/Markdown checks, diagrams, and task automation.
- `python` — Python linting, security, automation, and notebook MCP.
- `full-local` — all maintained local components, with duplicate providers inactive unless selected.
- `custom` — no implicit components or providers.

### `agentstack version`

Prints the product version and embedded source identity. Protected releases use
`git:<commit>`; verified but unreleased source bundles use `unreleased-base:<commit>` so the
output cannot be mistaken for immutable candidate or release evidence.
