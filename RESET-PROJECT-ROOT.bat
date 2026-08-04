REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal
cd /d "%~dp0"
taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$p=Join-Path $env:APPDATA 'LocalCode\config.json'; if(Test-Path $p){$c=Get-Content -Raw $p|ConvertFrom-Json}else{$c=[pscustomobject]@{}}; $c|Add-Member -Force NoteProperty root_project_dir (Join-Path $env:USERPROFILE 'Projekte'); $c|Add-Member -Force NoteProperty last_project ''; $c|Add-Member -Force NoteProperty port 32145; New-Item -ItemType Directory -Force (Split-Path $p)|Out-Null; $c|ConvertTo-Json|Set-Content -Encoding UTF8 $p"
if errorlevel 1 (
  echo [FEHLER] Projektwurzel konnte nicht zurueckgesetzt werden.
  pause
  exit /b 1
)
start "LocalCode 4.5.0" "%~dp0dist\LocalCode.exe"
exit /b 0
