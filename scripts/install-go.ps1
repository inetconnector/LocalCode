# SPDX-License-Identifier: Apache-2.0
param(
    [Parameter(Mandatory = $true)]
    [string]$Destination
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$isGerman = (Get-UICulture).TwoLetterISOLanguageName -eq 'de'
function L([string]$de, [string]$en) { if ($isGerman) { $de } else { $en } }

$Destination = [IO.Path]::GetFullPath($Destination)
$toolsDir = Split-Path -Parent $Destination
New-Item -ItemType Directory -Force -Path $toolsDir | Out-Null

Write-Host (L '[INFO] Ermittle die aktuelle stabile Go-Version ...' '[INFO] Determining the current stable Go version ...')
$releases = Invoke-RestMethod -Uri 'https://go.dev/dl/?mode=json' -UseBasicParsing
$release = $releases | Where-Object { $_.stable -eq $true } | Select-Object -First 1
if (-not $release) {
    throw (L 'Keine stabile Go-Version in der offiziellen Go-Downloadliste gefunden.' 'No stable Go version was found in the official Go download list.')
}

$file = $release.files | Where-Object {
    $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'archive'
} | Select-Object -First 1
if (-not $file) {
    throw (L 'Kein Windows-amd64-Archiv in der offiziellen Go-Downloadliste gefunden.' 'No Windows amd64 archive was found in the official Go download list.')
}

$zip = Join-Path $toolsDir $file.filename
$url = 'https://go.dev/dl/' + $file.filename
Write-Host ((L '[INFO] Lade {0} ...' '[INFO] Downloading {0} ...') -f $file.filename)
Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing

$actual = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLowerInvariant()
$expected = ([string]$file.sha256).ToLowerInvariant()
if ($actual -ne $expected) {
    Remove-Item -Force $zip -ErrorAction SilentlyContinue
    throw ((L 'SHA-256-Prüfung fehlgeschlagen. Erwartet: {0}, erhalten: {1}' 'SHA-256 verification failed. Expected: {0}, received: {1}') -f $expected, $actual)
}
Write-Host (L '[OK] Download-SHA-256 stimmt.' '[OK] Download SHA-256 verified.')

if (Test-Path $Destination) {
    Remove-Item -Recurse -Force $Destination
}
Expand-Archive -Path $zip -DestinationPath $toolsDir -Force
Remove-Item -Force $zip

$goExe = Join-Path $Destination 'bin\go.exe'
if (-not (Test-Path $goExe)) {
    throw ((L 'Go wurde entpackt, aber go.exe fehlt: {0}' 'Go was extracted, but go.exe is missing: {0}') -f $goExe)
}
Write-Host ((L '[OK] Portables Go installiert: {0}' '[OK] Portable Go installed: {0}') -f $goExe)
