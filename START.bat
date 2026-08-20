REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
cd /d "%~dp0"

powershell.exe -NoLogo -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "%~dp0scripts\needs-build.ps1" -Root "%CD%"
if errorlevel 1 (
    call "%~dp0BUILD.bat"
    if errorlevel 1 exit /b 1
)

taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -WindowStyle Hidden -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
timeout /t 1 /nobreak >nul
start "LocalCode" "%~dp0dist\LocalCode.exe"
exit /b 0
