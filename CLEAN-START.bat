REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
set "LC_LANG=en"
for /f "delims=" %%L in ('powershell.exe -NoLogo -NoProfile -Command "(Get-UICulture).TwoLetterISOLanguageName" 2^>nul') do set "LC_LANG=%%L"
if /I "%LC_LANG%"=="de" (set "STOP=Beende alte LocalCode-Prozesse ..."&set "MISSING=[FEHLER] dist\LocalCode.exe fehlt.") else (set "STOP=Stopping old LocalCode processes ..."&set "MISSING=[ERROR] dist\LocalCode.exe is missing.")
echo %STOP%
taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1
timeout /t 1 /nobreak >nul
if not exist "%~dp0dist\LocalCode.exe" goto :rebuild
if exist "%~dp0dist\REBUILD-NATIVE.txt" goto :rebuild
goto :after_rebuild

:rebuild
call "%~dp0BUILD.bat"
if errorlevel 1 exit /b 1

:after_rebuild
start "LocalCode 6.3.0" "%~dp0dist\LocalCode.exe"
exit /b 0
