// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	if androidManifest == "" && exists("AndroidManifest.xml") {
		androidManifest = filepath.Join(project, "AndroidManifest.xml")
	}
	if androidManifest != "" {
		if gradleWrapper {
			return projectPlan{Kind: "android-gradle", BuildTool: "gradle", BuildArgs: []string{"--no-daemon", "assembleDebug"}, Descriptor: "Android-Projekt mit Gradle Wrapper"}
		}
		return projectPlan{Kind: "android-sdk", BuildTool: "javac", Descriptor: "Android-Projekt mit Android SDK"}
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

func findAndroidJar() string {
	sdk := os.Getenv("ANDROID_SDK_ROOT")
	if sdk == "" {
		sdk = os.Getenv("ANDROID_HOME")
	}
	if sdk == "" {
		sdk = filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk")
	}
	platformsDir := filepath.Join(sdk, "platforms")
	entries, err := os.ReadDir(platformsDir)
	if err != nil {
		return ""
	}
	var platforms []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(strings.ToLower(e.Name()), "android-") {
			platforms = append(platforms, e.Name())
		}
	}
	sort.Slice(platforms, func(i, j int) bool {
		return platforms[i] > platforms[j]
	})
	for _, p := range platforms {
		candidate := filepath.Join(platformsDir, p, "android.jar")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

func buildAndroidSDKProject(ctx context.Context, project string, cfg Config) (string, error) {
	manifest := ""
	for _, cand := range []string{
		filepath.Join(project, "app", "src", "main", "AndroidManifest.xml"),
		filepath.Join(project, "src", "main", "AndroidManifest.xml"),
		filepath.Join(project, "AndroidManifest.xml"),
	} {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			manifest = cand
			break
		}
	}
	if manifest == "" {
		return "", errors.New("AndroidManifest.xml nicht gefunden")
	}
	resDir := filepath.Join(filepath.Dir(manifest), "res")
	if st, err := os.Stat(resDir); err != nil || !st.IsDir() {
		resDir = filepath.Join(project, "res")
	}
	javaDir := filepath.Join(filepath.Dir(manifest), "java")
	if st, err := os.Stat(javaDir); err != nil || !st.IsDir() {
		javaDir = filepath.Join(project, "src")
	}

	androidJar := findAndroidJar()
	if androidJar == "" {
		return "", errors.New("Android platform android.jar nicht im Android SDK gefunden")
	}

	aapt2 := discoverTool(project, "aapt2", cfg, false)
	if !aapt2.Available {
		return "", &ToolNotFoundError{Info: aapt2, Detail: "aapt2 ist für das Kompilieren von Android-Ressourcen erforderlich."}
	}
	javac := discoverTool(project, "javac", cfg, false)
	if !javac.Available {
		return "", &ToolNotFoundError{Info: javac, Detail: "javac ist für das Kompilieren von Java-Quellcode erforderlich."}
	}
	d8 := discoverTool(project, "d8", cfg, false)
	if !d8.Available {
		return "", &ToolNotFoundError{Info: d8, Detail: "d8 ist für die DEX-Bytecode-Generierung erforderlich."}
	}
	zipalign := discoverTool(project, "zipalign", cfg, false)
	if !zipalign.Available {
		return "", &ToolNotFoundError{Info: zipalign, Detail: "zipalign ist für das Alignment des APKs erforderlich."}
	}
	apksigner := discoverTool(project, "apksigner", cfg, false)
	if !apksigner.Available {
		return "", &ToolNotFoundError{Info: apksigner, Detail: "apksigner ist für das Signieren des APKs erforderlich."}
	}
	keytool := discoverTool(project, "keytool", cfg, false)

	buildDir := filepath.Join(project, "build", "outputs", "apk", "debug")
	classesDir := filepath.Join(project, "build", "classes")
	genDir := filepath.Join(project, "build", "gen")
	compiledResDir := filepath.Join(project, "build", "compiled-res")

	_ = os.RemoveAll(filepath.Join(project, "build"))
	for _, d := range []string{buildDir, classesDir, genDir, compiledResDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
	}

	var logs []string

	// 1. aapt2 compile
	resZip := filepath.Join(compiledResDir, "resources.zip")
	if st, err := os.Stat(resDir); err == nil && st.IsDir() {
		compileOut, err := runResolvedTool(ctx, project, "aapt2", []string{"compile", "--dir", resDir, "-o", resZip}, cfg)
		logs = append(logs, "AAPT2 COMPILE:\n"+compileOut)
		if err != nil {
			return strings.Join(logs, "\n\n"), err
		}
	}

	// 2. aapt2 link
	unalignedApk := filepath.Join(buildDir, "app-unaligned.apk")
	linkArgs := []string{"link", "-I", androidJar, "--manifest", manifest, "-o", unalignedApk, "--java", genDir, "--auto-add-overlay"}
	if _, err := os.Stat(resZip); err == nil {
		linkArgs = append(linkArgs, resZip)
	}
	linkOut, err := runResolvedTool(ctx, project, "aapt2", linkArgs, cfg)
	logs = append(logs, "AAPT2 LINK:\n"+linkOut)
	if err != nil {
		return strings.Join(logs, "\n\n"), err
	}

	// 3. Collect Java files + R.java
	var javaSources []string
	_ = filepath.WalkDir(javaDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".java") {
			javaSources = append(javaSources, p)
		}
		return nil
	})
	_ = filepath.WalkDir(genDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".java") {
			javaSources = append(javaSources, p)
		}
		return nil
	})

	if len(javaSources) == 0 {
		return strings.Join(logs, "\n\n"), errors.New("keine Java-Quelldateien im Projekt gefunden")
	}

	// 4. javac
	javacArgs := []string{"-encoding", "UTF-8", "-source", "17", "-target", "17", "-cp", androidJar, "-d", classesDir}
	javacArgs = append(javacArgs, javaSources...)
	javacOut, err := runResolvedTool(ctx, project, "javac", javacArgs, cfg)
	logs = append(logs, "JAVAC:\n"+javacOut)
	if err != nil {
		return strings.Join(logs, "\n\n"), err
	}

	// 5. d8 DEX
	var classFiles []string
	_ = filepath.WalkDir(classesDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".class") {
			classFiles = append(classFiles, p)
		}
		return nil
	})
	d8Args := []string{"--output", buildDir, "--lib", androidJar}
	d8Args = append(d8Args, classFiles...)
	d8Out, err := runResolvedTool(ctx, project, "d8", d8Args, cfg)
	logs = append(logs, "D8:\n"+d8Out)
	if err != nil {
		return strings.Join(logs, "\n\n"), err
	}

	// 6. Pack classes.dex into unaligned apk
	dexFile := filepath.Join(buildDir, "classes.dex")
	if st, err := os.Stat(dexFile); err != nil || st.IsDir() {
		return strings.Join(logs, "\n\n"), errors.New("classes.dex was not generated by d8")
	}
	if err := appendFileToZipArchive(unalignedApk, dexFile, "classes.dex"); err != nil {
		logs = append(logs, fmt.Sprintf("ZIP INJECT WARNING: %v", err))
		if java := discoverTool(project, "java", cfg, false); java.Available {
			jarTool := filepath.Join(filepath.Dir(java.Path), "jar.exe")
			if runtime.GOOS != "windows" {
				jarTool = filepath.Join(filepath.Dir(java.Path), "jar")
			}
			if st, err := os.Stat(jarTool); err == nil && !st.IsDir() {
				runDirectTool(ctx, buildDir, jarTool, []string{"uf", unalignedApk, "classes.dex"}, cfg)
			}
		}
	}

	// 7. zipalign
	alignedApk := filepath.Join(buildDir, "app-aligned.apk")
	alignOut, err := runResolvedTool(ctx, project, "zipalign", []string{"-f", "-p", "4", unalignedApk, alignedApk}, cfg)
	logs = append(logs, "ZIPALIGN:\n"+alignOut)
	if err != nil {
		return strings.Join(logs, "\n\n"), err
	}

	// 8. Keytool (if debug.keystore missing)
	keystore := filepath.Join(buildDir, "debug.keystore")
	if keytool.Available {
		runResolvedTool(ctx, project, "keytool", []string{
			"-genkeypair", "-v", "-keystore", keystore, "-alias", "androiddebugkey",
			"-keyalg", "RSA", "-keysize", "2048", "-validity", "10000",
			"-storepass", "android", "-keypass", "android", "-dname", "CN=Debug, O=LocalCode, C=DE",
		}, cfg)
	}

	// 9. apksigner
	finalApk := filepath.Join(buildDir, "app-debug.apk")
	signerOut, err := runResolvedTool(ctx, project, "apksigner", []string{
		"sign", "--ks", keystore, "--ks-pass", "pass:android", "--key-pass", "pass:android",
		"--out", finalApk, alignedApk,
	}, cfg)
	logs = append(logs, "APKSIGNER:\n"+signerOut)
	if err != nil {
		return strings.Join(logs, "\n\n"), err
	}

	return "ERKANNTES PROJEKT:\n" + projectInfo(project, cfg) + "\n\n" + strings.Join(logs, "\n\n") + "\n\nFINAL APK: " + finalApk, nil
}

