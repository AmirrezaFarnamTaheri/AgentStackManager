#requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$')][string]$Version,
    [Parameter(Mandatory)][ValidatePattern('^[0-9A-Fa-f]{40}$')][string]$CertificateThumbprint,
    [string]$TimestampUrl = 'http://timestamp.digicert.com',
    [string]$OutputDirectory = 'dist'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root $OutputDirectory
$tag = "v$Version"
$thumbprint = ($CertificateThumbprint -replace '\s','').ToUpperInvariant()

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
    Invoke-Checked git @('tag','-v',$tag)
    return $head
}
function Assert-Toolchain {
    $versionText = (go version).Trim()
    if ($versionText -notmatch '\bgo1\.26\.5\b') { throw "Release requires Go 1.26.5; found $versionText" }
    foreach ($tool in @('git','go','govulncheck','syft','signtool','tar')) {
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
function Sign-And-Verify([string]$Path) {
    Invoke-Checked signtool @('sign','/sha1',$thumbprint,'/fd','SHA256','/tr',$TimestampUrl,'/td','SHA256',$Path)
    Invoke-Checked signtool @('verify','/pa','/all','/v',$Path)
    $signature = Get-AuthenticodeSignature -LiteralPath $Path
    if ($signature.Status -ne 'Valid') { throw "Invalid Authenticode signature for $Path: $($signature.Status)" }
    if (($signature.SignerCertificate.Thumbprint -replace '\s','').ToUpperInvariant() -ne $thumbprint) { throw "Unexpected signer for $Path" }
}
function Sign-ScriptAndVerify([string]$Path) {
    $certificate=Get-Item "Cert:\CurrentUser\My\$thumbprint" -ErrorAction Stop
    $result=Set-AuthenticodeSignature -FilePath $Path -Certificate $certificate -TimestampServer $TimestampUrl -HashAlgorithm SHA256
    if ($result.Status -ne 'Valid') { throw "Unable to sign PowerShell launcher: $($result.StatusMessage)" }
    $signature=Get-AuthenticodeSignature -LiteralPath $Path
    if ($signature.Status -ne 'Valid') { throw "Invalid PowerShell launcher signature: $($signature.Status)" }
    if (($signature.SignerCertificate.Thumbprint -replace '\s','').ToUpperInvariant() -ne $thumbprint) { throw 'Unexpected PowerShell launcher signer' }
}
function Write-Checksums([string]$Directory) {
    Get-ChildItem $Directory -File | Where-Object Name -ne 'SHA256SUMS.txt' | Sort-Object Name | ForEach-Object {
        $hash = Get-FileHash -Algorithm SHA256 -LiteralPath $_.FullName
        "$($hash.Hash.ToLowerInvariant())  $($_.Name)"
    } | Set-Content (Join-Path $Directory 'SHA256SUMS.txt') -Encoding utf8NoBOM
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
        foreach($line in Get-Content -LiteralPath $manifest.FullName) {
            if ($line -notmatch '^([0-9a-fA-F]{64})\s{2}(.+)$') { throw "Invalid checksum line in $Archive`: $line" }
            $file=Join-Path $bundleRoot $Matches[2]
            if (-not (Test-Path -LiteralPath $file)) { throw "Bundle member missing: $($Matches[2])" }
            $actual=(Get-FileHash -Algorithm SHA256 -LiteralPath $file).Hash.ToLowerInvariant()
            if ($actual -ne $Matches[1].ToLowerInvariant()) { throw "Bundle digest mismatch: $($Matches[2])" }
        }
        $setup=Join-Path $bundleRoot 'AgentStack-Setup.exe'
        $console=Join-Path $bundleRoot "agentstack-windows-$Arch.exe"
        $script=Join-Path $bundleRoot 'AgentStack-Setup.ps1'
        foreach($file in @($setup,$console,$script)) {
            $signature=Get-AuthenticodeSignature -LiteralPath $file
            if ($signature.Status -ne 'Valid') { throw "Invalid bundle signature: $file" }
            if (($signature.SignerCertificate.Thumbprint -replace '\s','').ToUpperInvariant() -ne $thumbprint) { throw "Unexpected bundle signer: $file" }
        }
        foreach($required in @('provenance.json','agentstack-catalog.cdx.json',"agentstack-binary-$Arch.cdx.json",'agentstack.openvex.json','component-licenses.json')) {
            if (-not (Test-Path -LiteralPath (Join-Path $bundleRoot $required))) { throw "Bundle assurance file missing: $required" }
        }
        if ($Arch -eq 'amd64') {
            & $setup --verify-only | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Signed x64 graphical setup pair verification failed' }
            & $console version | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Signed x64 console version smoke failed' }
            & $console catalog | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Signed x64 console catalog smoke failed' }
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
    $coverage=Join-Path $dist 'coverage.out'
    Invoke-Checked go @('test',"-coverprofile=$coverage",'./...')
    & (Join-Path $root 'scripts/check-critical-coverage.sh') $coverage
    if ($LASTEXITCODE -ne 0) { throw 'Critical-path coverage gate failed' }
    Invoke-Checked govulncheck @('./...')

    $baseFlags = "-s -w -buildid= -X main.version=$Version"
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
        Sign-And-Verify $console
        $consoleHash=(Get-FileHash -Algorithm SHA256 $console).Hash.ToLowerInvariant()
        $setupFlags = "$baseFlags -X main.defaultMode=setup -X main.consoleSHA256=$consoleHash -X main.publisherThumbprint=$thumbprint -H=windowsgui"
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
        Sign-And-Verify $setup
    }

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
    Sign-ScriptAndVerify (Join-Path $dist 'AgentStack-Setup.ps1')
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
    }

    $sourceTar=Join-Path $dist 'source.tar'
    $sourceRoot=Join-Path $dist "AgentStackManager-$Version-source"
    New-Item -ItemType Directory -Path $sourceRoot | Out-Null
    Invoke-Checked git @('archive','--format=tar','--output',$sourceTar,$tag)
    Invoke-Checked tar @('-xf',$sourceTar,'-C',$sourceRoot)
    Remove-Item $sourceTar
    Invoke-Checked go @('run','./cmd/releasepack','--root',$sourceRoot,'--out',(Join-Path $dist "AgentStackManager-$Version-source.zip"),'--prefix',"AgentStackManager-$Version-source")
    Write-Checksums $dist
    Write-Host "Release candidates created at $dist" -ForegroundColor Green
}
finally {
    Pop-Location
    Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue
}
