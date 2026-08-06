# SPDX-License-Identifier: Apache-2.0
param(
    [Parameter(Mandatory = $true)]
    [string]$Root
)

$ErrorActionPreference = 'Stop'
try {
    $Root = [IO.Path]::GetFullPath($Root)
    $Exe = Join-Path $Root 'dist\LocalCode.exe'
    $Marker = Join-Path $Root 'dist\REBUILD-NATIVE.txt'
    if (-not (Test-Path -LiteralPath $Exe -PathType Leaf)) { exit 1 }
    if (Test-Path -LiteralPath $Marker -PathType Leaf) { exit 1 }

    $Inputs = @(
        (Join-Path $Root 'src'),
        (Join-Path $Root 'BUILD.bat'),
        (Join-Path $Root 'VERSION.txt'),
        (Join-Path $Root 'scripts\build.ps1'),
        (Join-Path $Root 'scripts\install-go.ps1'),
        (Join-Path $Root 'scripts\needs-build.ps1')
    )
    $Latest = Get-ChildItem -LiteralPath $Inputs -Recurse -File -ErrorAction Stop |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1
    $ExeInfo = Get-Item -LiteralPath $Exe -ErrorAction Stop
    if ($Latest -and $Latest.LastWriteTimeUtc -gt $ExeInfo.LastWriteTimeUtc) { exit 1 }
    exit 0
} catch {
    Write-Host ('[INFO] Rebuild check failed; a safe rebuild will be attempted: ' + $_.Exception.Message)
    exit 1
}
