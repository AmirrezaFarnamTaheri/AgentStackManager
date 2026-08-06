# ASM Fabric Phase 6: Constrained External Adapter Protocol

## Evidence basis

Phase 6 implements the next dependency-ordered slice after the Phase 5 in-process conformance corpus:

- a versioned stdin/stdout protocol for separately compiled adapter executables;
- executable admission pinned to exact SHA-256 bytes;
- one fresh child process per request;
- strict schema and operation negotiation;
- per-request deadlines and bounded request, response, diagnostic, executable, and argument sizes;
- capability intersection against a reviewed built-in adapter ceiling;
- crash, timeout, malformed-response, path-escape, state-machine-divergence, and capability-drift isolation;
- differential execution against the embedded Phase 5 corpus;
- a sealed read-only operator report;
- no registration, activation, target mutation, plan authorization, or authority transfer.

The reports recommend out-of-process or WASI adapter execution, explicit capability negotiation, visible loss reporting, restricted ambient authority, signed or pinned adapter identity, and conformance gating before promotion. Phase 6 implements the bounded process and evidence layer in target-native Go. It does not claim that a normal operating-system child process is equivalent to a kernel, container, or WASI sandbox.

## Converged decision

The host follows this closed path:

```text
absolute executable path + pinned sha256 + fixed argv
  -> lstat/open identity check
  -> bounded copy into private 0700 session directory
  -> staged-byte digest verification
  -> one-request child process in private cwd and synthetic environment
  -> protocol/contract/target/operation negotiation
  -> raw external capability snapshot
  -> intersection with reviewed built-in capability ceiling
  -> Phase 5 reference and candidate conformance runs
  -> per-case evidence comparison
  -> sealed external conformance report
  -> process and session teardown
```

The external executable is an evidence-producing codec candidate. It never becomes a Resource Hub or `mcplink` adapter registration through this phase.

## Protocol identity

The wire protocol is:

- protocol: `fabric.asm.dev/external-adapter/v1alpha1`;
- adapter contract: `fabric.asm.dev/adapter/v1alpha1`;
- descriptor: `fabric.asm.dev/external-adapter-descriptor/v1alpha1`;
- capability intersection report: `fabric.asm.dev/external-adapter-intersection/v1alpha1`;
- differential report: `fabric.asm.dev/external-adapter-conformance-report/v1alpha1`.

Each invocation receives exactly one strict JSON request and must return exactly one strict JSON response. Required operations are:

- `capabilities`;
- `discover`;
- `import`;
- `render`;
- `plan`;
- `verify`.

A handshake must select the exact supported protocol and adapter contract, identify the requested canonical target, and advertise all required operations. Every later response must echo the request ID and operation. Unknown fields, malformed envelopes, missing results, invalid error codes, identity mismatches, and trailing JSON fail closed.

## Admission and process bounds

Default and maximum host bounds are:

| Boundary | Default | Maximum accepted by this phase |
| --- | ---: | ---: |
| Per-request deadline | 5 seconds | 30 seconds |
| Request bytes | 1 MiB | 1 MiB |
| Response bytes | 1 MiB | 1 MiB |
| Standard error bytes | 64 KiB | 64 KiB |
| Executable bytes | 128 MiB | 128 MiB |
| Fixed arguments | up to 16 | 16 |
| One argument | 4 KiB | 4 KiB |
| All fixed arguments | 16 KiB | 16 KiB |

Admission requires:

- an absolute executable path;
- an exact lowercase `sha256:<64 hexadecimal characters>` digest;
- a regular non-symlink source file;
- executable mode on non-Windows systems;
- stable file identity between path inspection and opened-file inspection;
- complete bounded copying into a generated private session directory;
- a second digest check over the staged bytes;
- a deterministic digest over the exact argument vector.

The admitted descriptor records executable digest and size, argument digest, protocol, target, adapter identity/version, aliases, operations, and its own seal. It deliberately does not publish the original executable path.

## Invocation isolation

Every method call launches a new process. The host:

- never uses a shell;
- supplies exact argument boundaries;
- uses a private working directory;
- replaces the inherited environment with a small synthetic environment;
- redirects home and temporary paths into the private session directory;
- does not pass parent secrets or arbitrary environment variables;
- bounds stdout and stderr independently;
- cancels on deadline or output overflow;
- treats any stderr on an otherwise successful protocol response as failure;
- terminates the identity-checked process group on Unix and the assigned Job Object on Windows;
- removes the generated session directory when the admitted adapter closes.

This is process-level containment, not a complete security sandbox. Optional hard memory, CPU, and active-process ceilings are enforced by Windows Job Objects or delegated Linux cgroup v2 scopes; requests fail closed where those controllers are unavailable. The host still does not enforce network namespaces, syscall policy, filesystem namespaces, code signatures, or WASI capabilities. An unbounded invocation uses identity-checked Unix process-group cleanup. Consequently, Phase 6 remains a conformance and evidence path for explicitly supplied local executables, not an untrusted-plugin activation path.

