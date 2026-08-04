param(
    [Parameter(Mandatory = $true)]
    [string]$Destination
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Destination = [IO.Path]::GetFullPath($Destination)
$toolsDir = Split-Path -Parent $Destination
New-Item -ItemType Directory -Force -Path $toolsDir | Out-Null

Write-Host '[INFO] Ermittle die aktuelle stabile Go-Version ...'
$releases = Invoke-RestMethod -Uri 'https://go.dev/dl/?mode=json' -UseBasicParsing
$release = $releases | Where-Object { $_.stable -eq $true } | Select-Object -First 1
if (-not $release) {
    throw 'Keine stabile Go-Version in der offiziellen Go-Downloadliste gefunden.'
}

$file = $release.files | Where-Object {
    $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'archive'
} | Select-Object -First 1
if (-not $file) {
    throw 'Kein Windows-amd64-Archiv in der offiziellen Go-Downloadliste gefunden.'
}

$zip = Join-Path $toolsDir $file.filename
$url = 'https://go.dev/dl/' + $file.filename
Write-Host ('[INFO] Lade {0} ...' -f $file.filename)
Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing

$actual = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLowerInvariant()
$expected = ([string]$file.sha256).ToLowerInvariant()
if ($actual -ne $expected) {
    Remove-Item -Force $zip -ErrorAction SilentlyContinue
    throw "SHA-256-Pruefung fehlgeschlagen. Erwartet: $expected, erhalten: $actual"
}
Write-Host '[OK] Download-SHA-256 stimmt.'

if (Test-Path $Destination) {
    Remove-Item -Recurse -Force $Destination
}
Expand-Archive -Path $zip -DestinationPath $toolsDir -Force
Remove-Item -Force $zip

$goExe = Join-Path $Destination 'bin\go.exe'
if (-not (Test-Path $goExe)) {
    throw "Go wurde entpackt, aber go.exe fehlt: $goExe"
}
Write-Host ('[OK] Portables Go installiert: {0}' -f $goExe)
