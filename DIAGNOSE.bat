REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
cd /d "%~dp0"
call "%~dp0BUILD.bat"
if errorlevel 1 exit /b 1
"%~dp0dist\LocalCode-Debug.exe" --diagnose
echo.
pause
exit /b %ERRORLEVEL%
