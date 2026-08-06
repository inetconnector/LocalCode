REM SPDX-License-Identifier: Apache-2.0
@echo off
setlocal EnableExtensions
chcp 65001 >nul
cd /d "%~dp0"
set "VERSION=6.3.0"
set "LC_LANG=en"
for /f "delims=" %%L in ('powershell.exe -NoLogo -NoProfile -Command "(Get-UICulture).TwoLetterISOLanguageName" 2^>nul') do set "LC_LANG=%%L"
if /I "%LC_LANG%"=="de" (
  set "TITLE=LocalCode %VERSION% bauen"
  set "NO_GO=[INFO] Keine unterstützte Go-Version gefunden. Lade die aktuelle offizielle portable Go-Version ..."
  set "OLD_GO=[INFO] Die gefundene Go-Version wird nicht mehr unterstützt. Aktualisiere projektlokal auf die aktuelle stabile Version ..."
  set "GO_MISSING=[FEHLER] go.exe wurde nicht gefunden:"
  set "STEP1=[1/6] Formatiere Quellcode ..."
  set "STEP2=[2/6] Führe isolierte Tests aus ..."
  set "STEP3=[3/6] Führe go vet aus ..."
  set "STEP4=[4/6] Wiederhole Tests in zufälliger Reihenfolge ..."
  set "STEP5=[5/6] Baue Windows-GUI ..."
  set "STEP6=[6/6] Baue Diagnoseprogramm ..."
  set "SUCCESS=[OK] BUILD ERFOLGREICH"
  set "PROGRAM=Programm:"
  set "DIAGNOSTIC=Diagnose:"
  set "FAILED=[FEHLER] BUILD FEHLGESCHLAGEN"
  set "FAIL_HELP=Das Fenster bleibt offen. Die konkrete Meldung steht oben."
) else (
  set "TITLE=Build LocalCode %VERSION%"
  set "NO_GO=[INFO] No supported Go version was found. Downloading the current official portable Go release ..."
  set "OLD_GO=[INFO] The detected Go version is no longer supported. Updating to the current stable release inside the project ..."
  set "GO_MISSING=[ERROR] go.exe was not found:"
  set "STEP1=[1/6] Formatting source code ..."
  set "STEP2=[2/6] Running isolated tests ..."
  set "STEP3=[3/6] Running go vet ..."
  set "STEP4=[4/6] Re-running tests in randomized order ..."
  set "STEP5=[5/6] Building Windows GUI ..."
  set "STEP6=[6/6] Building diagnostics executable ..."
  set "SUCCESS=[OK] BUILD SUCCEEDED"
  set "PROGRAM=Application:"
  set "DIAGNOSTIC=Diagnostics:"
  set "FAILED=[ERROR] BUILD FAILED"
  set "FAIL_HELP=This window remains open. The specific error is shown above."
)
title %TITLE%

taskkill /F /IM LocalCode.exe >nul 2>&1
powershell.exe -NoLogo -NoProfile -Command "$n='Local'+'Codex'; Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force" >nul 2>&1

set "GOEXE="
if exist "%~dp0.tools\go\bin\go.exe" set "GOEXE=%~dp0.tools\go\bin\go.exe"
if not defined GOEXE for /f "delims=" %%G in ('where go.exe 2^>nul') do if not defined GOEXE set "GOEXE=%%G"

if defined GOEXE (
    set "LC_GOEXE=%GOEXE%"
    powershell.exe -NoLogo -NoProfile -Command "$raw=(& $env:LC_GOEXE env GOVERSION 2^>$null); $m=[regex]::Match($raw,'^go(\d+)\.(\d+)'); if(-not $m.Success){exit 1}; $major=[int]$m.Groups[1].Value; $minor=[int]$m.Groups[2].Value; if($major -gt 1 -or ($major -eq 1 -and $minor -ge 25)){exit 0}else{exit 1}"
    if errorlevel 1 (
        echo %OLD_GO%
        set "GOEXE="
    )
)

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

REM Never let build tests read or modify the real LocalCode profile.
set "LC_TEST_ROOT=%TEMP%\LocalCode-Build-Test-%RANDOM%-%RANDOM%"
set "LOCALCODE_CONFIG_HOME=%LC_TEST_ROOT%\config"
set "LOCALCODE_CACHE_HOME=%LC_TEST_ROOT%\cache"
set "LOCALCODE_USER_HOME=%LC_TEST_ROOT%\home"
mkdir "%LOCALCODE_CONFIG_HOME%" "%LOCALCODE_CACHE_HOME%" "%LOCALCODE_USER_HOME%" >nul 2>&1

pushd "%~dp0src"
echo.
echo %STEP1%
"%GOEXE%" fmt ./...
if errorlevel 1 goto :fail_popd

echo.
echo %STEP2%
"%GOEXE%" test -count=1 -timeout=180s ./...
if errorlevel 1 goto :fail_popd

echo.
echo %STEP3%
"%GOEXE%" vet ./...
if errorlevel 1 goto :fail_popd

echo.
echo %STEP4%
"%GOEXE%" test -shuffle=on -count=1 -timeout=180s ./...
if errorlevel 1 goto :fail_popd

echo.
echo %STEP5%
set "CGO_ENABLED=0"
set "GOOS=windows"
set "GOARCH=amd64"
"%GOEXE%" build -trimpath -ldflags "-s -w -H=windowsgui -X main.version=%VERSION%" -o "%~dp0dist\LocalCode.exe" .
if errorlevel 1 goto :fail_popd

echo.
echo %STEP6%
"%GOEXE%" build -trimpath -ldflags "-X main.version=%VERSION%-debug" -o "%~dp0dist\LocalCode-Debug.exe" .
if errorlevel 1 goto :fail_popd
popd
rmdir /S /Q "%LC_TEST_ROOT%" >nul 2>&1

powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -Command "$a=(Get-FileHash -Algorithm SHA256 '%~dp0dist\LocalCode.exe').Hash; $b=(Get-FileHash -Algorithm SHA256 '%~dp0dist\LocalCode-Debug.exe').Hash; @('LocalCode.exe  '+$a,'LocalCode-Debug.exe  '+$b) | Set-Content -Encoding ascii '%~dp0CHECKSUMS-SHA256.txt'"
del /Q "%~dp0dist\REBUILD-NATIVE.txt" >nul 2>&1

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
if defined LC_TEST_ROOT rmdir /S /Q "%LC_TEST_ROOT%" >nul 2>&1
echo.
echo ============================================================
echo %FAILED%
echo ============================================================
echo %FAIL_HELP%
pause
exit /b 1
