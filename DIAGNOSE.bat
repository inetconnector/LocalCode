REM SPDX-License-Identifier: Apache-2.0
﻿@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
if not exist "%~dp0dist\LocalCode-Debug.exe" (
    call "%~dp0BUILD.bat"
    if errorlevel 1 exit /b 1
)
"%~dp0dist\LocalCode-Debug.exe" --diagnose
echo.
pause
