# SPDX-License-Identifier: Apache-2.0
param(
    [switch]$SkipBuild = $false
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$Dist = Join-Path $Root 'dist'
$PortalDownloads = 'C:\Users\frede\Projekte\ComputeMesh\portal\downloads'

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "LocalCode -> ComputeMesh Portal Downloads Synchronization" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

if (-not $SkipBuild) {
    Write-Host "`n[1/3] Building Windows binaries, installer, Android APK and VSIX ..." -ForegroundColor Green
    & (Join-Path $PSScriptRoot 'build.ps1')
    & (Join-Path $PSScriptRoot 'build-installer.ps1') -SkipTests
    & (Join-Path $PSScriptRoot 'build-android.ps1')
    & (Join-Path $PSScriptRoot 'package-vsix.ps1')
}

if (-not (Test-Path -LiteralPath $PortalDownloads)) {
    New-Item -ItemType Directory -Path $PortalDownloads -Force | Out-Null
}

Write-Host "`n[2/3] Staging release artifacts to ComputeMesh portal downloads directory ..." -ForegroundColor Green

$artifacts = @(
    @{ Source = (Join-Path $Dist 'LocalCode-Setup.exe'); DestName = 'LocalCode-Setup.exe' },
    @{ Source = (Join-Path $Dist 'LocalCode-Remote-debug.apk'); DestName = 'LocalCode-Remote-debug.apk' },
    @{ Source = (Join-Path $Dist 'LocalCode-Remote-debug.apk'); DestName = 'LocalCode-Remote.apk' },
    @{ Source = (Join-Path $Dist 'localcode-0.1.0.vsix'); DestName = 'localcode-0.1.0.vsix' }
)

foreach ($item in $artifacts) {
    if (-not (Test-Path -LiteralPath $item.Source)) {
        throw "Required artifact missing: $($item.Source)"
    }
    $target = Join-Path $PortalDownloads $item.DestName
    Copy-Item -LiteralPath $item.Source -Destination $target -Force
    $hash = (Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant()
    $size = (Get-Item -LiteralPath $target).Length
    Write-Host "  -> $($item.DestName) ($size bytes) [SHA256: $hash]" -ForegroundColor White
}

Write-Host "`n[3/3] Downloads staging verified successfully." -ForegroundColor Green
Write-Host "Plesk server sync target: https://mesh.inetconnector.com/#downloads" -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan
