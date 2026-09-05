# SPDX-License-Identifier: Apache-2.0
param(
    [switch]$SkipTests = $false
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$Dist = Join-Path $Root 'dist'
$Source = Join-Path $Root 'src'
$Assets = Join-Path $Root 'assets'

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "Building LocalCode Windows Installer Package" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

# 1. Build main binaries if missing
$mainExe = Join-Path $Dist 'LocalCode.exe'
$debugExe = Join-Path $Dist 'LocalCode-Debug.exe'

if (-not (Test-Path $mainExe) -or -not (Test-Path $debugExe)) {
    Write-Host "Main binaries missing in dist. Running build.ps1 ..." -ForegroundColor Yellow
    & (Join-Path $PSScriptRoot 'build.ps1')
}

# 2. Stage launcher icon and compile native Go Setup Installer
$iconSource = Join-Path $Assets 'localcode.ico'
$iconOut = Join-Path $Dist 'localcode.ico'
if (-not (Test-Path -LiteralPath $iconSource)) {
    throw "Installer icon missing: $iconSource"
}
Copy-Item -LiteralPath $iconSource -Destination $iconOut -Force

$setupResourceOut = Join-Path $Source 'cmd\localcode-setup\rsrc_windows_amd64.syso'
$windres = Get-Command windres.exe -ErrorAction SilentlyContinue
if (-not $windres) {
    $windresCandidates = @(
        'C:\msys64\ucrt64\bin\windres.exe',
        'C:\msys64\mingw64\bin\windres.exe',
        'C:\msys64\usr\bin\windres.exe'
    )
    foreach ($cand in $windresCandidates) {
        if (Test-Path -LiteralPath $cand) {
            $windres = Get-Item -LiteralPath $cand
            break
        }
    }
}

if ($windres) {
    $windresPath = $null
    if ($windres.PSObject.Properties.Name -contains 'Source') {
        $windresPath = $windres.Source
    }
    if (-not $windresPath -and $windres.PSObject.Properties.Name -contains 'Path') {
        $windresPath = $windres.Path
    }
    if (-not $windresPath -and $windres.PSObject.Properties.Name -contains 'FullName') {
        $windresPath = $windres.FullName
    }
    if (-not $windresPath) {
        throw "Could not resolve windres.exe path"
    }
    $tempRc = Join-Path ([IO.Path]::GetTempPath()) ('localcode-setup-' + [Guid]::NewGuid().ToString('N') + '.rc')
    try {
        $escapedIconSource = $iconSource -replace '\\', '/'
        Set-Content -LiteralPath $tempRc -Value "1 ICON `"$escapedIconSource`"" -Encoding ASCII
        & $windresPath -O coff -o $setupResourceOut $tempRc
        if ($LASTEXITCODE -ne 0) {
            throw "Failed to generate setup icon resource"
        }
    }
    finally {
        Remove-Item -LiteralPath $tempRc -Force -ErrorAction SilentlyContinue
    }
} else {
    $fallbackResource = Join-Path $Source 'rsrc_windows_amd64.syso'
    if (-not (Test-Path -LiteralPath $fallbackResource)) {
        throw "windres.exe not found and fallback Windows icon resource is missing: $fallbackResource"
    }
    Copy-Item -LiteralPath $fallbackResource -Destination $setupResourceOut -Force
}

Write-Host "  -> Staged launcher icon: $iconOut" -ForegroundColor Green
Write-Host "  -> Prepared setup icon resource: $setupResourceOut" -ForegroundColor Green

Write-Host "`n[1/2] Compiling native Windows Setup (LocalCode-Setup.exe) ..." -ForegroundColor Green
$setupSource = Join-Path $Source 'cmd\localcode-setup\main.go'
$setupOut = Join-Path $Dist 'LocalCode-Setup.exe'

Push-Location $Source
try {
    & go build -ldflags="-H=windowsgui -s -w" -o $setupOut $setupSource
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to build LocalCode-Setup.exe"
    }
}
finally {
    Pop-Location
}

Write-Host "  -> Generated: $setupOut" -ForegroundColor Green

# 3. Check for Inno Setup compiler (ISCC.exe)
Write-Host "`n[2/2] Checking Inno Setup compiler (optional) ..." -ForegroundColor Green
$iscc = Get-Command ISCC.exe -ErrorAction SilentlyContinue
if (-not $iscc) {
    $candidates = @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 5\ISCC.exe"
    )
    foreach ($cand in $candidates) {
        if (Test-Path $cand) {
            $iscc = Get-Item $cand
            break
        }
    }
}

if ($iscc) {
    Write-Host "  -> Running Inno Setup compiler: $($iscc.FullName)" -ForegroundColor Green
    $issFile = Join-Path $Root 'installer\localcode-setup.iss'
    & $iscc.FullName $issFile
} else {
    Write-Host "  -> Inno Setup (ISCC.exe) not in PATH (native LocalCode-Setup.exe is ready)." -ForegroundColor Gray
}

Write-Host "`n============================================================" -ForegroundColor Cyan
Write-Host "[OK] INSTALLER BUILD SUCCEEDED" -ForegroundColor Cyan
Write-Host "Setup Installer: $setupOut" -ForegroundColor White
Write-Host "============================================================" -ForegroundColor Cyan