## Capability intersection

An external adapter cannot expand the reviewed target surface. Its raw capability snapshot must use the same semantic adapter identity, implementation version, target, and target-version range as the reviewed built-in reference.

The effective capability is an intersection:

- aliases, deployment modes, scopes, transports, and fields are intersected;
- stronger support claims are reduced to the reviewed ceiling;
- external-only artifact kinds or modes are removed and reported as `restricted`;
- missing or weaker reviewed behavior is reported as `candidate-limited`;
- artifact directory/format or MCP registration-structure divergence is rejected rather than normalized;
- raw, ceiling, and effective capability digests are sealed into the intersection report.

Restrictions are evidence, not authorization. They do not modify the built-in reference and cannot grant the external process additional behavior.

## Differential conformance

`RunConformance` runs the exact Phase 5 corpus twice in one synthetic environment:

1. against the reviewed built-in adapter;
2. against the admitted external adapter through the constrained process host.

The sealed report includes:

- the admitted executable descriptor;
- the capability intersection report;
- the complete reviewed reference report;
- the complete candidate report;
- sorted per-case status/evidence mismatches;
- derived matched/mismatched/restriction counts;
- a final pass/fail value and report digest.

Passing requires both reports to pass and every candidate case to match the reference evidence digest. Capability restrictions remain visible even when all effective conformance cases match. Derived summary or mismatch tampering is rejected on verification.

## Operator surface

```text
agentstack hub adapter-external-conformance \
  --executable ABSOLUTE_PATH \
  --sha256 sha256:<hex> \
  --target TARGET \
  [--arg EXACT_ARGUMENT]... \
  [--timeout 5s]
```

The service resolves the target or alias to the reviewed built-in adapter, creates synthetic project/target/home/AGY paths, admits the pinned executable, runs the differential corpus, prints the full sealed report, and exits non-zero when the candidate fails.

The command never points the candidate at the user's actual project, home directory, target roots, Resource Hub, CAS, SQLite database, or MCP configuration. It cannot register the adapter or use it for a sync/apply operation.

## Defects converted into guardrails

Phase 6 testing established these fail-closed invariants:

1. source bytes are copied and rehashed before execution, so later source replacement does not alter the admitted session;
2. source symlinks and digest mismatches are rejected;
3. parent environment secrets are absent from the child environment;
4. deadline, stdout, stderr, crash, malformed response, request-ID mismatch, and diagnostic-on-success conditions terminate the invocation;
5. an external render path outside the synthetic target root cannot pass;
6. external plan transitions must exactly match the reviewed core state machine;
7. external verify results must exactly match core postcondition semantics;
8. capability changes during one admitted session are rejected;
9. report summaries, mismatch sets, descriptors, and capability intersections are independently resealed.

## Validation coverage

Tests cover:

- byte-pinned admission and protocol negotiation;
- private staging and cleanup;
- environment sanitization;
- full Phase 5 differential conformance;
- capability restriction and candidate limitation;
- digest mismatch and symlink rejection;
- deadline, output, crash, malformed-response, identity, and stderr failure paths;
- target-root escape and plan-state divergence;
- report tampering;
- capability drift inside one session;
- end-to-end read-only CLI execution.

The full conformance subprocess suite runs under normal execution. Race-instrumented subprocess startup is prohibitively expensive, so race validation exercises the host admission, process, intersection, drift, and CLI orchestration paths while the full 64-case differential suite remains covered by normal execution.

## Authority boundary

Phase 6 does not change any existing authority:

- Resource Hub remains the sole resource deployment authority;
- `mcplink` remains the sole MCP client registration authority;
- reviewed plans, confirmations, backups, rollback, and postcondition checks remain in core;
- the built-in adapters remain the reviewed capability ceilings;
- the external executable cannot persist a registration or become selectable during sync/apply;
- a passing report is compatibility evidence only.

## Explicit non-goals

Phase 6 does not:

- provide a kernel, container, seccomp, AppContainer, network/filesystem sandbox, or WASI runtime;
- block all network or filesystem access available to the current OS user;
- claim memory, CPU, or process quotas on a host that cannot enforce the requested controller;
- verify a publisher signature or package manifest;
- download, install, discover, or auto-update adapter executables;
- create a durable adapter registry or lockfile;
- activate an external adapter in Resource Hub or MCP linking;
- replace protected native CI or signed release provenance.

The next dependency-ordered slice is a signed/publisher-bound external adapter package and lockfile plus a true WASI or OS-sandbox execution backend. Only after those controls and protected native evidence exist should reviewed plans be allowed to reference an external adapter descriptor.
