param(
    [string]$Version = "v0.6.0"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$metadata = go mod download -json "github.com/go-gremlins/gremlins@$Version" | ConvertFrom-Json
if ($LASTEXITCODE -ne 0 -or -not $metadata.Dir) {
    throw "Unable to download Gremlins $Version"
}

$staging = Join-Path ([System.IO.Path]::GetTempPath()) ("agentstack-gremlins-source-{0}" -f [guid]::NewGuid().ToString("N"))
try {
    Copy-Item -LiteralPath $metadata.Dir -Destination $staging -Recurse
    Get-ChildItem -LiteralPath $staging -Recurse -File | ForEach-Object { $_.IsReadOnly = $false }

    $coverageFile = Join-Path $staging "internal/coverage/coverage.go"
    $coverage = [System.IO.File]::ReadAllText($coverageFile)
    $coverageNeedle = "path, _ = filepath.Rel(c.mod.CallingDir, path)"
    if (-not $coverage.Contains($coverageNeedle)) {
        throw "Gremlins $Version coverage implementation changed; review the Windows path workaround"
    }
    $coverage = $coverage.Replace($coverageNeedle, "$coverageNeedle`n`tpath = filepath.ToSlash(path)")
    [System.IO.File]::WriteAllText($coverageFile, $coverage, [System.Text.UTF8Encoding]::new($false))

    $workdirFile = Join-Path $staging "internal/engine/workdir/workdir.go"
    $workdir = [System.IO.File]::ReadAllText($workdirFile)
    $sourceNeedle = "s, err := os.Open(srcPath)`n`tif err != nil {"
    $destinationNeedle = "d, err := os.OpenFile(dstPath, os.O_CREATE|os.O_RDWR, fileMode)`n`tif err != nil {"
    if (-not $workdir.Contains($sourceNeedle) -or -not $workdir.Contains($destinationNeedle)) {
        throw "Gremlins $Version workdir implementation changed; review the Windows file-handle workaround"
    }
    $workdir = $workdir.Replace($sourceNeedle, "s, err := os.Open(srcPath)`n`tif err != nil {")
    $workdir = $workdir.Replace("`t}`n`t//nolint:gosec // dstPath", "`t}`n`tdefer s.Close()`n`t//nolint:gosec // dstPath")
    $workdir = $workdir.Replace($destinationNeedle, "$destinationNeedle")
    $workdir = $workdir.Replace("`t}`n`n`tif _, err = io.Copy(d, s)", "`t}`n`tdefer d.Close()`n`n`tif _, err = io.Copy(d, s)")
    [System.IO.File]::WriteAllText($workdirFile, $workdir, [System.Text.UTF8Encoding]::new($false))

    Push-Location $staging
    try {
        go install ./cmd/gremlins
        if ($LASTEXITCODE -ne 0) {
            throw "Unable to install the patched Gremlins $Version command"
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    Remove-Item -LiteralPath $staging -Recurse -Force -ErrorAction SilentlyContinue
}
