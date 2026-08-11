// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeTestExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if filepath.Ext(path) == "" {
			path += ".cmd"
		}
		if err := os.WriteFile(path, []byte("@echo off\r\n"+body+"\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func isolateWindowsToolDiscovery(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	for key, value := range map[string]string{
		"USERPROFILE":       filepath.Join(base, "profile"),
		"LOCALAPPDATA":      filepath.Join(base, "local"),
		"APPDATA":           filepath.Join(base, "roaming"),
		"ProgramFiles":      filepath.Join(base, "program-files"),
		"ProgramFiles(x86)": filepath.Join(base, "program-files-x86"),
		"ANDROID_HOME":      "",
		"ANDROID_SDK_ROOT":  "",
		"PATH":              "",
	} {
		t.Setenv(key, value)
	}
}

func buildWindowsADBTestExecutable(t *testing.T, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	sourceDir := t.TempDir()
	source := `package main
import (
  "fmt"
  "os"
)
func main() {
  if len(os.Args) > 1 && os.Args[1] == "version" {
    fmt.Println("Android Debug Bridge version 1.0.41")
    return
  }
  if len(os.Args) > 1 && os.Args[1] == "devices" {
    fmt.Println("List of devices attached")
    fmt.Println("SERIAL123\tdevice product:test model:Demo transport_id:1")
  }
}`
	file := filepath.Join(sourceDir, "main.go")
	if err := os.WriteFile(file, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	goexe := filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	cmd := exec.Command(goexe, "build", "-trimpath", "-o", target, file)
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build adb test helper: %v\n%s", err, output)
	}
}

