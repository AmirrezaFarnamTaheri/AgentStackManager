#requires -Version 7.0
[CmdletBinding()]
param()
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    $required=@(
        'README.md','docs/CLI_REFERENCE.md','docs/USER_GUIDE.md','docs/SECURITY.md',
        'docs/PRIVACY.md','docs/OPERATIONS.md','docs/SUPPLY_CHAIN.md','docs/THREAT_MODEL.md',
        'docs/GOVERNANCE.md','docs/RELEASE.md','docs/architecture.md','docs/UX_DESIGN.md','docs/UI_LIFECYCLE_WORKSPACE.md',
        'docs/CONVERGENCE.md','docs/convergence/DONOR_ANALYSIS.md','docs/convergence/TRUST_AND_STATE.md','docs/convergence/OMISSION_AUDIT.md','docs/convergence/PREMORTEM.md','docs/convergence/VALIDATION.md','docs/convergence/RUNBOOK.md','docs/convergence/ADOPTION.csv','docs/convergence/SURFACES.csv','docs/convergence/TEST_TRACEABILITY.csv',
        'docs/audit/ASM-001-040-closure.md','docs/audit/EXTERNAL-REPORT-ACCEPTED-ITEMS.md',
        'docs/audit/EXTERNAL-REPORT-ACCEPTED-ITEMS.json'
    )
    foreach($file in $required) {
        if (-not (Test-Path -LiteralPath $file) -or (Get-Item -LiteralPath $file).Length -eq 0) { throw "Missing or empty documentation: $file" }
    }
    $ledger=Get-Content 'docs/audit/ASM-001-040-closure.md' -Raw
    foreach($number in 1..40) {
        $id='ASM-{0:D3}' -f $number
        $count=([regex]::Matches($ledger,[regex]::Escape("| $id |"))).Count
        if ($count -ne 1) { throw "Closure ledger must contain $id exactly once; found $count" }
    }
    if ($ledger -match '\| ASM-[0-9]{3} \| (Open|Unaddressed|Deferred|Rejected) \|') { throw 'Closure ledger contains an unresolved finding status' }
    $files=Get-ChildItem README.md,docs -File -Recurse -Filter '*.md'
    $text=($files | Get-Content -Raw) -join "`n"
    foreach($pattern in @('agentstack apply --profile','terminates them after each request','terminates children after each call','Linux amd64 console CLI')) {
        if ($text -match [regex]::Escape($pattern)) { throw "Stale documentation contract: $pattern" }
    }
    foreach($requiredText in @('agentstack apply --plan-id','agentstack backup restore --id','agentstack data policy','agentstack owned remove','Go 1.26.5','Windows Job Object resource ceilings','installation tracker')) {
        if ($text -notmatch [regex]::Escape($requiredText)) { throw "Required documentation text missing: $requiredText" }
    }
    $catalog=Get-Content internal/catalog/default.json -Raw | ConvertFrom-Json
    if ($catalog.profiles.Count -ne 10) { throw "Unexpected profile count: $($catalog.profiles.Count)" }
    $userText=(Get-Content README.md,docs/CLI_REFERENCE.md,docs/USER_GUIDE.md -Raw) -join "`n"
    foreach($profile in $catalog.profiles) {
        if ($userText -notmatch [regex]::Escape("``$($profile.id)``")) { throw "Profile missing from user documentation: $($profile.id)" }
    }
    Write-Host 'Documentation contracts passed.' -ForegroundColor Green
}
finally { Pop-Location }
