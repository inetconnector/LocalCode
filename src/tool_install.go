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

var toolRuntimeGOOS = runtime.GOOS
var toolHTTPClient = func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }
var androidPlatformToolsURL = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
var minGitReleaseURL = "https://api.github.com/repos/git-for-windows/git/releases/latest"
var nodeIndexURL = "https://nodejs.org/dist/index.json"
var nodeDistBaseURL = "https://nodejs.org/dist"
var githubCLIReleaseURL = "https://api.github.com/repos/cli/cli/releases/latest"
var dotnetInstallURL = "https://dot.net/v1/dotnet-install.ps1"
var vsBuildToolsURL = "https://aka.ms/vs/17/release/vs_BuildTools.exe"

type ToolInstallError struct {
	Tool   string
	Detail string
}

func (e *ToolInstallError) Error() string {
	if e == nil {
		return "tool installation failed"
	}
	if strings.TrimSpace(e.Detail) == "" {
		return "tool installation failed: " + e.Tool
	}
	return "tool installation failed: " + e.Tool + ": " + e.Detail
}

func toolInstallSupported(name string) bool {
	return strings.TrimSpace(profileForTool(name).InstallKind) != ""
}

func toolInstallPreview(name string, cfg Config) string {
	profile := profileForTool(name)
	if profile.InstallKind == "" {
		return localizeConfigText(cfg, "Für dieses Werkzeug ist kein automatischer Installer konfiguriert.", "No automatic installer is configured for this tool.")
	}
	return localizeConfigText(cfg,
		fmt.Sprintf("LocalCode installiert %s benutzerlokal beziehungsweise über den offiziellen Windows-Installer und prüft anschließend den gefundenen Pfad. Installationsart: %s", profile.DisplayName, profile.InstallKind),
		fmt.Sprintf("LocalCode will install %s for the current user or through the official Windows installer, then verify the discovered path. Installation method: %s", profile.DisplayName, profile.InstallKind))
}

