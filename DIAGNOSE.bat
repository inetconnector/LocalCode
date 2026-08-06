REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
if not exist "%~dp0dist\LocalCode-Debug.exe" goto :rebuild
if exist "%~dp0dist\REBUILD-NATIVE.txt" goto :rebuild
goto :after_rebuild

:rebuild
call "%~dp0BUILD.bat"
if errorlevel 1 exit /b 1

:after_rebuild
"%~dp0dist\LocalCode-Debug.exe" --diagnose
echo.
pause
