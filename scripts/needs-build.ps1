# SPDX-License-Identifier: Apache-2.0
param(
    [Parameter(Mandatory = $true)]
    [string]$Root,
    [string]$LogPath = ''
)

$ErrorActionPreference = 'Stop'

function Write-CheckLog([string]$Message) {
    if ([string]::IsNullOrWhiteSpace($LogPath)) { return }
    try {
        $parent = Split-Path -Parent $LogPath
        if ($parent) { New-Item -ItemType Directory -Force -Path $parent | Out-Null }
        Add-Content -LiteralPath $LogPath -Value ('[{0:yyyy-MM-dd HH:mm:ss.fff}] needs-build: {1}' -f (Get-Date), $Message) -Encoding Ascii
    } catch {
    }
}

function ConvertTo-SHA256Hex([string]$Text) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try {
        $bytes = [Text.Encoding]::UTF8.GetBytes($Text)
        return ([BitConverter]::ToString($sha.ComputeHash($bytes)) -replace '-', '').ToUpperInvariant()
    } finally {
        $sha.Dispose()
    }
}

function Get-GitSourceState([string]$Root, [string[]]$Paths) {
    $git = Get-Command 'git.exe' -ErrorAction SilentlyContinue
    if (-not $git) { $git = Get-Command 'git' -ErrorAction SilentlyContinue }
    if (-not $git) { return $null }
    $head = (& $git.Source -C $Root rev-parse HEAD 2>$null)
    if ($LASTEXITCODE -ne 0) { return $null }
    $args = @('-C', $Root, 'status', '--porcelain=v1', '--untracked-files=all', '--')
    foreach ($path in $Paths) { $args += $path }
    $status = (& $git.Source @args 2>$null) -join "`n"
    if ($LASTEXITCODE -ne 0) { return $null }
    return [pscustomobject]@{
        Head = ([string]$head).Trim()
        StatusSHA256 = ConvertTo-SHA256Hex $status
        Dirty = -not [string]::IsNullOrWhiteSpace($status)
    }
}

try {
    $Root = [IO.Path]::GetFullPath($Root)
    $Exe = Join-Path $Root 'dist\LocalCode.exe'
    $Marker = Join-Path $Root 'dist\REBUILD-NATIVE.txt'
    $StatePath = Join-Path $Root 'dist\build-state.json'
    $TrackedPaths = @(
        'src',
        'BUILD.bat',
        'VERSION.txt',
        'scripts\build.ps1',
        'scripts\install-go.ps1',
        'scripts\needs-build.ps1'
    )
    if (-not (Test-Path -LiteralPath $Exe -PathType Leaf)) {
        Write-CheckLog 'dist\LocalCode.exe missing'
        exit 1
    }
    if (Test-Path -LiteralPath $Marker -PathType Leaf) {
        Write-CheckLog 'dist\REBUILD-NATIVE.txt present'
        exit 1
    }

    if (Test-Path -LiteralPath $StatePath -PathType Leaf) {
        $state = Get-Content -LiteralPath $StatePath -Raw | ConvertFrom-Json
        $gitState = Get-GitSourceState $Root $TrackedPaths
        if ($gitState -and $state.git_head -and $state.git_status_sha256) {
            if ($state.git_head -eq $gitState.Head -and $state.git_status_sha256 -eq $gitState.StatusSHA256) {
                if (-not $gitState.Dirty) {
                    Write-CheckLog 'fast git fingerprint matched'
                    exit 0
                }
                Write-CheckLog 'dirty git fingerprint matched; verifying file times'
            } else {
                Write-CheckLog 'git fingerprint changed'
                exit 1
            }
        }
        Write-CheckLog 'build-state exists but git fingerprint is unavailable'
    } else {
        Write-CheckLog 'build-state missing; using filesystem fallback'
    }

    $Inputs = @()
    foreach ($path in $TrackedPaths) {
        $Inputs += (Join-Path $Root $path)
    }
    $Latest = Get-ChildItem -LiteralPath $Inputs -Recurse -File -ErrorAction Stop |
        Sort-Object LastWriteTimeUtc -Descending |
        Select-Object -First 1
    $ExeInfo = Get-Item -LiteralPath $Exe -ErrorAction Stop
    if ($Latest -and $Latest.LastWriteTimeUtc -gt $ExeInfo.LastWriteTimeUtc) {
        Write-CheckLog ('newer input: ' + $Latest.FullName)
        exit 1
    }
    Write-CheckLog 'filesystem fallback matched'
    exit 0
} catch {
    Write-CheckLog ('check failed; rebuild will be attempted: ' + $_.Exception.Message)
    exit 1
}
