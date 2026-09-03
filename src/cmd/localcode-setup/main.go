// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

const (
	AppVersion     = "6.9.0"
	AppDisplayName = "LocalCode"
	AppPublisher   = "inetconnector"
	AppWebsite     = "https://github.com/inetconnector/LocalCode"
	DefaultDirName = "LocalCode"
)

var (
	modUser32        = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW  = modUser32.NewProc("MessageBoxW")
	modShell32       = syscall.NewLazyDLL("shell32.dll")
	procShellExecute = modShell32.NewProc("ShellExecuteW")
)

const (
	mbOK              = 0x00000000
	mbOKCancel        = 0x00000001
	mbYesNo           = 0x00000004
	mbIconInformation = 0x00000040
	mbIconQuestion    = 0x00000020
	mbIconWarning     = 0x00000030
	mbIconError       = 0x00000010
	idOK              = 1
	idCancel          = 2
	idYes             = 6
	idNo              = 7
)

func showMsgBox(title, message string, flags uint32) int {
	if runtime.GOOS != "windows" {
		fmt.Printf("[%s] %s\n", title, message)
		return idOK
	}
	pTitle, _ := syscall.UTF16PtrFromString(title)
	pMsg, _ := syscall.UTF16PtrFromString(message)
	ret, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(pMsg)), uintptr(unsafe.Pointer(pTitle)), uintptr(flags))
	return int(ret)
}

func defaultInstallDir() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		home, _ := os.UserHomeDir()
		localAppData = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localAppData, "Programs", DefaultDirName)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func createShortcut(targetPath, shortcutPath, description, arguments string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	psScript := fmt.Sprintf(`
$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut('%s')
$Shortcut.TargetPath = '%s'
$Shortcut.Arguments = '%s'
$Shortcut.Description = '%s'
$Shortcut.WorkingDirectory = '%s'
$Shortcut.Save()
`, strings.ReplaceAll(shortcutPath, "'", "''"),
		strings.ReplaceAll(targetPath, "'", "''"),
		strings.ReplaceAll(arguments, "'", "''"),
		strings.ReplaceAll(description, "'", "''"),
		strings.ReplaceAll(filepath.Dir(targetPath), "'", "''"))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	return cmd.Run()
}

func addToUserPath(dir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	psScript := fmt.Sprintf(`
$dir = '%s'
$current = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not $current) { $current = '' }
$parts = $current -split ';' | Where-Object { $_ -ne '' -and $_ -ne $dir }
$newPath = ($parts + $dir) -join ';'
[Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
`, strings.ReplaceAll(dir, "'", "''"))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	return cmd.Run()
}

func removeFromUserPath(dir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	psScript := fmt.Sprintf(`
$dir = '%s'
$current = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($current) {
    $parts = $current -split ';' | Where-Object { $_ -ne '' -and $_ -ne $dir }
    $newPath = $parts -join ';'
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
}
`, strings.ReplaceAll(dir, "'", "''"))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	return cmd.Run()
}

