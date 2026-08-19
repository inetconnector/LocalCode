//go:build windows

// SPDX-License-Identifier: Apache-2.0

package main

import (
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