func buildProject(ctx context.Context, project string, cfg Config) (string, error) {
	plan := detectProjectPlan(project)
	if plan.Kind == "unknown" {
		return projectInfo(project, cfg), errors.New("Kein Buildsystem oder keine Quellcode-Dateien erkannt. Wenn dies ein neues Projekt oder eine neue Android-App ist, erstelle zuerst die erforderlichen Quellcode-Dateien (z.B. AndroidManifest.xml, res/ und Java-Quellen mit write_file) und führe build_project danach erneut aus.")
	}
	if plan.Kind == "android-sdk" {
		return buildAndroidSDKProject(ctx, project, cfg)
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
	if plan.Kind != "android-gradle" && plan.Kind != "android-sdk" {
		return projectInfo(project, cfg), errors.New("selected project is not recognized as an Android project")
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
	args := []string{"-s", ready[0].Serial, "install", "-r", "-d", "-g", apk}
	installOut, installErr := runResolvedTool(ctx, project, "adb", args, cfg)
	result := buildOut + "\n\nAPK: " + apk + "\nGERÄT: " + ready[0].Line + "\n\nADB INSTALL:\n" + installOut
	if installErr != nil {
		return result, installErr
	}

	manifestPath := findProjectManifest(project)
	if manifestPath != "" {
		if pkg, launcher, err := extractManifestPackageAndLauncher(manifestPath); err == nil && pkg != "" && launcher != "" {
			component := pkg + "/" + launcher
			if !strings.Contains(launcher, ".") {
				component = pkg + "/." + launcher
			}
			startArgs := []string{"-s", ready[0].Serial, "shell", "am", "start", "-n", component}
			startOut, startErr := runResolvedTool(ctx, project, "adb", startArgs, cfg)
			result += "\n\nADB START:\n" + startOut
			if startErr != nil {
				return result, startErr
			}
		}
	}
	return result, nil
}

type androidManifestXML struct {
	XMLName     xml.Name `xml:"manifest"`
	Package     string   `xml:"package,attr"`
	Application struct {
		Activities []struct {
			Name         string `xml:"name,attr"`
			IntentFilter []struct {
				Action []struct {
					Name string `xml:"name,attr"`
				} `xml:"action"`
				Category []struct {
					Name string `xml:"name,attr"`
				} `xml:"category"`
			} `xml:"intent-filter"`
		} `xml:"activity"`
	} `xml:"application"`
}

func findProjectManifest(project string) string {
	candidates := []string{
		filepath.Join(project, "AndroidManifest.xml"),
		filepath.Join(project, "app", "src", "main", "AndroidManifest.xml"),
		filepath.Join(project, "src", "main", "AndroidManifest.xml"),
	}
	for _, path := range candidates {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path
		}
	}
	return ""
}

func extractManifestPackageAndLauncher(manifestPath string) (pkg string, launcher string, err error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", "", err
	}
	var manifest androidManifestXML
	if err := xml.Unmarshal(data, &manifest); err != nil {
		return "", "", err
	}
	pkg = strings.TrimSpace(manifest.Package)
	if pkg == "" {
		return "", "", errors.New("manifest missing package attribute")
	}
	for _, act := range manifest.Application.Activities {
		isMain := false
		isLauncher := false
		for _, filter := range act.IntentFilter {
			for _, a := range filter.Action {
				if strings.EqualFold(strings.TrimSpace(a.Name), "android.intent.action.MAIN") {
					isMain = true
				}
			}
			for _, c := range filter.Category {
				if strings.EqualFold(strings.TrimSpace(c.Name), "android.intent.category.LAUNCHER") {
					isLauncher = true
				}
			}
		}
		if isMain && isLauncher {
			launcher = strings.TrimSpace(act.Name)
			break
		}
	}
	if launcher == "" && len(manifest.Application.Activities) > 0 {
		launcher = strings.TrimSpace(manifest.Application.Activities[0].Name)
	}
	return pkg, launcher, nil
}

