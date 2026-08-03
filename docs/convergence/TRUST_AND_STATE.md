# Convergence Trust and State Model

## Authority map

| Plane | Reads without confirmation | Writes requiring confirmation/review | Source of truth |
| --- | --- | --- | --- |
| Resource hub | list, audit, source drift inspection | import replacement, sync, refresh, remove, restore | versioned registry plus canonical resource content |
| Project context | scan, score, read, search, Git context | context refresh apply | project tree plus managed blocks |
| Workspace | list, get, recall, search, render, verify | create/update, remember, forget, artifact add/remove, delete | versioned workspace/memory/artifact stores |
| MCP linking | inspect and create plan | link/unlink apply | client config plus AgentStack-owned router entry |
| Routines | list, due, history | put/delete and confirmed run | versioned routine store plus run receipts |
| UI | authenticated status and operation receipts | delegates to the same service confirmation gates | never authoritative |

## State machines

### Resource synchronization

`imported -> audited -> planned -> confirmed -> applied`

Forbidden transitions:

- remote result directly to imported;
- blocked audit to plan without explicit `allow-risk` in the reviewed plan;
- plan to apply after expiry, registry drift, source drift, target drift, or digest mismatch;
- foreign destination to update/remove;
- import alone to target activation.

Recovery: replacement/removal backups and managed destination state permit confirmed restoration or safe pruning. Failed registry commits restore prior canonical content.

Admission is bounded to 10,000 files, 16 MiB per file, and 64 MiB total per resource. Static audit uses independent 10,000-file, 1 MiB-per-file, and 64 MiB-total ceilings. Oversized trees fail before canonical or target authority changes.

### Context refresh

`scanned -> scored/inspected -> planned -> confirmed -> applied`

The plan binds canonical project root, project fingerprint, target paths, before digests, after digests, and expiry. A changed project or target file invalidates apply. Managed blocks preserve surrounding user text.

### Memory

`constructed -> validated(scope,size) -> persisted(digest) -> recalled/searched -> expired/forgotten`

Project, workspace, and session layers require explicit scope. Recall order is session, workspace, project, user. Expired or digest-mismatched entries cannot be returned.

### MCP client link

`inspected -> planned(link|unlink) -> confirmed -> revalidated -> applied`

A foreign `agentstack-router` entry is a conflict. Secret-bearing or structurally ambiguous same-name entries are foreign. Unlink removes only the recognized AgentStack entry. Codex changes flow through the reviewed registration code. Plans retain only intent and digests; apply reconstructs the desired file from the revalidated live configuration. Recovery records contain only the previous AgentStack registration or its absence, never the complete client configuration.

### Routine execution

`defined -> scheduled/due -> confirmed -> running -> succeeded|failed -> receipt persisted -> schedule advanced`

Steps execute in order and stop on first failure. The receipt is redacted and versioned. On restart, the newest receipt reconciles stale `LastRun` and `NextRun` fields. A run cannot start without confirmation.
Routine definitions reject secret-bearing positional arguments and assignments. The complete run is bounded to 24 hours in addition to per-step deadlines.

## Trust boundaries

### Filesystem

- All project reads remain under a canonical root.
- Symlink escapes, traversal, oversized files, unsupported types, and unsafe archive paths fail closed.
- Canonical fabric stores live under the private ASM data root.
- Destination writes are limited to registered targets or explicit client config paths.
- Workspace multi-file commits use a durable active pointer and snapshot journal. Readers fail closed during a live transaction and restore a stale interrupted transaction after the recovery horizon.

### External processes

- Git inspection and routine commands use `runner.CommandRunner`.
- Direct binary plus argument arrays replace shell strings.
- Deadlines and output limits are mandatory.
- MCP child processes remain under the managed runtime and OS-specific supervision.

### Remote systems

- Donor marketplace search, hard-coded APIs, and credentialed connectors do not obtain local write authority.
- Operators may provide a reviewed local checkout or an explicit bounded adapter.
- Secrets remain in provider/OS mechanisms and cross the shared redaction boundary before durable diagnostic evidence.

### Client configuration

- Existing unrelated MCP entries are preserved.
- Same-name foreign entries block mutation.
- Before-state digests are rechecked at apply.
- Duplicate-key documents fail closed. Full client files and unrelated environment values are never copied into plan or backup state.
- UI discovery is advisory; files and registration state remain authoritative.

## Persistence schemas

| Store | Schema | Version |
| --- | --- | ---: |
| workspaces | `agentstack.workspace.items` | 1 |
| memory | `agentstack.workspace.memory` | 1 |
| artifacts | `agentstack.workspace.artifacts` | 1 |
| routines | `agentstack.routines` | 1 |
| routine receipt | `agentstack.routine-run` | 1 |
| resource registry | resource registry version | 1 |
| resource target state | managed state version | 1 |

Legacy raw JSON maps load through compatibility readers. The next mutation writes an envelope. Unknown future versions are rejected.
