# Operations and Recovery

## Health checks

```text
agentstack status
agentstack inventory
agentstack mcp doctor
agentstack diagnostics --out agentstack-diagnostics.zip
```

`status` summarizes catalog and inventory state. `mcp doctor` performs protocol-level probes. Diagnostics contain sanitized status, recent events, catalog metadata, minimized inventory, and state summaries.

## Correlation and events

Mutating operations and MCP child activity emit local JSONL events with timestamps, type, level, correlation ID, server/component digest, duration, and status. Logs rotate at a bounded size and expire under the privacy retention policy.

## Failed installation

- A failed prerequisite suppresses dependent actions.
- The transaction journal records the action before and after execution.
- A successful package-manager process must also pass its catalog postcondition.
- Managed router configuration is not rewritten after a failed install phase.
- Already-completed third-party package installs are reported for manual package-manager recovery; AgentStack does not destructively guess at rollback.

## Backup restore

Always preview first:

```text
agentstack backup
agentstack backup restore --id ID --preview
agentstack backup restore --id ID --yes
```

Restore refuses digest mismatch, target substitution, unknown backup identity, or structurally invalid content. Router restoration must also pass live MCP validation.

## Concurrent operation

One mutation lease is allowed per user data root. Another process receives a locked/busy error. Lease ownership is PID/instance-aware and stale leases are recovered conservatively.

## Client registration conflicts

AgentStack repairs only an entry it can prove is its own. A foreign `agentstack-router` conflict, malformed configuration, or failed Codex lookup stops the operation and preserves the original data. AGY modifications are backed up before writing.

## Incident response

1. Stop active AgentStack sessions.
2. Preserve the transaction ID and relevant backup ID.
3. Run `agentstack diagnostics`.
4. Preview an indexed restore if a managed file is affected.
5. Use provider/package-manager native commands for third-party package rollback.
6. Clear operational data only after retaining required incident evidence.

## Long-running UI mutations

The loopback UI never holds an HTTP response open for installation or MCP initialization.
`POST /apply`, `POST /mcp/init`, and `POST /install-self` return `202 Accepted` with an
operation ID and token-protected status URL. The browser polls that URL until the operation
is `succeeded` or `failed`. The backend operation uses the manager process context rather
than the request context, so a client disconnect or the server's 60-second write deadline
cannot turn a completed mutation into an apparent request failure. Shutdown remains locked
while a mutation is active.
