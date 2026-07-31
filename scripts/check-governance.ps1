#requires -Version 7.0
[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$root=Split-Path -Parent $PSScriptRoot
$owners=Get-Content -Raw (Join-Path $root '.github/CODEOWNERS')
if ($owners -notmatch '@AmirrezaFarnamTaheri' -or $owners -match 'placeholder|agentstack-maintainers|CHANGE_ME|TODO') { throw 'CODEOWNERS is missing the reviewed owner or contains a placeholder' }
$rules=Get-Content -Raw (Join-Path $root '.github/rulesets/main-protection.json') | ConvertFrom-Json -Depth 100
if ($rules.enforcement -ne 'active') { throw 'main ruleset is not active' }
$types=@($rules.rules | ForEach-Object type)
foreach($required in @('non_fast_forward','deletion','required_signatures','pull_request','required_status_checks')) { if ($types -notcontains $required) { throw "main ruleset lacks $required" } }
$pr=$rules.rules | Where-Object type -eq 'pull_request'
if (-not $pr.parameters.require_code_owner_review -or -not $pr.parameters.dismiss_stale_reviews_on_push -or $pr.parameters.required_approving_review_count -lt 1) { throw 'pull-request governance is weaker than required' }
$checks=$rules.rules | Where-Object type -eq 'required_status_checks'
if (-not $checks.parameters.strict_required_status_checks_policy -or @($checks.parameters.required_status_checks).Count -lt 5) { throw 'required status-check policy is incomplete' }
$release=Get-Content -Raw (Join-Path $root '.github/environments/release-policy.json') | ConvertFrom-Json -Depth 20
$expectedSecrets=@('RELEASE_TAG_PUBLIC_KEY_BASE64','SIGNING_CERT_THUMBPRINT','SIGNING_PFX_BASE64','SIGNING_PFX_PASSWORD')
if ($release.required_reviewers -lt 1 -or -not $release.prevent_self_review -or (Compare-Object (@($release.required_secrets) | Sort-Object) ($expectedSecrets | Sort-Object))) { throw 'release environment policy is incomplete' }
$goVersion=(Get-Content -Raw (Join-Path $root '.go-version')).Trim()
if ($goVersion -ne '1.26.5') { throw '.go-version must pin Go 1.26.5' }
$workflowText=(Get-Content -Raw (Join-Path $root '.github/workflows/verify.yml')) + (Get-Content -Raw (Join-Path $root '.github/workflows/release.yml'))
$releaseWorkflow=Get-Content -Raw (Join-Path $root '.github/workflows/release.yml')
$windowsE2E=Get-Content -Raw (Join-Path $root 'scripts/windows-e2e.ps1')
if ($windowsE2E -notmatch [regex]::Escape("if (`$Architecture -eq 'amd64')")) { throw 'Windows E2E must gate the race detector to supported windows/amd64 runners' }
$requiredExecutableScripts=@('scripts/build.sh','scripts/check-critical-coverage.sh','scripts/check-docs.sh','scripts/check-governance.sh','scripts/fuzz.sh')
foreach($script in $requiredExecutableScripts) {
    $entry=git -C $root ls-files -s -- $script
    if ($LASTEXITCODE -ne 0 -or $entry -notmatch '^100755\s') { throw "Required CI script is not executable in Git: $script" }
}
foreach($requiredPattern in @('workflow_dispatch:', '(?m)^concurrency:', 'cancel-in-progress:\s*false', 'gh attestation verify', 'gh release create', 'timeout-minutes:', 'git/ref/tags/', "object\.type\s+-ne\s+'tag'", 'ref=refs/tags/\$tag')) {
    if ($releaseWorkflow -notmatch $requiredPattern) { throw "Release workflow is missing required control: $requiredPattern" }
}
$preflightIndex=$releaseWorkflow.IndexOf('name: Validate requested release tag',[StringComparison]::Ordinal)
$checkoutIndex=$releaseWorkflow.IndexOf('uses: actions/checkout@',[StringComparison]::Ordinal)
if ($preflightIndex -lt 0 -or $checkoutIndex -lt 0 -or $preflightIndex -ge $checkoutIndex) { throw 'Release tag validation must run before checkout' }
foreach($requiredAction in @(
    'actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1',
    'actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0',
    'actions/setup-node@820762786026740c76f36085b0efc47a31fe5020 # v7.0.0',
    'actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1',
    'actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1'
)) {
    if ($workflowText -notmatch [regex]::Escape($requiredAction)) { throw "Required Node 24-compatible action pin is missing: $requiredAction" }
}
foreach($deprecatedAction in @(
    'actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2',
    'actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c # v6.4.0',
    'actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4.4.0',
    'actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4.6.2',
    'actions/download-artifact@d3f86a106a0bac45b974a628896c90dbdf5c8093 # v4.3.0'
)) {
    if ($workflowText -match [regex]::Escape($deprecatedAction)) { throw "Deprecated Node 20-era action pin remains: $deprecatedAction" }
}
if ($releaseWorkflow -match 'softprops/action-gh-release') { throw 'Release publication must use the authenticated GitHub CLI, not a third-party release action' }
if ($workflowText -notmatch "go-version-file:\s*'\.go-version'") { throw 'GitHub workflows must use .go-version' }
$circle=Get-Content -Raw (Join-Path $root '.circleci/config.yml')
if ($circle -notmatch 'cimg/go:1\.26\.5@sha256:6686a1ac4e71bc198b461caa82640547a0a44fa2378a4e4d450b1c8e63ddf31b') { throw 'CircleCI image is not pinned to the reviewed digest' }
if ($workflowText -notmatch 'golang.org/x/vuln/cmd/govulncheck@v1\.1\.4') { throw 'govulncheck is not pinned to v1.1.4' }
if ($workflowText -notmatch 'github.com/rhysd/actionlint/cmd/actionlint@v1\.7\.12') { throw 'actionlint is not pinned to v1.7.12' }
if ($workflowText -notmatch 'github.com/anchore/syft/cmd/syft@v1\.50\.0') { throw 'Syft is not pinned to v1.50.0' }
$workflowLines=Get-ChildItem (Join-Path $root '.github/workflows') -Filter '*.yml' | ForEach-Object { Get-Content $_.FullName }
$uses=@($workflowLines | Where-Object { $_ -match '^\s*-?\s*uses:\s*[^@\s]+@' })
if (-not $uses) { throw 'No GitHub Action references were found' }
foreach($line in $uses) {
    if ($line -notmatch '@[0-9a-f]{40}(\s*#.*)?\s*$') { throw "GitHub Action is not pinned to a full immutable commit SHA: $line" }
}
Write-Host 'Repository governance policy files are internally consistent.' -ForegroundColor Green