func registerUninstaller(installDir string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	mainExe := filepath.Join(installDir, "LocalCode.exe")
	setupExe := filepath.Join(installDir, "LocalCode-Setup.exe")

	psScript := fmt.Sprintf(`
$regPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\LocalCode"
if (-not (Test-Path $regPath)) {
    New-Item -Path $regPath -Force | Out-Null
}
Set-ItemProperty -Path $regPath -Name "DisplayName" -Value "%s"
Set-ItemProperty -Path $regPath -Name "DisplayVersion" -Value "%s"
Set-ItemProperty -Path $regPath -Name "Publisher" -Value "%s"
Set-ItemProperty -Path $regPath -Name "InstallLocation" -Value "%s"
Set-ItemProperty -Path $regPath -Name "DisplayIcon" -Value "%s"
Set-ItemProperty -Path $regPath -Name "UninstallString" -Value "\"%s\" --uninstall"
Set-ItemProperty -Path $regPath -Name "QuietUninstallString" -Value "\"%s\" --uninstall --silent"
Set-ItemProperty -Path $regPath -Name "HelpLink" -Value "%s"
Set-ItemProperty -Path $regPath -Name "URLInfoAbout" -Value "%s"
Set-ItemProperty -Path $regPath -Name "NoModify" -Value 1 -Type DWord
Set-ItemProperty -Path $regPath -Name "NoRepair" -Value 1 -Type DWord
`,
		strings.ReplaceAll(AppDisplayName, `"`, `\"`),
		strings.ReplaceAll(AppVersion, `"`, `\"`),
		strings.ReplaceAll(AppPublisher, `"`, `\"`),
		strings.ReplaceAll(installDir, `"`, `\"`),
		strings.ReplaceAll(mainExe, `"`, `\"`),
		strings.ReplaceAll(setupExe, `"`, `\"`),
		strings.ReplaceAll(setupExe, `"`, `\"`),
		strings.ReplaceAll(AppWebsite, `"`, `\"`),
		strings.ReplaceAll(AppWebsite, `"`, `\"`),
	)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	return cmd.Run()
}

func unregisterUninstaller() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	psScript := `
$regPath = "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\LocalCode"
if (Test-Path $regPath) {
    Remove-Item -Path $regPath -Recurse -Force
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	return cmd.Run()
}

func install(targetDir string, silent, launchAfter bool) error {
	return installFromSource(targetDir, "", silent, launchAfter)
}

func installFromSource(targetDir, sourceDir string, silent, launchAfter bool) error {
	var exePath string
	if sourceDir == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot resolve current installer path: %w", err)
		}
		sourceDir = filepath.Dir(exePath)
	}

	if !silent {
		resp := showMsgBox(
			"LocalCode Setup",
			fmt.Sprintf("Möchten Sie %s (Version %s) jetzt installieren?\n\nZielverzeichnis:\n%s", AppDisplayName, AppVersion, targetDir),
			mbYesNo|mbIconQuestion,
		)
		if resp != idYes {
			return nil
		}
	}

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	filesToCopy := []string{
		"LocalCode.exe",
		"LocalCode-Debug.exe",
		"START.bat",
		"FAST-START.bat",
		"README.md",
		"LICENSE",
	}

	for _, file := range filesToCopy {
		src := filepath.Join(sourceDir, file)
		if _, statErr := os.Stat(src); statErr == nil {
			dst := filepath.Join(targetDir, file)
			if err := copyFile(src, dst); err != nil {
				return fmt.Errorf("failed copying %s: %w", file, err)
			}
		}
	}

	// Also copy installer itself as LocalCode-Setup.exe / uninstaller in target directory
	targetSetup := filepath.Join(targetDir, "LocalCode-Setup.exe")
	_ = copyFile(exePath, targetSetup)

	// Create Start Menu Shortcuts
	appData := os.Getenv("APPDATA")
	if appData != "" {
		startMenuDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "LocalCode")
		_ = os.MkdirAll(startMenuDir, 0o755)

		mainExe := filepath.Join(targetDir, "LocalCode.exe")
		debugExe := filepath.Join(targetDir, "LocalCode-Debug.exe")

		_ = createShortcut(mainExe, filepath.Join(startMenuDir, "LocalCode.lnk"), "LocalCode AI Development Workstation", "")
		_ = createShortcut(debugExe, filepath.Join(startMenuDir, "LocalCode Diagnose & Debug.lnk"), "LocalCode Systemdiagnose und Debug-Konsole", "--diagnose")
	}

	// Create Desktop Shortcut
	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		desktopShortcut := filepath.Join(userProfile, "Desktop", "LocalCode.lnk")
		mainExe := filepath.Join(targetDir, "LocalCode.exe")
		_ = createShortcut(mainExe, desktopShortcut, "LocalCode AI Development Workstation", "")
	}

	// Add to User PATH
	_ = addToUserPath(targetDir)

	// Register in Windows Installed Apps
	_ = registerUninstaller(targetDir)

	if !silent {
		res := showMsgBox(
			"LocalCode Setup",
			fmt.Sprintf("%s wurde erfolgreich installiert!\n\nVerknüpfungen wurden im Startmenü und auf dem Desktop erstellt.\n\nMöchten Sie LocalCode jetzt starten?", AppDisplayName),
			mbYesNo|mbIconInformation,
		)
		if res == idYes || launchAfter {
			mainExe := filepath.Join(targetDir, "LocalCode.exe")
			_ = exec.Command(mainExe).Start()
		}
	} else if launchAfter {
		mainExe := filepath.Join(targetDir, "LocalCode.exe")
		_ = exec.Command(mainExe).Start()
	}

	return nil
}

func uninstall(installDir string, silent bool) error {
	if !silent {
		resp := showMsgBox(
			"LocalCode Deinstallation",
			fmt.Sprintf("Möchten Sie %s wirklich von Ihrem Computer entfernen?", AppDisplayName),
			mbYesNo|mbIconWarning,
		)
		if resp != idYes {
			return nil
		}
	}

	// Remove Start Menu Shortcuts
	appData := os.Getenv("APPDATA")
	if appData != "" {
		startMenuDir := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "LocalCode")
		_ = os.RemoveAll(startMenuDir)
	}

	// Remove Desktop Shortcut
	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		desktopShortcut := filepath.Join(userProfile, "Desktop", "LocalCode.lnk")
		_ = os.Remove(desktopShortcut)
	}

	// Remove from PATH
	_ = removeFromUserPath(installDir)

	// Remove Registry entry
	_ = unregisterUninstaller()

	// Self-deletion script for remaining files
	psScript := fmt.Sprintf(`
