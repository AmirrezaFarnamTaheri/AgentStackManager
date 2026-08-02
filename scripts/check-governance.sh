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
jq -e '.required_reviewers >= 1 and .prevent_self_review == true and .deployment_branch_policy.allowed_branch_pattern == "main" and .deployment_branch_policy.allowed_tag_pattern == "v*" and (.required_secrets | sort) == (["RELEASE_TAG_PUBLIC_KEY_BASE64","SIGNING_CERT_THUMBPRINT","SIGNING_PFX_BASE64","SIGNING_PFX_PASSWORD"] | sort)' .github/environments/release-policy.json >/dev/null
test "$(cat .go-version)" = '1.26.5'
grep -q 'cimg/go:1.26.5@sha256:6686a1ac4e71bc198b461caa82640547a0a44fa2378a4e4d450b1c8e63ddf31b' .circleci/config.yml
grep -Rqs 'golang.org/x/vuln/cmd/govulncheck@v1.1.4' .github/workflows .circleci/config.yml
grep -q 'github.com/rhysd/actionlint/cmd/actionlint@v1.7.12' .github/workflows/verify.yml
if grep -Fq 'actionlint -color never' .github/workflows/verify.yml; then
  echo 'actionlint color mode must not be passed as a positional file argument' >&2
  exit 1
fi
grep -Eq '^[[:space:]]+actionlint[[:space:]]*$' .github/workflows/verify.yml || {
  echo 'Verify workflow must invoke actionlint without positional arguments' >&2
  exit 1
}
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
for script in scripts/build.sh scripts/check-benchmarks.sh scripts/check-critical-coverage.sh scripts/check-docs.sh scripts/check-governance.sh scripts/check-source-archive-build.sh scripts/fuzz.sh scripts/verify-source-manifest.sh scripts/write-source-manifest.sh; do
  [[ -f "$script" ]] || { echo "required CI script is missing: $script" >&2; exit 1; }
  bash -n "$script"
done
release_workflow=.github/workflows/release.yml
grep -q 'workflow_dispatch:' "$release_workflow"
grep -q '^concurrency:' "$release_workflow"
grep -q 'cancel-in-progress: false' "$release_workflow"
grep -q 'gh attestation verify' "$release_workflow"
grep -q 'gh release create' "$release_workflow"
grep -q 'timeout-minutes:' "$release_workflow"
grep -Fq 'git/ref/tags/' "$release_workflow"
grep -Fq "object.type -ne 'tag'" "$release_workflow"
grep -Fq '"refs/tags/$tag"' "$release_workflow"
grep -Fq 'git merge-base --is-ancestor' "$release_workflow"
grep -Fq 'gitsign verify-tag' "$release_workflow"
grep -Fq 'verification.verified' "$release_workflow"
preflight_line="$(grep -n -m1 'name: Validate requested release tag' "$release_workflow" | cut -d: -f1)"
checkout_line="$(grep -n -m1 'uses: actions/checkout@' "$release_workflow" | cut -d: -f1)"
if [[ -z "$preflight_line" || -z "$checkout_line" || "$preflight_line" -ge "$checkout_line" ]]; then
  echo 'Release tag validation must run before checkout' >&2
  exit 1
fi
for required_action in \
  'actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1' \
  'actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0' \
  'actions/setup-node@820762786026740c76f36085b0efc47a31fe5020 # v7.0.0' \
  'actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1' \
  'actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1'; do
  grep -RqsF "$required_action" .github/workflows || {
    echo "Required Node 24-compatible action pin is missing: $required_action" >&2
    exit 1
  }
done
for deprecated_action in \
  'actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2' \
  'actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c # v6.4.0' \
  'actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4.4.0' \
  'actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4.6.2' \
  'actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093 # v4.3.0'; do
  if grep -RqsF "$deprecated_action" .github/workflows; then
    echo "Deprecated Node 20-era action pin remains: $deprecated_action" >&2
    exit 1
  fi
done
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
