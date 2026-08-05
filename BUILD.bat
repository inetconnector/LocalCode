REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
chcp 65001 >nul
cd /d "%~dp0"
set "LC_LANG=en"
for /f "delims=" %%L in ('powershell.exe -NoLogo -NoProfile -Command "(Get-UICulture).TwoLetterISOLanguageName" 2^>nul') do set "LC_LANG=%%L"
if /I "%LC_LANG%"=="de" (
  set "TITLE=LocalCode 6.0.0 bauen"
  set "NO_GO=[INFO] Go ist nicht installiert. Lade eine portable offizielle Go-Version ..."
  set "GO_MISSING=[FEHLER] go.exe wurde nicht gefunden:"
  set "STEP1=[1/5] Formatiere Quellcode ..."
  set "STEP2=[2/5] Führe Tests aus ..."
  set "STEP3=[3/5] Führe go vet aus ..."
  set "STEP4=[4/5] Baue Windows-GUI ..."
  set "STEP5=[5/5] Baue Diagnoseprogramm ..."
  set "SUCCESS=[OK] BUILD ERFOLGREICH"
  set "PROGRAM=Programm:"
  set "DIAGNOSTIC=Diagnose:"
  set "FAILED=[FEHLER] BUILD FEHLGESCHLAGEN"
  set "FAIL_HELP=Das Fenster bleibt offen. Die konkrete Meldung steht oben."
) else (
  set "TITLE=Build LocalCode 6.0.0"
  set "NO_GO=[INFO] Go is not installed. Downloading an official portable Go distribution ..."
  set "GO_MISSING=[ERROR] go.exe was not found:"
  set "STEP1=[1/5] Formatting source code ..."
  set "STEP2=[2/5] Running tests ..."
  set "STEP3=[3/5] Running go vet ..."
  set "STEP4=[4/5] Building Windows GUI ..."
  set "STEP5=[5/5] Building diagnostics executable ..."
  set "SUCCESS=[OK] BUILD SUCCEEDED"
  set "PROGRAM=Application:"
  set "DIAGNOSTIC=Diagnostics:"
  set "FAILED=[ERROR] BUILD FAILED"
  set "FAIL_HELP=This window remains open. The specific error is shown above."
)
title %TITLE%

taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1

set "VERSION=6.0.0"
set "GOEXE="
if exist "%~dp0.tools\go\bin\go.exe" set "GOEXE=%~dp0.tools\go\bin\go.exe"
if not defined GOEXE for /f "delims=" %%G in ('where go.exe 2^>nul') do if not defined GOEXE set "GOEXE=%%G"

if not defined GOEXE (
    echo %NO_GO%
    powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\install-go.ps1" -Destination "%~dp0.tools\go"
    if errorlevel 1 goto :fail
    set "GOEXE=%~dp0.tools\go\bin\go.exe"
)
if not exist "%GOEXE%" (
    echo %GO_MISSING% %GOEXE%
    goto :fail
)

"%GOEXE%" version
if errorlevel 1 goto :fail
if not exist "%~dp0dist" mkdir "%~dp0dist"

pushd "%~dp0src"
echo.
echo %STEP1%
"%GOEXE%" fmt ./...
if errorlevel 1 goto :fail_popd

echo.
echo %STEP2%
"%GOEXE%" test -count=1 ./...
if errorlevel 1 goto :fail_popd

echo.
echo %STEP3%
"%GOEXE%" vet ./...
if errorlevel 1 goto :fail_popd

echo.
echo %STEP4%
set "CGO_ENABLED=0"
set "GOOS=windows"
set "GOARCH=amd64"
"%GOEXE%" build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=%VERSION%" -o "%~dp0dist\LocalCode.exe" .
if errorlevel 1 goto :fail_popd

echo.
echo %STEP5%
"%GOEXE%" build -trimpath -ldflags "-X main.version=%VERSION%-debug" -o "%~dp0dist\LocalCode-Debug.exe" .
if errorlevel 1 goto :fail_popd
popd

powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$a=(Get-FileHash -Algorithm SHA256 '%~dp0dist\LocalCode.exe').Hash; $b=(Get-FileHash -Algorithm SHA256 '%~dp0dist\LocalCode-Debug.exe').Hash; @('LocalCode.exe  '+$a,'LocalCode-Debug.exe  '+$b) | Set-Content -Encoding ascii '%~dp0CHECKSUMS-SHA256.txt'"

echo.
echo ============================================================
echo %SUCCESS% - LocalCode %VERSION%
echo ============================================================
echo %PROGRAM% %~dp0dist\LocalCode.exe
echo %DIAGNOSTIC% %~dp0dist\LocalCode-Debug.exe --diagnose
echo.
exit /b 0

:fail_popd
popd
:fail
echo.
echo ============================================================
echo %FAILED%
echo ============================================================
echo %FAIL_HELP%
pause
exit /b 1
