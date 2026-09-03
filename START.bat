REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
cd /d "%~dp0"

set "START_LOG_DIR=%~dp0logs"
set "START_LOG=%START_LOG_DIR%\start.log"
if not exist "%START_LOG_DIR%" mkdir "%START_LOG_DIR%" >nul 2>&1
>>"%START_LOG%" echo [%DATE% %TIME%] START requested in %CD%

powershell.exe -NoLogo -NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File "%~dp0scripts\needs-build.ps1" -Root "%CD%" -LogPath "%START_LOG%"
if errorlevel 1 (
    >>"%START_LOG%" echo [%DATE% %TIME%] Build required.
    call "%~dp0BUILD.bat"
    if errorlevel 1 (
        >>"%START_LOG%" echo [%DATE% %TIME%] Build failed with exit code %ERRORLEVEL%.
        exit /b 1
    )
) else (
    >>"%START_LOG%" echo [%DATE% %TIME%] Build is current.
)

taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -WindowStyle Hidden -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
timeout /t 1 /nobreak >nul
set "LOCALCODE_FAST_START=1"
set "LOCALCODE_SUPPRESS_FATAL_DIALOGS=1"
>>"%START_LOG%" echo [%DATE% %TIME%] Launching LocalCode fast startup.
start "LocalCode" "%~dp0dist\LocalCode.exe"
exit /b 0