func TestParseAndroidSDKFromLocalProperties(t *testing.T) {
	project := t.TempDir()
	want := filepath.Join(project, "Android SDK")
	encoded := strings.ReplaceAll(want, `\`, `\\`)
	if err := os.WriteFile(filepath.Join(project, "local.properties"), []byte("sdk.dir="+encoded+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := parseAndroidSDKFromLocalProperties(project); got != filepath.Clean(want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDiscoverADBFromProjectSDKAndDiagnoseDevice(t *testing.T) {
	project := t.TempDir()
	sdk := filepath.Join(project, "sdk")
	adb := filepath.Join(sdk, "platform-tools", executableName("adb"))
	if runtime.GOOS == "windows" {
		buildWindowsADBTestExecutable(t, adb)
	} else {
		body := `if [ "$1" = "version" ]; then echo "Android Debug Bridge version 1.0.41"; exit 0; fi
if [ "$1" = "devices" ]; then printf "List of devices attached\nSERIAL123\tdevice product:test model:Demo transport_id:1\n"; exit 0; fi
exit 0`
		writeTestExecutable(t, adb, body)
	}
	if err := os.WriteFile(filepath.Join(project, "local.properties"), []byte("sdk.dir="+strings.ReplaceAll(sdk, `\`, `\\`)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	info := discoverTool(project, "adb", cfg, true)
	if !info.Available {
		t.Fatalf("adb not discovered: %#v", info)
	}
	if filepath.Clean(info.Path) != filepath.Clean(adb) {
		t.Fatalf("path = %q want %q", info.Path, adb)
	}
	if !strings.Contains(info.Version, "Android Debug Bridge") {
		t.Fatalf("version missing: %q", info.Version)
	}
	if len(info.Diagnostics) == 0 || !strings.Contains(info.Diagnostics[0], "1 Gerät") {
		t.Fatalf("diagnostic missing device: %#v", info.Diagnostics)
	}
}

func TestRunResolvedToolPreservesStdoutAndStderrOnFailure(t *testing.T) {
	project := t.TempDir()
	tool := filepath.Join(project, "broken-tool")
	body := `echo visible-output
echo visible-error >&2
exit 7`
	if runtime.GOOS == "windows" {
		tool += ".cmd"
		body = `echo visible-output
echo visible-error 1>&2
exit /b 7`
	}
	writeTestExecutable(t, tool, body)
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{"broken-tool": tool}, EnvironmentVars: map[string]string{}})
	text, err := runResolvedTool(context.Background(), project, "broken-tool", nil, cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"visible-output", "visible-error", "Exitcode: 7"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestRewriteKnownToolCommandUsesAbsoluteADBPath(t *testing.T) {
	project := t.TempDir()
	adb := filepath.Join(project, executableName("adb"))
	writeTestExecutable(t, adb, "exit 0")
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{"adb": adb}, EnvironmentVars: map[string]string{}})
	rewritten, note, err := rewriteKnownToolCommand(project, "adb devices -l", cfg, "powershell")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, adb) || !strings.Contains(note, "automatisch aufgelöst") {
		t.Fatalf("rewrite failed: %q / %q", rewritten, note)
	}
}

func TestSameQuestionAndActionSignature(t *testing.T) {
	if !sameQuestion("Möchten Sie ein Git-Repository initialisieren?", "Möchten Sie ein Git Repository initialisieren") {
		t.Fatal("equivalent questions not detected")
	}
	a := AgentAction{Action: "run_tool", Tool: "adb", Args: []string{"devices", "-l"}, Message: "Prüfe Gerät"}
	b := a
	b.Message = "ADB noch einmal prüfen"
	if actionSignature(a) != actionSignature(b) {
		t.Fatal("tool action signature must not depend on cosmetic message")
	}
}

func TestContinuationBlocksRepeatedQuestion(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1, "modified_at": time.Now()}}})
		case "/api/chat":
			calls++
			content := `{"action":"ask_user","message":"Möchten Sie ein Git-Repository initialisieren?"}`
			if calls >= 3 {
				content = `{"action":"finish","message":"Antwort verarbeitet; die Rückfrage wurde nicht wiederholt."}`
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	project := t.TempDir()
	client := NewOllamaClient()
	client.BaseURL = server.URL
	cfg := normalizeConfig(Config{SchemaVersion: 3, RootProjectDir: project, LastProject: project, LastModel: "test-model", AutoDiscoverTools: false, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	state := NewAppState(cfg, client)
	t.Cleanup(state.Close)
	if err := state.StartAgent("Prüfe Git", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitAgentStopped(t, state)
	if err := state.StartAgent("ja", "test-model", nil); err != nil {
		t.Fatal(err)
	}
	waitAgentStopped(t, state)
	state.mu.RLock()
	events := append([]UIEvent(nil), state.Events...)
	state.mu.RUnlock()
	var warning, final bool
	for _, ev := range events {
		warning = warning || (ev.Type == "warning" && strings.Contains(ev.Message, "Rückfrage"))
		final = final || (ev.Type == "final" && strings.Contains(ev.Message, "nicht wiederholt"))
	}
	if !warning || !final {
		t.Fatalf("expected warning and final, calls=%d events=%#v", calls, events)
	}
}

func waitAgentStopped(t *testing.T, state *AppState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		state.mu.RUnlock()
		if !running {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("agent did not stop")
}

func TestDiscoverGenericProjectLocalTool(t *testing.T) {
	project := t.TempDir()
	tool := filepath.Join(project, "tools", "customlint")
	if runtime.GOOS == "windows" {
		tool += ".cmd"
	}
	writeTestExecutable(t, tool, "echo customlint-ready")
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	info := discoverTool(project, "customlint", cfg, false)
	if !info.Available {
		t.Fatalf("project-local generic tool not discovered: %#v", info)
	}
	if filepath.Clean(info.Path) != filepath.Clean(tool) {
		t.Fatalf("path=%q want=%q", info.Path, tool)
	}
}

func TestDiscoverNodeFromWingetPackage(t *testing.T) {
	if runtime.GOOS == "windows" {
		isolateWindowsToolDiscovery(t)
	}
	oldGOOS := toolRegistryGOOS
	t.Cleanup(func() { toolRegistryGOOS = oldGOOS })
	toolRegistryGOOS = "windows"

	project := t.TempDir()
	local := os.Getenv("LOCALAPPDATA")
	nodeRoot := filepath.Join(local, "Microsoft", "WinGet", "Packages", "OpenJS.NodeJS.LTS_Microsoft.Winget.Source_8wekyb3d8bbwe", "node-v24.15.0-win-x64")
	for _, name := range []string{"node.exe", "npm.cmd", "npx.cmd"} {
		if err := os.MkdirAll(nodeRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nodeRoot, name), []byte("@echo off\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	for _, name := range []string{"node", "npm", "npx"} {
		info := discoverTool(project, name, cfg, false)
		if !info.Available {
			t.Fatalf("%s was not discovered from WinGet package; searched: %v", name, info.SearchedPath)
		}
		if !strings.Contains(info.Path, "Microsoft"+string(filepath.Separator)+"WinGet"+string(filepath.Separator)+"Packages") || info.Source != "WinGet" {
			t.Fatalf("%s path/source = %q/%q", name, info.Path, info.Source)
		}
	}
}

func TestADBNoDevicePerformsSingleRecoveryAndRetry(t *testing.T) {
	project := t.TempDir()
	tool := filepath.Join(project, "adb")
	state := filepath.Join(project, "adb-state")
	body := `if [ "$1" = "version" ]; then echo "Android Debug Bridge version 1.0.41"; exit 0; fi
if [ "$1" = "start-server" ]; then echo started >> "` + state + `"; exit 0; fi
if [ "$1" = "reconnect" ]; then echo reconnected >> "` + state + `"; exit 0; fi
if [ "$1" = "devices" ]; then
  if [ -f "` + state + `" ]; then printf "List of devices attached\\nSERIAL123\\tdevice product:test model:Demo transport_id:1\\n"; else printf "List of devices attached\\n\\n"; fi
  exit 0
fi
exit 0`
	if runtime.GOOS == "windows" {
		tool += ".cmd"
		stateWin := strings.ReplaceAll(state, `/`, `\`)
		body = `if "%1"=="version" (echo Android Debug Bridge version 1.0.41& exit /b 0)
if "%1"=="start-server" (echo started>>"` + stateWin + `"& exit /b 0)
if "%1"=="reconnect" (echo reconnected>>"` + stateWin + `"& exit /b 0)
if "%1"=="devices" (if exist "` + stateWin + `" (echo List of devices attached& echo SERIAL123 device product:test model:Demo transport_id:1) else (echo List of devices attached)& exit /b 0)
exit /b 0`
	}
	writeTestExecutable(t, tool, body)
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{"adb": tool}, EnvironmentVars: map[string]string{}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	text, err := runResolvedTool(ctx, project, "adb", []string{"devices", "-l"}, cfg)
	if err != nil {
		t.Fatalf("runResolvedTool failed: %v\n%s", err, text)
	}
	if !strings.Contains(text, "1 Gerät") || !strings.Contains(text, "Wiederholungsversuch") {
		t.Fatalf("missing recovered device diagnostic: %s", text)
	}
}

func TestToolDiscoverySettingsMigrateFromOlderSchema(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 2})
	if cfg.SchemaVersion != 10 || !cfg.AutoDiscoverTools || !cfg.AutoResearchToolHelp || !cfg.ContextCompactionEnabled {
		t.Fatalf("tool and compaction settings were not migrated: %#v", cfg)
	}
	if cfg.ToolOverrides == nil {
		t.Fatal("tool override map must be initialized")
	}
}

func TestMissingToolReturnsTypedError(t *testing.T) {
	if runtime.GOOS == "windows" {
		isolateWindowsToolDiscovery(t)
	}
	project := t.TempDir()
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}, NetworkEnabled: false})
	text, err := runResolvedTool(context.Background(), project, "adb", []string{"devices", "-l"}, cfg)
	if err == nil {
		t.Fatal("expected missing-tool error")
	}
	var missing *ToolNotFoundError
	if !errors.As(err, &missing) {
		t.Fatalf("expected ToolNotFoundError, got %T: %v", err, err)
	}
	if missing.Info.Name != "adb" || (!strings.Contains(text, "Durchsuchte Pfade") && !strings.Contains(text, "Searched paths")) {
		t.Fatalf("unexpected missing tool detail: %#v\n%s", missing, text)
	}
}

func TestContinuationClassificationAvoidsStaleQuestionHijack(t *testing.T) {
	question := "Es scheint, dass ADB fehlt. Möchten Sie den Befehl erneut ausführen?"
	if !likelyContinuationAnswer(question, "nochmal versuchen") {
		t.Fatal("short retry answer should continue")
	}
	if !likelyContinuationAnswer(question, "das Handy ist verbunden, such adb") {
		t.Fatal("ADB-specific answer should continue")
	}
	if likelyContinuationAnswer(question, "schau im internet die neuesten nachrichten") {
		t.Fatal("new web research task must not continue stale ADB question")
	}
}

func TestNormalizeWebSearchUsesTaskFallback(t *testing.T) {
	a := normalizeAgentAction(AgentAction{Action: "web_search", Message: "Web search"}, "neueste Nachrichten zu KI")
	if a.Query != "neueste Nachrichten zu KI" {
		t.Fatalf("query=%q", a.Query)
	}
	b := normalizeAgentAction(AgentAction{Action: "web_search", Message: "Offizielle ADB Dokumentation"}, "ignored")
	if b.Query != "Offizielle ADB Dokumentation" {
		t.Fatalf("query=%q", b.Query)
	}
}

func TestGitReadUsesConfiguredToolOverride(t *testing.T) {
	project := t.TempDir()
	gitTool := filepath.Join(project, "tools", "git")
	if runtime.GOOS == "windows" {
		gitTool += ".cmd"
	}
	body := `echo OVERRIDE-GIT "$@"`
	if runtime.GOOS == "windows" {
		body = `echo OVERRIDE-GIT %*`
	}
	writeTestExecutable(t, gitTool, body)
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{"git": gitTool}, EnvironmentVars: map[string]string{}})
	out, err := gitRead(project, cfg, "status", "--short")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "OVERRIDE-GIT") {
		t.Fatalf("configured git tool was not used: %s", out)
	}
}

func TestInstallMetadataForGitAndADB(t *testing.T) {
	for _, name := range []string{"git", "adb", "fastboot"} {
		profile := profileForTool(name)
		if strings.TrimSpace(profile.InstallKind) == "" {
			t.Fatalf("%s should define controlled Windows installation metadata", name)
		}
		if strings.TrimSpace(toolInstallPreview(name, defaultConfig())) == "" {
			t.Fatalf("%s install preview is empty", name)
		}
	}
}

func TestExtractZipSafe(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "tool.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("platform-tools/adb.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("test"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := extractZipSafe(archive, dest); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dest, "platform-tools", "adb.exe")); err != nil || string(data) != "test" {
		t.Fatalf("extracted file invalid: %q %v", data, err)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("../escape.txt")
	_, _ = w.Write([]byte("bad"))
	_ = zw.Close()
	_ = f.Close()
	if err := extractZipSafe(archive, t.TempDir()); err == nil {
		t.Fatal("expected traversal rejection")
	}
}

func TestMissingToolForActionDetectsKnownCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		isolateWindowsToolDiscovery(t)
	}
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	missing := missingToolForAction(t.TempDir(), cfg, AgentAction{Action: "run_command", Command: "definitely-not-a-real-tool-xyz --version"})
	if missing != nil {
		t.Fatalf("unknown command must remain a normal shell command: %#v", missing)
	}

	profile := profileForTool("adb")
	if runtime.GOOS != "windows" {
		// On non-Windows test hosts install support is intentionally unavailable,
		// but discovery still needs to recognize adb as a known tool.
		if profile.Name != "adb" {
			t.Fatalf("adb profile missing")
		}
		return
	}
	missing = missingToolForAction(t.TempDir(), cfg, AgentAction{Action: "run_command", Command: "adb devices"})
	if missing == nil || missing.Info.Name != "adb" {
		t.Fatalf("expected missing adb preflight, got %#v", missing)
	}
}

func TestBlockedAvoidanceQuestion(t *testing.T) {
	blocked, _ := blockedAvoidanceQuestion("analysiere das projekt", "Es wurde kein Git-Repository initialisiert. Möchten Sie ein neues Git-Repository erstellen?")
	if !blocked {
		t.Fatal("irrelevant git init question should be blocked")
	}
	blocked, _ = blockedAvoidanceQuestion("initialisiere git und committe alles", "Möchten Sie ein neues Git-Repository initialisieren?")
	if blocked {
		t.Fatal("explicit git task must allow git question")
	}
	blocked, _ = blockedAvoidanceQuestion("verteile die app", "Bitte stellen Sie sicher, dass ADB korrekt installiert und in Ihrem Systempfad verfügbar ist.")
	if !blocked {
		t.Fatal("tool-avoidance question should be blocked")
	}
}

func TestDetectProjectPlanAndroid(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gradlew"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "app", "src", "main")
	if err := os.MkdirAll(manifest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifest, "AndroidManifest.xml"), []byte("<manifest/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := detectProjectPlan(root)
	if plan.Kind != "android-gradle" || plan.BuildTool != "gradle" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestParseADBDevices(t *testing.T) {
	devices := parseADBDevices("List of devices attached\nABC123\tdevice product:x model:y\nXYZ\tunauthorized usb:1\n")
	if len(devices) != 2 || devices[0].Serial != "ABC123" || devices[0].State != "device" || devices[1].State != "unauthorized" {
		t.Fatalf("unexpected devices: %#v", devices)
	}
}

func TestTaskAutomationHint(t *testing.T) {
	if !strings.Contains(taskAutomationHint("kompiliere das projekt"), "build_project") {
		t.Fatal("build task should route to build_project")
	}
	if !strings.Contains(taskAutomationHint("verteile das an das verbundene handy"), "deploy_android") {
		t.Fatal("deployment task should route to deploy_android")
	}
	if !strings.Contains(taskAutomationHint("schau im internet die neuesten nachrichten"), "web_search") {
		t.Fatal("news task should route to web_search")
	}
}

func TestDeployAndroidDeterministicFlow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("portable shell fixtures are exercised on non-Windows CI")
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "gradlew"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(root, "app", "src", "main")
	if err := os.MkdirAll(manifest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(manifest, "AndroidManifest.xml"), []byte("<manifest/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	javaTool := filepath.Join(root, "tools", "java")
	gradleTool := filepath.Join(root, "tools", "gradle")
	adbTool := filepath.Join(root, "tools", "adb")
	writeTestExecutable(t, javaTool, `echo 'openjdk version 17'`)
	writeTestExecutable(t, gradleTool, `mkdir -p app/build/outputs/apk/debug; echo apk > app/build/outputs/apk/debug/app-debug.apk; echo BUILD-SUCCESS`)
	writeTestExecutable(t, adbTool, `if [ "$1" = "devices" ]; then echo 'List of devices attached'; printf 'SERIAL123\tdevice product:test model:test\n'; else echo Success; fi`)
	cfg := normalizeConfig(Config{SchemaVersion: 3, AutoDiscoverTools: true, ToolOverrides: map[string]string{"java": javaTool, "gradle": gradleTool, "adb": adbTool}, EnvironmentVars: map[string]string{}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := deployAndroid(ctx, root, cfg)
	if err != nil {
		t.Fatalf("deploy failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "BUILD-SUCCESS") || !strings.Contains(out, "Success") || !strings.Contains(out, "SERIAL123") {
		t.Fatalf("unexpected deployment output:\n%s", out)
	}
}

func TestAnalyzeIntentDoesNotRequireGitOrWrites(t *testing.T) {
	intent := classifyTaskIntent("analysiere das projekt")
	if intent.Kind != "analyze" || intent.GitRequested {
		t.Fatalf("unexpected intent: %#v", intent)
	}
	if allowed, _ := actionAllowedForIntent(intent, AgentAction{Action: "replace_text", Path: "README.md"}); allowed {
		t.Fatal("analysis intent must not allow file mutation")
	}
	if allowed, _ := actionAllowedForIntent(intent, AgentAction{Action: "read_file", Path: "README.md"}); !allowed {
		t.Fatal("analysis intent must allow reads")
	}
}

func TestGitContextDoesNotExposeFatalStatusForAnalysis(t *testing.T) {
	project := t.TempDir()
	cfg := normalizeConfig(Config{SchemaVersion: 4, GitEnabled: true, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	context := gitContextForTask(project, cfg, "analysiere das projekt")
	if strings.Contains(strings.ToLower(context), "fatal") || !strings.Contains(context, "nicht erforderlich") {
		t.Fatalf("unexpected git context: %s", context)
	}
}

func TestSuggestedGitActionIsExecutedFromYesIntent(t *testing.T) {
	action := suggestedActionForQuestion("Möchten Sie ein neues Git-Repository initialisieren?")
	if action == nil || action.Action != "git" || len(action.Args) == 0 || action.Args[0] != "init" {
		t.Fatalf("unexpected suggested action: %#v", action)
	}
	if !isAffirmativeAnswer("ja, mach das") {
		t.Fatal("affirmative answer was not recognized")
	}
}

func TestContextCompactionThreshold(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 4, ContextLength: 4096, ContextCompactionEnabled: true, ContextCompactionThresholdPercent: 50, ContextCompactionKeepRecent: 8})
	messages := []OllamaMessage{{Role: "system", Content: strings.Repeat("s", 2000)}, {Role: "user", Content: strings.Repeat("u", 7000)}}
	if !shouldCompactMessages(messages, cfg) {
		t.Fatalf("expected compaction, estimated=%d", estimateMessageTokens(messages))
	}
	state := deterministicContextSummary(messages, "analysiere das projekt")
	if state.OriginalTask != "analysiere das projekt" || strings.TrimSpace(renderCompactedState(state)) == "" {
		t.Fatalf("invalid compacted state: %#v", state)
	}
}

func TestContextCompactionFitsRecentMessagesToBudget(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 4, ContextLength: 4096, ContextCompactionEnabled: true, ContextCompactionThresholdPercent: 50, ContextCompactionKeepRecent: 12})
	messages := []OllamaMessage{{Role: "system", Content: "system"}}
	for i := 0; i < 16; i++ {
		messages = append(messages, OllamaMessage{Role: "user", Content: strings.Repeat(fmt.Sprintf("recent-%02d ", i), 3000), Thinking: strings.Repeat("hidden", 1000)})
	}
	state := deterministicContextSummary(messages, "build")
	result := buildCompactedMessages(messages, state, 12)
	if estimateMessageTokens(result) <= postCompactionTokenTarget(cfg) {
		t.Fatal("test fixture did not exceed post-compaction target")
	}
	fitted := truncateCompactedRecentMessages(result, postCompactionTokenTarget(cfg))
	if got, wantMax := estimateMessageTokens(fitted), postCompactionTokenTarget(cfg); got > wantMax {
		t.Fatalf("fitted context still too large: got %d want <= %d", got, wantMax)
	}
	for _, message := range fitted {
		if message.Thinking != "" {
			t.Fatal("thinking content was not stripped from compacted recent messages")
		}
	}
}

func TestContextToolResultLimitIsBounded(t *testing.T) {
	small := contextToolResultLimit(Config{ContextLength: 4096})
	large := contextToolResultLimit(Config{ContextLength: 131072})
	if small < 8000 || small > 60000 {
		t.Fatalf("small context limit out of bounds: %d", small)
	}
	if large != 60000 {
		t.Fatalf("large context limit = %d, want cap 60000", large)
	}
}

func TestInitializeGitRepositoryAndGitignore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed in test environment")
	}
	project := t.TempDir()
	cfg := normalizeConfig(Config{SchemaVersion: 4, GitEnabled: true, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := initializeGitRepository(ctx, project, cfg)
	if err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	if !isGitRepository(project, cfg) {
		t.Fatal("repository not created")
	}
	if _, err := os.Stat(filepath.Join(project, ".gitignore")); err != nil {
		t.Fatalf(".gitignore missing: %v", err)
	}
}

func TestContextCompactionSettingsArePresentInUI(t *testing.T) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, id := range []string{"setContextCompaction", "setCompactionThreshold", "setCompactionKeepRecent"} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Fatalf("missing context compaction UI field %s", id)
		}
	}
	for _, jsonField := range []string{"context_compaction_enabled", "context_compaction_threshold_percent", "context_compaction_keep_recent"} {
		if !strings.Contains(html, jsonField) {
			t.Fatalf("missing context compaction setting binding %s", jsonField)
		}
	}
}