func installKnownTool(ctx context.Context, project, name string, cfg Config) (Config, string, error) {
	profile := profileForTool(name)
	if profile.InstallKind == "" {
		return cfg, "", fmt.Errorf("no automatic installer is configured for %s", profile.DisplayName)
	}
	if !cfg.SetupDownloadsEnabled {
		return cfg, "", errors.New("downloads for automatic setup are disabled")
	}
	if toolRuntimeGOOS != "windows" {
		return cfg, "", errors.New("automatic tool installation is currently implemented for Windows")
	}
	if cfg.ToolOverrides == nil {
		cfg.ToolOverrides = map[string]string{}
	}
	setOverride := func(tool, path string) {
		if strings.TrimSpace(path) != "" {
			cfg.ToolOverrides[tool] = path
		}
	}
	switch profile.InstallKind {
	case "android-platform-tools":
		root, out, err := installAndroidPlatformTools(ctx)
		if err != nil {
			return cfg, out, err
		}
		setOverride("adb", filepath.Join(root, "platform-tools", executableName("adb")))
		setOverride("fastboot", filepath.Join(root, "platform-tools", executableName("fastboot")))
		return cfg, out, nil
	case "mingit":
		path, out, err := installPortableGit(ctx)
		if err == nil {
			setOverride("git", path)
		}
		return cfg, out, err
	case "node-portable":
		root, out, err := installPortableNode(ctx)
		if err != nil {
			return cfg, out, err
		}
		setOverride("node", filepath.Join(root, nodeToolFileName("node")))
		setOverride("npm", filepath.Join(root, nodeToolFileName("npm")))
		setOverride("npx", filepath.Join(root, nodeToolFileName("npx")))
		return cfg, out, nil
	case "gh-portable":
		path, out, err := installPortableGitHubCLI(ctx)
		if err == nil {
			setOverride("gh", path)
		}
		return cfg, out, err
	case "dotnet-sdk":
		root, out, err := installDotnetSDK(ctx, project)
		if err == nil {
			setOverride("dotnet", filepath.Join(root, executableName("dotnet")))
		}
		return cfg, out, err
	case "vs-build-tools":
		path, out, err := installVisualStudioBuildTools(ctx)
		if err == nil {
			setOverride("msbuild", path)
		}
		return cfg, out, err
	case "winget":
		if strings.TrimSpace(profile.WingetID) == "" {
			return cfg, "", fmt.Errorf("no winget package is configured for %s", profile.DisplayName)
		}
		out, err := installWithWinget(ctx, profile.WingetID)
		if err == nil {
			info := discoverTool(project, profile.Name, cfg, true)
			if info.Available {
				setOverride(profile.Name, info.Path)
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
	req.Header.Set("User-Agent", localCodeUserAgent()+" tool-installer")
	client := toolHTTPClient(10 * time.Minute)
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
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func extractZipSafe(zipPath, destination string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	cleanRoot, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, file := range r.File {
		name := filepath.Clean(file.Name)
		if name == "." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
			return fmt.Errorf("unsafe zip entry: %s", file.Name)
		}
		target := filepath.Join(cleanRoot, name)
		if !pathWithin(cleanRoot, target) {
			return fmt.Errorf("unsafe zip entry: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, 2<<30))
		closeErr := out.Close()
		_ = rc.Close()
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
	zipPath := filepath.Join(root, "platform-tools.zip")
	if err := downloadToFile(ctx, androidPlatformToolsURL, zipPath); err != nil {
		return root, "Download Google Android Platform-Tools fehlgeschlagen.", err
	}
	extractDir := filepath.Join(root, "extract")
	_ = os.RemoveAll(extractDir)
	if err := extractZipSafe(zipPath, extractDir); err != nil {
		return root, "Entpacken Google Android Platform-Tools fehlgeschlagen.", err
	}
	platformDir := filepath.Join(extractDir, "platform-tools")
	if _, err := os.Stat(filepath.Join(platformDir, executableName("adb"))); err != nil {
		return root, "Google-Archiv enthielt kein adb.", err
	}
	finalDir := filepath.Join(root, "platform-tools")
	_ = os.RemoveAll(finalDir)
	if err := os.Rename(platformDir, finalDir); err != nil {
		return root, "Verschieben Google Android Platform-Tools fehlgeschlagen.", err
	}
	return root, "Google Android Platform-Tools installiert: " + finalDir, nil
}

func latestMinGitURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, minGitReleaseURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", localCodeUserAgent()+" tool-installer")
	resp, err := toolHTTPClient(30 * time.Second).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub release metadata HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return "", err
	}
	for _, asset := range payload.Assets {
		lower := strings.ToLower(asset.Name)
		if strings.Contains(lower, "mingit") && strings.Contains(lower, "64-bit") && strings.HasSuffix(lower, ".zip") && !strings.Contains(lower, "busybox") {
			return asset.URL, nil
		}
	}
	return "", errors.New("no 64-bit MinGit ZIP found in latest Git-for-Windows release")
}

func installPortableGit(ctx context.Context) (string, string, error) {
	root := localToolDir("mingit")
	url, err := latestMinGitURL(ctx)
	if err != nil {
		return "", "", err
	}
	zipPath := filepath.Join(root, "mingit.zip")
	if err := downloadToFile(ctx, url, zipPath); err != nil {
		return "", "Download MinGit fehlgeschlagen.", err
	}
	extractDir := filepath.Join(root, "current")
	_ = os.RemoveAll(extractDir)
	if err := extractZipSafe(zipPath, extractDir); err != nil {
		return "", "Entpacken MinGit fehlgeschlagen.", err
	}
	gitPath := filepath.Join(extractDir, "cmd", executableName("git"))
	if _, err := os.Stat(gitPath); err != nil {
		return "", "MinGit-Archiv enthielt git.exe nicht am erwarteten Pfad.", err
	}
	return gitPath, "Portable MinGit aus dem offiziellen Git-for-Windows GitHub Release installiert: " + gitPath, nil
}

func findWingetExecutable() string {
	if path, err := exec.LookPath("winget.exe"); err == nil {
		return path
	}
	candidates := []string{
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps", "winget.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "WindowsApps", "Microsoft.DesktopAppInstaller_8wekyb3d8bbwe", "winget.exe"),
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

func installWithWinget(ctx context.Context, packageID string) (string, error) {
	winget := findWingetExecutable()
	if winget == "" {
		return "", errors.New("winget was not found")
	}
	args := []string{"install", "--id", packageID, "--exact", "--source", "winget", "--accept-source-agreements", "--accept-package-agreements", "--silent", "--disable-interactivity"}
	cmd := exec.CommandContext(ctx, winget, args...)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func installDotnetSDK(ctx context.Context, project string) (string, string, error) {
	root := localToolDir("dotnet")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return root, "", err
	}
	script := filepath.Join(root, "dotnet-install.ps1")
	if err := downloadToFile(ctx, dotnetInstallURL, script); err != nil {
		return root, "Download dotnet-install.ps1 fehlgeschlagen.", err
	}
	args := []string{"-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Channel", "LTS", "-InstallDir", root, "-NoPath"}
	cmd := exec.CommandContext(ctx, "powershell.exe", args...)
	hideCommandWindow(cmd)
	cmd.Dir = project
	out, err := cmd.CombinedOutput()
	if err != nil {
		return root, strings.TrimSpace(string(out)), err
	}
	dotnet := filepath.Join(root, executableName("dotnet"))
	if _, statErr := os.Stat(dotnet); statErr != nil {
		return root, strings.TrimSpace(string(out)), statErr
	}
	return root, "Microsoft dotnet-install.ps1 ausgeführt.\n" + strings.TrimSpace(string(out)), nil
}

func installVisualStudioBuildTools(ctx context.Context) (string, string, error) {
	root := localToolDir("vs-build-tools")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", "", err
	}
	installer := filepath.Join(root, "vs_BuildTools.exe")
	if err := downloadToFile(ctx, vsBuildToolsURL, installer); err != nil {
		return "", "Download Visual Studio Build Tools fehlgeschlagen.", err
	}
	installRoot := filepath.Join(root, "installation")
	args := []string{"--quiet", "--wait", "--norestart", "--nocache", "--installPath", installRoot, "--add", "Microsoft.VisualStudio.Workload.MSBuildTools", "--includeRecommended"}
	cmd := exec.CommandContext(ctx, installer, args...)
	hideCommandWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", strings.TrimSpace(string(out)), err
	}
	candidates := []string{
		filepath.Join(installRoot, "MSBuild", "Current", "Bin", "MSBuild.exe"),
		filepath.Join(installRoot, "MSBuild", "17.0", "Bin", "MSBuild.exe"),
	}
	for _, candidate := range candidates {
		if st, statErr := os.Stat(candidate); statErr == nil && !st.IsDir() {
			return candidate, "Visual Studio Build Tools installiert: " + candidate, nil
		}
	}
	return "", strings.TrimSpace(string(out)), errors.New("MSBuild.exe was not found after Visual Studio Build Tools installation")
}

func installPortableNode(ctx context.Context) (string, string, error) {
	indexReq, err := http.NewRequestWithContext(ctx, http.MethodGet, nodeIndexURL, nil)
	if err != nil {
		return "", "", err
	}
	indexReq.Header.Set("User-Agent", localCodeUserAgent()+" tool-installer")
	resp, err := toolHTTPClient(30 * time.Second).Do(indexReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("Node index HTTP %d", resp.StatusCode)
	}
	var releases []struct {
		Version string `json:"version"`
		LTS     any    `json:"lts"`
		Files   []string `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&releases); err != nil {
		return "", "", err
	}
	selected := ""
	for _, release := range releases {
		isLTS := false
		switch value := release.LTS.(type) {
		case string:
			isLTS = strings.TrimSpace(value) != ""
		case bool:
			isLTS = value
		}
		if !isLTS {
			continue
		}
		for _, file := range release.Files {
			if file == "win-x64-zip" {
				selected = release.Version
				break
			}
		}
		if selected != "" {
			break
		}
	}
	if selected == "" {
		return "", "", errors.New("no Node.js LTS win-x64-zip release found")
	}
	base := strings.TrimRight(nodeDistBaseURL, "/")
	name := "node-" + selected + "-win-x64.zip"
	url := base + "/" + selected + "/" + name
	root := localToolDir("node")
	zipPath := filepath.Join(root, name)
	if err := downloadToFile(ctx, url, zipPath); err != nil {
		return "", "Download Node.js LTS fehlgeschlagen.", err
	}
	extractRoot := filepath.Join(root, "extract")
	_ = os.RemoveAll(extractRoot)
	if err := extractZipSafe(zipPath, extractRoot); err != nil {
		return "", "Entpacken Node.js LTS fehlgeschlagen.", err
	}
	entries, err := os.ReadDir(extractRoot)
	if err != nil {
		return "", "", err
	}
	var source string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(strings.ToLower(entry.Name()), "node-") {
			source = filepath.Join(extractRoot, entry.Name())
			break
		}
	}
	if source == "" {
		return "", "", errors.New("Node.js archive did not contain the expected directory")
	}
	current := filepath.Join(root, "current")
	_ = os.RemoveAll(current)
	if err := os.Rename(source, current); err != nil {
		return "", "", err
	}
	if _, err := os.Stat(filepath.Join(current, nodeToolFileName("node"))); err != nil {
		return "", "", err
	}
	return current, "Portable Node.js LTS " + selected + " installiert: " + current, nil
}

func installPortableGitHubCLI(ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubCLIReleaseURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", localCodeUserAgent()+" tool-installer")
	resp, err := toolHTTPClient(30 * time.Second).Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("GitHub CLI release metadata HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return "", "", err
	}
	assetURL := ""
	assetName := ""
	for _, asset := range payload.Assets {
		lower := strings.ToLower(asset.Name)
		if strings.Contains(lower, "windows_amd64") && strings.HasSuffix(lower, ".zip") {
			assetURL = asset.URL
			assetName = asset.Name
			break
		}
	}
	if assetURL == "" {
		return "", "", errors.New("no Windows amd64 GitHub CLI ZIP found in latest release")
	}
	root := localToolDir("gh")
	zipPath := filepath.Join(root, assetName)
	if err := downloadToFile(ctx, assetURL, zipPath); err != nil {
		return "", "Download GitHub CLI fehlgeschlagen.", err
	}
	extractRoot := filepath.Join(root, "extract")
	_ = os.RemoveAll(extractRoot)
	if err := extractZipSafe(zipPath, extractRoot); err != nil {
		return "", "Entpacken GitHub CLI fehlgeschlagen.", err
	}
	var ghPath string
	_ = filepath.WalkDir(extractRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr == nil && !d.IsDir() && strings.EqualFold(d.Name(), executableName("gh")) {
			ghPath = path
			return io.EOF
		}
		return nil
	})
	if ghPath == "" {
		return "", "", errors.New("GitHub CLI archive did not contain gh.exe")
	}
	return ghPath, "GitHub CLI aus dem offiziellen GitHub release installiert: " + ghPath, nil
}
