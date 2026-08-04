// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	body := `if [ "$1" = "version" ]; then echo "Android Debug Bridge version 1.0.41"; exit 0; fi
if [ "$1" = "devices" ]; then printf "List of devices attached\\nSERIAL123\\tdevice product:test model:Demo transport_id:1\\n"; exit 0; fi
exit 0`
	if runtime.GOOS == "windows" {
		body = `if "%1"=="version" (echo Android Debug Bridge version 1.0.41& exit /b 0)
if "%1"=="devices" (echo List of devices attached& echo SERIAL123 device product:test model:Demo transport_id:1& exit /b 0)
exit /b 0`
	}
	writeTestExecutable(t, adb, body)
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
	if cfg.SchemaVersion != 3 || !cfg.AutoDiscoverTools || !cfg.AutoResearchToolHelp {
		t.Fatalf("tool settings were not migrated: %#v", cfg)
	}
	if cfg.ToolOverrides == nil {
		t.Fatal("tool override map must be initialized")
	}
}
