// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type ToolNotFoundError struct {
	Info   ToolInfo
	Detail string
}

func (e *ToolNotFoundError) Error() string {
	if e == nil {
		return "tool not found"
	}
	return "tool not found: " + e.Info.Name
}

func toolInstallSupported(name string) bool {
	p := profileForTool(name)
	return runtime.GOOS == "windows" && strings.TrimSpace(p.InstallKind) != ""
}

func toolInstallPreview(name string, cfg Config) string {
	p := profileForTool(name)
	switch p.InstallKind {
	case "android-platform-tools":
		return localizeConfigText(cfg, "Installiert die offiziellen Android SDK Platform-Tools (adb/fastboot) benutzerlokal unter LocalCode\\tools. Keine Administratorrechte erforderlich. Mit der Genehmigung bestätigst du, dass du die auf der offiziellen Android-Downloadseite angezeigten SDK-Lizenzbedingungen akzeptierst.", "Installs the official Android SDK Platform-Tools (adb/fastboot) for the current user under LocalCode\\tools. Administrator rights are not required. By approving, you confirm acceptance of the SDK license terms shown on the official Android download page.")
	case "mingit":
		return localizeConfigText(cfg, "Installiert die offizielle portable MinGit-Ausgabe benutzerlokal unter LocalCode\\tools. Falls verfügbar, kann alternativ Git for Windows über WinGet installiert werden.", "Installs the official portable MinGit distribution for the current user under LocalCode\\tools. If available, Git for Windows can be installed through WinGet as a fallback.")
	case "dotnet-sdk":
		return localizeConfigText(cfg, "Installiert das passende .NET SDK benutzerlokal mit dem offiziellen dotnet-install.ps1. Wenn global.json vorhanden ist, wird dessen SDK-Version verwendet; andernfalls der aktuelle LTS-Kanal.", "Installs the appropriate .NET SDK for the current user with the official dotnet-install.ps1 script. If global.json exists, its SDK version is used; otherwise the current LTS channel is installed.")
	case "vs-build-tools":
		return localizeConfigText(cfg, "Installiert die offiziellen Visual Studio Build Tools mit dem Workload Microsoft.VisualStudio.Workload.MSBuildTools. Die Installation benötigt Administratorrechte, kann mehrere Gigabyte herunterladen und zeigt gegebenenfalls eine UAC-Abfrage.", "Installs the official Visual Studio Build Tools with the Microsoft.VisualStudio.Workload.MSBuildTools workload. Installation requires administrator rights, may download several gigabytes, and can display a UAC prompt.")
	case "winget":
		return localizeConfigText(cfg, fmt.Sprintf("Installiert %s über Windows Package Manager (Paket %s). Windows kann dafür eine UAC-Bestätigung anzeigen.", p.DisplayName, p.WingetID), fmt.Sprintf("Installs %s through Windows Package Manager (package %s). Windows may display a UAC confirmation.", p.DisplayName, p.WingetID))
	default:
		return localizeConfigText(cfg, "Für dieses Werkzeug ist keine sichere automatische Installation hinterlegt.", "No safe automatic installer is configured for this tool.")
	}
}

