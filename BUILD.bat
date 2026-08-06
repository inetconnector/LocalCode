REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
cd /d "%~dp0"
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\build.ps1"
set "BUILD_RC=%ERRORLEVEL%"
if not "%BUILD_RC%"=="0" (
    echo.
    echo ============================================================
    echo [ERROR] BUILD FAILED
    echo ============================================================
    echo The window remains open. The specific error is shown above.
    pause
)
exit /b %BUILD_RC%
