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
