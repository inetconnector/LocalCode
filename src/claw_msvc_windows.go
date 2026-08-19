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
	detail := strings.TrimSpace("Authenticode: "+verified+"\n$ vs_buildtools.exe "+strings.Join(args, " ")+"\n"+string(output))
	if runErr != nil {
		return "", truncateText(detail, 120000), runErr
	}
	compiler := findClawVCToolchain()
	if compiler == "" {
		return "", truncateText(detail, 120000), errors.New("Visual C++ installer completed but cl.exe was not found")
	}
	return compiler, truncateText(detail+"\nVerified compiler: "+compiler, 120000), nil
}
