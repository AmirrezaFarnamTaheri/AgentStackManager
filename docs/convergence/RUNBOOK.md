# Unified Fabric Operator Runbook

Every mutation is either an explicit confirmed command or the application of an expiring reviewed plan.

## Resource admission and sync

```powershell
agentstack hub import --id review --kind skill --path .\review-skill --tag security --target codex
agentstack hub audit --id review
agentstack hub target-add --id project-codex --agent codex --root C:\src\project --mode copy
agentstack hub plan-sync --target project-codex --resource review
agentstack hub apply-sync --plan-id <PLAN_ID> --digest <DIGEST> --yes
```

Use `--allow-risk` only during planning after inspecting every blocking finding. It is captured in the reviewed plan and does not bypass destination conflict checks.

## Source update

```powershell
agentstack hub plan-refresh --resource review
agentstack hub apply-refresh --plan-id <PLAN_ID> --digest <DIGEST> --yes
agentstack hub backups
agentstack hub restore --backup <BACKUP_ID> --yes
```

The tracked local source, canonical resource, registry, plan, and expiry must all still match.

## Project context

```powershell
agentstack context scan --root C:\src\project
agentstack context score --root C:\src\project --target codex --target claude
agentstack context read --root C:\src\project --path internal\app\service.go
agentstack context search --root C:\src\project --query "reviewed plan" --limit 20
agentstack context git --root C:\src\project
agentstack context plan --root C:\src\project --target codex --target claude
agentstack context apply --plan-id <PLAN_ID> --digest <DIGEST> --yes
```

## Workspace and memory

```powershell
agentstack workspace create --id asm --name ASM --root C:\src\asm --prompt "Goal: ${goal}"
agentstack memory remember --layer workspace --scope asm --key release --value "Use protected CI"
agentstack memory recall --workspace asm --key release
agentstack workspace render --id asm --var goal="Prepare release"
```

Project, workspace, and session memory require scope. Use `memory forget ... --yes` for deletion.

## Artifacts

```powershell
agentstack artifact add --workspace asm --id report --path .\report.md --media-type text/markdown
agentstack artifact verify --id report
agentstack artifact list --workspace asm
agentstack artifact remove --id report --yes
```

## MCP clients

```powershell
agentstack mcp clients plan --root C:\src\project --mode link --client codex --client claude --client cursor
agentstack mcp clients apply --root C:\src\project --plan-id <PLAN_ID> --digest <DIGEST> --yes
```

A foreign same-name router entry blocks apply. Unlink uses `--mode unlink` and removes only the recognized AgentStack entry.

The plan file contains only client identity, action, path, and before/after digests. ASM rebuilds the desired configuration from the live file after before-state verification. Recovery records contain only the prior `agentstack-router` registration or absence; inspect them under the private MCP-link backup directory when reconstructing a prior link state.

## Routines

Create a strict JSON document, then:

```powershell
agentstack routine put --file .\routine.json
agentstack routine list
agentstack routine due
agentstack routine run --id morning --yes
agentstack routine history --id morning --limit 20
```

Command steps execute a named binary directly with arguments, a deadline, and a 1 MiB output limit. A complete run is capped at 24 hours. Secret-bearing arguments and assignments are rejected when a routine is admitted; use explicit environment/file/reference parameters instead. Output and errors are redacted before run receipts are stored.

## Canonical graph and CAS shadow stage

```powershell
agentstack hub graph
agentstack hub cas-stage > .\asm-v1-cas-receipt.json
agentstack hub cas-verify --receipt .\asm-v1-cas-receipt.json
agentstack hub cas-restore --receipt .\asm-v1-cas-receipt.json --resource review --destination .\restored-review --yes
```

The stage is additive: Resource Hub v1 remains authoritative. Keep the receipt with the matching CAS root. Verification fails when the receipt is malformed, any referenced object is corrupt, or the current Resource Hub graph differs from the graph that was staged. Restore only to a new destination. ASM refuses destination and entry replacement. If a tree restore fails after destination reservation, inspect the retained directory and `.agentstack-restore-incomplete-*` marker; ASM deliberately does not delete published content.


## SQLite shadow metadata

```powershell
agentstack hub db-stage
agentstack hub db-inspect
agentstack hub db-verify
agentstack hub db-backup --destination .\metadata-backup.db --yes
```

The database is a rebuildable shadow index, not Resource Hub authority. `db-stage` also refreshes and verifies CAS. `db-inspect` never creates missing state. `db-verify` fails when Resource Hub changed after the stored receipt or when CAS/database evidence is corrupt. Back up only to a new destination; ASM never replaces an existing database backup.

## Adapter capabilities and loss policy

Inspect all built-in target capabilities or one canonical target/alias:

```powershell
agentstack hub adapters --project-root C:\src\project --target-root C:\src\project
agentstack hub adapters --project-root C:\src\project --target gemini
```

Every emitted snapshot has a digest. Review artifact support, field mappings, destination directories, deployment modes, MCP registration mode/location, and aliases before enabling a target. The command is read-only.

For a native-only rollout, require zero reported loss:

```powershell
$plan = agentstack hub plan-sync --target codex-project --resource review-skill --deny-loss | ConvertFrom-Json
agentstack hub apply-sync --plan-id $plan.id --digest $plan.digest --yes
```

Without `--deny-loss`, inspect `$plan.lossReport` and every operation's `fidelity` and `losses`. A `partial` report means preserved but not target-normalized content; `lossy` means fallback or omission; `blocked` is never executable. Apply rejects adapter-version/capability drift and altered operation-level loss evidence after review.

MCP client plans carry the same capability and loss snapshots automatically:

```powershell
$plan = agentstack mcp clients plan --root C:\src\project --client codex --client claude --client cursor | ConvertFrom-Json
$plan.capabilitySnapshots
$plan.lossReports
agentstack mcp clients apply --root C:\src\project --plan-id $plan.id --digest $plan.digest --yes
```

Adapters do not write target files or invoke client commands. Resource Hub and MCP client linking retain inspection, confirmation, mutation, rollback, and recovery authority.

## External adapter differential conformance

Use only a local executable whose exact bytes and provenance you have independently reviewed. Compute and pin its digest before invocation:

```powershell
$adapter = (Resolve-Path .\my-adapter.exe).Path
$sha = (Get-FileHash -Algorithm SHA256 $adapter).Hash.ToLowerInvariant()
$report = agentstack hub adapter-external-conformance `
  --executable $adapter `
  --sha256 "sha256:$sha" `
  --target codex | ConvertFrom-Json
$report.summary
$report.intersection.changes
$report.mismatches
```

Use repeated `--arg` flags only for fixed executable arguments; ASM preserves exact boundaries and never invokes a shell. The default deadline is five seconds per protocol operation and may be reduced or increased to at most 30 seconds with `--timeout`.

Review all three evidence layers:

1. `descriptor` binds the staged executable bytes, size, exact argument vector, protocol, target, adapter identity, version, and required operations;
2. `intersection` shows raw candidate claims removed or weakened by the reviewed built-in ceiling;
3. `reference`, `candidate`, and `mismatches` show the complete Phase 5 differential evidence.

A successful command is compatibility evidence only. Do not copy the executable into ASM state, point Resource Hub or MCP plans at it, or treat it as trusted production code. Phase 6 has no external-adapter registry or activation path. The host does not provide network namespaces, filesystem namespaces, syscall filtering, CPU/memory quotas, publisher signatures, Windows Job Objects, or WASI isolation. Run only explicitly reviewed local candidates in a separately controlled operating-system account or VM when stronger isolation is required.
