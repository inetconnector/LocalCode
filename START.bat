REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal
cd /d "%~dp0"
if not exist "%~dp0dist\LocalCode.exe" (
    call "%~dp0BUILD.bat"
    if errorlevel 1 exit /b 1
)
taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
timeout /t 1 /nobreak >nul
start "LocalCode 4.5.0" "%~dp0dist\LocalCode.exe"
exit /b 0