type adbActionRequest struct {
	Action string `json:"action"` // "devices", "install", "launch", "stop", "logcat", "screenshot", "reverse", "tcpip", "connect"
	Serial string `json:"serial,omitempty"`
	Target string `json:"target,omitempty"` // component, package, IP, or apk path
}

func listConnectedADBDevices(ctx context.Context, project string, cfg Config) ([]adbDevice, error) {
	adbInfo := discoverTool(project, "adb", cfg, false)
	if !adbInfo.Available {
		return nil, &ToolNotFoundError{Info: adbInfo, Detail: "ADB is required to list devices."}
	}
	res := runDirectTool(ctx, project, adbInfo.Path, []string{"devices", "-l"}, cfg)
	res.Tool = "adb"
	if res.Err != nil {
		return nil, res.Err
	}
	return parseADBDevices(res.Stdout + "\n" + res.Stderr), nil
}

func runADBAction(ctx context.Context, project string, req adbActionRequest, cfg Config) (string, error) {
	adbInfo := discoverTool(project, "adb", cfg, false)
	if !adbInfo.Available {
		return "", &ToolNotFoundError{Info: adbInfo, Detail: "ADB is required to execute action."}
	}
	var args []string
	if strings.TrimSpace(req.Serial) != "" {
		args = append(args, "-s", strings.TrimSpace(req.Serial))
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "devices":
		args = []string{"devices", "-l"}
	case "install":
		apk := strings.TrimSpace(req.Target)
		if apk == "" {
			apks := findAPKs(project)
			if len(apks) == 0 {
				return "", errors.New("no APK found in project to install")
			}
			apk = apks[0]
		}
		args = append(args, "install", "-r", "-d", "-g", apk)
	case "launch":
		target := strings.TrimSpace(req.Target)
		if target == "" {
			manifestPath := findProjectManifest(project)
			if manifestPath != "" {
				pkg, launcher, _ := extractManifestPackageAndLauncher(manifestPath)
				if pkg != "" && launcher != "" {
					target = pkg + "/" + launcher
					if !strings.Contains(launcher, ".") {
						target = pkg + "/." + launcher
					}
				}
			}
		}
		if target == "" {
			return "", errors.New("target activity component is required to launch")
		}
		args = append(args, "shell", "am", "start", "-n", target)
	case "stop":
		pkg := strings.TrimSpace(req.Target)
		if pkg == "" {
			manifestPath := findProjectManifest(project)
			if manifestPath != "" {
				p, _, _ := extractManifestPackageAndLauncher(manifestPath)
				pkg = p
			}
		}
		if pkg == "" {
			return "", errors.New("target package name is required to stop app")
		}
		args = append(args, "shell", "am", "force-stop", pkg)
	case "logcat":
		args = append(args, "logcat", "-d", "-v", "time", "-t", "150")
		if strings.TrimSpace(req.Target) != "" {
			args = append(args, "-s", strings.TrimSpace(req.Target))
		}
	case "reverse":
		port := "32145"
		if strings.TrimSpace(req.Target) != "" {
			port = strings.TrimSpace(req.Target)
		}
		args = append(args, "reverse", "tcp:"+port, "tcp:"+port)
	case "tcpip":
		port := "5555"
		if strings.TrimSpace(req.Target) != "" {
			port = strings.TrimSpace(req.Target)
		}
		args = append(args, "tcpip", port)
	case "connect":
		if strings.TrimSpace(req.Target) == "" {
			return "", errors.New("target host:port is required to connect")
		}
		args = []string{"connect", strings.TrimSpace(req.Target)}
	default:
		return "", fmt.Errorf("unsupported ADB action: %s", req.Action)
	}

	return runResolvedTool(ctx, project, "adb", args, cfg)
}

