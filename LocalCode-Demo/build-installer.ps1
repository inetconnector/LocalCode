# Pac-Man Arcade Windows Installer
$ErrorActionPreference = "Stop"

$InstallDir = "$env:LOCALAPPDATA\Programs\PacmanArcade"
Write-Host "[Pac-Man Arcade Setup] Installing to: $InstallDir" -ForegroundColor Cyan

if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

$SourceDir = $PSScriptRoot
$Files = Get-ChildItem -Path $SourceDir -Exclude "build-installer.ps1", "INSTALL.bat"
foreach ($f in $Files) {
    Copy-Item -Path $f.FullName -Destination (Join-Path $InstallDir $f.Name) -Recurse -Force
}

# Create Desktop Shortcut
$WshShell = New-Object -ComObject WScript.Shell
$DesktopPath = [Environment]::GetFolderPath("Desktop")
$ShortcutPath = Join-Path $DesktopPath "Pac-Man Arcade.lnk"
$Shortcut = $WshShell.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = (Join-Path $InstallDir "start-pacman.bat")
$Shortcut.WorkingDirectory = $InstallDir
$Shortcut.Description = "Classic 1980 Pac-Man Arcade"
$Shortcut.Save()

# Create Start Menu Shortcut
$StartMenuPath = [Environment]::GetFolderPath("StartMenu")
$ProgramsPath = Join-Path $StartMenuPath "Programs"
$StartShortcutPath = Join-Path $ProgramsPath "Pac-Man Arcade.lnk"
$StartShortcut = $WshShell.CreateShortcut($StartShortcutPath)
$StartShortcut.TargetPath = (Join-Path $InstallDir "start-pacman.bat")
$StartShortcut.WorkingDirectory = $InstallDir
$StartShortcut.Description = "Classic 1980 Pac-Man Arcade"
$StartShortcut.Save()

# Uninstaller
$UninstallBat = @"
@echo off
echo Uninstalling Pac-Man Arcade...
del /q "$DesktopPath\Pac-Man Arcade.lnk" 2>nul
del /q "$StartShortcutPath" 2>nul
rmdir /s /q "$InstallDir" 2>nul
echo Done.
pause
"@
Set-Content -Path (Join-Path $InstallDir "uninstall.bat") -Value $UninstallBat

Write-Host "[Pac-Man Arcade Setup] Installation completed successfully!" -ForegroundColor Green
