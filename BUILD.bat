REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
chcp 65001 >nul
cd /d "%~dp0"
title LocalCode 4.5.0 bauen

taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1

set "VERSION=4.5.0"
set "GOEXE="
if exist "%~dp0.tools\go\bin\go.exe" set "GOEXE=%~dp0.tools\go\bin\go.exe"
if not defined GOEXE for /f "delims=" %%G in ('where go.exe 2^>nul') do if not defined GOEXE set "GOEXE=%%G"

if not defined GOEXE (
    echo [INFO] Go ist nicht installiert. Lade eine portable offizielle Go-Version ...
    powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\install-go.ps1" -Destination "%~dp0.tools\go"
    if errorlevel 1 goto :fail
    set "GOEXE=%~dp0.tools\go\bin\go.exe"
)
if not exist "%GOEXE%" (
    echo [FEHLER] go.exe wurde nicht gefunden: %GOEXE%
    goto :fail
)

"%GOEXE%" version
if errorlevel 1 goto :fail
if not exist "%~dp0dist" mkdir "%~dp0dist"

pushd "%~dp0src"
echo.
echo [1/5] Formatiere Quellcode ...
"%GOEXE%" fmt ./...
if errorlevel 1 goto :fail_popd

echo.
echo [2/5] Fuehre Tests aus ...
"%GOEXE%" test -count=1 ./...
if errorlevel 1 goto :fail_popd

echo.
echo [3/5] Fuehre go vet aus ...
"%GOEXE%" vet ./...
if errorlevel 1 goto :fail_popd

echo.
echo [4/5] Baue Windows GUI ...
set "CGO_ENABLED=0"
set "GOOS=windows"
set "GOARCH=amd64"
"%GOEXE%" build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=%VERSION%" -o "%~dp0dist\LocalCode.exe" .
if errorlevel 1 goto :fail_popd

echo.
echo [5/5] Baue Diagnoseprogramm ...
"%GOEXE%" build -trimpath -ldflags "-X main.version=%VERSION%-debug" -o "%~dp0dist\LocalCode-Debug.exe" .
if errorlevel 1 goto :fail_popd
popd

powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$a=(Get-FileHash -Algorithm SHA256 '%~dp0dist\LocalCode.exe').Hash; $b=(Get-FileHash -Algorithm SHA256 '%~dp0dist\LocalCode-Debug.exe').Hash; @('LocalCode.exe  '+$a,'LocalCode-Debug.exe  '+$b) | Set-Content -Encoding ascii '%~dp0CHECKSUMS-SHA256.txt'"

echo.
echo ============================================================
echo [OK] BUILD ERFOLGREICH - LocalCode %VERSION%
echo ============================================================
echo Programm: %~dp0dist\LocalCode.exe
echo Diagnose: %~dp0dist\LocalCode-Debug.exe --diagnose
echo.
exit /b 0

:fail_popd
popd
:fail
echo.
echo ============================================================
echo [FEHLER] BUILD FEHLGESCHLAGEN
echo ============================================================
echo Das Fenster bleibt offen. Die konkrete Meldung steht oben.
pause
exit /b 1
