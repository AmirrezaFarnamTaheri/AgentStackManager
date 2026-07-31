#requires -Version 7.0
[CmdletBinding()]
param([string]$Version = 'dev')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root 'dist-dev'
Push-Location $root
try {
    $dirty = git status --porcelain=v1 --untracked-files=all
    if ($dirty) { throw "Development build requires a clean tree:`n$dirty" }
    Remove-Item $dist -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path $dist | Out-Null
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed' }
    go test -race ./...
    if ($LASTEXITCODE -ne 0) { throw 'go race tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    $revision=(git rev-parse HEAD).Trim()
    $flags="-s -w -buildid= -X main.version=$Version"
    foreach ($arch in @('amd64','arm64')) {
        $env:CGO_ENABLED='0'; $env:GOOS='windows'; $env:GOARCH=$arch
        $path=Join-Path $dist "agentstack-windows-$arch.exe"
        go build -trimpath -buildvcs=true -ldflags $flags -o $path ./cmd/agentstack
        if ($LASTEXITCODE -ne 0) { throw "build failed for $arch" }
        $metadata=(go version -m $path) -join "`n"
        if ($metadata -notmatch "vcs\.revision=$revision" -or $metadata -notmatch 'vcs\.modified=false') { throw "invalid VCS metadata for $path" }
    }
    Write-Host 'Unsigned development binaries created. Public releases require scripts/release.ps1 and Authenticode credentials.' -ForegroundColor Yellow
}
finally {
    Pop-Location
    Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
