REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal
cd /d "%~dp0"
echo Beende alte LocalCode-Prozesse ...
taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
timeout /t 1 /nobreak >nul
if not exist "%~dp0dist\LocalCode.exe" (
    echo [FEHLER] dist\LocalCode.exe fehlt. Starte zuerst BUILD-AND-RUN.bat.
    pause
    exit /b 1
)
start "LocalCode 4.6.0" "%~dp0dist\LocalCode.exe"
exit /b 0
