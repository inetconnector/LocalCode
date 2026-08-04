@echo off
setlocal
cd /d "%~dp0"
taskkill /F /IM LocalCodex.exe >nul 2>&1
call "%~dp0BUILD.bat"
if errorlevel 1 exit /b 1
start "LocalCodex 4.3.0" "%~dp0dist\LocalCodex.exe"
exit /b 0
