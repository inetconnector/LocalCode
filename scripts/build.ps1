# SPDX-License-Identifier: Apache-2.0
param()

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 2.0

$Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$Version = (Get-Content -LiteralPath (Join-Path $Root 'VERSION.txt') -Raw).Trim()
$Dist = Join-Path $Root 'dist'
$Source = Join-Path $Root 'src'
$TestRoot = Join-Path $env:TEMP ('LocalCode-Build-Test-' + [Guid]::NewGuid().ToString('N'))
$OriginalLocation = (Get-Location).Path
$OriginalEnvironment = @{}
$ExitCode = 0

function Save-EnvironmentVariable([string]$Name) {
    $script:OriginalEnvironment[$Name] = [Environment]::GetEnvironmentVariable($Name, 'Process')
}

function Restore-Environment {
    foreach ($entry in $script:OriginalEnvironment.GetEnumerator()) {
        [Environment]::SetEnvironmentVariable([string]$entry.Key, $entry.Value, 'Process')
    }
}

function Invoke-Native([string]$Label, [string]$Executable, [string[]]$Arguments) {
    Write-Host ''
    Write-Host $Label
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw ($Label + ' failed with exit code ' + $LASTEXITCODE)
    }
}

function Test-GoVersion([string]$GoExe) {
    try {
        $raw = (& $GoExe env GOVERSION 2>$null)
        if ($LASTEXITCODE -ne 0) { return $false }
        $match = [regex]::Match([string]$raw, '^go(\d+)\.(\d+)')
        if (-not $match.Success) { return $false }
        $major = [int]$match.Groups[1].Value
        $minor = [int]$match.Groups[2].Value
        return ($major -gt 1 -or ($major -eq 1 -and $minor -ge 25))
    } catch {
        return $false
    }
}

try {
    Write-Host ('LocalCode ' + $Version + ' native Windows build')
    Write-Host ('Project: ' + $Root)

    Get-Process -Name 'LocalCode' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
    $legacyName = 'Local' + 'Codex'
    Get-Process -Name $legacyName -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

    $GoExe = $null
    $LocalGo = Join-Path $Root '.tools\go\bin\go.exe'
    if (Test-Path -LiteralPath $LocalGo -PathType Leaf) {
        $GoExe = $LocalGo
    } else {
        $command = Get-Command 'go.exe' -ErrorAction SilentlyContinue
        if ($command) { $GoExe = $command.Source }
    }

    if (-not $GoExe -or -not (Test-GoVersion $GoExe)) {
        Write-Host '[INFO] No supported Go version was found. Installing the current official portable release.'
        & powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File (Join-Path $Root 'scripts\install-go.ps1') -Destination (Join-Path $Root '.tools\go')
        if ($LASTEXITCODE -ne 0) {
            throw ('Go installation failed with exit code ' + $LASTEXITCODE)
        }
        $GoExe = $LocalGo
    }

    if (-not (Test-Path -LiteralPath $GoExe -PathType Leaf)) {
        throw ('go.exe was not found: ' + $GoExe)
    }

    & $GoExe version
    if ($LASTEXITCODE -ne 0) { throw 'go version failed' }

    New-Item -ItemType Directory -Force -Path $Dist | Out-Null
    New-Item -ItemType Directory -Force -Path (Join-Path $TestRoot 'config'), (Join-Path $TestRoot 'cache'), (Join-Path $TestRoot 'home') | Out-Null

    foreach ($name in @('LOCALCODE_CONFIG_HOME','LOCALCODE_CACHE_HOME','LOCALCODE_USER_HOME','CGO_ENABLED','GOOS','GOARCH')) {
        Save-EnvironmentVariable $name
    }
    $env:LOCALCODE_CONFIG_HOME = Join-Path $TestRoot 'config'
    $env:LOCALCODE_CACHE_HOME = Join-Path $TestRoot 'cache'
    $env:LOCALCODE_USER_HOME = Join-Path $TestRoot 'home'
    [Environment]::SetEnvironmentVariable('CGO_ENABLED', $null, 'Process')
    [Environment]::SetEnvironmentVariable('GOOS', $null, 'Process')
    [Environment]::SetEnvironmentVariable('GOARCH', $null, 'Process')

    Set-Location -LiteralPath $Source
    Invoke-Native '[1/6] Formatting source code ...' $GoExe @('fmt','./...')
    Invoke-Native '[2/6] Running isolated tests ...' $GoExe @('test','-count=1','-timeout=240s','./...')
    Invoke-Native '[3/6] Running go vet ...' $GoExe @('vet','./...')
    Invoke-Native '[4/6] Re-running tests in randomized order ...' $GoExe @('test','-shuffle=on','-count=1','-timeout=240s','./...')

    $env:CGO_ENABLED = '0'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    Invoke-Native '[5/6] Building Windows GUI ...' $GoExe @(
        'build','-trimpath','-ldflags',('-s -w -H=windowsgui -X main.version=' + $Version),
        '-o',(Join-Path $Dist 'LocalCode.exe'),'.'
    )
    Invoke-Native '[6/6] Building diagnostics executable ...' $GoExe @(
        'build','-trimpath','-ldflags',('-X main.version=' + $Version + '-debug'),
        '-o',(Join-Path $Dist 'LocalCode-Debug.exe'),'.'
    )

    $checksums = @(
        ('LocalCode.exe  ' + (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $Dist 'LocalCode.exe')).Hash),
        ('LocalCode-Debug.exe  ' + (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $Dist 'LocalCode-Debug.exe')).Hash)
    )
    Set-Content -LiteralPath (Join-Path $Root 'CHECKSUMS-SHA256.txt') -Value $checksums -Encoding Ascii
    Remove-Item -LiteralPath (Join-Path $Dist 'REBUILD-NATIVE.txt') -Force -ErrorAction SilentlyContinue

    Write-Host ''
    Write-Host '============================================================'
    Write-Host ('[OK] BUILD SUCCEEDED - LocalCode ' + $Version)
    Write-Host '============================================================'
    Write-Host ('Application: ' + (Join-Path $Dist 'LocalCode.exe'))
    Write-Host ('Diagnostics: ' + (Join-Path $Dist 'LocalCode-Debug.exe') + ' --diagnose')
} catch {
    $ExitCode = 1
    Write-Host ''
    Write-Host ('[ERROR] ' + $_.Exception.Message) -ForegroundColor Red
} finally {
    Set-Location -LiteralPath $OriginalLocation
    Restore-Environment
    Remove-Item -LiteralPath $TestRoot -Recurse -Force -ErrorAction SilentlyContinue
}

exit $ExitCode
