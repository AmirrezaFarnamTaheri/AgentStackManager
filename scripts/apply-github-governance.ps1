#requires -Version 7.0
[CmdletBinding(SupportsShouldProcess,ConfirmImpact='High')]
param(
    [Parameter(Mandatory)][ValidatePattern('^[^/]+/[^/]+$')][string]$Repository,
    [Parameter()][ValidatePattern('^[A-Za-z0-9-]+$')][string]$ReleaseReviewerUser = 'AmirrezaFarnamTaheri',
    [switch]$VerifyOnly
)
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw 'GitHub CLI is required.' }
& gh auth status | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Authenticate GitHub CLI before applying governance.' }
$root=Split-Path -Parent $PSScriptRoot
$apiHeaders=@('-H','Accept: application/vnd.github+json','-H','X-GitHub-Api-Version: 2026-03-10')
$rules=Get-Content (Join-Path $root '.github/rulesets/main-protection.json') -Raw | ConvertFrom-Json -AsHashtable
$releasePolicy=Get-Content (Join-Path $root '.github/environments/release-policy.json') -Raw | ConvertFrom-Json -AsHashtable

function Invoke-GhJson([string[]]$Arguments) {
    $raw=& gh @Arguments
    if ($LASTEXITCODE -ne 0) { throw "gh failed: gh $($Arguments -join ' ')" }
    if (-not $raw) { return $null }
    return (($raw -join "`n") | ConvertFrom-Json -Depth 100)
}

function Get-Ruleset {
    $all=Invoke-GhJson (@('api')+$apiHeaders+@("repos/$Repository/rulesets",'--paginate'))
    return @($all) | Where-Object name -eq $rules.name | Select-Object -First 1
}

function Assert-EffectiveGovernance {
    $existing=Get-Ruleset
    if (-not $existing) { throw "Ruleset '$($rules.name)' is not applied to $Repository" }
    $effective=Invoke-GhJson (@('api')+$apiHeaders+@("repos/$Repository/rulesets/$($existing.id)"))
    if ($effective.enforcement -ne 'active') { throw 'Effective ruleset is not active' }
    $types=@($effective.rules | ForEach-Object type)
    foreach($required in @('deletion','non_fast_forward','required_signatures','pull_request','required_status_checks')) {
        if ($types -notcontains $required) { throw "Effective ruleset lacks $required" }
    }
    $pr=$effective.rules | Where-Object type -eq 'pull_request'
    if (-not $pr.parameters.require_code_owner_review -or -not $pr.parameters.dismiss_stale_reviews_on_push -or $pr.parameters.required_approving_review_count -lt 1) {
        throw 'Effective pull-request rules are weaker than repository policy'
    }
    $expectedChecks=@($rules.rules | Where-Object type -eq 'required_status_checks').parameters.required_status_checks.context
    $actualChecks=@($effective.rules | Where-Object type -eq 'required_status_checks').parameters.required_status_checks.context
    foreach($check in $expectedChecks) { if ($actualChecks -notcontains $check) { throw "Effective ruleset lacks required check: $check" } }

    $environment=Invoke-GhJson (@('api')+$apiHeaders+@("repos/$Repository/environments/$($releasePolicy.environment)"))
    if (-not $environment.protection_rules.required_reviewers) { throw 'Release environment lacks required reviewers' }
    if (-not $environment.protection_rules.prevent_self_review) { throw 'Release environment permits self-review' }
    if (-not $environment.deployment_branch_policy.custom_branch_policies) { throw 'Release environment lacks custom deployment policies' }
    $policies=Invoke-GhJson (@('api')+$apiHeaders+@("repos/$Repository/environments/$($releasePolicy.environment)/deployment-branch-policies"))
    $tagPolicy=@($policies.branch_policies) | Where-Object { $_.name -eq $releasePolicy.deployment_branch_policy.allowed_tag_pattern }
    if (-not $tagPolicy) { throw "Release environment lacks tag policy $($releasePolicy.deployment_branch_policy.allowed_tag_pattern)" }
    $secrets=Invoke-GhJson @('secret','list','--repo',$Repository,'--env',$releasePolicy.environment,'--json','name')
    $secretNames=@($secrets | ForEach-Object name)
    foreach($name in $releasePolicy.required_secrets) { if ($secretNames -notcontains $name) { throw "Release environment lacks required secret name: $name" } }
    [pscustomobject]@{Repository=$Repository;Ruleset=$effective.name;Environment=$environment.name;TagPolicy=$tagPolicy.name;Secrets=$secretNames}
}

if ($VerifyOnly) {
    Assert-EffectiveGovernance | ConvertTo-Json -Depth 10
    exit 0
}

$existing=Get-Ruleset
$payload=Join-Path $env:TEMP "agentstack-ruleset-$([guid]::NewGuid().ToString('N')).json"
$environmentPayload=Join-Path $env:TEMP "agentstack-environment-$([guid]::NewGuid().ToString('N')).json"
$tagPayload=Join-Path $env:TEMP "agentstack-tag-policy-$([guid]::NewGuid().ToString('N')).json"
try {
    $rules | ConvertTo-Json -Depth 100 | Set-Content -LiteralPath $payload -Encoding utf8NoBOM
    if ($existing) {
        if ($PSCmdlet.ShouldProcess("$Repository ruleset $($existing.id)",'Replace with repository policy')) {
            & gh api @apiHeaders --method PUT "repos/$Repository/rulesets/$($existing.id)" --input $payload | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Ruleset update failed.' }
        }
    } elseif ($PSCmdlet.ShouldProcess($Repository,'Create repository ruleset')) {
        & gh api @apiHeaders --method POST "repos/$Repository/rulesets" --input $payload | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Ruleset creation failed.' }
    }

    $reviewer=Invoke-GhJson (@('api')+$apiHeaders+@("users/$ReleaseReviewerUser"))
    $environmentDocument=[ordered]@{
        wait_timer=0
        prevent_self_review=$true
        reviewers=@([ordered]@{type='User';id=$reviewer.id})
        deployment_branch_policy=[ordered]@{protected_branches=$false;custom_branch_policies=$true}
    }
    $environmentDocument | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $environmentPayload -Encoding utf8NoBOM
    if ($PSCmdlet.ShouldProcess("$Repository environment $($releasePolicy.environment)",'Apply reviewer and deployment policy')) {
        & gh api @apiHeaders --method PUT "repos/$Repository/environments/$($releasePolicy.environment)" --input $environmentPayload | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Release environment update failed.' }
    }

    $policies=Invoke-GhJson (@('api')+$apiHeaders+@("repos/$Repository/environments/$($releasePolicy.environment)/deployment-branch-policies"))
    $desired=$releasePolicy.deployment_branch_policy.allowed_tag_pattern
    if (-not (@($policies.branch_policies) | Where-Object name -eq $desired)) {
        [ordered]@{name=$desired;type='tag'} | ConvertTo-Json | Set-Content -LiteralPath $tagPayload -Encoding utf8NoBOM
        if ($PSCmdlet.ShouldProcess("$Repository release tag policy $desired",'Create')) {
            & gh api @apiHeaders --method POST "repos/$Repository/environments/$($releasePolicy.environment)/deployment-branch-policies" --input $tagPayload | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Release tag policy creation failed.' }
        }
    }
}
finally {
    Remove-Item $payload,$environmentPayload,$tagPayload -ErrorAction SilentlyContinue
}
Write-Host 'Governance policy applied. Secret values are never created by this script; configure the required names, then verify:' -ForegroundColor Green
Write-Host "./scripts/apply-github-governance.ps1 -Repository $Repository -ReleaseReviewerUser $ReleaseReviewerUser -VerifyOnly"
