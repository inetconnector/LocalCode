@echo off
setlocal
cd /d "%~dp0"
echo Beende alte LocalCodex-Prozesse ...
taskkill /F /IM LocalCodex.exe >nul 2>&1
timeout /t 1 /nobreak >nul
if not exist "%~dp0dist\LocalCodex.exe" (
    echo [FEHLER] dist\LocalCodex.exe fehlt. Starte zuerst BUILD-AND-RUN.bat.
    pause
    exit /b 1
)
start "LocalCodex 4.3.0" "%~dp0dist\LocalCodex.exe"
exit /b 0
