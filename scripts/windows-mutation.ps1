param(
    [string]$Output = "mutation-windows.json"
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$config = Join-Path ([System.IO.Path]::GetTempPath()) ("agentstack-gremlins-windows-{0}.yaml" -f [guid]::NewGuid().ToString("N"))

try {
    $excluded = Get-ChildItem -Path $root -Recurse -File -Filter "*.go" |
        Where-Object { $_.Name -notmatch "_windows\.go$" } |
        ForEach-Object {
            $relative = [System.IO.Path]::GetRelativePath($root, $_.FullName).Replace("\", "/")
            "  - '^" + [regex]::Escape($relative) + "$'"
        } |
        Sort-Object

    $content = @(
        "silent: false"
        "unleash:"
        "  integration: false"
        "  workers: 2"
        "  timeout-coefficient: 3"
        "  threshold:"
        "    efficacy: 75"
        "    mutant-coverage: 65"
        "  exclude-files:"
    ) + $excluded

    [System.IO.File]::WriteAllLines($config, $content, [System.Text.UTF8Encoding]::new($false))
    Push-Location $root
    try {
        & gremlins unleash --config $config --output $Output
        if ($LASTEXITCODE -ne 0) {
            throw "Gremlins Windows mutation gate failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }
}
finally {
    Remove-Item -LiteralPath $config -Force -ErrorAction SilentlyContinue
}
