#requires -Version 7.0
[CmdletBinding()]
param([string]$Version = 'dev')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root 'dist-dev'

function Assert-SourceManifest([string]$Root) {
    & go run ./cmd/releasepack --root $Root --manifest-mode verify
    if ($LASTEXITCODE -ne 0) { throw 'Source closure verification failed' }
}

Push-Location $root
try {
    $gitRoot = ''
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $gitRoot = ((& git -C $root rev-parse --show-toplevel 2>$null) -join '').Trim()
    }
    $insideGit = $false
    if ($gitRoot) {
        $trimChars = [char[]]@([IO.Path]::DirectorySeparatorChar, [IO.Path]::AltDirectorySeparatorChar)
        $normalizedRoot = [IO.Path]::GetFullPath($root).TrimEnd($trimChars)
        $normalizedGitRoot = [IO.Path]::GetFullPath($gitRoot).TrimEnd($trimChars)
        $insideGit = [StringComparer]::OrdinalIgnoreCase.Equals($normalizedRoot, $normalizedGitRoot)
    }
    $buildVCS = 'false'
    if ($insideGit) {
        $dirty = git status --porcelain=v1 --untracked-files=all
        if ($dirty) { throw "Development build requires a clean tree:`n$dirty" }
        $revision = 'git:' + (git rev-parse HEAD).Trim()
        $buildVCS = 'true'
    }
    else {
        Assert-SourceManifest $root
        $revision = (Get-Content -LiteralPath (Join-Path $root 'SOURCE_REVISION') -Raw).Trim()
        if ($revision -notmatch '^(git|unreleased-base):[0-9A-Fa-f]{40}$') {
            throw 'SOURCE_REVISION must contain git:<40-hex> or unreleased-base:<40-hex>'
        }
    }
    Remove-Item $dist -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path $dist | Out-Null
    go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed' }
    go test -race ./...
    if ($LASTEXITCODE -ne 0) { throw 'go race tests failed' }
    go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed' }
    $sysoPath = Join-Path $root 'cmd/agentstack/icon_windows_amd64.syso'
    & go run ./cmd/resourcegen `
        -icon (Join-Path $root 'cmd/agentstack/icon.ico') `
        -manifest (Join-Path $root 'cmd/agentstack/agentstack.manifest') `
        -out $sysoPath
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed' }
    $flags="-s -w -buildid= -X main.version=$Version -X main.revision=$revision"
    foreach ($arch in @('amd64','arm64')) {
        $env:CGO_ENABLED='0'; $env:GOOS='windows'; $env:GOARCH=$arch
        $path=Join-Path $dist "agentstack-windows-$arch.exe"
        go build -trimpath "-buildvcs=$buildVCS" -ldflags $flags -o $path ./cmd/agentstack
        if ($LASTEXITCODE -ne 0) { throw "build failed for $arch" }
        if ($insideGit) {
            $metadata=(go version -m $path) -join "`n"
            $commit=$revision.Substring(4)
            if ($metadata -notmatch "vcs\.revision=$commit" -or $metadata -notmatch 'vcs\.modified=false') { throw "invalid VCS metadata for $path" }
        }
    }
    Write-Host "Unsigned development binaries created from $revision. Public releases require scripts/release.ps1 and protected provenance gates." -ForegroundColor Yellow
}
finally {
    Pop-Location
    Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
