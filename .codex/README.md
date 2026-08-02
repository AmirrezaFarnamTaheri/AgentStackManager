# Codex MCP trust boundaries

All npm-backed MCP launch specifications in `config.toml` use exact versions so
repository behavior changes only through reviewed commits. Version updates must
be reviewed like dependency changes and validated with `codex mcp list`.

The Exa entry uses the pinned `mcp-remote` client to connect to
`https://mcp.exa.ai/mcp`. This is an external network trust boundary: prompts
and query context sent through that server leave the local machine and are
handled under Exa's service terms. Use it only for tasks that require external
search, and do not send secrets, credentials, private source, or personal data.

Supabase is configured read-only. GitHub and every other networked MCP remain
read-only by project policy unless the user explicitly authorizes a scoped
external mutation.
