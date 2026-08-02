#requires -Version 5.1
[CmdletBinding()]
param(
    [switch]$NoLaunch,
    [string]$Profile = 'core'
)
$ErrorActionPreference = 'Stop'
$architecture = [Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
$binaryName = switch ($architecture) {
    'x64'   { 'agentstack-windows-amd64.exe' }
    'arm64' { 'agentstack-windows-arm64.exe' }
    default { throw "Unsupported Windows architecture: $architecture" }
}
$source = Join-Path $PSScriptRoot $binaryName
$manifest = Join-Path $PSScriptRoot 'SHA256SUMS.txt'
if (-not (Test-Path -LiteralPath $source)) { throw "Missing release binary: $source" }
if (-not (Test-Path -LiteralPath $manifest)) { throw "Missing release checksum manifest: $manifest" }
$expectedLine = Get-Content -LiteralPath $manifest | Where-Object { $_ -match "\s+$([regex]::Escape($binaryName))$" } | Select-Object -First 1
if (-not $expectedLine) { throw "Checksum manifest does not contain $binaryName" }
$expected = ($expectedLine -split '\s+')[0].ToLowerInvariant()
$actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $source).Hash.ToLowerInvariant()
if ($actual -ne $expected) { throw "Release binary checksum mismatch" }
Write-Host "Installing verified AgentStack Manager ($architecture)..." -ForegroundColor Cyan
& $source install-self
if ($LASTEXITCODE -ne 0) { throw "AgentStack self-install failed with exit code $LASTEXITCODE" }
$binDir = Join-Path $env:LOCALAPPDATA 'Programs\AgentStack\bin'
$installed = Join-Path $binDir 'agentstack.exe'
if (-not (Test-Path -LiteralPath $installed)) { throw "Installed binary was not found: $installed" }
if (($env:Path -split ';') -notcontains $binDir) { $env:Path = "$binDir;$env:Path" }
Write-Host "Installed: $installed" -ForegroundColor Green
Write-Host "Create a reviewable plan:" -ForegroundColor Yellow
Write-Host "  agentstack plan --profile $Profile" -ForegroundColor Yellow
Write-Host "Apply only the emitted plan identity:" -ForegroundColor Yellow
Write-Host "  agentstack apply --plan-id <id> --digest <sha256> --yes" -ForegroundColor Yellow
if (-not $NoLaunch) { & $installed ui }
