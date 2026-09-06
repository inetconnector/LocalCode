// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultInstallDir(t *testing.T) {
	dir := defaultInstallDir()
	if !strings.HasSuffix(filepath.ToSlash(dir), "Programs/LocalCode") {
		t.Errorf("expected defaultInstallDir to end in Programs/LocalCode, got %s", dir)
	}
}

func TestCopyFile(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	dst := filepath.Join(tmp, "sub", "dst.txt")

	_ = os.WriteFile(src, []byte("LocalCode Installer Test Payload"), 0o644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed reading copied file: %v", err)
	}
	if string(data) != "LocalCode Installer Test Payload" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestInstallPayloadIncludesLauncherIcon(t *testing.T) {
	files := installPayloadFiles()
	found := false
	for _, file := range files {
		if file == AppIconFile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("installer payload must include %s so shortcuts can use the LocalCode icon", AppIconFile)
	}
}

func TestInstalledIconPathRequiresExistingIcon(t *testing.T) {
	tmp := t.TempDir()
	if got := installedIconPath(tmp); got != "" {
		t.Fatalf("expected no icon before file exists, got %q", got)
	}
	iconPath := filepath.Join(tmp, AppIconFile)
	if err := os.WriteFile(iconPath, []byte("mock icon"), 0o644); err != nil {
		t.Fatalf("failed writing mock icon: %v", err)
	}
	if got := installedIconPath(tmp); got != iconPath {
		t.Fatalf("unexpected icon path: got %q want %q", got, iconPath)
	}
}

func TestPowerShellSingleQuoteEscapesEmbeddedQuotesAndApostrophes(t *testing.T) {
	got := psSingleQuote(`"C:\Users\O'Brien\App\LocalCode-Setup.exe" --uninstall`)
	want := `'"C:\Users\O''Brien\App\LocalCode-Setup.exe" --uninstall'`
	if got != want {
		t.Fatalf("unexpected PowerShell literal: got %q want %q", got, want)
	}
}

func TestInstallerSilentExecutionInTempDir(t *testing.T) {
	tmp := t.TempDir()
	srcFile := filepath.Join(tmp, "LocalCode.exe")
	_ = os.WriteFile(srcFile, []byte("mock binary"), 0o755)

	targetDir := filepath.Join(tmp, "InstalledApp")

	// Test installing into isolated directory
	if err := installFromSource(targetDir, tmp, true, false); err != nil {
		t.Fatalf("install failed: %v", err)
	}

	installedExe := filepath.Join(targetDir, "LocalCode.exe")
	if _, err := os.Stat(installedExe); err != nil {
		t.Errorf("expected %s to exist after install", installedExe)
	}
}

func TestCreateShortcut(t *testing.T) {
	tmp := t.TempDir()
	targetExe := filepath.Join(tmp, "app.exe")
	_ = os.WriteFile(targetExe, []byte("dummy exe"), 0o755)
	shortcutPath := filepath.Join(tmp, "app.lnk")
	iconPath := filepath.Join(tmp, "app.ico")
	_ = os.WriteFile(iconPath, []byte("dummy icon"), 0o644)

	err := createShortcut(targetExe, shortcutPath, "Test Description", "--test", iconPath)
	if err != nil {
		t.Logf("createShortcut returned: %v", err)
	}
}

func TestPathManagement(t *testing.T) {
	tmpDir := t.TempDir()
	if err := addToUserPath(tmpDir); err != nil {
		t.Logf("addToUserPath returned: %v", err)
	}
	if err := removeFromUserPath(tmpDir); err != nil {
		t.Logf("removeFromUserPath returned: %v", err)
	}
}

func TestRegisterAndUnregisterUninstaller(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "LocalCode.exe"), []byte("exe"), 0o755)
	_ = os.WriteFile(filepath.Join(tmpDir, "LocalCode-Setup.exe"), []byte("setup"), 0o755)

	if err := registerUninstaller(tmpDir); err != nil {
		t.Logf("registerUninstaller returned: %v", err)
	}
	if err := unregisterUninstaller(); err != nil {
		t.Logf("unregisterUninstaller returned: %v", err)
	}
}

func TestUninstallSilent(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "LocalCode.exe"), []byte("exe"), 0o755)

	if err := uninstall(tmpDir, true); err != nil {
		t.Fatalf("uninstall silent failed: %v", err)
	}
}
