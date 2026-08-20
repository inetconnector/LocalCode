//go:build windows

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindClawVCToolchainFindsLatestCompiler(t *testing.T) {
	programFiles := t.TempDir()
	t.Setenv("ProgramFiles", programFiles)
	t.Setenv("ProgramFiles(x86)", "")
	older := filepath.Join(programFiles, "Microsoft Visual Studio", "2022", "BuildTools", "VC", "Tools", "MSVC", "14.40", "bin", "Hostx64", "x64", "cl.exe")
	newer := filepath.Join(programFiles, "Microsoft Visual Studio", "2022", "BuildTools", "VC", "Tools", "MSVC", "14.41", "bin", "Hostx64", "x64", "cl.exe")
	for _, path := range []string{older, newer} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := findClawVCToolchain(); got != newer {
		t.Fatalf("findClawVCToolchain() = %q; want %q", got, newer)
	}
}

func TestClawVSDevCmdForCompilerUsesMatchingVisualStudioInstall(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Microsoft Visual Studio", "2022", "BuildTools")
	compiler := filepath.Join(root, "VC", "Tools", "MSVC", "14.41", "bin", "Hostx64", "x64", "cl.exe")
	devCmd := filepath.Join(root, "Common7", "Tools", "VsDevCmd.bat")
	for _, path := range []string{compiler, devCmd} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := clawVSDevCmdForCompiler(compiler); got != devCmd {
		t.Fatalf("clawVSDevCmdForCompiler() = %q; want %q", got, devCmd)
	}
	if err := os.Remove(devCmd); err != nil {
		t.Fatal(err)
	}
	if got := clawVSDevCmdForCompiler(compiler); got != "" {
		t.Fatalf("missing VsDevCmd must fail closed, got %q", got)
	}
}

func TestClawMSVCSetupSourceRequiresAuthenticodeAndVCTools(t *testing.T) {
	data, err := os.ReadFile("claw_msvc_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"Get-AuthenticodeSignature",
		"Status -ne 'Valid'",
		"Microsoft.VisualStudio.Workload.VCTools",
		"--includeRecommended",
		"defer os.Remove(bootstrapper)",
		"VsDevCmd.bat",
		"runClawCargoBuild",
		"-arch=x64",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Claw MSVC setup missing safety contract %q", required)
		}
	}
}

func writeFakeClawMSVC(t *testing.T, withDevCmd bool) (string, string) {
	t.Helper()
	programFiles := t.TempDir()
	t.Setenv("ProgramFiles", programFiles)
	t.Setenv("ProgramFiles(x86)", "")
	root := filepath.Join(programFiles, "Microsoft Visual Studio", "2026", "BuildTools")
	compiler := filepath.Join(root, "VC", "Tools", "MSVC", "14.50", "bin", "Hostx64", "x64", "cl.exe")
	if err := os.MkdirAll(filepath.Dir(compiler), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(compiler, []byte("test compiler"), 0o755); err != nil {
		t.Fatal(err)
	}
	devCmd := filepath.Join(root, "Common7", "Tools", "VsDevCmd.bat")
	if withDevCmd {
		if err := os.MkdirAll(filepath.Dir(devCmd), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(devCmd, []byte("@echo off\r\nexit /b 0\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return compiler, devCmd
}

func TestRunClawCargoBuildFailsClosedWithoutCompiler(t *testing.T) {
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("ProgramFiles(x86)", "")
	output, code, err := runClawCargoBuild(context.Background(), "cargo.exe", t.TempDir(), defaultConfig())
	if err == nil || !strings.Contains(err.Error(), "Visual C++ compiler is unavailable") {
		t.Fatalf("expected missing compiler error, got code=%d output=%q err=%v", code, output, err)
	}
	if code != -1 || output != "" {
		t.Fatalf("missing compiler result = code=%d output=%q; want -1 and empty output", code, output)
	}
}

func TestRunClawCargoBuildFailsClosedWithoutVsDevCmd(t *testing.T) {
	writeFakeClawMSVC(t, false)
	output, code, err := runClawCargoBuild(context.Background(), "cargo.exe", t.TempDir(), defaultConfig())
	if err == nil || !strings.Contains(err.Error(), "VsDevCmd.bat was not found") {
		t.Fatalf("expected missing VsDevCmd error, got code=%d output=%q err=%v", code, output, err)
	}
	if code != -1 || output != "" {
		t.Fatalf("missing VsDevCmd result = code=%d output=%q; want -1 and empty output", code, output)
	}
}

func TestRunClawCargoBuildUsesPreparedMSVCEnvironment(t *testing.T) {
	_, _ = writeFakeClawMSVC(t, true)
	cargo := filepath.Join(t.TempDir(), "fake cargo.cmd")
	if err := os.WriteFile(cargo, []byte("@echo off\r\necho cargo-ok\r\nexit /b 0\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	output, code, err := runClawCargoBuild(context.Background(), cargo, t.TempDir(), defaultConfig())
	if err != nil || code != 0 {
		t.Fatalf("prepared cargo build failed: code=%d output=%q err=%v", code, output, err)
	}
	if !strings.Contains(output, "cargo-ok") {
		t.Fatalf("cargo output = %q; want fake cargo execution", output)
	}
}

func TestVerifyMicrosoftAuthenticodeRejectsUnsignedBootstrapper(t *testing.T) {
	unsigned := filepath.Join(t.TempDir(), "unsigned.exe")
	if err := os.WriteFile(unsigned, []byte("not signed"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := verifyMicrosoftAuthenticode(context.Background(), unsigned, defaultConfig())
	if err == nil || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("unsigned bootstrapper must fail Authenticode verification, got %v", err)
	}
}

func TestEnsureClawMSVCToolchainUsesExistingVerifiedLayout(t *testing.T) {
	compiler, _ := writeFakeClawMSVC(t, true)
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	got, detail, err := ensureClawMSVCToolchain(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(got) != filepath.Clean(compiler) {
		t.Fatalf("compiler = %q; want %q", got, compiler)
	}
	if !strings.Contains(detail, "already available") {
		t.Fatalf("existing toolchain detail = %q", detail)
	}
}

func TestEnsureClawMSVCToolchainRejectsIncompleteExistingLayout(t *testing.T) {
	writeFakeClawMSVC(t, false)
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	_, _, err := ensureClawMSVCToolchain(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "VsDevCmd.bat is missing") {
		t.Fatalf("incomplete Visual C++ layout must fail closed, got %v", err)
	}
}

func TestEnsureClawMSVCToolchainHonorsDownloadPolicy(t *testing.T) {
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("ProgramFiles(x86)", "")
	cfg := defaultConfig()
	cfg.SetupDownloadsEnabled = false
	_, _, err := ensureClawMSVCToolchain(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "setup downloads are disabled") {
		t.Fatalf("disabled setup downloads must block Visual C++ installation, got %v", err)
	}
}