func installKnownTool(ctx context.Context, project, name string, cfg Config) (Config, string, error) {
	profile := profileForTool(name)
	if runtime.GOOS != "windows" {
		return cfg, "", errors.New("automatic tool installation is currently supported on Windows only")
	}
	if cfg.ToolOverrides == nil {
		cfg.ToolOverrides = map[string]string{}
	}
	switch profile.InstallKind {
	case "android-platform-tools":
		root, out, err := installAndroidPlatformTools(ctx)
		if err != nil {
			return cfg, out, err
		}
		adb := filepath.Join(root, "platform-tools", "adb.exe")
		fastboot := filepath.Join(root, "platform-tools", "fastboot.exe")
		if _, err := os.Stat(adb); err != nil {
			return cfg, out, fmt.Errorf("adb.exe missing after extraction: %w", err)
		}
		cfg.ToolOverrides["adb"] = adb
		if _, err := os.Stat(fastboot); err == nil {
			cfg.ToolOverrides["fastboot"] = fastboot
		}
		return cfg, out + localizeConfigText(cfg, "\nADB-Pfad: ", "\nADB path: ") + adb, nil
	case "mingit":
		git, out, err := installPortableGit(ctx)
		if err != nil {
			// WinGet is a useful fallback when GitHub download is temporarily unavailable.
			if profile.WingetID != "" {
				wingetOut, wingetErr := installWithWinget(ctx, profile.WingetID)
				out += localizeConfigText(cfg, "\n\nPortable Installation fehlgeschlagen; WinGet-Fallback:\n", "\n\nPortable installation failed; WinGet fallback:\n") + wingetOut
				if wingetErr == nil {
					return cfg, out, nil
				}
				return cfg, out, fmt.Errorf("portable Git failed: %v; winget failed: %w", err, wingetErr)
			}
			return cfg, out, err
		}
		cfg.ToolOverrides["git"] = git
		return cfg, out + localizeConfigText(cfg, "\nGit-Pfad: ", "\nGit path: ") + git, nil
	case "dotnet-sdk":
		dotnet, out, err := installDotnetSDK(ctx, project)
		if err != nil {
			return cfg, out, err
		}
		cfg.ToolOverrides["dotnet"] = dotnet
		return cfg, out + localizeConfigText(cfg, "\n.NET-Pfad: ", "\n.NET path: ") + dotnet, nil
	case "vs-build-tools":
		msbuild, out, err := installVisualStudioBuildTools(ctx)
		if err != nil {
			return cfg, out, err
		}
		cfg.ToolOverrides["msbuild"] = msbuild
		return cfg, out + localizeConfigText(cfg, "\nMSBuild-Pfad: ", "\nMSBuild path: ") + msbuild, nil
	case "winget":
		if profile.WingetID == "" {
			return cfg, "", errors.New("winget package id is missing")
		}
		out, err := installWithWinget(ctx, profile.WingetID)
		if err == nil {
			info := discoverTool(project, profile.Name, cfg, false)
			if info.Available {
				cfg.ToolOverrides[profile.Name] = info.Path
				out += localizeConfigText(cfg, "\nVerifizierter Pfad: ", "\nVerified path: ") + info.Path
			}
		}
		return cfg, out, err
	default:
		return cfg, "", fmt.Errorf("no automatic installer is configured for %s", profile.DisplayName)
	}
}

func localToolDir(name string) string {
	return filepath.Join(appDataDir(), "tools", name)
}

func downloadToFile(ctx context.Context, rawURL, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "LocalCode/4.8 tool-installer")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	tmp := target + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(resp.Body, 2<<30))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(target)
	return os.Rename(tmp, target)
}

func extractZipSafe(archive, destination string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	cleanRoot, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		name := filepath.Clean(filepath.FromSlash(f.Name))
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || name == ".." {
			return fmt.Errorf("unsafe zip entry: %s", f.Name)
		}
		target := filepath.Join(cleanRoot, name)
		if rel, err := filepath.Rel(cleanRoot, target); err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("zip entry escapes destination: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode()
		if mode == 0 {
			mode = 0o644
		}
		wc, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(wc, rc)
		closeErr := wc.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func installAndroidPlatformTools(ctx context.Context) (string, string, error) {
	root := localToolDir("android-platform-tools")
	archive := filepath.Join(appDataDir(), "downloads", "platform-tools-latest-windows.zip")
	const source = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
	if err := downloadToFile(ctx, source, archive); err != nil {
		return root, "Download: " + source, err
	}
	if err := extractZipSafe(archive, root); err != nil {
		return root, "Archiv: " + archive, err
	}
	return root, "Android SDK Platform-Tools wurden aus der offiziellen Google-Quelle installiert.\nQuelle: " + source, nil
}

type githubRelease struct {
	Assets []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func latestMinGitURL(ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/git-for-windows/git/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "LocalCode/4.8 tool-installer")
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("GitHub HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&release); err != nil {
		return "", "", err
	}
	for _, asset := range release.Assets {
		lower := strings.ToLower(asset.Name)
		if strings.HasPrefix(lower, "mingit-") && strings.Contains(lower, "64-bit") && strings.HasSuffix(lower, ".zip") && !strings.Contains(lower, "busybox") {
			return asset.BrowserDownloadURL, asset.Name, nil
		}
	}
	return "", "", errors.New("latest Git for Windows release has no 64-bit MinGit zip asset")
}

func installPortableGit(ctx context.Context) (string, string, error) {
	url, asset, err := latestMinGitURL(ctx)
	if err != nil {
		return "", "", err
	}
	root := localToolDir("mingit")
	archive := filepath.Join(appDataDir(), "downloads", asset)
	if err := downloadToFile(ctx, url, archive); err != nil {
		return "", "Download: " + url, err
	}
	if err := extractZipSafe(archive, root); err != nil {
		return "", "Archiv: " + archive, err
	}
	candidates := []string{filepath.Join(root, "cmd", "git.exe"), filepath.Join(root, "bin", "git.exe")}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, "Portable MinGit wurde aus dem offiziellen Git-for-Windows-Release installiert.\nQuelle: " + url, nil
		}
	}
	return "", "", errors.New("git.exe missing after MinGit extraction")
}

