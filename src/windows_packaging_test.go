// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsBatchLaunchersAreASCIIAndCRLF(t *testing.T) {
	root := filepath.Clean("..")
	names := []string{
		"START.bat",
		"BUILD.bat",
		"BUILD-AND-RUN.bat",
		"CLEAN-START.bat",
		"DIAGNOSE.bat",
		"RESET-PROJECT-ROOT.bat",
	}
	for _, name := range names {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) || bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
			t.Errorf("%s must not contain a BOM", name)
		}
		for i, b := range data {
			if b >= 0x80 {
				t.Errorf("%s contains non-ASCII byte 0x%02x at offset %d", name, b, i)
				break
			}
			if b == '\n' && (i == 0 || data[i-1] != '\r') {
				t.Errorf("%s contains a bare LF at offset %d", name, i)
				break
			}
		}
		if !bytes.Contains(data, []byte("\r\n")) {
			t.Errorf("%s does not contain CRLF line endings", name)
		}
	}
}

func TestStartBatchTracksPowerShellBuildDriver(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "START.bat"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(data))
	if !strings.Contains(text, `scripts\needs-build.ps1`) {
		t.Fatal("START.bat does not use scripts\\needs-build.ps1")
	}
	checker, err := os.ReadFile(filepath.Join("..", "scripts", "needs-build.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(string(checker)), `scripts\build.ps1`) {
		t.Fatal("scripts\\needs-build.ps1 does not track scripts\\build.ps1")
	}
}

func TestBuildBatchUsesPowerShellDriver(t *testing.T) {
	batch, err := os.ReadFile(filepath.Join("..", "BUILD.bat"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(batch))
	if !strings.Contains(text, `scripts\build.ps1`) {
		t.Fatal("BUILD.bat does not delegate to scripts\\build.ps1")
	}
	driver, err := os.ReadFile(filepath.Join("..", "scripts", "build.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	driverText := string(driver)
	for _, required := range []string{
		"Running isolated tests",
		"Running go vet",
		"Re-running tests in randomized order",
		"Building Windows GUI",
		"LocalCode.exe",
		"LocalCode-Debug.exe",
	} {
		if !strings.Contains(driverText, required) {
			t.Errorf("build driver is missing %q", required)
		}
	}
}

func TestWindowsWrapperLaunchersDelegateToCurrentBuildChecks(t *testing.T) {
	cases := map[string]string{
		"CLEAN-START.bat":        "START.bat",
		"RESET-PROJECT-ROOT.bat": "START.bat",
		"DIAGNOSE.bat":           "BUILD.bat",
	}
	for name, target := range cases {
		data, err := os.ReadFile(filepath.Join("..", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.ToLower(string(data)), strings.ToLower(target)) {
			t.Errorf("%s does not delegate to %s", name, target)
		}
	}
}

func TestWindowsPowerShellScriptEncodings(t *testing.T) {
	for _, name := range []string{"build.ps1", "needs-build.ps1", "reset-project-root.ps1"} {
		data, err := os.ReadFile(filepath.Join("..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
			t.Fatalf("scripts/%s is ASCII-only and must not need a BOM", name)
		}
		for i, b := range data {
			if b >= 0x80 {
				t.Fatalf("scripts/%s contains non-ASCII byte 0x%02x at offset %d", name, b, i)
			}
			if b == '\n' && (i == 0 || data[i-1] != '\r') {
				t.Fatalf("scripts/%s contains bare LF at offset %d", name, i)
			}
		}
	}

	installer, err := os.ReadFile(filepath.Join("..", "scripts", "install-go.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(installer, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("scripts/install-go.ps1 needs a UTF-8 BOM for Windows PowerShell 5.1")
	}
	for i, b := range installer {
		if b == '\n' && (i == 0 || installer[i-1] != '\r') {
			t.Fatalf("scripts/install-go.ps1 contains bare LF at offset %d", i)
		}
	}
}
