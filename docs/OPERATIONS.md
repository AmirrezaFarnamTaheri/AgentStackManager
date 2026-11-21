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


## Desktop host and backend process privacy

The default `agentstack ui` command starts the authenticated local service and one dedicated desktop application window. Closing the window cancels the server context and terminates the managed desktop process tree. Browser mode is opt-in through `ui --browser`; `ui --no-open` is intended for diagnostics and prints the loopback URL.

Installer and synchronization subprocesses run with bounded output, timeouts, process-tree containment, and hidden Windows child windows. Raw command lines, arguments, environment variables, stdout, stderr, npm progress, and shell escape sequences remain backend evidence. Public transaction and operation responses expose only sanitized method, category, normalized error code, observed evidence, root cause, repair action, and retryability.

## Parallel installation and synchronization

The tool runner schedules independent catalog actions concurrently while respecting component dependencies, global concurrency, and per-installer limits. Skill publication, configuration mutation, and writes to shared roots remain serialized. Cancellation stops new scheduling and terminates active process trees. Transaction actions retain deterministic plan order even when execution overlaps.

Multi-target resource synchronization uses a parent reviewed plan with digest-bound child plans. Apply revalidates target registrations, capabilities, roots, child digests, and expiration. Independent roots run in parallel; matching canonical roots are locked. One target failure does not roll back or misreport successfully verified independent targets. Every child emits a receipt, and rerunning against the verified desired state produces no changes.

## Failed installation

- A failed prerequisite suppresses dependent actions.
- The transaction journal records the action before and after execution.
- A successful package-manager process must also pass its catalog postcondition.
- Managed router configuration is not rewritten after a failed install phase.
- Already-completed third-party package installs are reported for manual package-manager recovery; AgentStack does not destructively guess at rollback.

## Operation outcome and recovery

Phase and outcome are independent. A long-running apply can be in preparing, installing, configuring, verifying, or finished phase. Its terminal outcome is `succeeded`, `partially_failed`, `failed`, or `cancelled`. The public result reports requested, processed, succeeded, failed, skipped, and unchanged counts. A finished operation with zero requested successes and one or more failures is a failed outcome, never a completed-success state.

The manager groups matching failures into a root-cause summary. Per-item sanitized diagnostics may include action, result, category, method, exit code, retryability, and recommended action. They never include commands, arguments, private paths, stdout, or stderr.

**Retry failed items** never reuses a consumed reviewed plan. It refreshes current inventory, reselects failed items plus their recursive dependencies, and creates a fresh reviewed plan for inspection and approval. Existing verified items are left unchanged and are counted separately from requested successes.

Before the first installer command, the Windows runner refreshes the effective process PATH so a desktop-launched manager can see package managers installed after login. If a package-manager executable is unavailable, repeated actions using that same method are collapsed under the shared installer prerequisite instead of spawning the same doomed command for every item.

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

## Unified fabric operations

Use the sequence `inspect -> plan -> review -> apply -> verify` for context refresh, resource synchronization/source refresh, and MCP client linking. Keep the returned plan ID and digest together; changes to source, registry, project fingerprint, destination, or client configuration invalidate apply.

Run `agentstack routine history` after scheduled work. The latest durable receipt repairs stale schedule fields after an interrupted state write. Receipt history is capped at 4,096 entries.

MCP-link plans do not contain rewritten client configuration. Apply re-reads and revalidates the live file, reconstructs the reviewed AgentStack-only change, and emits a minimal registration-only recovery record. A duplicate-key document or secret-bearing same-name entry blocks the operation.

Resource replacement and removal create recoverable backups. List them with `agentstack hub backups`; restore with the exact backup ID and `--yes`. Target sync preserves foreign files and prunes only paths recorded as AgentStack-managed.

Legacy workspace, memory, artifact, and routine stores migrate on the next successful mutation. Back up the AgentStack data root before a release upgrade when local state is operationally critical.

Workspace multi-file commits publish an active transaction pointer and snapshot journal. Live readers are fenced while the transaction is fresh; stale interrupted transactions restore their snapshots. Cleanup after a committed artifact mutation is non-authoritative and may be deferred without converting a successful commit into a failed operation.

## SQLite shadow metadata operations

Use `agentstack hub db-stage` to build or advance the rebuildable metadata index, `db-inspect` for database-only integrity checks, and `db-verify` to include current Resource Hub and CAS verification. `db-backup --destination PATH --yes` creates a verified no-overwrite online backup. Do not replace the active database with a backup in this phase; recovery is to preserve the suspect file, verify Resource Hub/CAS, and rebuild a new shadow database with `db-stage`.

Database commands require a CGO build with SQLite 3.37 or newer. A release that enables this preview must prove the native SQLite toolchain on each supported platform; CGO-disabled binaries report the feature unavailable while all established ASM operations continue to function.
