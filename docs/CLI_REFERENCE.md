# CLI Reference

The CLI emits JSON unless a command is explicitly interactive or produces file artifacts (`sbom --out` writes CycloneDX JSON to file, while `releasepack --out` writes a ZIP archive and emits no stdout JSON response). Mutating commands either consume a sealed plan or require an explicit operation-specific confirmation.

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
agentstack releasepack --root DIR --manifest-mode write
agentstack releasepack --root DIR --manifest-mode verify
agentstack releasepack --root DIR --out ZIP [--prefix NAME] --manifest-mode require
```

`releasepack` defaults to generic deterministic ZIP creation (`--manifest-mode none`). Source-bundle workflows use the provenance capsule:

- `write` validates `SOURCE_REVISION` and `SOURCE_PROVENANCE.json`, rejects runtime artifacts, then atomically writes `SOURCE_MANIFEST.sha256`.
- `verify` checks metadata, exact manifested file membership, and every SHA-256 digest.
- `require` performs verification, writes a ZIP containing only manifested files plus the manifest, reopens it, and rejects duplicate, missing, extra, or digest-mismatched members.

The standalone `cmd/releasepack` helper exposes the same modes for release scripts.

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

## Unified fabric commands

All commands emit JSON. Inspection and planning are non-destructive. Mutations require `--yes` or exact reviewed plan identity plus digest.

### Resource hub

```text
agentstack hub list
agentstack hub import --id ID --kind skill|agent|rule|command|prompt|mcp-server|context --path PATH [--name NAME] [--description TEXT] [--scope SCOPE] [--tag TAG]... [--target AGENT]... [--replace]
agentstack hub audit --id ID
agentstack hub targets
agentstack hub target-add --id ID --agent codex|claude|cursor|opencode|github-copilot|generic --root PATH [--mode auto|copy|link] [--enabled=true|false]
agentstack hub plan-sync --target ID [--resource ID]... [--prune] [--allow-risk]
agentstack hub apply-sync --plan-id ID --digest SHA --yes
agentstack hub plan-refresh [--resource ID]...
agentstack hub apply-refresh --plan-id ID --digest SHA --yes
agentstack hub backups
agentstack hub restore --backup ID --yes
agentstack hub remove --id ID --yes
```

`hub import` accepts a local regular file or directory. URL-shaped sources are not fetched. Import does not activate a resource. Activation occurs only through reviewed sync.

### Project context

```text
agentstack context scan --root PATH
agentstack context score --root PATH [--target AGENT]...
agentstack context read --root PATH --path RELATIVE
agentstack context search --root PATH --query TEXT [--limit N]
agentstack context git --root PATH
agentstack context plan --root PATH [--target AGENT]...
agentstack context apply --plan-id ID --digest SHA --yes
```

Reads and searches are confined to the canonical project root. Git inspection is read-only and bounded.

### Workspaces

```text
agentstack workspace list
agentstack workspace show --id ID
agentstack workspace create --id ID --name NAME [--type workspace|folder] [--parent ID] [--root PATH] [--prompt TEXT] [--resource ID]... [--routine ID]...
agentstack workspace update --file workspace.json
agentstack workspace render --id ID [--var key=value]...
agentstack workspace delete --id ID --yes
```

`workspace update` and routine definition commands accept strict JSON from a regular file no larger than 1 MiB. Unknown fields, duplicate keys, trailing content, symlinks, devices, and oversized input are rejected before mutation.

Folders are acyclic. Workspace roots are canonical local directories.

### Memory

```text
agentstack memory remember --layer user|project|workspace|session [--scope ID] --key KEY --value TEXT [--tag TAG]... [--source TEXT] [--ttl DURATION]
agentstack memory recall [--workspace ID] [--session ID] --key KEY
agentstack memory search [--workspace ID] [--session ID] [--query TEXT]
agentstack memory forget --layer LAYER --scope ID --key KEY --yes
```

Project, workspace, and session layers require scope. Recall precedence is session, workspace, project, then user.

### Artifacts

```text
agentstack artifact add --workspace ID --id ID --path FILE [--name NAME] [--media-type TYPE] [--replace]
agentstack artifact list [--workspace ID]
agentstack artifact verify --id ID
agentstack artifact remove --id ID --yes
```

Artifacts must be regular files, are size bounded, and are copied into content-addressed local storage.

### Routines

```text
agentstack routine put --file routine.json
agentstack routine list
agentstack routine due
agentstack routine history [--id ID] [--limit N]
agentstack routine run --id ID --yes
agentstack routine run-due --yes
agentstack routine remove --id ID --yes
```

Supported schedules: `manual`, `daily`, `weekdays`, and `interval`. Supported typed steps: `inventory`, `mcp-doctor`, `context-scan`, `context-score`, `memory-search`, `prompt-render`, `artifact-verify`, `resource-audit`, `resource-refresh-plan`, and `command`. Command steps invoke a direct binary and argument list, never a shell string.

### MCP client linking

```text
agentstack mcp clients plan [--root PATH] [--mode link|unlink] [--client codex|claude|cursor|agy|opencode]...
agentstack mcp clients apply [--root PATH] --plan-id ID --digest SHA --yes
```

Client plans preserve foreign entries and fail on a foreign same-name `agentstack-router` entry.
