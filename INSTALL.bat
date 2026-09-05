@echo off
setlocal
cd /d "%~dp0"

if not exist "dist\LocalCode-Setup.exe" (
    echo [LocalCode] Erstelle Windows Installer ...
    powershell -NoProfile -ExecutionPolicy Bypass -File "scripts\build-installer.ps1"
    if errorlevel 1 (
        echo [FEHLER] Installer-Kompilierung fehlgeschlagen.
        pause
        exit /b 1
    )
)

start "" "dist\LocalCode-Setup.exe" %*
endlocal
