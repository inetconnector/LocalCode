# SPDX-License-Identifier: Apache-2.0
param(
    [string]$Root = "",
    [int]$Port = 32146
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

if ($Port -le 0 -or $Port -gt 65535) {
    throw "Invalid Remote port: $Port"
}

if ([string]::IsNullOrWhiteSpace($Root)) {
    $Root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
} else {
    $Root = [IO.Path]::GetFullPath($Root)
}

$exe = Join-Path $Root 'dist\LocalCode.exe'
if (-not (Test-Path -LiteralPath $exe -PathType Leaf)) {
    throw "LocalCode.exe not found: $exe"
}

$principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
$isAdmin = $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    $args = @(
        '-NoLogo',
        '-NoProfile',
        '-ExecutionPolicy',
        'Bypass',
        '-File',
        $PSCommandPath,
        '-Root',
        $Root,
        '-Port',
        [string]$Port
    )
    Start-Process -FilePath 'powershell.exe' -Verb RunAs -Wait -ArgumentList $args
    exit $LASTEXITCODE
}

$name = "LocalCode Remote $Port"
$existing = Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue |
    Where-Object { $_.Enabled -eq 'True' -and $_.Direction -eq 'Inbound' -and $_.Action -eq 'Allow' } |
    Select-Object -First 1

if ($existing) {
    $portFilter = $existing | Get-NetFirewallPortFilter | Where-Object { $_.Protocol -eq 'TCP' -and $_.LocalPort -eq [string]$Port } | Select-Object -First 1
    $appFilter = $existing | Get-NetFirewallApplicationFilter | Where-Object { $_.Program -eq $exe } | Select-Object -First 1
    if ($portFilter -and $appFilter) {
        Write-Host "Firewall rule already present: $name"
        exit 0
    }
    Remove-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue
}

New-NetFirewallRule `
    -DisplayName $name `
    -Direction Inbound `
    -Action Allow `
    -Protocol TCP `
    -LocalPort $Port `
    -Profile Private `
    -RemoteAddress LocalSubnet `
    -Program $exe | Out-Null

Write-Host "Firewall rule installed: $name"
