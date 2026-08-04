@echo off
setlocal
cd /d "%~dp0"
if not exist "%~dp0dist\LocalCodex.exe" (
    call "%~dp0BUILD.bat"
    if errorlevel 1 exit /b 1
)
taskkill /F /IM LocalCodex.exe >nul 2>&1
timeout /t 1 /nobreak >nul
start "LocalCodex 4.3.0" "%~dp0dist\LocalCodex.exe"
exit /b 0
