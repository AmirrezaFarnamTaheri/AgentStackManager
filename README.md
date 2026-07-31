# AgentStack Manager

AgentStack Manager is a preservation-first Windows control plane for local engineering tools, MCP servers, Codex, and Antigravity/AGY. It discovers what is already present, produces a sealed reviewable plan, applies only that exact plan, records what it owns, and keeps unrelated software and configuration out of scope.

## Safety properties

- Existing commands, WinGet packages, global npm packages, uv tools, skills, and foreign MCP entries are adopted or preserved rather than replaced.
- A plan is bound to its catalog revision, minimized machine inventory, expiry, ID, and SHA-256 digest. `apply` accepts only that reviewed identity.
- Mutations use a cross-process lease and incremental transaction journal.
- Successful package-manager exit is followed by a component-specific postcondition check.
- Credential integrations require explicit selection and never store provider credentials.
- AgentStack-owned resources can be previewed, deactivated, removed, quarantined, backed up, restored, exported, or cleared without treating unrelated software as owned.
- The managed MCP router negotiates protocol versions, bounds output, performs live health probes, and reuses healthy child processes.
- Public release automation is fail-closed on a clean signed tag, supported Go toolchain, vulnerability scans, reproducibility checks, Authenticode verification, SBOM/license/VEX evidence, and Windows runtime gates.

## Windows installation

A public release contains an Authenticode-signed `AgentStack-Setup.exe`, its matching signed console binary, an internal checksum manifest, SBOMs, VEX, license inventory, and documentation. Extract the entire architecture-specific ZIP and double-click `AgentStack-Setup.exe`.

The setup verifies the embedded console digest and expected publisher certificate before installing:

```text
%LOCALAPPDATA%\Programs\AgentStack\bin\agentstack.exe
```

Unsigned development builds are intentionally not accepted as public installers.

## Safe first run

```powershell
agentstack inventory
$plan = agentstack plan --profile core | ConvertFrom-Json
$plan.actions | Format-Table
agentstack apply --plan-id $plan.id --digest $plan.digest --yes
agentstack mcp doctor
agentstack codex --profile core --
```

Changing the selected profile, component set, provider, catalog, or relevant machine inventory invalidates the plan. Build and review a new plan instead of reusing an old identity.

## Profiles

`core` is the minimal default. The compatibility profile `essential` retains the original broad 0.1 baseline but is not recommended as the starting point. Other focused profiles are `recommended`, `web-development`, `security`, `architecture`, `documentation`, `python`, `full-local`, and `custom`.

Run `agentstack profiles` or `agentstack help` for the authoritative embedded catalog.

## Documentation

- [User guide](docs/USER_GUIDE.md)
- [CLI reference](docs/CLI_REFERENCE.md)
- [Architecture](docs/architecture.md)
- [Security model](docs/SECURITY.md)
- [Privacy and data handling](docs/PRIVACY.md)
- [Operations and recovery](docs/OPERATIONS.md)
- [Supply-chain assurance](docs/SUPPLY_CHAIN.md)
- [Threat model](docs/THREAT_MODEL.md)
- [Repository governance](docs/GOVERNANCE.md)
- [Release process](docs/RELEASE.md)
- [Audit closure ledger](docs/audit/ASM-001-040-closure.md)
