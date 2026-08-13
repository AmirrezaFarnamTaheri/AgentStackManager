# AgentStack Manager full upgrade verification — 2026-08-06

## Scope

This verification covers the reliability, AI-app connection, failure-intelligence, operational-widget, and responsive UI upgrade requested after the 0.1.1 development build.

## Implemented behavior

- Embedded CSS and JavaScript use a content/version query token and HTTP `no-store` headers, preventing stale browser assets from retaining old result copy or layout rules.
- Windows UI and setup launches request UAC elevation before backend initialization. A private relaunch marker prevents elevation loops and explicit CLI commands remain available without forced elevation where appropriate.
- The Environments surface discovers supported AI-app configuration roots and registers or pauses Resource Hub targets through authenticated local API mutations.
- Failure output records method, category, normalized error code, observed evidence, affected tools, severity, and a concrete repair action. Unknown errors remain sanitized but identify the installer and code when available.
- Environment health, repair queue, history search/filtering, responsive result cards, brand rendering, and favicon behavior are included.
- WinGet catalog pins were refreshed for yq, Trivy, and scc; the scc identifier is `BenBoyter.scc`.

## Automated evidence

- `go test ./...`: passed.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- JavaScript syntax checks for every embedded UI module: passed.
- Documentation contracts: passed.
- Governance checks: passed.
- Windows amd64 cross-compilation: required before delivery and recorded in the delivery manifest.
- Reproducible-build comparison: required before delivery and recorded in the delivery manifest.

## Rendered evidence

The embedded UI was rendered in Chromium at desktop and mobile viewports with mocked authenticated local APIs. The validation exercised detected-to-connected AI-app state, root-cause output, technical details, filtering, and responsive layout. Evidence is delivered beside the executable.

## Platform limitation

The build host is Linux. UAC presentation and live WinGet installation cannot execute here. Those paths are covered by Windows-specific implementation, unit/contract tests, and Windows cross-compilation; native Windows smoke testing remains the final environment-specific check for a signed production release.
