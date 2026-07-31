#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

grep -q '@AmirrezaFarnamTaheri' .github/CODEOWNERS
if grep -Eq 'placeholder|agentstack-maintainers|CHANGE_ME|TODO' .github/CODEOWNERS; then
  echo 'CODEOWNERS contains a placeholder owner' >&2
  exit 1
fi
jq -e '
  .enforcement == "active" and
  ([.rules[].type] | index("non_fast_forward") != null) and
  ([.rules[].type] | index("deletion") != null) and
  ([.rules[].type] | index("required_signatures") != null) and
  ([.rules[].type] | index("pull_request") != null) and
  ([.rules[].type] | index("required_status_checks") != null) and
  ([.rules[] | select(.type=="pull_request") | .parameters.require_code_owner_review] | any) and
  ([.rules[] | select(.type=="pull_request") | .parameters.dismiss_stale_reviews_on_push] | any) and
  ([.rules[] | select(.type=="required_status_checks") | .parameters.strict_required_status_checks_policy] | any)
' .github/rulesets/main-protection.json >/dev/null
jq -e '.required_reviewers >= 1 and .prevent_self_review == true and (.required_secrets | sort) == (["RELEASE_TAG_PUBLIC_KEY_BASE64","SIGNING_CERT_THUMBPRINT","SIGNING_PFX_BASE64","SIGNING_PFX_PASSWORD"] | sort)' .github/environments/release-policy.json >/dev/null
test "$(cat .go-version)" = '1.26.5'
grep -q 'cimg/go:1.26.5@sha256:6686a1ac4e71bc198b461caa82640547a0a44fa2378a4e4d450b1c8e63ddf31b' .circleci/config.yml
grep -Rqs 'golang.org/x/vuln/cmd/govulncheck@v1.1.4' .github/workflows .circleci/config.yml
grep -q 'github.com/rhysd/actionlint/cmd/actionlint@v1.7.12' .github/workflows/verify.yml
grep -q 'github.com/anchore/syft/cmd/syft@v1.50.0' .github/workflows/release.yml
grep -Fq 'Remove-Item $bundle -Recurse -Force' scripts/release.ps1
grep -Fq 'Remove-Item $sourceRoot -Recurse -Force' scripts/release.ps1
grep -Fq "Get-ChildItem \$Directory -Recurse -File" scripts/release.ps1
grep -Fq 'Bundle contains an unlisted member' scripts/release.ps1
grep -Fq "Join-Path \$env:RUNNER_TEMP 'agentstack-release-coverage.out'" scripts/release.ps1
grep -Fq 'Assert-ReleaseOutput $dist' scripts/release.ps1
grep -Fq "if (\$Architecture -eq 'amd64')" scripts/windows-e2e.ps1
grep -Rqs "go-version-file: '.go-version'" .github/workflows/verify.yml .github/workflows/release.yml
for workflow in verify.yml release.yml; do test -s ".github/workflows/$workflow"; done
for script in scripts/build.sh scripts/check-critical-coverage.sh scripts/check-docs.sh scripts/check-governance.sh scripts/fuzz.sh; do
  [[ -x "$script" ]] || { echo "required CI script is not executable: $script" >&2; exit 1; }
done
release_workflow=.github/workflows/release.yml
grep -q 'workflow_dispatch:' "$release_workflow"
grep -q '^concurrency:' "$release_workflow"
grep -q 'cancel-in-progress: false' "$release_workflow"
grep -q 'gh attestation verify' "$release_workflow"
grep -q 'gh release create' "$release_workflow"
grep -q 'timeout-minutes:' "$release_workflow"
if grep -q 'softprops/action-gh-release' "$release_workflow"; then
  echo 'Release publication must use the authenticated GitHub CLI, not a third-party release action' >&2
  exit 1
fi
if grep -REn 'uses:[[:space:]]+[^@[:space:]]+@(v[0-9]+|main|master|[0-9a-f]{1,39})([[:space:]]|$)' .github/workflows; then
  echo 'Every third-party GitHub Action must be pinned to a full immutable 40-character commit SHA' >&2
  exit 1
fi
if grep -REn 'uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}([[:space:]]*#.*)?[[:space:]]*$' .github/workflows >/dev/null; then
  :
else
  echo 'No immutable GitHub Action references were found' >&2
  exit 1
fi
echo 'Repository governance policy files are internally consistent.'
