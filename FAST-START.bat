REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
cd /d "%~dp0"

if not exist "%~dp0dist\LocalCode.exe" (
    call "%~dp0BUILD.bat"
    if errorlevel 1 exit /b 1
)

taskkill /F /IM LocalCode.exe >nul 2>&1
set "LOCALCODE_FAST_START=1"
set "LOCALCODE_SUPPRESS_FATAL_DIALOGS=1"
start "" "%~dp0dist\LocalCode.exe" %*
exit /b 0
