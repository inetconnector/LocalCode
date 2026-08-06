REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
cd /d "%~dp0"
taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\reset-project-root.ps1"
if errorlevel 1 (
  echo [ERROR] The project root could not be reset.
  pause
  exit /b 1
)
call "%~dp0START.bat"
exit /b %ERRORLEVEL%
