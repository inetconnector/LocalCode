//go:build windows

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func findClawVCToolchain() string {
	var matches []string
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if strings.TrimSpace(base) == "" {
			continue
		}
		found, _ := filepath.Glob(filepath.Join(base, "Microsoft Visual Studio", "*", "*", "VC", "Tools", "MSVC", "*", "bin", "Hostx64", "x64", "cl.exe"))
		matches = append(matches, found...)
	}
	sort.Strings(matches)
	for index := len(matches) - 1; index >= 0; index-- {
		if info, err := os.Stat(matches[index]); err == nil && !info.IsDir() {
			return matches[index]
		}
	}
	return ""
}

func clawVSDevCmdForCompiler(compiler string) string {
	dir := filepath.Dir(filepath.Clean(compiler))
	for depth := 0; depth < 12; depth++ {
		if strings.EqualFold(filepath.Base(dir), "VC") {
			candidate := filepath.Join(filepath.Dir(dir), "Common7", "Tools", "VsDevCmd.bat")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func clawBatchQuotedPath(path string) (string, error) {
	if strings.ContainsAny(path, "\x00\r\n\"") {
		return "", errors.New("Windows build path contains unsupported command characters")
	}
	// Batch files expand percent-delimited environment variables even inside
	// quotes. Doubling percent signs keeps an unusual but valid path literal.
	return `"` + strings.ReplaceAll(path, "%", "%%") + `"`, nil
}

func writeClawCargoBuildScript(rustRoot, devCmd, cargoPath string) (string, error) {
	quotedDevCmd, err := clawBatchQuotedPath(devCmd)
	if err != nil {
		return "", err
	}
	quotedCargo, err := clawBatchQuotedPath(cargoPath)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp(rustRoot, ".localcode-claw-build-*.cmd")
	if err != nil {
		return "", err
	}
	path := file.Name()
	content := "@echo off\r\n" +
		"call " + quotedDevCmd + " -arch=x64 -host_arch=x64 >nul\r\n" +
		"if errorlevel 1 exit /b %errorlevel%\r\n" +
		quotedCargo + " build --workspace --release\r\n"
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func runClawCargoBuild(ctx context.Context, cargoPath, rustRoot string, cfg Config) (string, int, error) {
	compiler := findClawVCToolchain()
	if compiler == "" {
		return "", -1, errors.New("Visual C++ compiler is unavailable for the Windows Claw build")
	}
	devCmd := clawVSDevCmdForCompiler(compiler)
	if devCmd == "" {
		return "", -1, errors.New("VsDevCmd.bat was not found for the Visual C++ toolchain")
	}
	script, err := writeClawCargoBuildScript(rustRoot, devCmd, cargoPath)
	if err != nil {
		return "", -1, err
	}
	defer os.Remove(script)
	// Execute a relative temporary batch-file name. This deliberately avoids
	// passing a nested quoted command through Go's Windows argv quoting and
	// cmd.exe /S /C, which can turn quotes into literal backslash-quote pairs.
	return runCapturedCommand(ctx, "cmd.exe", []string{"/d", "/s", "/c", filepath.Base(script)}, commandEnvironment(cfg), rustRoot)
}

func verifyMicrosoftAuthenticode(ctx context.Context, path string, cfg Config) (string, error) {
	script := "$s=Get-AuthenticodeSignature -LiteralPath " + quotePowerShellLiteral(path) + "; " +
		"if($s.Status -ne 'Valid' -or -not $s.SignerCertificate -or $s.SignerCertificate.Subject -notmatch 'Microsoft'){Write-Error 'invalid Microsoft Authenticode signature'; exit 23}; " +
		"$s.SignerCertificate.Subject"
	output, code, err := runCapturedCommand(ctx, "powershell.exe", []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}, commandEnvironment(cfg), filepath.Dir(path))
	if err != nil || code != 0 {
		return strings.TrimSpace(output), fmt.Errorf("Visual Studio Build Tools signature verification failed with exit code %d: %w", code, err)
	}
	return strings.TrimSpace(output), nil
}

func ensureClawMSVCToolchain(ctx context.Context, cfg Config) (string, string, error) {
	if existing := findClawVCToolchain(); existing != "" {
		if clawVSDevCmdForCompiler(existing) == "" {
			return "", "", errors.New("Visual C++ compiler was found but VsDevCmd.bat is missing")
		}
		return existing, "Visual C++ toolchain already available: " + existing, nil
	}
	if !cfg.SetupDownloadsEnabled {
		return "", "", errors.New("Visual C++ Build Tools are required for the Windows Claw build and setup downloads are disabled")
	}
	bootstrapper := filepath.Join(appDataDir(), "downloads", "claw-vs_buildtools.exe")
	if err := downloadToFile(ctx, vsBuildToolsURL, bootstrapper); err != nil {
		return "", "Download: " + vsBuildToolsURL, err
	}
	defer os.Remove(bootstrapper)
	verified, err := verifyMicrosoftAuthenticode(ctx, bootstrapper, cfg)
	if err != nil {
		return "", verified, err
	}
	args := []string{
		"--quiet", "--wait", "--norestart", "--nocache",
		"--add", "Microsoft.VisualStudio.Workload.VCTools",
		"--includeRecommended",
	}
	cmd := exec.CommandContext(ctx, bootstrapper, args...)
	hideCommandWindow(cmd)
	cmd.Env = commandEnvironment(cfg)
	output, runErr := cmd.CombinedOutput()
	detail := strings.TrimSpace("Authenticode: " + verified + "\n$ vs_buildtools.exe " + strings.Join(args, " ") + "\n" + string(output))
	if runErr != nil {
		return "", truncateText(detail, 120000), runErr
	}
	compiler := findClawVCToolchain()
	if compiler == "" {
		return "", truncateText(detail, 120000), errors.New("Visual C++ installer completed but cl.exe was not found")
	}
	if clawVSDevCmdForCompiler(compiler) == "" {
		return "", truncateText(detail, 120000), errors.New("Visual C++ installer completed but VsDevCmd.bat was not found")
	}
	return compiler, truncateText(detail+"\nVerified compiler: "+compiler, 120000), nil
}
