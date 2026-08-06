REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
cd /d "%~dp0"
set "VERSION=6.1.0"
set "NEEDS_BUILD=0"
set "LC_ROOT=%~dp0"

if not exist "%~dp0dist\LocalCode.exe" set "NEEDS_BUILD=1"
if "%NEEDS_BUILD%"=="0" (
    powershell.exe -NoLogo -NoProfile -Command "$root=$env:LC_ROOT; $exe=Get-Item -LiteralPath (Join-Path $root 'dist\LocalCode.exe'); $inputs=@((Join-Path $root 'src'),(Join-Path $root 'BUILD.bat'),(Join-Path $root 'VERSION.txt'),(Join-Path $root 'scripts\install-go.ps1')); $latest=Get-ChildItem -LiteralPath $inputs -Recurse -File -ErrorAction Stop | Sort-Object LastWriteTimeUtc -Descending | Select-Object -First 1; if($latest -and $latest.LastWriteTimeUtc -gt $exe.LastWriteTimeUtc){exit 1}else{exit 0}"
    if errorlevel 1 set "NEEDS_BUILD=1"
)
if "%NEEDS_BUILD%"=="1" (
    call "%~dp0BUILD.bat"
    if errorlevel 1 exit /b 1
)

taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
timeout /t 1 /nobreak >nul
start "LocalCode %VERSION%" "%~dp0dist\LocalCode.exe"
exit /b 0
