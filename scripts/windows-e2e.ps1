#requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidateSet('amd64','arm64')][string]$Architecture
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

function Invoke-Native {
    param([Parameter(Mandatory)][string]$FilePath, [string[]]$Arguments = @())
    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$FilePath exited with $LASTEXITCODE" }
}

$repo = Split-Path -Parent $PSScriptRoot
Push-Location $repo
try {
    Invoke-Native go @('test','./...')
    if ($Architecture -eq 'amd64') {
        Invoke-Native go @('test','-race','./...')
    } else {
        Write-Host 'Go race detector is not supported on windows/arm64; Linux and Windows amd64 race gates remain required.' -ForegroundColor Yellow
    }
    Invoke-Native go @('vet','./...')

    $root = Join-Path $env:RUNNER_TEMP ("agentstack-e2e-$Architecture-" + [guid]::NewGuid().ToString('N'))
    $local = Join-Path $root 'LocalAppData'
    $profile = Join-Path $root ('User Profile With Spaces ' + ('x' * 80))
    $roaming = Join-Path $root 'Roaming'
    New-Item -ItemType Directory -Force -Path $local,$profile,$roaming | Out-Null
    $env:LOCALAPPDATA = $local
    $env:USERPROFILE = $profile
    $env:HOME = $profile
    $env:APPDATA = $roaming

    $seedUserPath = 'C:\Preserve-工具\bin;C:\Duplicate\bin;C:\Duplicate\bin;'
    [Environment]::SetEnvironmentVariable('Path',$seedUserPath,'User')

    $binary = Join-Path $root 'agentstack.exe'
    Invoke-Native go @('build','-trimpath','-o',$binary,'./cmd/agentstack')
    Invoke-Native $binary @('version')
    Invoke-Native $binary @('catalog')
    Invoke-Native $binary @('profiles')
    Invoke-Native $binary @('inventory')

    $setup = (& $binary setup --no-launch | Out-String | ConvertFrom-Json)
    $installed = [string]$setup.destination
    if (-not (Test-Path -LiteralPath $installed)) { throw "installed CLI missing: $installed" }
    if ([IO.Path]::GetFileName($installed) -ne 'agentstack.exe') { throw "unexpected installed name: $installed" }
    $userPath = [Environment]::GetEnvironmentVariable('Path','User')
    if (-not $userPath.StartsWith($seedUserPath,[StringComparison]::Ordinal)) { throw "pre-existing Unicode/duplicate user PATH was not preserved exactly: $userPath" }
    if (($userPath -split ';') -notcontains (Split-Path -Parent $installed)) { throw 'AgentStack directory was not appended to user PATH' }

    $dataRoot = Join-Path $local 'AgentStack'
    $acl = (& icacls $dataRoot | Out-String)
    if ($LASTEXITCODE -ne 0) { throw 'icacls audit failed' }
    if ($acl -match 'Everyone:' -or $acl -match 'BUILTIN\\Users:') { throw "data root exposes broad principals:`n$acl" }

    $plan = (& $installed plan --profile custom | Out-String | ConvertFrom-Json)
    if (-not $plan.id -or -not $plan.digest) { throw 'sealed plan identity missing' }
    $apply = (& $installed apply --plan-id $plan.id --digest $plan.digest --yes | Out-String | ConvertFrom-Json)
    if ($apply.transaction.status -ne 'succeeded') { throw "custom no-op apply failed: $($apply | ConvertTo-Json -Depth 20)" }
    & $installed apply --plan-id bogus --digest bogus --yes *> $null
    $invalidPlanExitCode = $LASTEXITCODE
    if ($invalidPlanExitCode -eq 0) { throw 'invalid reviewed plan was accepted' }
    # The nonzero result is expected and fully asserted above; do not leak it as
    # the successful PowerShell script's process exit code.
    $global:LASTEXITCODE = 0

    $stdout = Join-Path $root 'ui.stdout.log'
    $stderr = Join-Path $root 'ui.stderr.log'
    $process = Start-Process -FilePath $installed -ArgumentList @('ui','--no-open','--listen','127.0.0.1:0') -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
    try {
        $deadline = (Get-Date).AddSeconds(20)
        $url = $null
        while ((Get-Date) -lt $deadline) {
            if (Test-Path $stdout) {
                $text = Get-Content -Raw $stdout
                if ($text -match 'AgentStack Manager:\s+(http://\S+)') { $url = $matches[1]; break }
            }
            Start-Sleep -Milliseconds 100
        }
        if (-not $url) { throw "manager URL not emitted; stderr=$(Get-Content -Raw $stderr -ErrorAction SilentlyContinue)" }

        $managerUri = [uri]$url
        $statusUri = [uri]::new($managerUri, 'api/status')
        $shutdownUri = [uri]::new($managerUri, 'api/shutdown')

        $unauthorized = Invoke-WebRequest -Uri $statusUri -SkipHttpErrorCheck
        if ([int]$unauthorized.StatusCode -ne 403) {
            throw "unauthorized API request returned HTTP $([int]$unauthorized.StatusCode), expected 403"
        }

        $page = (Invoke-WebRequest -Uri $managerUri).Content
        if ($page -notmatch '<meta name="agentstack-token" content="([^"]+)">') { throw 'session token missing from secret page' }
        $token = $matches[1]
        $headers = @{'X-AgentStack-Token'=$token}
        $status = Invoke-RestMethod -Uri $statusUri -Headers $headers
        if (-not $status.localOnly) { throw 'manager did not report local-only mode' }
        Invoke-RestMethod -Method Post -Uri $shutdownUri -Headers $headers -ContentType 'application/json' -Body '{}' | Out-Null
        if (-not $process.WaitForExit(10000)) { throw 'manager did not stop after authenticated shutdown' }
    } finally {
        if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force }
    }

    Write-Host "Windows $Architecture end-to-end verification passed" -ForegroundColor Green
} finally {
    Pop-Location
}
