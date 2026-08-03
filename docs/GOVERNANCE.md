# Repository Governance

The repository includes the desired governance policy as code:

- `.github/CODEOWNERS` protects catalog, release, workflow, security, installer, MCP, and state paths.
- `.github/rulesets/main-protection.json` requires pull requests, code-owner review, stale-review dismissal, resolved threads, signed commits, linear history, no force push/deletion, and required checks.
- `.github/environments/release-policy.json` defines release reviewer plus protected `main` and `v*` deployment policy expectations.
- `scripts/check-governance.sh` and `.ps1` fail on placeholder owners or weakened controls.

These files do not magically configure a remote repository. An administrator must apply and verify them against the actual GitHub repository. With explicit repository-administrator authorization, run `scripts/apply-github-governance.ps1 -Repository OWNER/REPO -ReleaseReviewerUser USER`. It applies the reviewed branch ruleset, named release reviewer, self-review prohibition, and tag-only `v*` deployment policy. The script never creates secret values; configure the four required environment secrets separately, including the trusted release-tag public key, then rerun with `-VerifyOnly` to prove the effective server-side state and retain the JSON result as audit evidence.

Required checks are:

- Go assurance and vulnerability scan;
- mutation testing;
- Windows x64 end-to-end;
- Windows ARM64 end-to-end;
- accessibility and keyboard checks;
- signed release environment approval for tags.

Workflow and release-policy changes require code-owner review. Signing secrets are scoped to the protected release environment and are never available to pull-request workflows.

## Peer adoption governance

Every future donor-derived change must update the convergence ledger when it changes a recorded capability, target node, invariant, or test. New remote acquisition, credential storage, UI authority, shell execution, or durable schema requires an explicit trust-boundary review and migration/rollback plan.

File-level accounting is necessary but insufficient. Each material adopted, rejected, superseded, or inspired unit needs a decision rationale and suitable evidence. Shared smoke tests cannot stand in for independent semantics.
