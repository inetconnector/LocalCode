@echo off
setlocal
chcp 65001 >nul
cd /d "%~dp0"
if not exist "%~dp0dist\LocalCodex-Debug.exe" (
    call "%~dp0BUILD.bat"
    if errorlevel 1 exit /b 1
)
"%~dp0dist\LocalCodex-Debug.exe" --diagnose
echo.
pause
