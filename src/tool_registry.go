// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ToolInfo struct {
	Name             string   `json:"name"`
	DisplayName      string   `json:"display_name"`
	Available        bool     `json:"available"`
	Path             string   `json:"path,omitempty"`
	Source           string   `json:"source,omitempty"`
	Version          string   `json:"version,omitempty"`
	DocsURL          string   `json:"docs_url,omitempty"`
	InstallHint      string   `json:"install_hint,omitempty"`
	InstallSupported bool     `json:"install_supported,omitempty"`
	InstallPreview   string   `json:"install_preview,omitempty"`
	Diagnostics      []string `json:"diagnostics,omitempty"`
	SearchedPath     []string `json:"searched_paths,omitempty"`
}

type toolProfile struct {
	Name        string
	DisplayName string
	Aliases     []string
	VersionArgs []string
	DocsURL     string
	InstallHint string
	InstallKind string
	WingetID    string
}

var toolRegistryGOOS = runtime.GOOS

var toolProfiles = []toolProfile{
	{Name: "adb", DisplayName: "Android Debug Bridge", Aliases: []string{"adb", "adb.exe"}, VersionArgs: []string{"version"}, DocsURL: "https://developer.android.com/tools/adb", InstallHint: "Android SDK Platform-Tools installieren.", InstallKind: "android-platform-tools"},
	{Name: "fastboot", DisplayName: "Android Fastboot", Aliases: []string{"fastboot", "fastboot.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://developer.android.com/tools/releases/platform-tools", InstallHint: "Android SDK Platform-Tools installieren.", InstallKind: "android-platform-tools"},
	{Name: "sdkmanager", DisplayName: "Android SDK Manager", Aliases: []string{"sdkmanager", "sdkmanager.bat"}, VersionArgs: []string{"--version"}, DocsURL: "https://developer.android.com/tools/sdkmanager", InstallHint: "Android SDK Command-Line Tools installieren."},
	{Name: "emulator", DisplayName: "Android Emulator", Aliases: []string{"emulator", "emulator.exe"}, VersionArgs: []string{"-version"}, DocsURL: "https://developer.android.com/studio/run/emulator-commandline", InstallHint: "Android Emulator über den SDK Manager installieren."},
	{Name: "avdmanager", DisplayName: "Android Virtual Device Manager", Aliases: []string{"avdmanager", "avdmanager.bat"}, VersionArgs: []string{"--version"}, DocsURL: "https://developer.android.com/tools/avdmanager", InstallHint: "Android SDK Command-Line Tools installieren."},
	{Name: "aapt2", DisplayName: "Android Asset Packaging Tool 2", Aliases: []string{"aapt2", "aapt2.exe"}, VersionArgs: []string{"version"}, DocsURL: "https://developer.android.com/tools/aapt2", InstallHint: "Android SDK Build-Tools installieren."},
	{Name: "apksigner", DisplayName: "APK Signer", Aliases: []string{"apksigner", "apksigner.bat"}, VersionArgs: []string{"version"}, DocsURL: "https://developer.android.com/tools/apksigner", InstallHint: "Android SDK Build-Tools installieren."},
	{Name: "zipalign", DisplayName: "zipalign", Aliases: []string{"zipalign", "zipalign.exe"}, VersionArgs: []string{"-h"}, DocsURL: "https://developer.android.com/tools/zipalign", InstallHint: "Android SDK Build-Tools installieren."},
	{Name: "d8", DisplayName: "D8 Dex Compiler", Aliases: []string{"d8", "d8.bat"}, VersionArgs: []string{"--version"}, DocsURL: "https://developer.android.com/tools/d8", InstallHint: "Android SDK Build-Tools installieren."},
	{Name: "lint", DisplayName: "Android Lint", Aliases: []string{"lint", "lint.bat"}, VersionArgs: []string{"--version"}, DocsURL: "https://developer.android.com/studio/write/lint", InstallHint: "Android SDK Command-Line Tools installieren."},
	{Name: "gradle", DisplayName: "Gradle", Aliases: []string{"gradle", "gradle.bat", "gradlew", "gradlew.bat"}, VersionArgs: []string{"--version"}, DocsURL: "https://docs.gradle.org/current/userguide/command_line_interface.html", InstallHint: "Bevorzugt den Gradle Wrapper des Projekts verwenden."},
	{Name: "java", DisplayName: "Java", Aliases: []string{"java", "java.exe"}, VersionArgs: []string{"-version"}, DocsURL: "https://learn.microsoft.com/java/openjdk/", InstallHint: "JDK oder die in Android Studio enthaltene JBR verwenden.", InstallKind: "winget", WingetID: "Microsoft.OpenJDK.17"},
	{Name: "javac", DisplayName: "Java Compiler", Aliases: []string{"javac", "javac.exe"}, VersionArgs: []string{"-version"}, DocsURL: "https://docs.oracle.com/en/java/javase/21/docs/specs/man/javac.html", InstallHint: "JDK oder Android Studio JBR installieren.", InstallKind: "winget", WingetID: "Microsoft.OpenJDK.17"},
	{Name: "keytool", DisplayName: "Java Keytool", Aliases: []string{"keytool", "keytool.exe"}, VersionArgs: []string{"-help"}, DocsURL: "https://docs.oracle.com/en/java/javase/21/docs/specs/man/keytool.html", InstallHint: "JDK oder Android Studio JBR installieren.", InstallKind: "winget", WingetID: "Microsoft.OpenJDK.17"},
	{Name: "jarsigner", DisplayName: "Java JAR Signer", Aliases: []string{"jarsigner", "jarsigner.exe"}, VersionArgs: []string{"-help"}, DocsURL: "https://docs.oracle.com/en/java/javase/21/docs/specs/man/jarsigner.html", InstallHint: "JDK oder Android Studio JBR installieren.", InstallKind: "winget", WingetID: "Microsoft.OpenJDK.17"},
	{Name: "git", DisplayName: "Git", Aliases: []string{"git", "git.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://git-scm.com/docs", InstallHint: "Git for Windows oder eine portable MinGit-Version installieren.", InstallKind: "mingit", WingetID: "Git.Git"},
	{Name: "powershell", DisplayName: "PowerShell", Aliases: []string{"powershell", "powershell.exe", "pwsh", "pwsh.exe"}, VersionArgs: []string{"-NoLogo", "-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"}, DocsURL: "https://learn.microsoft.com/powershell/", InstallHint: "Windows PowerShell aktivieren oder PowerShell 7 installieren.", InstallKind: "winget", WingetID: "Microsoft.PowerShell"},
	{Name: "gh", DisplayName: "GitHub CLI", Aliases: []string{"gh", "gh.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://cli.github.com/manual/", InstallHint: "GitHub CLI installieren; Login interaktiv mit gh auth login.", InstallKind: "gh-portable", WingetID: "GitHub.cli"},
	{Name: "node", DisplayName: "Node.js", Aliases: []string{"node", "node.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://nodejs.org/docs/latest/api/", InstallHint: "Node.js LTS installieren.", InstallKind: "node-portable", WingetID: "OpenJS.NodeJS.LTS"},
	{Name: "npm", DisplayName: "npm", Aliases: []string{"npm", "npm.cmd"}, VersionArgs: []string{"--version"}, DocsURL: "https://docs.npmjs.com/cli/", InstallHint: "Wird mit Node.js installiert.", InstallKind: "node-portable", WingetID: "OpenJS.NodeJS.LTS"},
	{Name: "npx", DisplayName: "npx", Aliases: []string{"npx", "npx.cmd"}, VersionArgs: []string{"--version"}, DocsURL: "https://docs.npmjs.com/cli/commands/npx", InstallHint: "Wird mit npm installiert.", InstallKind: "node-portable", WingetID: "OpenJS.NodeJS.LTS"},
	{Name: "python", DisplayName: "Python", Aliases: []string{"python", "python.exe", "py", "py.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://docs.python.org/3/", InstallHint: "Python installieren und dem PATH hinzufügen.", InstallKind: "winget", WingetID: "Python.Python.3.13"},
	{Name: "pip", DisplayName: "pip", Aliases: []string{"pip", "pip.exe", "pip3", "pip3.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://pip.pypa.io/en/stable/cli/", InstallHint: "pip mit Python installieren.", InstallKind: "winget", WingetID: "Python.Python.3.13"},
	{Name: "go", DisplayName: "Go", Aliases: []string{"go", "go.exe"}, VersionArgs: []string{"version"}, DocsURL: "https://go.dev/doc/", InstallHint: "Go installieren.", InstallKind: "winget", WingetID: "GoLang.Go"},
	{Name: "dotnet", DisplayName: ".NET", Aliases: []string{"dotnet", "dotnet.exe"}, VersionArgs: []string{"--info"}, DocsURL: "https://learn.microsoft.com/dotnet/core/tools/", InstallHint: ".NET SDK installieren.", InstallKind: "dotnet-sdk"},
	{Name: "cargo", DisplayName: "Cargo", Aliases: []string{"cargo", "cargo.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://doc.rust-lang.org/cargo/", InstallHint: "Rust über rustup installieren."},
	{Name: "rustc", DisplayName: "Rust Compiler", Aliases: []string{"rustc", "rustc.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://doc.rust-lang.org/rustc/", InstallHint: "Rust über rustup installieren."},
	{Name: "docker", DisplayName: "Docker", Aliases: []string{"docker", "docker.exe"}, VersionArgs: []string{"version", "--format", "{{.Client.Version}}"}, DocsURL: "https://docs.docker.com/reference/cli/docker/", InstallHint: "Docker Desktop installieren und starten.", InstallKind: "winget", WingetID: "Docker.DockerDesktop"},
	{Name: "cmake", DisplayName: "CMake", Aliases: []string{"cmake", "cmake.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://cmake.org/cmake/help/latest/", InstallHint: "CMake installieren.", InstallKind: "winget", WingetID: "Kitware.CMake"},
	{Name: "ninja", DisplayName: "Ninja", Aliases: []string{"ninja", "ninja.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://ninja-build.org/manual.html", InstallHint: "Ninja installieren oder die Android-SDK-Kopie verwenden."},
	{Name: "ssh", DisplayName: "OpenSSH Client", Aliases: []string{"ssh", "ssh.exe"}, VersionArgs: []string{"-V"}, DocsURL: "https://learn.microsoft.com/windows-server/administration/openssh/openssh-overview", InstallHint: "Windows OpenSSH Client Feature installieren."},
	{Name: "scp", DisplayName: "Secure Copy", Aliases: []string{"scp", "scp.exe"}, VersionArgs: []string{"-V"}, DocsURL: "https://man.openbsd.org/scp", InstallHint: "Windows OpenSSH Client Feature installieren."},
	{Name: "curl", DisplayName: "curl", Aliases: []string{"curl", "curl.exe"}, VersionArgs: []string{"--version"}, DocsURL: "https://curl.se/docs/manpage.html", InstallHint: "curl installieren oder die Windows-Systemkopie verwenden."},
	{Name: "msbuild", DisplayName: "MSBuild", Aliases: []string{"msbuild", "MSBuild.exe"}, VersionArgs: []string{"-version"}, DocsURL: "https://learn.microsoft.com/visualstudio/msbuild/msbuild-command-line-reference", InstallHint: "Visual Studio Build Tools installieren.", InstallKind: "vs-build-tools"},
	{Name: "devenv", DisplayName: "Visual Studio IDE", Aliases: []string{"devenv", "devenv.exe"}, VersionArgs: []string{"/Command", "File.Exit"}, DocsURL: "https://learn.microsoft.com/visualstudio/ide/reference/devenv-command-line-switches", InstallHint: "Visual Studio installieren."},
	{Name: "vswhere", DisplayName: "Visual Studio Locator", Aliases: []string{"vswhere", "vswhere.exe"}, VersionArgs: []string{"-help"}, DocsURL: "https://github.com/microsoft/vswhere", InstallHint: "vswhere wird mit Visual Studio Installer installiert."},
	{Name: "nuget", DisplayName: "NuGet CLI", Aliases: []string{"nuget", "nuget.exe"}, VersionArgs: []string{"help"}, DocsURL: "https://learn.microsoft.com/nuget/reference/nuget-exe-cli-reference", InstallHint: "NuGet CLI installieren oder dotnet restore verwenden."},
}

func canonicalToolName(name string) string {
	name = strings.ToLower(strings.TrimSpace(filepath.Base(name)))
	for _, p := range toolProfiles {
		if name == p.Name {
			return p.Name
		}
		for _, a := range p.Aliases {
			if name == strings.ToLower(a) {
				return p.Name
			}
		}
	}
	return strings.TrimSuffix(name, ".exe")
}

func profileForTool(name string) toolProfile {
	canonical := canonicalToolName(name)
	for _, p := range toolProfiles {
		if p.Name == canonical {
			return p
		}
	}
	aliases := []string{canonical}
	if toolRegistryGOOS == "windows" {
		aliases = append(aliases, canonical+".exe", canonical+".cmd", canonical+".bat")
	}
	return toolProfile{Name: canonical, DisplayName: canonical, Aliases: aliases}
}

func parseAndroidSDKFromLocalProperties(project string) string {
	if strings.TrimSpace(project) == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(project, "local.properties"))
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "sdk.dir=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "sdk.dir="))
		value = strings.ReplaceAll(value, `\:`, `:`)
		value = strings.ReplaceAll(value, `\\`, `\`)
		value = strings.ReplaceAll(value, `\/`, `/`)
		return filepath.Clean(value)
	}
	return ""
}

func androidSDKRoots(project string) []string {
	roots := []string{}
	if sdk := parseAndroidSDKFromLocalProperties(project); sdk != "" {
		roots = append(roots, sdk)
	}
	for _, key := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			roots = append(roots, v)
		}
	}
	if toolRegistryGOOS == "windows" {
		if v := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); v != "" {
			roots = append(roots, filepath.Join(v, "Android", "Sdk"))
			roots = append(roots, filepath.Join(v, "LocalCode", "tools", "android-platform-tools"))
		}
		if v := strings.TrimSpace(os.Getenv("USERPROFILE")); v != "" {
			roots = append(roots, filepath.Join(v, "AppData", "Local", "Android", "Sdk"))
		}
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if strings.TrimSpace(base) != "" {
				roots = append(roots, filepath.Join(base, "Android", "android-sdk"))
			}
		}
		// LocalCode's managed Platform-Tools install lives below the app config directory.
		roots = append(roots, filepath.Join(appDataDir(), "tools", "android-platform-tools"))
	}
	return uniquePaths(roots)
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(os.ExpandEnv(p))
		key := strings.ToLower(p)
		if !seen[key] {
			seen[key] = true
			out = append(out, p)
		}
	}
	return out
}

func toolCandidatePaths(project string, profile toolProfile, cfg Config) []struct{ path, source string } {
	var out []struct{ path, source string }
	add := func(path, source string) {
		if strings.TrimSpace(path) != "" {
			out = append(out, struct{ path, source string }{filepath.Clean(os.ExpandEnv(path)), source})
		}
	}
	if override := strings.TrimSpace(cfg.ToolOverrides[profile.Name]); override != "" {
		add(override, "Einstellung")
	}
	// Project-local tools are common in reproducible repositories. Check these
	// before global installations so wrappers and pinned binaries win.
	for _, dir := range []string{"", "bin", "tools", ".tools", "scripts", filepath.Join("node_modules", ".bin")} {
		for _, alias := range profile.Aliases {
			add(filepath.Join(project, dir, alias), "Projekt")
		}
	}
	if profile.Name == "gradle" {
		for _, n := range []string{"gradlew.bat", "gradlew"} {
			add(filepath.Join(project, n), "Projekt-Wrapper")
		}
	}
	if profile.Name == "java" || profile.Name == "keytool" || profile.Name == "jarsigner" {
		if root := strings.TrimSpace(os.Getenv("JAVA_HOME")); root != "" {
			add(filepath.Join(root, "bin", executableName(profile.Name)), "JAVA_HOME")
		}
	}
	if profile.Name == "cargo" || profile.Name == "rustc" {
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, ".cargo", "bin", executableName(profile.Name)), "Rustup")
		}
	}
	if profile.Name == "adb" || profile.Name == "fastboot" || profile.Name == "emulator" || profile.Name == "sdkmanager" || profile.Name == "avdmanager" || profile.Name == "aapt2" || profile.Name == "apksigner" || profile.Name == "zipalign" || profile.Name == "d8" || profile.Name == "lint" || profile.Name == "ninja" {
		for _, sdk := range androidSDKRoots(project) {
			switch profile.Name {
			case "adb", "fastboot":
				add(filepath.Join(sdk, "platform-tools", executableName(profile.Name)), "Android SDK")
			case "emulator":
				add(filepath.Join(sdk, "emulator", executableName("emulator")), "Android SDK")
			case "sdkmanager", "avdmanager", "lint":
				add(filepath.Join(sdk, "cmdline-tools", "latest", "bin", scriptName(profile.Name)), "Android SDK")
				matches, _ := filepath.Glob(filepath.Join(sdk, "cmdline-tools", "*", "bin", scriptName(profile.Name)))
				for _, m := range matches {
					add(m, "Android SDK")
				}
			case "aapt2", "apksigner", "zipalign", "d8":
				fileName := executableName(profile.Name)
				if profile.Name == "apksigner" || profile.Name == "d8" {
					fileName = scriptName(profile.Name)
				}
				matches, _ := filepath.Glob(filepath.Join(sdk, "build-tools", "*", fileName))
				sort.Strings(matches)
				for i := len(matches) - 1; i >= 0; i-- {
					add(matches[i], "Android SDK Build-Tools")
				}
			case "ninja":
				matches, _ := filepath.Glob(filepath.Join(sdk, "cmake", "*", "bin", executableName("ninja")))
				for _, m := range matches {
					add(m, "Android SDK")
				}
			}
		}
	}
	for _, alias := range profile.Aliases {
		if p, err := exec.LookPath(alias); err == nil {
			add(p, "PATH")
		}
	}
	for _, candidate := range visualStudioToolPaths(profile.Name) {
		add(candidate[0], candidate[1])
	}
	if toolRegistryGOOS == "windows" {
		pf := os.Getenv("ProgramFiles")
		pf86 := os.Getenv("ProgramFiles(x86)")
		local := os.Getenv("LOCALAPPDATA")
		home := os.Getenv("USERPROFILE")
		switch profile.Name {
		case "git":
			add(filepath.Join(pf, "Git", "cmd", "git.exe"), "Standardpfad")
		case "gh":
			add(filepath.Join(pf, "GitHub CLI", "gh.exe"), "Standardpfad")
		case "node", "npm", "npx":
			add(filepath.Join(pf, "nodejs", nodeToolFileName(profile.Name)), "Standardpfad")
			add(filepath.Join(local, "Programs", "nodejs", nodeToolFileName(profile.Name)), "Benutzerinstallation")
			for _, pattern := range []string{
				filepath.Join(local, "Microsoft", "WinGet", "Packages", "OpenJS.NodeJS*_Microsoft.Winget.Source_*", "node-v*-win-x64", nodeToolFileName(profile.Name)),
				filepath.Join(local, "Microsoft", "WinGet", "Packages", "OpenJS.NodeJS*_Microsoft.Winget.Source_*", "nodejs", nodeToolFileName(profile.Name)),
			} {
				matches, _ := filepath.Glob(pattern)
				sort.Strings(matches)
				for i := len(matches) - 1; i >= 0; i-- {
					add(matches[i], "WinGet")
				}
			}
		case "go":
			add(filepath.Join(pf, "Go", "bin", "go.exe"), "Standardpfad")
		case "dotnet":
			add(filepath.Join(pf, "dotnet", "dotnet.exe"), "Standardpfad")
		case "docker":
			add(filepath.Join(pf, "Docker", "Docker", "resources", "bin", "docker.exe"), "Docker Desktop")
		case "cmake":
			add(filepath.Join(pf, "CMake", "bin", "cmake.exe"), "Standardpfad")
		case "java", "keytool", "jarsigner":
			add(filepath.Join(pf, "Android", "Android Studio", "jbr", "bin", executableName(profile.Name)), "Android Studio JBR")
			add(filepath.Join(pf86, "Android", "Android Studio", "jbr", "bin", executableName(profile.Name)), "Android Studio JBR")
			for _, pattern := range []string{
				filepath.Join(pf, "Java", "*", "bin", executableName(profile.Name)),
				filepath.Join(pf, "Microsoft", "jdk-*", "bin", executableName(profile.Name)),
				filepath.Join(pf, "Microsoft", "jdk-*", "*", "bin", executableName(profile.Name)),
			} {
				matches, _ := filepath.Glob(pattern)
				for _, m := range matches {
					add(m, "JDK")
				}
			}
		case "python", "pip":
			target := "python.exe"
			if profile.Name == "pip" {
				target = filepath.Join("Scripts", "pip.exe")
			}
			matches, _ := filepath.Glob(filepath.Join(local, "Programs", "Python", "Python*", target))
			for _, m := range matches {
				add(m, "Python Launcher")
			}
			if profile.Name == "python" {
				add(filepath.Join(home, "AppData", "Local", "Microsoft", "WindowsApps", "python.exe"), "WindowsApps")
			}
		case "ssh", "scp":
			add(filepath.Join(os.Getenv("WINDIR"), "System32", "OpenSSH", executableName(profile.Name)), "Windows OpenSSH")
		case "curl":
			add(filepath.Join(os.Getenv("WINDIR"), "System32", "curl.exe"), "Windows System32")
		case "vswhere":
			add(filepath.Join(pf86, "Microsoft Visual Studio", "Installer", "vswhere.exe"), "Visual Studio Installer")
			add(filepath.Join(pf, "Microsoft Visual Studio", "Installer", "vswhere.exe"), "Visual Studio Installer")
		case "msbuild":
			for _, base := range []string{pf, pf86} {
				matches, _ := filepath.Glob(filepath.Join(base, "Microsoft Visual Studio", "*", "*", "MSBuild", "Current", "Bin", "MSBuild.exe"))
				for _, m := range matches {
					add(m, "Visual Studio")
				}
			}
		}
	}
	seen := map[string]bool{}
	filtered := make([]struct{ path, source string }, 0, len(out))
	for _, c := range out {
		if c.path == "." || strings.TrimSpace(c.path) == "" {
			continue
		}
		key := strings.ToLower(c.path)
		if !seen[key] {
			seen[key] = true
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func executableName(name string) string {
	if toolRegistryGOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func scriptName(name string) string {
	if toolRegistryGOOS == "windows" {
		switch name {
		case "npm", "npx":
			return name + ".cmd"
		case "sdkmanager", "avdmanager", "apksigner", "d8", "lint", "gradle":
			return name + ".bat"
		}
	}
	return name
}

func nodeToolFileName(name string) string {
	if name == "node" {
		return executableName(name)
	}
	return scriptName(name)
}

func discoverTool(project, name string, cfg Config, withVersion bool) ToolInfo {
	profile := profileForTool(name)
	info := ToolInfo{Name: profile.Name, DisplayName: profile.DisplayName, DocsURL: profile.DocsURL, InstallHint: localizedToolInstallHint(cfg, profile.InstallHint), InstallSupported: toolInstallSupported(profile.Name), InstallPreview: toolInstallPreview(profile.Name, cfg)}
	candidates := toolCandidatePaths(project, profile, cfg)
	for _, c := range candidates {
		info.SearchedPath = append(info.SearchedPath, c.path)
		st, err := os.Stat(c.path)
		if err != nil || st.IsDir() {
			continue
		}
		info.Available = true
		info.Path = c.path
		info.Source = c.source
		break
	}
	if info.Available && withVersion && len(profile.VersionArgs) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		res := runDirectTool(ctx, project, info.Path, profile.VersionArgs, cfg)
		cancel()
		version := strings.TrimSpace(res.Stdout)
		if version == "" {
			version = strings.TrimSpace(res.Stderr)
		}
		if version != "" {
			info.Version = truncateText(strings.Split(version, "\n")[0], 300)
		}
	}
	if profile.Name == "adb" && info.Available && withVersion {
		info.Diagnostics = append(info.Diagnostics, diagnoseADBPath(project, info.Path, cfg)...)
	}
	return info
}

func toolInventory(project string, cfg Config, withVersion bool) []ToolInfo {
	infos := make([]ToolInfo, 0, len(toolProfiles))
	for _, p := range toolProfiles {
		infos = append(infos, discoverTool(project, p.Name, cfg, withVersion))
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].DisplayName < infos[j].DisplayName })
	return infos
}

func toolInventorySummary(project string, cfg Config) string {
	if !cfg.AutoDiscoverTools {
		return "Automatische Werkzeugerkennung ist deaktiviert."
	}
	infos := toolInventory(project, cfg, false)
	var b strings.Builder
	for _, info := range infos {
		if info.Available {
			fmt.Fprintf(&b, "- %s: VERFÜGBAR (%s) [%s]\n", info.Name, info.Path, info.Source)
		} else {
			fmt.Fprintf(&b, "- %s: nicht gefunden\n", info.Name)
		}
	}
	return b.String()
}

type ToolRunResult struct {
	Tool       string
	Path       string
	Args       []string
	CWD        string
	Stdout     string
	Stderr     string
	ExitCode   int
	Duration   time.Duration
	TimedOut   bool
	Cancelled  bool
	Started    bool
	Err        error
	Diagnostic string
}

func (r ToolRunResult) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tool: %s\nPfad: %s\nArbeitsordner: %s\nArgumente: %s\nExitcode: %d\nDauer: %s\n", r.Tool, r.Path, r.CWD, quoteArgs(r.Args), r.ExitCode, r.Duration.Round(time.Millisecond))
	if r.TimedOut {
		b.WriteString("Status: TIMEOUT\n")
	} else if r.Cancelled {
		b.WriteString("Status: ABGEBROCHEN\n")
	} else if r.Err != nil {
		b.WriteString("Status: FEHLER\n")
	} else {
		b.WriteString("Status: OK\n")
	}
	if strings.TrimSpace(r.Stdout) != "" {
		b.WriteString("\nSTDOUT:\n")
		b.WriteString(truncateText(r.Stdout, 120000))
		if !strings.HasSuffix(r.Stdout, "\n") {
			b.WriteByte('\n')
		}
	}
	if strings.TrimSpace(r.Stderr) != "" {
		b.WriteString("\nSTDERR:\n")
		b.WriteString(truncateText(r.Stderr, 60000))
		if !strings.HasSuffix(r.Stderr, "\n") {
			b.WriteByte('\n')
		}
	}
	if strings.TrimSpace(r.Diagnostic) != "" {
		b.WriteString("\nDIAGNOSE:\n")
		b.WriteString(r.Diagnostic)
		if !strings.HasSuffix(r.Diagnostic, "\n") {
			b.WriteByte('\n')
		}
	}
	if r.Err != nil {
		b.WriteString("\nFEHLERDETAIL:\n")
		b.WriteString(r.Err.Error())
		b.WriteByte('\n')
	}
	return b.String()
}

func quoteArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			parts[i] = strconv.Quote(a)
		} else {
			parts[i] = a
		}
	}
	return strings.Join(parts, " ")
}

func buildWindowsCommandLine(path string, args []string) string {
	quote := func(v string) string {
		if v == "" || strings.ContainsAny(v, " \t&|<>^()\"") {
			return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
		}
		return v
	}
	parts := []string{quote(path)}
	for _, arg := range args {
		parts = append(parts, quote(arg))
	}
	return strings.Join(parts, " ")
}

func runDirectTool(ctx context.Context, project, path string, args []string, cfg Config) ToolRunResult {
	start := time.Now()
	result := ToolRunResult{Tool: canonicalToolName(path), Path: path, Args: append([]string(nil), args...), CWD: project, ExitCode: -1}
	var cmd *exec.Cmd
	if toolRegistryGOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".cmd" || ext == ".bat" {
			parts := []string{"/D", "/S", "/C", buildWindowsCommandLine(path, args)}
			cmd = exec.Command("cmd.exe", parts...)
		} else {
			cmd = exec.Command(path, args...)
		}
	} else {
		cmd = exec.Command(path, args...)
	}
	if canonicalToolName(path) == "adb" && len(args) > 0 && (args[0] == "devices" || args[0] == "version" || args[0] == "start-server" || args[0] == "kill-server") {
		cmd.Dir = os.TempDir()
	} else {
		cmd.Dir = project
	}
	cmd.Env = commandEnvironment(cfg)
	hideCommandWindow(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		result.Err = err
		result.Duration = time.Since(start)
		result.Stdout = stdout.String()
		result.Stderr = stderr.String()
		return result
	}
	result.Started = true
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		result.Err = err
	case <-ctx.Done():
		result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		result.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		killProcessTree(cmd)
		result.Err = ctx.Err()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.Duration = time.Since(start)
	return result
}

func runResolvedTool(ctx context.Context, project, name string, args []string, cfg Config) (string, error) {
	info := discoverTool(project, name, cfg, false)
	if !info.Available {
		var b strings.Builder
		fmt.Fprintf(&b, localizeConfigText(cfg, "%s wurde nicht gefunden.\n", "%s was not found.\n"), info.DisplayName)
		if len(info.SearchedPath) > 0 {
			b.WriteString(localizeConfigText(cfg, "Durchsuchte Pfade:\n", "Searched paths:\n"))
			for _, p := range info.SearchedPath {
				b.WriteString("- " + p + "\n")
			}
		}
		if info.InstallHint != "" {
			b.WriteString(localizeConfigText(cfg, "Installationshinweis: ", "Installation guidance: ") + info.InstallHint + "\n")
		}
		if info.DocsURL != "" {
			b.WriteString(localizeConfigText(cfg, "Offizielle Dokumentation: ", "Official documentation: ") + info.DocsURL + "\n")
		}
		if cfg.AutoResearchToolHelp && cfg.NetworkEnabled && cfg.WebSearchProvider != "disabled" {
			researchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			query := info.DisplayName + " official installation command line documentation Windows"
			if u, parseErr := url.Parse(info.DocsURL); parseErr == nil && u.Hostname() != "" {
				query = "site:" + u.Hostname() + " " + query
			}
			if results, searchErr := webSearch(researchCtx, cfg, query, 3); searchErr == nil && len(results) > 0 {
				b.WriteString(localizeConfigText(cfg, "\nAutomatisch recherchierte offizielle Hilfe:\n", "\nAutomatically researched official guidance:\n") + formatWebResults(results))
			}
			cancel()
		}
		return b.String(), &ToolNotFoundError{Info: info, Detail: b.String()}
	}
	res := runDirectTool(ctx, project, info.Path, args, cfg)
	res.Tool = info.Name
	if info.Name == "adb" {
		res = enrichADBResult(ctx, project, info.Path, args, cfg, res)
	}
	if res.Err != nil && info.DocsURL != "" && shouldResearchToolFailure(info.Name, args, res) {
		if res.Diagnostic != "" {
			res.Diagnostic += "\n"
		}
		res.Diagnostic += localizeConfigText(cfg, "Offizielle Dokumentation: ", "Official documentation: ") + info.DocsURL
		if cfg.AutoResearchToolHelp && cfg.NetworkEnabled && cfg.WebSearchProvider != "disabled" {
			researchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			query := info.DisplayName + " official documentation " + strings.Join(args, " ")
			if u, err := url.Parse(info.DocsURL); err == nil && u.Hostname() != "" {
				query = "site:" + u.Hostname() + " " + query
			}
			if results, err := webSearch(researchCtx, cfg, query, 3); err == nil && len(results) > 0 {
				res.Diagnostic += "\n\nAutomatisch recherchierte offizielle Hilfe:\n" + formatWebResults(results)
			}
			cancel()
		}
	}
	text := res.Text()
	return text, res.Err
}

func shouldResearchToolFailure(name string, args []string, res ToolRunResult) bool {
	text := strings.ToLower(res.Stdout + "\n" + res.Stderr)
	joined := strings.ToLower(strings.Join(args, " "))
	if name == "git" && (strings.Contains(text, "not a git repository") || strings.HasPrefix(joined, "--no-pager status")) {
		return false
	}
	if name == "adb" && (strings.HasPrefix(joined, "devices") || strings.Contains(text, "unauthorized") || strings.Contains(text, "offline")) {
		return false
	}
	for _, marker := range []string{"unknown option", "unrecognized option", "unknown command", "not recognized as", "command not found", "no such file or directory", "usage:"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func diagnoseADBPath(project, path string, cfg Config) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	res := runDirectTool(ctx, project, path, []string{"devices", "-l"}, cfg)
	return []string{adbDiagnostic(res.Stdout, res.Stderr, res.Err)}
}

func enrichADBResult(ctx context.Context, project, path string, args []string, cfg Config, initial ToolRunResult) ToolRunResult {
	joined := strings.ToLower(strings.Join(args, " "))
	if !strings.HasPrefix(joined, "devices") {
		return initial
	}
	initial.Diagnostic = adbDiagnostic(initial.Stdout, initial.Stderr, initial.Err)
	combined := strings.ToLower(initial.Stdout + "\n" + initial.Stderr)
	noDevice := initial.Err == nil && strings.Contains(combined, "list of devices attached") && adbDeviceCount(initial.Stdout+"\n"+initial.Stderr) == 0
	if initial.Err != nil || noDevice || strings.Contains(combined, "offline") || strings.Contains(combined, "cannot connect to daemon") {
		startCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		_ = runDirectTool(startCtx, project, path, []string{"start-server"}, cfg)
		cancel()
		if noDevice {
			reconnectCtx, reconnectCancel := context.WithTimeout(ctx, 12*time.Second)
			_ = runDirectTool(reconnectCtx, project, path, []string{"reconnect"}, cfg)
			reconnectCancel()
		}
		retryCtx, retryCancel := context.WithTimeout(ctx, 12*time.Second)
		retry := runDirectTool(retryCtx, project, path, args, cfg)
		retryCancel()
		retry.Tool = "adb"
		reason := "adb start-server"
		if noDevice {
			reason = "adb start-server und adb reconnect"
		}
		retry.Diagnostic = "Automatischer Wiederholungsversuch nach " + reason + ".\n" + adbDiagnostic(retry.Stdout, retry.Stderr, retry.Err)
		if adbDeviceCount(retry.Stdout+"\n"+retry.Stderr) == 0 {
			// Multiple SDKs frequently ship different adb.exe copies. Try every
			// discovered installation once and keep the first one that really sees
			// a device instead of assuming the first path is authoritative.
			profile := profileForTool("adb")
			for _, candidate := range toolCandidatePaths(project, profile, cfg) {
				if strings.EqualFold(candidate.path, path) {
					continue
				}
				if st, statErr := os.Stat(candidate.path); statErr != nil || st.IsDir() {
					continue
				}
				altCtx, altCancel := context.WithTimeout(ctx, 10*time.Second)
				alt := runDirectTool(altCtx, project, candidate.path, args, cfg)
				altCancel()
				if adbDeviceCount(alt.Stdout+"\n"+alt.Stderr) > 0 {
					alt.Tool = "adb"
					alt.Diagnostic = "Alternative ADB-Installation mit verbundenem Gerät ausgewählt: " + candidate.path + " [" + candidate.source + "]\n" + adbDiagnostic(alt.Stdout, alt.Stderr, alt.Err)
					return alt
				}
			}
			hostDiag := strings.TrimSpace(androidHostDeviceDiagnostic())
			if hostDiag != "" {
				retry.Diagnostic += "\n\n" + hostDiag
			}
		}
		return retry
	}
	return initial
}

func adbDeviceCount(text string) int {
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "list of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && (fields[1] == "device" || fields[1] == "unauthorized" || fields[1] == "offline" || fields[1] == "recovery" || fields[1] == "sideload") {
			count++
		}
	}
	return count
}

func adbDiagnostic(stdout, stderr string, err error) string {
	text := strings.ReplaceAll(stdout+"\n"+stderr, "\r", "")
	lower := strings.ToLower(text)
	if strings.Contains(lower, "unauthorized") {
		return "ADB wurde gefunden, aber mindestens ein Gerät ist nicht autorisiert. Gerät entsperren und den RSA-Dialog für USB-Debugging bestätigen."
	}
	if strings.Contains(lower, "offline") {
		return "ADB wurde gefunden, aber mindestens ein Gerät ist offline. Der Server wird einmal neu gestartet; danach Kabel/WLAN-Debugging prüfen."
	}
	deviceCount := adbDeviceCount(text)
	if deviceCount > 0 {
		return fmt.Sprintf("ADB funktioniert und meldet %d Gerät(e).", deviceCount)
	}
	if err == nil && strings.Contains(lower, "list of devices attached") {
		return "ADB funktioniert, aber es wurde kein Gerät aufgelistet. Das ist kein Installationsfehler. USB-Debugging, Datenkabel, Treiber oder Wireless-Debugging prüfen."
	}
	if err != nil {
		return "ADB-Aufruf ist fehlgeschlagen. Maßgeblich sind Pfad, Exitcode, STDOUT und STDERR oben; nicht erneut denselben Befehl ohne neue Diagnose ausführen."
	}
	return "ADB-Ausgabe konnte keinem bekannten Gerätestatus zugeordnet werden."
}

func actionSignature(a AgentAction) string {
	if a.Action == "ask_user" {
		return "ask_user|" + normalizedQuestion(a.Message)
	}
	return strings.ToLower(strings.TrimSpace(a.Action + "|" + a.Tool + "|" + a.Command + "|" + strings.Join(a.Args, "|") + "|" + a.Path + "|" + a.Query + "|" + a.URL + "|" + a.Source + "|" + a.Destination + "|" + a.Task + "|" + a.Name))
}

func normalizedQuestion(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "?", "", ".", "", ",", "", "!", "", "-", " ", ":", " ", ";", " ")
	return strings.Join(strings.Fields(replacer.Replace(s)), " ")
}

func sameQuestion(a, b string) bool {
	a = normalizedQuestion(a)
	b = normalizedQuestion(b)
	if a == "" || b == "" {
		return false
	}
	return a == b || strings.Contains(a, b) || strings.Contains(b, a)
}

func knownToolName(name string) (string, bool) {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(strings.Trim(name, `"'`))))
	for _, p := range toolProfiles {
		if base == p.Name {
			return p.Name, true
		}
		for _, a := range p.Aliases {
			if base == strings.ToLower(a) {
				return p.Name, true
			}
		}
	}
	return "", false
}

func splitCommandHead(command string) (head, rest string, ok bool) {
	s := strings.TrimSpace(command)
	if s == "" || strings.ContainsAny(s, "\r\n") {
		return "", "", false
	}
	if strings.HasPrefix(s, "&") || strings.HasPrefix(s, "|") || strings.HasPrefix(s, ".") {
		return "", "", false
	}
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		for i := 1; i < len(s); i++ {
			if s[i] == quote {
				return s[1:i], strings.TrimSpace(s[i+1:]), true
			}
		}
		return "", "", false
	}
	for i, r := range s {
		if r == ' ' || r == '\t' {
			return s[:i], strings.TrimSpace(s[i:]), true
		}
		if strings.ContainsRune("|;&><()", r) {
			return "", "", false
		}
	}
	return s, "", true
}

func rewriteKnownToolCommand(project, command string, cfg Config, shell string) (string, string, error) {
	if !cfg.AutoDiscoverTools {
		return command, "", nil
	}
	head, rest, ok := splitCommandHead(command)
	if !ok || strings.ContainsAny(head, `\/`) {
		return command, "", nil
	}
	name, known := knownToolName(head)
	if !known {
		return command, "", nil
	}
	info := discoverTool(project, name, cfg, false)
	if !info.Available {
		var detail strings.Builder
		fmt.Fprintf(&detail, "%s wurde vor der Ausführung nicht gefunden.\n", info.DisplayName)
		if len(info.SearchedPath) > 0 {
			detail.WriteString("Durchsuchte Pfade:\n")
			for _, p := range info.SearchedPath {
				detail.WriteString("- " + p + "\n")
			}
		}
		if info.DocsURL != "" {
			detail.WriteString("Offizielle Dokumentation: " + info.DocsURL + "\n")
		}
		return command, detail.String(), &ToolNotFoundError{Info: info, Detail: detail.String()}
	}
	quoted := info.Path
	switch shell {
	case "powershell":
		quoted = "& '" + strings.ReplaceAll(info.Path, "'", "''") + "'"
	case "cmd":
		quoted = `"` + strings.ReplaceAll(info.Path, `"`, `""`) + `"`
	default:
		quoted = strconv.Quote(info.Path)
	}
	if rest != "" {
		quoted += " " + rest
	}
	return quoted, fmt.Sprintf("Werkzeug %s automatisch aufgelöst: %s", name, info.Path), nil
}
