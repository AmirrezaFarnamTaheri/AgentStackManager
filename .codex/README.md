# Codex MCP trust boundaries

All npm-backed MCP dependencies are exact and integrity-locked in
`package-lock.json`. Lifecycle scripts are disabled, and `config.toml` launches
only locally installed binaries in offline mode. Prepare or refresh the runtime
after checkout with:

```powershell
npm ci --prefix .codex --ignore-scripts
codex mcp list
```

Without that reviewed install step, MCP startup fails closed instead of
downloading executable code. Version updates must include the regenerated lock
file and receive the same review as application dependencies.

The Exa entry uses the locked `mcp-remote` client to connect to
`https://mcp.exa.ai/mcp`. This is an external network trust boundary: prompts
and query context sent through that server leave the local machine and are
handled under Exa's service terms. Use it only for tasks that require external
search, and do not send secrets, credentials, private source, or personal data.

Supabase is configured read-only. GitHub and every other networked MCP remain
read-only by project policy unless the user explicitly authorizes a scoped
external mutation.

The GitHub entry uses the same integrity-locked transport with GitHub's
official `/readonly` remote MCP endpoint. GitHub enforces that endpoint by
omitting write-capable tools even when a selected toolset contains them.
Authentication is completed by the transport's OAuth flow; no token is stored
in this repository. The retired npm GitHub MCP package is deliberately
excluded because it has no non-vulnerable release. A write-capable endpoint
must be a separate, user-scoped configuration and still requires explicit
authorization under the repository's external-action policy.