func appendFileToZipArchive(zipPath, filePath, entryName string) error {
	dexBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	tmpZip := zipPath + ".tmp"
	outFile, err := os.Create(tmpZip)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(outFile)

	for _, f := range r.File {
		if f.Name == entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmpZip)
			return err
		}
		header := f.FileHeader
		w, err := zw.CreateHeader(&header)
		if err != nil {
			_ = rc.Close()
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmpZip)
			return err
		}
		_, err = io.Copy(w, rc)
		_ = rc.Close()
		if err != nil {
			_ = zw.Close()
			_ = outFile.Close()
			_ = os.Remove(tmpZip)
			return err
		}
	}

	dexHeader := &zip.FileHeader{
		Name:   entryName,
		Method: zip.Store,
	}
	dexHeader.Modified = time.Now()
	w, err := zw.CreateHeader(dexHeader)
	if err != nil {
		_ = zw.Close()
		_ = outFile.Close()
		_ = os.Remove(tmpZip)
		return err
	}
	if _, err := w.Write(dexBytes); err != nil {
		_ = zw.Close()
		_ = outFile.Close()
		_ = os.Remove(tmpZip)
		return err
	}

	if err := zw.Close(); err != nil {
		_ = outFile.Close()
		_ = os.Remove(tmpZip)
		return err
	}
	if err := outFile.Close(); err != nil {
		_ = os.Remove(tmpZip)
		return err
	}
	_ = r.Close()

	return os.Rename(tmpZip, zipPath)
}
