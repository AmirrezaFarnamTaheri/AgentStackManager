#requires -Version 7.0
[CmdletBinding()]
param([string]$Version = 'dev')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root 'dist-dev'

function Assert-SourceManifest([string]$Root) {
    $manifest = Join-Path $Root 'SOURCE_MANIFEST.sha256'
    if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) { throw 'SOURCE_MANIFEST.sha256 is missing' }

    $expected = [Collections.Generic.Dictionary[string,string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($line in Get-Content -LiteralPath $manifest) {
        if ($line -notmatch '^([0-9a-fA-F]{64})  \./(.+)$') { throw "Invalid source manifest line: $line" }
        $digest = $Matches[1].ToLowerInvariant()
        $manifestRelative = './' + $Matches[2].Replace('\', '/')
        if (-not $expected.TryAdd($manifestRelative, $digest)) {
            throw "Source manifest contains a duplicate path: $manifestRelative"
        }

        $relative = $Matches[2].Replace('/', [IO.Path]::DirectorySeparatorChar)
        $path = Join-Path $Root $relative
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Source manifest file is missing: $manifestRelative" }
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash
        if ($actual.ToLowerInvariant() -ne $digest) { throw "Source manifest digest mismatch: $manifestRelative" }
    }

    $actualFiles = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($item in Get-ChildItem -LiteralPath $Root -Force -Recurse) {
        $relative = [IO.Path]::GetRelativePath($Root, $item.FullName).Replace('\', '/')
        if ($relative -eq 'SOURCE_MANIFEST.sha256' -or $relative -match '^(?:\.git|dist|dist-dev)(?:/|$)') {
            continue
        }
        if ($item.LinkType) { throw "Source tree contains an unsupported symbolic link: ./$relative" }
        if ($item.PSIsContainer) { continue }
        if (-not ($item -is [IO.FileInfo])) { throw "Source tree contains an unsupported filesystem node: ./$relative" }
        [void]$actualFiles.Add("./$relative")
    }

    foreach ($path in $expected.Keys) {
        if (-not $actualFiles.Contains($path)) { throw "Source manifest file is missing: $path" }
    }
    foreach ($path in $actualFiles) {
        if (-not $expected.ContainsKey($path)) { throw "Source tree contains an unlisted file: $path" }
    }
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
