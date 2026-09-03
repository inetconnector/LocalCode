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
