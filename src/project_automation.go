// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type projectPlan struct {
	Kind       string
	BuildTool  string
	BuildArgs  []string
	Descriptor string
}

func detectProjectPlan(project string) projectPlan {
	exists := func(name string) bool {
		st, err := os.Stat(filepath.Join(project, name))
		return err == nil && !st.IsDir()
	}
	firstGlob := func(pattern string) string {
		matches, _ := filepath.Glob(filepath.Join(project, pattern))
		sort.Strings(matches)
		if len(matches) == 0 {
			return ""
		}
		return matches[0]
	}

	gradleWrapper := exists("gradlew.bat") || exists("gradlew")
	androidManifest := firstGlob(filepath.Join("*", "src", "main", "AndroidManifest.xml"))
	if androidManifest == "" && exists(filepath.Join("app", "src", "main", "AndroidManifest.xml")) {
		androidManifest = filepath.Join(project, "app", "src", "main", "AndroidManifest.xml")
	}
	if gradleWrapper && androidManifest != "" {
		return projectPlan{Kind: "android-gradle", BuildTool: "gradle", BuildArgs: []string{"--no-daemon", "assembleDebug"}, Descriptor: "Android-Projekt mit Gradle Wrapper"}
	}
	if exists("go.mod") {
		return projectPlan{Kind: "go", BuildTool: "go", BuildArgs: []string{"build", "./..."}, Descriptor: "Go-Modul"}
	}
	if exists("Cargo.toml") {
		return projectPlan{Kind: "rust", BuildTool: "cargo", BuildArgs: []string{"build"}, Descriptor: "Rust/Cargo-Projekt"}
	}
	if exists("package.json") {
		if data, err := os.ReadFile(filepath.Join(project, "package.json")); err == nil {
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if json.Unmarshal(data, &pkg) == nil {
				if strings.TrimSpace(pkg.Scripts["build"]) != "" {
					return projectPlan{Kind: "node", BuildTool: "npm", BuildArgs: []string{"run", "build"}, Descriptor: "Node.js-Projekt mit build-Skript"}
				}
			}
		}
		return projectPlan{Kind: "node", Descriptor: "Node.js-Projekt ohne build-Skript"}
	}
	if sln := firstGlob("*.sln"); sln != "" {
		return projectPlan{Kind: "visual-studio", BuildTool: "msbuild", BuildArgs: []string{filepath.Base(sln), "/m", "/restore", "/p:Configuration=Debug"}, Descriptor: "Visual-Studio-Lösung"}
	}
	if csproj := firstGlob("*.csproj"); csproj != "" {
		data, _ := os.ReadFile(csproj)
		if strings.Contains(string(data), "<Project Sdk=") {
			return projectPlan{Kind: "dotnet", BuildTool: "dotnet", BuildArgs: []string{"build", filepath.Base(csproj)}, Descriptor: ".NET-SDK-Projekt"}
		}
		return projectPlan{Kind: "visual-studio", BuildTool: "msbuild", BuildArgs: []string{filepath.Base(csproj), "/m", "/restore", "/p:Configuration=Debug"}, Descriptor: "klassisches MSBuild-Projekt"}
	}
	if gradleWrapper {
		return projectPlan{Kind: "gradle", BuildTool: "gradle", BuildArgs: []string{"--no-daemon", "build"}, Descriptor: "Gradle-Projekt mit Wrapper"}
	}
	if exists("CMakeLists.txt") {
		return projectPlan{Kind: "cmake", BuildTool: "cmake", Descriptor: "CMake-Projekt"}
	}
	if exists("pyproject.toml") || exists("setup.py") || exists("requirements.txt") {
		return projectPlan{Kind: "python", BuildTool: "python", BuildArgs: []string{"-m", "compileall", "."}, Descriptor: "Python-Projekt"}
	}
	return projectPlan{Kind: "unknown", Descriptor: "Kein unterstütztes Buildsystem wurde eindeutig erkannt"}
}

func projectInfo(project string, cfg Config) string {
	plan := detectProjectPlan(project)
	var b strings.Builder
	fmt.Fprintf(&b, "Projektart: %s\nBeschreibung: %s\n", plan.Kind, plan.Descriptor)
	if plan.BuildTool != "" {
		info := discoverTool(project, plan.BuildTool, cfg, true)
		fmt.Fprintf(&b, "Buildwerkzeug: %s\nVerfügbar: %t\n", info.DisplayName, info.Available)
		if info.Path != "" {
			fmt.Fprintf(&b, "Pfad: %s [%s]\n", info.Path, info.Source)
		}
		if len(plan.BuildArgs) > 0 {
			fmt.Fprintf(&b, "Vorgesehene Argumente: %s\n", quoteArgs(plan.BuildArgs))
		}
	}
	if plan.Kind == "android-gradle" {
		apks := findAPKs(project)
		fmt.Fprintf(&b, "Gefundene APKs: %d\n", len(apks))
		for i, apk := range apks {
			if i >= 10 {
				break
			}
			fmt.Fprintf(&b, "- %s\n", apk)
		}
		adb := discoverTool(project, "adb", cfg, true)
		fmt.Fprintf(&b, "ADB verfügbar: %t\n", adb.Available)
		if adb.Path != "" {
			fmt.Fprintf(&b, "ADB-Pfad: %s [%s]\n", adb.Path, adb.Source)
		}
		for _, diag := range adb.Diagnostics {
			fmt.Fprintf(&b, "ADB-Diagnose: %s\n", diag)
		}
	}
	return b.String()
}

