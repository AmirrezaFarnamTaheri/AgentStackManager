# AgentStack Manager Implementation Plan

> **For agentic workers:** Implement task-by-task with red/green verification and review each boundary before continuing.

**Goal:** Build a self-contained Windows installer/manager that preserves all existing tools, installs missing essentials, offers optional and credentialed integrations, deduplicates only the active managed profile, and initializes MCP services before launching Codex or AGY.

**Architecture:** A single Go executable owns discovery, planning, transactions, MCP routing, session preparation, and a local browser-based GUI. The component catalog is embedded JSON and can also be overridden from disk. Windows-specific self-install and PATH behavior is isolated behind platform files; all planning and configuration behavior is cross-platform and unit tested.

**Tech Stack:** Go 1.23 (language) / Go 1.26.5 release toolchain, embedded HTML/CSS/JavaScript, PowerShell build helpers, JSON catalogs, Windows `winget`, npm, uv/uvx, Codex CLI, AGY/Gemini JSON MCP configuration.

## Global Constraints

- Never uninstall or upgrade pre-existing third-party software automatically.
- Never overwrite an existing skill, MCP entry, agent constitution, or unrelated configuration.
- Back up a config immediately before a managed write.
- Credential-dependent components require explicit selection and never trigger login silently.
- Duplicate providers are preserved on disk and only deactivated in AgentStack-managed profiles.
- Default installation is per-user and appends one AgentStack directory to user PATH.
- `agentstack codex` and `agentstack agy` must initialize and validate MCP requirements before launching.

---

### Task 1: Catalog and typed domain model
- Add component, profile, install, router, inventory, and plan types.
- Embed and validate the default catalog.
- Tests: reject duplicate component IDs and profile references to unknown components.

### Task 2: Preservation-first inventory and planning
- Detect commands without changing the machine.
- Compute keep/install/preserve-inactive/consent-required/skip actions.
- Tests: installed tools are kept; dominated installed tools are preserved inactive; credential components require explicit consent.

### Task 3: Transactional runner and state store
- Execute only install actions through an injected command runner.
- Persist transaction records and inventory snapshots atomically.
- Tests: dry run executes nothing; failures are recorded; unrelated actions continue only when safe.

### Task 4: Managed MCP router
- Implement MCP JSON-RPC over stdio.
- Expose list-servers, list-tools, call-tool, and doctor tools.
- Spawn children lazily and forward initialize/tools requests.
- Tests: initialization, tool listing, malformed requests, and child forwarding with a fake MCP child.

### Task 5: MCP config and client registration
- Generate a managed router config from the active profile.
- Add one non-conflicting Codex entry through `codex mcp`.
- Merge one AGY entry into existing JSON with backup and no replacement.
- Tests: merge preserves unknown keys and existing entries; conflict returns a non-mutating result.

### Task 6: Session preparation and launch
- Doctor required commands, warm selected child packages, initialize router config, then launch Codex or AGY with forwarded arguments.
- Tests: launch is blocked on failed required checks and arguments are preserved.

### Task 7: Self-install and PATH
- Copy the running executable into the per-user AgentStack directory.
- Append the bin directory to user PATH only when absent.
- Provide preview-only cleanup and self-install behavior that never removes user data.
- Tests: path append is idempotent and preserves all existing segments.

### Task 8: Embedded manager UI
- Serve a loopback-only HTTP application with inventory, catalog, plan, apply, doctor, and session endpoints.
- Provide essential/recommended/local/credential selections and clear preservation outcomes.
- Tests: API status codes, loopback binding, plan request validation, and no apply without explicit confirmation.

### Task 9: CLI integration
- Implement `ui`, `status`, `inventory`, `plan`, `apply`, `mcp init`, `mcp doctor`, `mcp-router`, `codex`, `agy`, `install-self`, `backup`, and `cleanup --preview`.
- Tests: parser routing and safe defaults.

### Task 10: Build and release bundle
- Build Linux verification binary and Windows amd64/arm64 binaries.
- Add PowerShell installer launcher and checksum manifest.
- Run unit tests, vet, static checks, smoke tests, and archive the source/release bundle.
