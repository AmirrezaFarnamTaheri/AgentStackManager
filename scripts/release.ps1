#requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$')][string]$Version,
    [string]$OutputDirectory = 'dist'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root $OutputDirectory
$tag = "v$Version"

function Invoke-Checked([string]$File, [string[]]$Arguments) {
    & $File @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$File failed with exit code $LASTEXITCODE" }
}
function Assert-CleanTaggedSource {
    $dirty = git status --porcelain=v1 --untracked-files=all
    if ($dirty) { throw "Release requires a clean tree:`n$dirty" }
    $head = (git rev-parse HEAD).Trim()
    $tagCommit = (git rev-list -n 1 $tag 2>$null).Trim()
    if (-not $tagCommit -or $head -ne $tagCommit) { throw "HEAD $head is not exactly tag $tag" }
    $originMain = (git rev-parse refs/remotes/origin/main 2>$null).Trim()
    if (-not $originMain) { throw 'Release requires a fetched refs/remotes/origin/main' }
    if ($head -ne $originMain) { throw "Release tag $tag points to $head, but origin/main is $originMain" }
    if ((git cat-file -t "refs/tags/$tag").Trim() -ne 'tag') { throw "$tag must be an annotated tag" }
    Invoke-Checked git @('tag','-v',$tag) | Out-Host
    return $head
}
function Assert-Toolchain {
    $versionText = (go version).Trim()
    if ($versionText -notmatch '\bgo1\.26\.5\b') { throw "Release requires Go 1.26.5; found $versionText" }
    foreach ($tool in @('git','go','govulncheck','syft','tar','bash')) {
        if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "Required release tool is missing: $tool" }
    }
}
function Assert-Governance {
    & (Join-Path $root 'scripts/check-governance.ps1')
    if ($LASTEXITCODE -ne 0) { throw 'Repository governance validation failed' }
    & (Join-Path $root 'scripts/check-docs.ps1')
    if ($LASTEXITCODE -ne 0) { throw 'Documentation contract validation failed' }
}
function Build-Binary([string]$Arch,[string]$Destination,[string]$Flags) {
    $env:CGO_ENABLED='0'; $env:GOOS='windows'; $env:GOARCH=$Arch
    Invoke-Checked go @('build','-trimpath','-buildvcs=true','-ldflags',$Flags,'-o',$Destination,'./cmd/agentstack')
}
function Assert-BuildInfo([string]$Path,[string]$Revision) {
    $metadata = (go version -m $Path) -join "`n"
    if ($metadata -notmatch "vcs\.revision=$Revision") { throw "$Path does not embed revision $Revision" }
    if ($metadata -notmatch 'vcs\.modified=false') { throw "$Path was built from a modified tree" }
    if ($metadata -notmatch 'go1\.26\.5') { throw "$Path was not built with Go 1.26.5" }
}
function Write-Checksums([string]$Directory) {
    Get-ChildItem $Directory -Recurse -File | Where-Object Name -ne 'SHA256SUMS.txt' | ForEach-Object {
        $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName
        [pscustomobject]@{
            Relative=[IO.Path]::GetRelativePath($Directory,$_.FullName).Replace('\','/')
            Hash=$hash.Hash.ToLowerInvariant()
        }
    } | Sort-Object Relative | ForEach-Object {
        "$($_.Hash)  $($_.Relative)"
    } | Set-Content (Join-Path $Directory 'SHA256SUMS.txt') -Encoding utf8NoBOM
}
function Write-SourceManifest([string]$Directory) {
    $manifest=Join-Path $Directory 'SOURCE_MANIFEST.sha256'
    Get-ChildItem $Directory -Recurse -File | Where-Object FullName -ne $manifest | ForEach-Object {
        $relative=[IO.Path]::GetRelativePath($Directory,$_.FullName).Replace('\','/')
        [pscustomobject]@{ Relative=$relative; Hash=(Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant() }
    } | Sort-Object Relative | ForEach-Object {
        "$($_.Hash)  ./$($_.Relative)"
    } | Set-Content $manifest -Encoding utf8NoBOM
}
function Assert-ReleaseOutput([string]$Directory) {
    $expected=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach($name in @(
        "AgentStackManager-$Version-windows-amd64.zip",
        "AgentStackManager-$Version-windows-arm64.zip",
        "AgentStackManager-$Version-source.zip",
        'agentstack-windows-amd64.exe',
        'agentstack-windows-arm64.exe',
        'AgentStack-Setup-windows-amd64.exe',
        'AgentStack-Setup-windows-arm64.exe',
        'AgentStack-Setup.ps1',
        'SHA256SUMS.txt',
        'agentstack-catalog.cdx.json',
        'agentstack-binary-amd64.cdx.json',
        'agentstack-binary-arm64.cdx.json',
        'agentstack.openvex.json',
        'provenance.json',
        'component-licenses.json'
    )) { [void]$expected.Add($name) }
    foreach($item in Get-ChildItem $Directory -Force) {
        if ($item.PSIsContainer) { throw "Unexpected release output directory: $($item.Name)" }
        if (-not $expected.Remove($item.Name)) { throw "Unexpected release output file: $($item.Name)" }
    }
    if ($expected.Count -ne 0) { throw "Release output is missing: $(([string[]]$expected | Sort-Object) -join ', ')" }
}
function Write-Provenance([string]$Revision) {
    foreach($required in @('GITHUB_SERVER_URL','GITHUB_REPOSITORY','GITHUB_RUN_ID','GITHUB_WORKFLOW_REF')) {
        if (-not (Get-Item "Env:$required" -ErrorAction SilentlyContinue).Value) { throw "Release provenance requires $required" }
    }
    $subjects=@()
    Get-ChildItem $dist -File | Where-Object Extension -in @('.exe','.ps1') | Sort-Object Name | ForEach-Object {
        $subjects += [ordered]@{
            name=$_.Name
            digest=[ordered]@{ sha256=(Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName).Hash.ToLowerInvariant() }
        }
    }
    $sourceUri="$env:GITHUB_SERVER_URL/$env:GITHUB_REPOSITORY"
    $document=[ordered]@{
        _type='https://in-toto.io/Statement/v1'
        subject=$subjects
        predicateType='https://slsa.dev/provenance/v1'
        predicate=[ordered]@{
            buildDefinition=[ordered]@{
                buildType="$sourceUri/.github/workflows/release.yml@v1"
                externalParameters=[ordered]@{ version=$Version; tag=$tag }
                internalParameters=[ordered]@{
                    goVersion=(go version).Trim()
                    catalogSha256=(Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $root 'internal/catalog/default.json')).Hash.ToLowerInvariant()
                }
                resolvedDependencies=@([ordered]@{
                    uri="git+$sourceUri@refs/tags/$tag"
                    digest=[ordered]@{ sha1=$Revision }
                })
            }
            runDetails=[ordered]@{
                builder=[ordered]@{ id="$env:GITHUB_WORKFLOW_REF" }
                metadata=[ordered]@{
                    invocationId="$sourceUri/actions/runs/$env:GITHUB_RUN_ID"
                    finishedOn=(Get-Date).ToUniversalTime().ToString('o')
                }
            }
        }
    }
    $document | ConvertTo-Json -Depth 30 | Set-Content (Join-Path $dist 'provenance.json') -Encoding utf8NoBOM
}

function Assert-Bundle([string]$Archive,[string]$Arch) {
    $extract=Join-Path $env:TEMP "agentstack-bundle-$Arch-$([guid]::NewGuid().ToString('N'))"
    try {
        Expand-Archive -LiteralPath $Archive -DestinationPath $extract
        $manifest=Get-ChildItem $extract -Recurse -Filter SHA256SUMS.txt | Select-Object -First 1
        if (-not $manifest) { throw "Bundle $Archive has no internal checksum manifest" }
        $bundleRoot=Split-Path -Parent $manifest.FullName
        $expected=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
        foreach($line in Get-Content -LiteralPath $manifest.FullName) {
            if ($line -notmatch '^([0-9a-fA-F]{64})\s{2}(.+)$') { throw "Invalid checksum line in $Archive`: $line" }
            $relative=$Matches[2].Replace('\','/')
            if (-not $expected.Add($relative)) { throw "Duplicate checksum member in ${Archive}: $relative" }
            $file=Join-Path $bundleRoot $relative
            if (-not (Test-Path -LiteralPath $file -PathType Leaf)) { throw "Bundle member missing: $relative" }
            $actual=(Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
            if ($actual -ne $Matches[1].ToLowerInvariant()) { throw "Bundle digest mismatch: $relative" }
        }
        foreach($file in Get-ChildItem $bundleRoot -Recurse -File) {
            if ($file.FullName -eq $manifest.FullName) { continue }
            $relative=[IO.Path]::GetRelativePath($bundleRoot,$file.FullName).Replace('\','/')
            if (-not $expected.Contains($relative)) { throw "Bundle contains an unlisted member: $relative" }
        }
        $setup=Join-Path $bundleRoot 'AgentStack-Setup.exe'
        $console=Join-Path $bundleRoot "agentstack-windows-$Arch.exe"
        $script=Join-Path $bundleRoot 'AgentStack-Setup.ps1'
        foreach($required in @('provenance.json','agentstack-catalog.cdx.json',"agentstack-binary-$Arch.cdx.json",'agentstack.openvex.json','component-licenses.json')) {
            if (-not (Test-Path -LiteralPath (Join-Path $bundleRoot $required))) { throw "Bundle assurance file missing: $required" }
        }
        if ($Arch -eq 'amd64') {
            & $setup --verify-only | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'x64 setup launcher pair verification failed' }
            & $console version | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'x64 console version smoke failed' }
            & $console catalog | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'x64 console catalog smoke failed' }
        }
    }
    finally { Remove-Item $extract -Recurse -Force -ErrorAction SilentlyContinue }
}

Push-Location $root
try {
    Assert-Toolchain
    Assert-Governance
    $revision = Assert-CleanTaggedSource
    Remove-Item $dist -Recurse -Force -ErrorAction SilentlyContinue
    New-Item -ItemType Directory -Path $dist | Out-Null

    Invoke-Checked go @('test','./...')
    Invoke-Checked go @('test','-race','./...')
    Invoke-Checked go @('vet','./...')
    $coverage=Join-Path $env:RUNNER_TEMP 'agentstack-release-coverage.out'
    Remove-Item $coverage -Force -ErrorAction SilentlyContinue
    Invoke-Checked go @('test',"-coverprofile=$coverage",'./...')
    Invoke-Checked bash @('scripts/check-critical-coverage.sh',$coverage)
    Remove-Item $coverage -Force
    Invoke-Checked govulncheck @('./...')

$baseFlags = "-s -w -X main.version=$Version -X main.revision=git:$revision"
    foreach ($arch in @('amd64','arm64')) {
        $first = Join-Path $dist "agentstack-$arch.repro1.exe"
        $second = Join-Path $dist "agentstack-$arch.repro2.exe"
        Build-Binary $arch $first $baseFlags
        Build-Binary $arch $second $baseFlags
        $one=(Get-FileHash -Algorithm SHA256 $first).Hash
        $two=(Get-FileHash -Algorithm SHA256 $second).Hash
        if ($one -ne $two) { throw "Unsigned $arch builds are not reproducible" }
        Remove-Item $second
        $console = Join-Path $dist "agentstack-windows-$arch.exe"
        Move-Item $first $console
        Assert-BuildInfo $console $revision
        Invoke-Checked govulncheck @('-mode','binary',$console)
        $consoleHash=(Get-FileHash -Algorithm SHA256 $console).Hash.ToLowerInvariant()
        # Keep the setup launcher portable across the pinned Go toolchain's
        # Windows linker; the launcher remains fully functional as a console
        # executable and is integrity-checked by its SHA-256 manifest.
        $setupFlags = "$baseFlags -X main.defaultMode=setup -X main.consoleSHA256=$consoleHash"
        $setupFirst = Join-Path $dist "AgentStack-Setup-windows-$arch.repro1.exe"
        $setupSecond = Join-Path $dist "AgentStack-Setup-windows-$arch.repro2.exe"
        Build-Binary $arch $setupFirst $setupFlags
        Build-Binary $arch $setupSecond $setupFlags
        $setupOne=(Get-FileHash -Algorithm SHA256 $setupFirst).Hash
        $setupTwo=(Get-FileHash -Algorithm SHA256 $setupSecond).Hash
        if ($setupOne -ne $setupTwo) { throw "Unsigned setup $arch builds are not reproducible" }
        Remove-Item $setupSecond
        $setup = Join-Path $dist "AgentStack-Setup-windows-$arch.exe"
        Move-Item $setupFirst $setup
        Assert-BuildInfo $setup $revision
    }

    # Build the catalog generator for the runner, not the last cross-compiled
    # Windows/ARM64 target left by Build-Binary.
    $env:GOOS = ''
    $env:GOARCH = ''
    $env:CGO_ENABLED = '0'
    Invoke-Checked go @('run','./cmd/agentstack-sbom','--version',$Version,'--out',(Join-Path $dist 'agentstack-catalog.cdx.json'))
    foreach($arch in @('amd64','arm64')) {
        Invoke-Checked syft @((Join-Path $dist "agentstack-windows-$arch.exe"),'-o',"cyclonedx-json=$(Join-Path $dist "agentstack-binary-$arch.cdx.json")")
    }
    & govulncheck -format openvex ./... | Set-Content (Join-Path $dist 'agentstack.openvex.json') -Encoding utf8NoBOM
    if ($LASTEXITCODE -ne 0) { throw 'govulncheck OpenVEX generation failed' }
    Copy-Item supply-chain/component-licenses.json (Join-Path $dist 'component-licenses.json')
    Copy-Item README.md,LICENSE,CHANGELOG.md $dist
    Copy-Item docs (Join-Path $dist 'docs') -Recurse
    Copy-Item scripts/AgentStack-Setup.ps1 $dist
    Write-Provenance $revision

    foreach ($arch in @('amd64','arm64')) {
        $name="AgentStackManager-$Version-windows-$arch"
        $bundle=Join-Path $dist $name
        New-Item -ItemType Directory -Path $bundle | Out-Null
        Copy-Item (Join-Path $dist "agentstack-windows-$arch.exe") $bundle
        Copy-Item (Join-Path $dist "AgentStack-Setup-windows-$arch.exe") (Join-Path $bundle 'AgentStack-Setup.exe')
        Copy-Item (Join-Path $dist 'AgentStack-Setup.ps1'),(Join-Path $dist 'README.md'),(Join-Path $dist 'LICENSE'),(Join-Path $dist 'CHANGELOG.md'),(Join-Path $dist 'agentstack-catalog.cdx.json'),(Join-Path $dist "agentstack-binary-$arch.cdx.json"),(Join-Path $dist 'agentstack.openvex.json'),(Join-Path $dist 'component-licenses.json'),(Join-Path $dist 'provenance.json') $bundle
        Copy-Item (Join-Path $dist 'docs') (Join-Path $bundle 'docs') -Recurse
        Write-Checksums $bundle
        $archive=Join-Path $dist "$name.zip"
        Invoke-Checked go @('run','./cmd/releasepack','--root',$bundle,'--out',$archive,'--prefix',$name)
        Assert-Bundle $archive $arch
        Remove-Item $bundle -Recurse -Force
    }

    $sourceTar=Join-Path $dist 'source.tar'
    $sourceRoot=Join-Path $dist "AgentStackManager-$Version-source"
    New-Item -ItemType Directory -Path $sourceRoot | Out-Null
    Invoke-Checked git @('archive','--format=tar','--output',$sourceTar,$tag)
    Invoke-Checked tar @('-xf',$sourceTar,'-C',$sourceRoot)
    Remove-Item $sourceTar
    Set-Content (Join-Path $sourceRoot 'SOURCE_REVISION') "git:$revision" -Encoding utf8NoBOM
    [ordered]@{
        schemaVersion=1
        status='protected-release-candidate'
        baseRevision=$revision
        candidateRevision=$revision
        releaseTag=$tag
        repository="$env:GITHUB_SERVER_URL/$env:GITHUB_REPOSITORY"
        workflowRef=$env:GITHUB_WORKFLOW_REF
        runId=$env:GITHUB_RUN_ID
    } | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $sourceRoot 'SOURCE_PROVENANCE.json') -Encoding utf8NoBOM
    Write-SourceManifest $sourceRoot
    Invoke-Checked go @('run','./cmd/releasepack','--root',$sourceRoot,'--out',(Join-Path $dist "AgentStackManager-$Version-source.zip"),'--prefix',"AgentStackManager-$Version-source")
    Remove-Item $sourceRoot -Recurse -Force
    Remove-Item (Join-Path $dist 'README.md'),(Join-Path $dist 'LICENSE'),(Join-Path $dist 'CHANGELOG.md') -Force
    Remove-Item (Join-Path $dist 'docs') -Recurse -Force
    Write-Checksums $dist
    Assert-ReleaseOutput $dist
    Write-Host "Release candidates created at $dist" -ForegroundColor Green
}
finally {
    Pop-Location
    Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