func findWingetExecutable() string {
	if p, err := exec.LookPath("winget.exe"); err == nil {
		return p
	}
	for _, candidate := range []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps", "winget.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "WindowsApps", "Microsoft.DesktopAppInstaller_8wekyb3d8bbwe", "winget.exe"),
	} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

func installWithWinget(ctx context.Context, packageID string) (string, error) {
	winget := findWingetExecutable()
	if winget == "" {
		return "", errors.New("Windows Package Manager (winget) was not found")
	}
	args := []string{"install", "--id", packageID, "--exact", "--source", "winget", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}
	cmd := exec.CommandContext(ctx, winget, args...)
	hideCommandWindow(cmd)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	text := fmt.Sprintf("$ winget %s\n%s", strings.Join(args, " "), string(output))
	return truncateText(text, 120000), err
}

func installDotnetSDK(ctx context.Context, project string) (string, string, error) {
	root := localToolDir("dotnet")
	script := filepath.Join(appDataDir(), "downloads", "dotnet-install.ps1")
	const source = "https://dot.net/v1/dotnet-install.ps1"
	if err := downloadToFile(ctx, source, script); err != nil {
		return "", "Download: " + source, err
	}
	args := []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-InstallDir", root, "-Architecture", "x64"}
	globalJSON := filepath.Join(project, "global.json")
	if st, err := os.Stat(globalJSON); err == nil && !st.IsDir() {
		args = append(args, "-JSonFile", globalJSON)
	} else {
		args = append(args, "-Channel", "LTS", "-Quality", "GA")
	}
	cmd := exec.CommandContext(ctx, "powershell.exe", args...)
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	text := "Offizielles .NET-Installationsskript: " + source + "\n" + string(output)
	if err != nil {
		return "", truncateText(text, 120000), err
	}
	dotnet := filepath.Join(root, "dotnet.exe")
	if st, statErr := os.Stat(dotnet); statErr != nil || st.IsDir() {
		return "", truncateText(text, 120000), errors.New("dotnet.exe missing after official installer completed")
	}
	return dotnet, truncateText(text, 120000), nil
}

func installVisualStudioBuildTools(ctx context.Context) (string, string, error) {
	bootstrapper := filepath.Join(appDataDir(), "downloads", "vs_buildtools.exe")
	const source = "https://aka.ms/vs/stable/vs_buildtools.exe"
	if err := downloadToFile(ctx, source, bootstrapper); err != nil {
		return "", "Download: " + source, err
	}
	args := []string{
		"--quiet", "--wait", "--norestart", "--nocache",
		"--add", "Microsoft.VisualStudio.Workload.MSBuildTools",
		"--includeRecommended",
	}
	cmd := exec.CommandContext(ctx, bootstrapper, args...)
	hideCommandWindow(cmd)
	output, err := cmd.CombinedOutput()
	text := "$ vs_buildtools.exe " + strings.Join(args, " ") + "\n" + string(output)
	if err != nil {
		return "", truncateText(text, 120000), err
	}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		matches, _ := filepath.Glob(filepath.Join(base, "Microsoft Visual Studio", "*", "*", "MSBuild", "Current", "Bin", "MSBuild.exe"))
		sort.Strings(matches)
		for i := len(matches) - 1; i >= 0; i-- {
			if st, statErr := os.Stat(matches[i]); statErr == nil && !st.IsDir() {
				return matches[i], truncateText(text, 120000), nil
			}
		}
	}
	return "", truncateText(text, 120000), errors.New("MSBuild.exe missing after Visual Studio Build Tools installer completed")
}
