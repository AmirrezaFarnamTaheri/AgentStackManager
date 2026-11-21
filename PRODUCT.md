# Product

<!-- impeccable:product-schema 1 -->

## Platform

windows

## Users

- **Inferred from repository evidence:** Windows developers and technical operators configuring local AI engineering tools, MCP servers, Codex, and Antigravity/AGY.
- **Inferred secondary audience:** Security-conscious maintainers reviewing exactly what AgentStack will preserve, install, repair, configure, or leave inactive before authorizing mutation.

## Product Purpose

AgentStack Manager is a local, preservation-first control plane for inspecting an existing developer workstation, selecting a capability profile, building a reviewed change plan, applying only the reviewed actions, and preparing a managed MCP routing surface. Success means users can improve their local agent stack without losing unrelated tools, configuration, or credentials.

## Positioning

AgentStack is differentiated by a review-before-mutation mechanism: it inventories first, binds a plan to machine state and a digest, preserves foreign resources, and mutates only the exact AgentStack-owned or user-authorized scope.

## Operating Context

- Runs locally on Windows and exposes a loopback-only browser manager plus CLI workflows.
- Primary browser workflow: read Home, inspect Environments, select and review exact Changes, authorize once, then follow installation and recovery in Activity. The CLI retains explicit plan ID/digest terminology.
- Long-running mutations return durable operation receipts and remain observable through status polling.
- Users may move between browser UI, PowerShell, Codex, and AGY during setup and diagnosis.

## Capabilities and Constraints

- Existing installations and foreign MCP registrations must be adopted or preserved rather than replaced.
- Credential integrations and runtime upgrades require explicit opt-in.
- The UI must preserve existing route IDs, field IDs, backend endpoint contracts, and operation-lock behavior.
- The web surface is embedded static HTML, CSS, and JavaScript served by Go. No frontend package manager or framework is present.
- The interface must remain usable without external assets, network fonts, or third-party runtime dependencies.
- **Open decision inferred from absent user interview:** the lifecycle workspace may simplify terminology and layout, but it cannot weaken reviewed-plan identity, authorization, safety claims, or destructive-action confirmation.

## Brand Commitments

- Product name: AgentStack Manager.
- Voice: precise, calm, operational, and explicit about scope and safety.
- Core promise: preserve what exists, show exact changes, require deliberate authorization.
- **Inferred visual commitment:** security and operational trust should feel engineered rather than promotional.

## Evidence on Hand

- Product and safety claims in `README.md` and `docs/`.
- Existing browser UI in `internal/ui/web/`.
- UI behavior contracts in `internal/ui/web_contract_test.go` and backend routes in `internal/ui/server.go`.
- No approved customer logos, testimonials, marketing metrics, photography, or external brand assets are present. Future design work must not fabricate them.

## Product Principles

1. Preserve before changing.
2. Make state and scope visible before authorization.
3. Keep safety language concrete and actionable.
4. Support fast expert operation without abandoning first-time clarity.
5. Treat local privacy, credentials, and recovery as first-class product behavior.

## Accessibility & Inclusion

- Preserve semantic landmarks, skip navigation, keyboard access, visible focus, live operation status, and reduced-motion behavior already represented in the UI.
- Target WCAG 2.2 AA contrast and 44px minimum touch targets for primary mobile controls.