func buildProject(ctx context.Context, project string, cfg Config) (string, error) {
	plan := detectProjectPlan(project)
	if plan.Kind == "unknown" {
		return projectInfo(project, cfg), errors.New("build system could not be detected")
	}
	if plan.Kind == "node" && plan.BuildTool == "" {
		return projectInfo(project, cfg), errors.New("package.json has no build script")
	}
	if plan.Kind == "android-gradle" || plan.Kind == "gradle" {
		java := discoverTool(project, "java", cfg, false)
		if !java.Available {
			return projectInfo(project, cfg), &ToolNotFoundError{Info: java, Detail: "Java ist für den Gradle-Wrapper erforderlich."}
		}
	}
	if plan.Kind == "cmake" {
		var outputs []string
		if st, err := os.Stat(filepath.Join(project, "build")); err != nil || !st.IsDir() {
			out, err := runResolvedTool(ctx, project, "cmake", []string{"-S", ".", "-B", "build"}, cfg)
			outputs = append(outputs, "CMAKE CONFIGURE:\n"+out)
			if err != nil {
				return strings.Join(outputs, "\n\n"), err
			}
		}
		out, err := runResolvedTool(ctx, project, "cmake", []string{"--build", "build", "--config", "Debug"}, cfg)
		outputs = append(outputs, "CMAKE BUILD:\n"+out)
		return strings.Join(outputs, "\n\n"), err
	}
	out, err := runResolvedTool(ctx, project, plan.BuildTool, plan.BuildArgs, cfg)
	return "ERKANNTES PROJEKT:\n" + projectInfo(project, cfg) + "\nBUILD:\n" + out, err
}

type adbDevice struct {
	Serial string
	State  string
	Line   string
}

func parseADBDevices(text string) []adbDevice {
	var out []adbDevice
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "list of devices") || strings.HasPrefix(line, "*") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			switch fields[1] {
			case "device", "unauthorized", "offline", "recovery", "sideload":
				out = append(out, adbDevice{Serial: fields[0], State: fields[1], Line: line})
			}
		}
	}
	return out
}

func findAPKs(project string) []string {
	type item struct {
		path string
		mod  time.Time
	}
	var items []item
	_ = filepath.WalkDir(project, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if name == ".git" || name == ".gradle" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".apk") {
			return nil
		}
		lower := strings.ToLower(path)
		if strings.Contains(lower, "androidtest") || strings.Contains(lower, "unaligned") {
			return nil
		}
		info, statErr := d.Info()
		if statErr == nil {
			items = append(items, item{path: path, mod: info.ModTime()})
		}
		return nil
	})
	sort.Slice(items, func(i, j int) bool {
		iDebug := strings.Contains(strings.ToLower(items[i].path), "debug")
		jDebug := strings.Contains(strings.ToLower(items[j].path), "debug")
		if iDebug != jDebug {
			return iDebug
		}
		return items[i].mod.After(items[j].mod)
	})
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.path)
	}
	return out
}

func deployAndroid(ctx context.Context, project string, cfg Config) (string, error) {
	plan := detectProjectPlan(project)
	if plan.Kind != "android-gradle" {
		return projectInfo(project, cfg), errors.New("selected project is not recognized as an Android Gradle project")
	}
	buildOut, err := buildProject(ctx, project, cfg)
	if err != nil {
		return buildOut, err
	}
	apks := findAPKs(project)
	if len(apks) == 0 {
		return buildOut, errors.New("build succeeded but no APK was found")
	}
	adbInfo := discoverTool(project, "adb", cfg, false)
	if !adbInfo.Available {
		return buildOut, &ToolNotFoundError{Info: adbInfo, Detail: "ADB is required to deploy the APK."}
	}
	devicesResult := runDirectTool(ctx, project, adbInfo.Path, []string{"devices", "-l"}, cfg)
	devicesResult.Tool = "adb"
	if devicesResult.Err != nil {
		return buildOut + "\n\nADB DEVICES:\n" + devicesResult.Text(), devicesResult.Err
	}
	devices := parseADBDevices(devicesResult.Stdout + "\n" + devicesResult.Stderr)
	var ready []adbDevice
	for _, device := range devices {
		if device.State == "device" {
			ready = append(ready, device)
		}
	}
	if len(ready) == 0 {
		detail := buildOut + "\n\nADB DEVICES:\n" + devicesResult.Text() + "\n" + adbDiagnostic(devicesResult.Stdout, devicesResult.Stderr, devicesResult.Err)
		return detail, errors.New("no authorized online Android device is available")
	}
	if len(ready) > 1 {
		var lines []string
		for _, device := range ready {
			lines = append(lines, device.Line)
		}
		return buildOut + "\n\nMehrere Geräte sind verbunden:\n" + strings.Join(lines, "\n"), errors.New("multiple Android devices require an explicit serial")
	}
	apk := apks[0]
	args := []string{"-s", ready[0].Serial, "install", "-r", apk}
	installOut, installErr := runResolvedTool(ctx, project, "adb", args, cfg)
	result := buildOut + "\n\nAPK: " + apk + "\nGERÄT: " + ready[0].Line + "\n\nADB INSTALL:\n" + installOut
	return result, installErr
}
