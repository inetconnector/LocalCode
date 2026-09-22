// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowsPlatformDiscoveryAndSafeFailureBranches(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	t.Setenv("ProgramFiles", empty)
	t.Setenv("ProgramFiles(x86)", empty)
	t.Setenv("LOCALAPPDATA", empty)

	candidates := chromiumBrowserCandidates()
	if len(candidates) < 6 {
		t.Fatalf("expected standard Chromium candidates, got %#v", candidates)
	}
	if err := openChromiumApp("http://127.0.0.1:1", false); err == nil {
		t.Fatal("openChromiumApp should fail when no candidate exists")
	}
	if err := openChromiumApp("http://127.0.0.1:1", true); err == nil {
		t.Fatal("compact openChromiumApp should fail when no candidate exists")
	}
	if err := openBrowser("http://127.0.0.1:1"); err == nil {
		t.Fatal("openBrowser should fail with browser and rundll32 unavailable")
	}
	if err := openStartupBrowser("http://127.0.0.1:1"); err == nil {
		t.Fatal("openStartupBrowser should fail with browser launchers unavailable")
	}
	if err := startOllamaDetached(filepath.Join(empty, "missing-ollama.exe")); err == nil {
		t.Fatal("starting missing Ollama executable must fail")
	}

	ollamaPath := filepath.Join(empty, "Programs", "Ollama", "ollama.exe")
	if err := os.MkdirAll(filepath.Dir(ollamaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ollamaPath, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := findOllamaExecutable(); !strings.EqualFold(got, ollamaPath) {
		t.Fatalf("findOllamaExecutable=%q want %q", got, ollamaPath)
	}

	if got := windowsPathToWSL(`C:\Work Dir\Project\file.txt`); got != `/mnt/c/Work Dir/Project/file.txt` {
		t.Fatalf("windowsPathToWSL=%q", got)
	}
	if got := windowsPathToWSL(`relative\path`); got != `relative/path` {
		t.Fatalf("relative windowsPathToWSL=%q", got)
	}
	if selected, err := selectDirectory(empty, "en"); err == nil || selected != "" {
		t.Fatalf("selectDirectory without PowerShell selected=%q err=%v", selected, err)
	}
	if gpu := detectGPU(); gpu != "" {
		t.Fatalf("detectGPU should be empty without nvidia-smi, got %q", gpu)
	}
	if diagnostic := androidHostDeviceDiagnostic(); strings.TrimSpace(diagnostic) == "" {
		t.Fatal("Android host diagnostic must always return guidance or detected devices")
	}
	unsigned := filepath.Join(empty, "unsigned.exe")
	if err := os.WriteFile(unsigned, []byte("not signed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyAuthenticodeSignature(unsigned); err == nil {
		t.Fatal("unsigned/non-executable fixture must not pass Authenticode verification")
	}
	if language := detectSystemLanguage(); strings.TrimSpace(language) == "" {
		t.Fatal("system language must not be empty")
	}
}

func TestOpenProjectTargetVisualStudioFallsBackWithoutProjectFile(t *testing.T) {
	root := t.TempDir()
	originalStarter := startProjectTargetProcess
	t.Cleanup(func() { startProjectTargetProcess = originalStarter })
	var calls []string
	startProjectTargetProcess = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return nil
	}

	cfg := defaultConfig()
	if err := openProjectTarget(root, "visualstudio", cfg); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !strings.HasPrefix(strings.ToLower(calls[0]), "explorer.exe ") || !strings.Contains(calls[0], root) {
		t.Fatalf("Visual Studio without a solution/project should fall back to Explorer, calls=%#v", calls)
	}

	sln := filepath.Join(root, "Demo.sln")
	if err := os.WriteFile(sln, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	devenv := filepath.Join(root, "devenv.exe")
	if err := os.WriteFile(devenv, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root)
	calls = nil
	if err := openProjectTarget(root, "visualstudio", cfg); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !strings.HasPrefix(strings.ToLower(calls[0]), strings.ToLower(devenv)) || !strings.Contains(calls[0], sln) {
		t.Fatalf("Visual Studio should receive the solution path, calls=%#v", calls)
	}
}

func TestBuildProjectUsesControlledToolOverrides(t *testing.T) {
	tools := t.TempDir()
	goTool := writeWindowsCmdFixture(t, tools, "go-fixture", `
if "%1"=="version" (echo go version go1.25.13 windows/amd64& exit /b 0)
echo fixture-go %*
exit /b 0`)
	cmakeTool := writeWindowsCmdFixture(t, tools, "cmake-fixture", `
if "%1"=="--version" (echo cmake version 4.0.0& exit /b 0)
echo fixture-cmake %*
exit /b 0`)

	cfg := defaultConfig()
	cfg.ToolOverrides = map[string]string{"go": goTool, "cmake": cmakeTool}
	cfg.AutoResearchToolHelp = false
	cfg.NetworkEnabled = false

	unknown := t.TempDir()
	if out, err := buildProject(context.Background(), unknown, cfg); err == nil || !strings.Contains(out, "unknown") {
		t.Fatalf("unknown build output=%q err=%v", out, err)
	}

	node := t.TempDir()
	if err := os.WriteFile(filepath.Join(node, "package.json"), []byte(`{"scripts":{"test":"echo test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := buildProject(context.Background(), node, cfg); err == nil || !strings.Contains(out, "Node.js") {
		t.Fatalf("node-without-build output=%q err=%v", out, err)
	}

	goProject := t.TempDir()
	if err := os.WriteFile(filepath.Join(goProject, "go.mod"), []byte("module fixture\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := buildProject(context.Background(), goProject, cfg)
	if err != nil || !strings.Contains(out, "fixture-go build ./...") || !strings.Contains(out, "Go-Modul") {
		t.Fatalf("Go build output=%q err=%v", out, err)
	}

	cmakeProject := t.TempDir()
	if err := os.WriteFile(filepath.Join(cmakeProject, "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = buildProject(context.Background(), cmakeProject, cfg)
	if err != nil || !strings.Contains(out, "CMAKE CONFIGURE") || !strings.Contains(out, "CMAKE BUILD") || !strings.Contains(out, "fixture-cmake") {
		t.Fatalf("CMake build output=%q err=%v", out, err)
	}
}

func androidProjectFixture(t *testing.T, tools, adbBody string) (string, Config) {
	t.Helper()
	project := t.TempDir()
	gradle := writeWindowsCmdFixture(t, project, "gradlew", `
if "%1"=="--version" (echo Gradle 9.0& exit /b 0)
echo fixture-gradle %*
exit /b 0`)
	if err := os.Rename(gradle, filepath.Join(project, "gradlew.bat")); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(project, "app", "src", "main", "AndroidManifest.xml")
	if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`<manifest package="fixture"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	apk := filepath.Join(project, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	if err := os.MkdirAll(filepath.Dir(apk), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(apk, []byte("fixture apk"), 0o600); err != nil {
		t.Fatal(err)
	}
	javaTool := writeWindowsCmdFixture(t, tools, "java-"+filepath.Base(project), `
if "%1"=="-version" (echo openjdk version 17 1>&2& exit /b 0)
echo fixture-java %*
exit /b 0`)
	adbTool := writeWindowsCmdFixture(t, tools, "adb-"+filepath.Base(project), adbBody)
	cfg := defaultConfig()
	cfg.ToolOverrides = map[string]string{"java": javaTool, "adb": adbTool}
	cfg.AutoResearchToolHelp = false
	cfg.NetworkEnabled = false
	return project, cfg
}

func TestDeployAndroidControlledDeviceBranches(t *testing.T) {
	tools := t.TempDir()
	readyProject, readyCfg := androidProjectFixture(t, tools, `
if "%1"=="version" (echo Android Debug Bridge version 1.0.41& exit /b 0)
if "%1"=="devices" (
  echo List of devices attached
  echo SERIAL123 device product:fixture model:Fixture
  exit /b 0
)
echo Success
exit /b 0`)
	out, err := deployAndroid(context.Background(), readyProject, readyCfg)
	if err != nil || !strings.Contains(out, "SERIAL123") || !strings.Contains(out, "Success") || !strings.Contains(out, "app-debug.apk") {
		t.Fatalf("ready Android deploy output=%q err=%v", out, err)
	}

	multiProject, multiCfg := androidProjectFixture(t, tools, `
if "%1"=="version" (echo Android Debug Bridge version 1.0.41& exit /b 0)
if "%1"=="devices" (
  echo List of devices attached
  echo SERIAL1 device product:one
  echo SERIAL2 device product:two
  exit /b 0
)
echo Success
exit /b 0`)
	out, err = deployAndroid(context.Background(), multiProject, multiCfg)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "multiple") || !strings.Contains(out, "SERIAL1") || !strings.Contains(out, "SERIAL2") {
		t.Fatalf("multiple-device deploy output=%q err=%v", out, err)
	}

	offlineProject, offlineCfg := androidProjectFixture(t, tools, `
if "%1"=="version" (echo Android Debug Bridge version 1.0.41& exit /b 0)
if "%1"=="devices" (
  echo List of devices attached
  echo SERIALX unauthorized product:fixture
  exit /b 0
)
echo Success
exit /b 0`)
	out, err = deployAndroid(context.Background(), offlineProject, offlineCfg)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "no authorized") || !strings.Contains(out, "unauthorized") {
		t.Fatalf("unauthorized-device deploy output=%q err=%v", out, err)
	}

	notAndroid := t.TempDir()
	out, err = deployAndroid(context.Background(), notAndroid, readyCfg)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not recognized") || !strings.Contains(out, "unknown") {
		t.Fatalf("non-Android deploy output=%q err=%v", out, err)
	}
}

func TestTrayLoadAppIcon(t *testing.T) {
	h := loadAppIcon()
	if h != 0 {
		procDestroyIcon.Call(uintptr(h))
	}
}

func TestLocalCodeAppWindows(t *testing.T) {
	_ = isLocalCodeAppWindow("LocalCode", "Chrome_WidgetWin_1", "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe", "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe")
	_ = localCodeAppWindows("non-existent-browser.exe")
}

func TestRunPowerShellScript(t *testing.T) {
	cfg := Config{CommandTimeout: 10}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := runPowerShellScript(ctx, cfg, "Write-Output 'localcode-test'")
	if err != nil || !strings.Contains(out, "localcode-test") {
		t.Errorf("expected powershell output 'localcode-test', got %q, err=%v", out, err)
	}

	escaped := escapePowerShellString(`"quoted"`)
	if !strings.Contains(escaped, `\"`) {
		t.Errorf("expected escaped powershell string")
	}
}

func TestWindowsSuppressFatalAndAtomicErrors(t *testing.T) {
	t.Setenv("LOCALCODE_SUPPRESS_FATAL_DIALOGS", "1")
	if !suppressFatalDialogs() {
		t.Errorf("expected suppressFatalDialogs to be true")
	}
	t.Setenv("LOCALCODE_SUPPRESS_FATAL_DIALOGS", "0")
	if suppressFatalDialogs() {
		t.Errorf("expected suppressFatalDialogs to be false for 0")
	}

	errNil := windowsAtomicReplaceError("TestAPI", nil)
	if errNil == nil || !strings.Contains(errNil.Error(), "TestAPI failed") {
		t.Errorf("unexpected windowsAtomicReplaceError for nil: %v", errNil)
	}

	errWithErr := windowsAtomicReplaceError("TestAPI", os.ErrNotExist)
	if errWithErr == nil || !strings.Contains(errWithErr.Error(), "file does not exist") {
		t.Errorf("unexpected windowsAtomicReplaceError with err: %v", errWithErr)
	}

	_ = isTrayRequested()
}

func TestRenderWithChromiumDefault(t *testing.T) {
	tempDir := t.TempDir()
	sourceSVG := filepath.Join(tempDir, "test.svg")
	targetPNG := filepath.Join(tempDir, "test.png")
	targetWebP := filepath.Join(tempDir, "test.webp")
	if err := os.WriteFile(sourceSVG, []byte("<svg xmlns='http://www.w3.org/2000/svg' width='10' height='10'></svg>"), 0o644); err != nil {
		t.Fatalf("failed to write svg: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := Config{}
	_, _ = renderPNGWithChromiumDefault(ctx, cfg, sourceSVG, targetPNG, 10, 10)
	_, _ = renderWebPWithChromiumDefault(ctx, cfg, sourceSVG, targetWebP, 10, 10)
}
