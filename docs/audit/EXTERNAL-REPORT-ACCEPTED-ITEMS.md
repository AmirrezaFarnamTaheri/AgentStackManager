# External report claim disposition and accepted implementation set

This ledger records the external-report claims that survived independent validation, the claims revised into narrower useful improvements, and the claims that remain rejected. It is separate from the historical ASM-001–ASM-040 closure ledger.

| Claim ID | Disposition | Implemented control | Verification |
|---|---|---|---|
| R1-DUP-PROC | Accepted — verified maintainability defect | `processctl.IsAlive` is the sole OS process-liveness implementation; duplicate `state/process_alive_*` files were removed | unit/race tests; Windows test-package cross-compilation |
| R1-ACL | Accepted after revision — permission continuity, not proven exposure | `safefile.Replace` captures destination permission metadata, applies it to the staged replacement, and preserves POSIX mode or Windows DACL continuity | POSIX mode regression plus native Windows explicit-DACL replacement test |
| R1-PATH | Accepted after narrowing | `internal/pathenv` owns Windows PATH comparison/merge semantics; self-install preserves unrelated persistent PATH bytes, transports non-ASCII values through UTF-16LE/Base64, and appends only AgentStack | path normalization, idempotence, duplicate-preservation, Unicode round-trip, runner, self-install, and native Windows preservation tests |
| R2-RESOURCE | Promoted enhancement | catalog-managed MCP children start suspended and resume only after Windows Job Objects enforce explicit memory, CPU-rate, and active-process ceilings | catalog/config tests, Windows Job Object structure test, native runnable limited-job test |
| R3-PGID | Accepted after revision — narrow Unix race hardening | termination records PID/PGID identity, records a Linux kernel start-time token when available, refuses mismatches or PID reuse, probes a surviving group after leader exit, and then signals only the recorded group | injected identity-change, leader-exit, process-tree, timeout, and race tests |
| R3-BUSY | Promoted UX enhancement | one shared busy-state controller, operation live region, main/button `aria-busy`, control locking, focus preservation, reduced motion, and coherent Lucide navigation icons | embedded UI contract tests, JavaScript syntax check, Playwright/axe operation-feedback gate |
| RELEASE-EVIDENCE | Retained as a release gate, not claimed complete | existing signed-tag workflow must execute native x64/ARM64, Authenticode, SBOM/VEX/provenance, accessibility, and artifact checks | protected GitHub environment evidence required before production release |

## Claims not promoted

The WebSocket/CORS, Zip Slip, missing planner-cycle detection, unsynchronized MCP capability map, unbounded stdio, POSIX signature-success stub, fabricated throughput, fabricated coverage, and fabricated AI-slop measurements remain rejected because they do not describe the authoritative source.

## Release boundary

All source-level accepted items are implemented. Windows-specific DACL and hard-limit behavior is **release-gated** until the required native x64 and ARM64 jobs execute successfully against the candidate tag. Cross-compilation is evidence of build compatibility, not runtime proof.
