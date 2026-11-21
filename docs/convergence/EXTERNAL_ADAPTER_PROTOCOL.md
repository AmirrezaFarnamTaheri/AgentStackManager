# External Adapter Protocol Reference

## Status

The Phase 6 external adapter protocol is an experimental, read-only conformance boundary. It permits ASM to execute a locally supplied, SHA-256-pinned adapter candidate and compare it with one reviewed built-in adapter. It does not install, register, activate, or authorize the candidate.

## Transport

An adapter executable must implement one request per process over standard input and standard output:

1. read one JSON request;
2. validate the envelope and deadline;
3. perform the named pure adapter operation;
4. write one JSON response;
5. exit successfully without writing diagnostics to standard error.

ASM launches a new process for every operation. There is no persistent protocol session and no shell intermediary.

## Request envelope

```json
{
  "apiVersion": "fabric.asm.dev/external-adapter/v1alpha1",
  "requestId": "opaque-host-identity",
  "operation": "handshake|capabilities|discover|import|render|plan|verify",
  "adapterContract": "fabric.asm.dev/adapter/v1alpha1",
  "deadline": "RFC3339 timestamp",
  "payload": {}
}
```

The request ID, operation, adapter contract, and deadline are mandatory. Payload schemas are the corresponding Go adapter-contract request types encoded as strict JSON.

## Response envelope

Successful response:

```json
{
  "apiVersion": "fabric.asm.dev/external-adapter/v1alpha1",
  "requestId": "same request identity",
  "operation": "same operation",
  "result": {}
}
```

Failed response:

```json
{
  "apiVersion": "fabric.asm.dev/external-adapter/v1alpha1",
  "requestId": "same request identity",
  "operation": "same operation",
  "error": {
    "code": "invalid-request|unsupported-operation|adapter-error|internal-error",
    "message": "bounded diagnostic"
  }
}
```

A response must contain exactly one of `result` or `error`. ASM rejects request/operation identity drift, unknown error codes, malformed or trailing JSON, oversized output, stderr on success, non-zero exit, or missed deadline.

## Handshake

The host begins with:

```json
{
  "supportedProtocols": ["fabric.asm.dev/external-adapter/v1alpha1"],
  "adapterContract": "fabric.asm.dev/adapter/v1alpha1",
  "target": "canonical target",
  "environment": {
    "projectRoot": "synthetic absolute path",
    "targetRoot": "synthetic absolute path",
    "homeDir": "synthetic absolute path",
    "agyConfig": "synthetic absolute path"
  }
}
```

The adapter returns:

```json
{
  "protocolVersion": "fabric.asm.dev/external-adapter/v1alpha1",
  "adapterContract": "fabric.asm.dev/adapter/v1alpha1",
  "target": "same canonical target",
  "adapterId": "same reviewed semantic adapter identity",
  "adapterVersion": "same reviewed implementation version",
  "aliases": [],
  "operations": ["capabilities", "discover", "import", "plan", "render", "verify"]
}
```

The external executable is expected to emulate one reviewed built-in target. A new semantic adapter ID or version is not admitted by Phase 6.

## Operation result payloads

| Operation | Result wrapper |
| --- | --- |
| `capabilities` | `{ "capability": CapabilitySet }` |
| `discover` | `{ "artifacts": [ObservedArtifact...] }` |
| `import` | `{ "artifact": Artifact, "lossReport": LossReport }` |
| `render` | `{ "rendered": RenderedSet, "lossReport": LossReport }` |
| `plan` | `{ "operations": [ProposedOperation...] }` |
| `verify` | `{ "verification": VerificationResult }` |

The canonical request/result structures are defined by `internal/adapters`. Implementations must preserve their schema identities, digests, ordering, path confinement, loss identities, and closed lifecycle semantics.

## Go reference server

A Go executable can use `external.ServeOne` as its process entry point:

```go
func main() {
    adapter := buildPureAdapter()
    if err := external.ServeOne(context.Background(), adapter, os.Stdin, os.Stdout); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(2)
    }
}
```

`ServeOne` is a protocol reference and test helper. It does not provide admission, process containment, package trust, or activation. Those remain host responsibilities.

## Capability ceiling

The raw candidate capability must match the reviewed target's semantic identity and structural locations. ASM computes an effective capability by intersecting the candidate with the built-in ceiling. External-only claims are removed; weaker candidate support remains weaker; structural destination or MCP-registration divergence is rejected.

The sealed intersection report identifies:

- raw capability digest;
- reviewed ceiling digest;
- effective capability digest;
- every `restricted` or `candidate-limited` path.

## Execution environment

ASM provides a deliberately small environment:

- `ASM_EXTERNAL_ADAPTER=1`;
- `ASM_EXTERNAL_ADAPTER_PROTOCOL=<protocol>`;
- private `HOME` and temporary paths;
- deterministic C locale on non-Windows;
- only `SystemRoot`/`WINDIR` additionally retained on Windows when present.

No parent application secrets or arbitrary environment variables are copied. The working directory is private and contains only the staged adapter executable and temporary work area.

This environment reduction is not complete OS sandboxing. The operator may request hard memory, CPU, and active-process ceilings. Windows enforces them with a Job Object; Linux requires a writable delegated cgroup v2 hierarchy and atomically places the child with `UseCgroupFD`; unsupported hosts fail closed when limits are requested. Network and filesystem access remain outside this boundary. Adapter authors must not rely on ambient credentials or unrestricted host access. A future WASI or stronger OS-sandbox backend is still required before activation.

## Operator invocation

PowerShell example:

```powershell
$adapter = (Resolve-Path .\my-adapter.exe).Path
$sha = (Get-FileHash -Algorithm SHA256 $adapter).Hash.ToLowerInvariant()
agentstack hub adapter-external-conformance `
  --executable $adapter `
  --sha256 "sha256:$sha" `
  --target codex `
  --memory-bytes 536870912 `
  --cpu-percent 50 `
  --max-processes 8
```

A failed candidate still produces a complete sealed report when the protocol remains usable. Admission, crash, deadline, malformed-response, or other host failures return an error without activation.

## Security and promotion rule

Do not treat a passing Phase 6 report as permission to use an executable for production mutation. External adapter activation remains blocked until ASM has, at minimum:

- publisher/package identity and signature verification;
- a durable lockfile and revocation model;
- a true WASI or OS-level sandbox with network, filesystem, process, CPU, and memory controls;
- protected native platform tests;
- plan-bound adapter descriptor identity;
- explicit promotion and rollback procedures.
