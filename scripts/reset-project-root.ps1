# SPDX-License-Identifier: Apache-2.0
param()

$ErrorActionPreference = 'Stop'
$Path = Join-Path $env:APPDATA 'LocalCode\config.json'
if (Test-Path -LiteralPath $Path) {
    $Config = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
} else {
    $Config = [pscustomobject]@{}
}
$Config | Add-Member -Force NoteProperty root_project_dir (Join-Path $env:USERPROFILE 'Projekte')
$Config | Add-Member -Force NoteProperty last_project ''
$Config | Add-Member -Force NoteProperty port 32145
New-Item -ItemType Directory -Force -Path (Split-Path -Parent $Path) | Out-Null
$Json = $Config | ConvertTo-Json -Depth 20
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText($Path, $Json, $Utf8NoBom)