Start-Sleep -Seconds 1
$dir = '%s'
if (Test-Path $dir) {
    Get-ChildItem -Path $dir -Exclude "LocalCode-Setup.exe" -Recurse | Remove-Item -Force -Recurse -ErrorAction SilentlyContinue
    Remove-Item -Path $dir -Force -Recurse -ErrorAction SilentlyContinue
}
`, strings.ReplaceAll(installDir, "'", "''"))

	_ = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", psScript).Start()

	if !silent {
		showMsgBox(
			"LocalCode Deinstallation",
			fmt.Sprintf("%s wurde erfolgreich deinstalliert.", AppDisplayName),
			mbOK|mbIconInformation,
		)
	}

	return nil
}

func main() {
	var (
		flagSilent    bool
		flagUninstall bool
		flagLaunch    bool
		flagDir       string
	)

	flag.BoolVar(&flagSilent, "silent", false, "Run setup/uninstall silently without GUI dialogs")
	flag.BoolVar(&flagSilent, "s", false, "Run setup/uninstall silently (shorthand)")
	flag.BoolVar(&flagUninstall, "uninstall", false, "Uninstall LocalCode")
	flag.BoolVar(&flagUninstall, "u", false, "Uninstall LocalCode (shorthand)")
	flag.BoolVar(&flagLaunch, "launch", false, "Launch LocalCode immediately after setup")
	flag.StringVar(&flagDir, "dir", "", "Custom target installation directory")

	flag.Parse()

	targetDir := flagDir
	if targetDir == "" {
		targetDir = defaultInstallDir()
	}

	if flagUninstall {
		if err := uninstall(targetDir, flagSilent); err != nil {
			if !flagSilent {
				showMsgBox("LocalCode Deinstallation Fehler", err.Error(), mbOK|mbIconError)
			} else {
				fmt.Fprintln(os.Stderr, "Uninstall error:", err)
			}
			os.Exit(1)
		}
		return
	}

	if err := install(targetDir, flagSilent, flagLaunch); err != nil {
		if !flagSilent {
			showMsgBox("LocalCode Setup Fehler", err.Error(), mbOK|mbIconError)
		} else {
			fmt.Fprintln(os.Stderr, "Install error:", err)
		}
		os.Exit(1)
	}
}
