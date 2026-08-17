// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func isolateWave3Config(t *testing.T) Config {
	t.Helper()
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("LOCALCODE_CACHE_HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	cfg.LastProject = cfg.RootProjectDir
	cfg.EditingEngine = editingEngineNative
	cfg.MCPServers = nil
	cfg.AutoResearchToolHelp = false
	cfg.NetworkEnabled = false
	cfg.ToolOverrides = map[string]string{}
	return cfg
}

func waitForPendingWave3(t *testing.T, state *AppState) *PendingAction {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		pending := state.Pending
		state.mu.RUnlock()
		if pending != nil {
			return pending
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for pending approval")
	return nil
}

func TestRuntimeBootstrapAgainstLocalOllamaFixture(t *testing.T) {
	cfg := isolateWave3Config(t)
	cfg.OllamaAutoInstall = false
	cfg.OllamaAutoPull = true
	cfg.SetupDownloadsEnabled = true

	var pulled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			if pulled.Load() {
				_, _ = io.WriteString(w, `{"models":[{"name":"qwen2.5-coder:14b","size":123,"modified_at":"2026-08-16T20:00:00Z"}]}`)
			} else {
				_, _ = io.WriteString(w, `{"models":[]}`)
			}
		case "/api/pull":
			pulled.Store(true)
			_, _ = io.WriteString(w, "{\"status\":\"pulling manifest\",\"completed\":50,\"total\":100}\n")
			_, _ = io.WriteString(w, "{\"status\":\"success\",\"completed\":100,\"total\":100}\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("OLLAMA_HOST", server.URL)
	cfg.OllamaURL = server.URL
	client := &OllamaClient{BaseURL: server.URL, HTTP: server.Client(), ContextLength: cfg.ContextLength}

	var progress []BootstrapProgress
	reporter := func(p BootstrapProgress) { progress = append(progress, p) }
	detail, err := ensureOllamaInstalledAndRunningWithProgress(context.Background(), cfg, client, reporter)
	if err != nil || !strings.Contains(detail, server.URL) || len(progress) == 0 {
		t.Fatalf("ensure Ollama detail=%q progress=%d err=%v", detail, len(progress), err)
	}

	existing := []ModelInfo{{Name: defaultCodingModel}}
	models, details, err := ensureConfiguredModelsWithProgress(context.Background(), cfg, client, existing, reporter)
	if err != nil || len(models) != 1 || len(details) != 0 {
		t.Fatalf("existing models=%#v details=%#v err=%v", models, details, err)
	}

	noPull := cfg
	noPull.OllamaAutoPull = false
	if _, _, err := ensureConfiguredModelsWithProgress(context.Background(), noPull, client, nil, reporter); err == nil || !strings.Contains(strings.ToLower(err.Error()), "disabled") {
		t.Fatalf("expected auto-pull disabled error, got %v", err)
	}

	noDownloads := cfg
	noDownloads.SetupDownloadsEnabled = false
	if _, _, err := ensureConfiguredModelsWithProgress(context.Background(), noDownloads, client, nil, reporter); err == nil || !strings.Contains(strings.ToLower(err.Error()), "downloads") {
		t.Fatalf("expected downloads-disabled error, got %v", err)
	}

	pulled.Store(false)
	models, details, err = ensureConfiguredModelsWithProgress(context.Background(), cfg, client, nil, reporter)
	if err != nil || len(models) != 1 || models[0].Name != defaultCodingModel || len(details) != 1 || !pulled.Load() {
		t.Fatalf("pulled models=%#v details=%#v pulled=%v err=%v", models, details, pulled.Load(), err)
	}
}

func TestEditingEngineBootstrapSafeBranches(t *testing.T) {
	cfg := isolateWave3Config(t)
	ctx := context.Background()

	updated, detail, err := ensureSelectedEditingEngineRuntimeWithProgress(ctx, cfg, nil)
	if err != nil || updated.EditingEngine != editingEngineNative || !strings.Contains(detail, "native") {
		t.Fatalf("native updated=%q detail=%q err=%v", updated.EditingEngine, detail, err)
	}

	disabled := cfg
	disabled.EditingEngine = editingEngineClaude
	disabled.ClaudeCodeEnabled = false
	updated, detail, err = ensureSelectedEditingEngineRuntimeWithProgress(ctx, disabled, nil)
	if err != nil || updated.EditingEngine != editingEngineNative || !strings.Contains(strings.ToLower(detail), "disabled") {
		t.Fatalf("disabled updated=%q detail=%q err=%v", updated.EditingEngine, detail, err)
	}

	missing := cfg
	missing.EditingEngine = editingEngineClaude
	missing.ClaudeCodeEnabled = true
	missing.ClaudeCodeAutoInstall = false
	missing.ClaudeCodeExecutable = filepath.Join(t.TempDir(), "missing-claude.exe")
	if _, _, err := ensureSelectedEditingEngineRuntimeWithProgress(ctx, missing, nil); err == nil {
		t.Fatal("missing Claude Code without auto-install must fail")
	}
	missing.ClaudeCodeAutoInstall = true
	missing.SetupDownloadsEnabled = false
	if _, _, err := ensureSelectedEditingEngineRuntimeWithProgress(ctx, missing, nil); err == nil || !strings.Contains(strings.ToLower(err.Error()), "downloads") {
		t.Fatalf("expected missing-engine download policy error, got %v", err)
	}

	tools := t.TempDir()
	claude := writeWindowsCmdFixture(t, tools, "claude-wave3", `
if "%1"=="--version" (echo claude-code 9.9.9& exit /b 0)
if "%1"=="auth" (echo logged-in& exit /b 0)
exit /b 0`)
	installedClaude := cfg
	installedClaude.EditingEngine = editingEngineClaude
	installedClaude.ClaudeCodeEnabled = true
	installedClaude.ClaudeCodeAutoInstall = false
	installedClaude.ClaudeCodeExecutable = claude
	updated, detail, err = ensureSelectedEditingEngineRuntimeWithProgress(ctx, installedClaude, nil)
	if err != nil || updated.ClaudeCodeExecutable == "" || !strings.Contains(detail, "9.9.9") {
		t.Fatalf("installed Claude executable=%q detail=%q err=%v", updated.ClaudeCodeExecutable, detail, err)
	}

	opencode := writeWindowsCmdFixture(t, tools, "opencode-wave3", `
if "%1"=="--version" (echo 8.8.8& exit /b 0)
if "%1"=="auth" (echo ollama local& exit /b 0)
exit /b 0`)
	installedOpenCode := cfg
	installedOpenCode.EditingEngine = editingEngineOpenCode
	installedOpenCode.OpenCodeEnabled = true
	installedOpenCode.OpenCodeAutoInstall = false
	installedOpenCode.OpenCodeExecutable = opencode
	installedOpenCode.OpenCodeModel = "ollama/qwen2.5-coder:14b"
	updated, detail, err = ensureSelectedEditingEngineRuntimeWithProgress(ctx, installedOpenCode, nil)
	if err != nil || updated.OpenCodeExecutable == "" || !strings.Contains(detail, "8.8.8") {
		t.Fatalf("installed OpenCode executable=%q detail=%q err=%v", updated.OpenCodeExecutable, detail, err)
	}
}

func TestDiagnosticsAndServerStartupWithLocalFixtures(t *testing.T) {
	cfg := isolateWave3Config(t)
	cfg.LastModel = defaultCodingModel
	cfg.LastProject = ""
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	var emptyModels atomic.Bool
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if emptyModels.Load() {
			_, _ = io.WriteString(w, `{"models":[]}`)
			return
		}
		_, _ = io.WriteString(w, `{"models":[{"name":"qwen2.5-coder:14b","size":1,"modified_at":"2026-08-16T20:00:00Z"}]}`)
	}))
	defer ollama.Close()
	t.Setenv("OLLAMA_HOST", ollama.URL)
	if code := runDiagnostics(); code != 0 {
		t.Fatalf("runDiagnostics success code=%d", code)
	}
	emptyModels.Store(true)
	if code := runDiagnostics(); code != 1 {
		t.Fatalf("runDiagnostics empty-model code=%d", code)
	}

	state := NewAppState(cfg, &OllamaClient{BaseURL: ollama.URL, HTTP: ollama.Client(), ContextLength: cfg.ContextLength})
	defer state.Close()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	busyPort := occupied.Addr().(*net.TCPAddr).Port
	baseURL, err := startHTTPServer(state, busyPort)
	_ = occupied.Close()
	if err != nil || strings.TrimSpace(baseURL) == "" {
		t.Fatalf("startHTTPServer URL=%q err=%v", baseURL, err)
	}
	resp, err := http.Get(baseURL + "/api/ping")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("desktop ping status=%d", resp.StatusCode)
	}

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	remotePort := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()
	remoteCfg := cfg
	remoteCfg.RemoteEnabled = true
	remoteCfg.RemoteBindHost = "127.0.0.1"
	remoteCfg.RemotePort = remotePort
	urls, err := startRemoteHTTPServer(state, remoteCfg)
	if err != nil || len(urls) != 1 {
		t.Fatalf("remote URLs=%#v err=%v", urls, err)
	}
	resp, err = http.Get(urls[0] + "/api/ping")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("remote ping status=%d", resp.StatusCode)
	}

	if urls, err := startRemoteHTTPServer(state, Config{RemoteEnabled: false}); err != nil || urls != nil {
		t.Fatalf("disabled remote URLs=%#v err=%v", urls, err)
	}
}

func TestRemoteDeviceLifecycleHandlers(t *testing.T) {
	cfg := isolateWave3Config(t)
	now := time.Now()
	cfg.RemoteDevices = []RemoteDevice{
		{ID: "valid", Name: "Phone", TokenHash: "abc", PairedAt: now.Add(-time.Hour), LastSeenAt: now.Add(-time.Minute)},
		{ID: "expired", Name: "Old", TokenHash: "def", PairedAt: now.Add(-31 * 24 * time.Hour)},
	}
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	state := NewAppState(cfg, NewOllamaClient())
	defer state.Close()
	server := NewServer(state)

	views := state.RemoteDeviceViews()
	if len(views) != 1 || views[0].ID != "valid" || views[0].ExpiresAt.IsZero() {
		t.Fatalf("remote views=%#v", views)
	}
	if err := state.RevokeRemoteDevice(""); err == nil {
		t.Fatal("empty remote device id must fail")
	}

	recorder := httptest.NewRecorder()
	server.handleRemoteDevices(recorder, httptest.NewRequest(http.MethodGet, "/api/remote/devices", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "valid") || strings.Contains(recorder.Body.String(), "expired") {
		t.Fatalf("device listing status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	server.handleRemoteDevices(recorder, httptest.NewRequest(http.MethodPost, "/api/remote/devices", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("device method status=%d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	server.handleRemoteRevoke(recorder, httptest.NewRequest(http.MethodGet, "/api/remote/revoke", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("revoke method status=%d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	server.handleRemoteRevoke(recorder, httptest.NewRequest(http.MethodPost, "/api/remote/revoke", strings.NewReader("{")))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("bad revoke JSON status=%d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	server.handleRemoteRevoke(recorder, httptest.NewRequest(http.MethodPost, "/api/remote/revoke", strings.NewReader(`{"id":"missing"}`)))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing revoke status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	server.handleRemoteRevoke(recorder, httptest.NewRequest(http.MethodPost, "/api/remote/revoke", strings.NewReader(`{"id":"valid"}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("valid revoke status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if len(state.RemoteDeviceViews()) != 0 {
		t.Fatalf("remote devices remain after revoke: %#v", state.RemoteDeviceViews())
	}
}

func TestMCPSetupSafeBranches(t *testing.T) {
	cfg := isolateWave3Config(t)
	project := cfg.LastProject
	ctx := context.Background()

	managedUVDir := filepath.Join(appDataDir(), "tools", "uv")
	if err := os.MkdirAll(managedUVDir, 0o755); err != nil {
		t.Fatal(err)
	}
	managedUV := filepath.Join(managedUVDir, "uvx.exe")
	if err := os.WriteFile(managedUV, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	uvPath, detail, err := installUV(ctx)
	if err != nil || !strings.EqualFold(uvPath, managedUV) || !strings.Contains(detail, "already installed") {
		t.Fatalf("installUV path=%q detail=%q err=%v", uvPath, detail, err)
	}

	fetchServer := MCPServerConfig{Name: "fetch-wave3", DisplayName: "Fetch", Enabled: true, Transport: "stdio", Preset: "fetch", Command: "uvx", AutoInstall: true}
	cfg.MCPServers = []MCPServerConfig{fetchServer}
	updated, detail, err := installMCPDependency(ctx, project, cfg, fetchServer)
	if err != nil || !strings.EqualFold(updated.MCPServers[0].Command, managedUV) || !strings.Contains(detail, "already installed") {
		t.Fatalf("fetch dependency command=%q detail=%q err=%v", updated.MCPServers[0].Command, detail, err)
	}

	disabled := cfg
	disabled.SetupDownloadsEnabled = false
	if _, _, err := installMCPDependency(ctx, project, disabled, fetchServer); err == nil || !strings.Contains(strings.ToLower(err.Error()), "downloads") {
		t.Fatalf("downloads-disabled MCP error=%v", err)
	}
	if _, _, err := installMCPDependency(ctx, project, cfg, MCPServerConfig{Name: "unknown", Preset: "unknown"}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "no managed installer") {
		t.Fatalf("unknown MCP installer error=%v", err)
	}

	filesystemCfg := cfg
	filesystemCfg.MCPServers = []MCPServerConfig{{Name: "filesystem-wave3", DisplayName: "Filesystem", Enabled: true, Transport: "builtin", Preset: "filesystem"}}
	state := NewAppState(filesystemCfg, NewOllamaClient())
	defer state.Close()
	if _, _, followUp, err := state.prepareMCPServer(ctx, project, filesystemCfg, AgentAction{Action: "mcp_list_tools", Server: "filesystem-wave3"}); err != nil || followUp {
		t.Fatalf("filesystem prepare followUp=%v err=%v", followUp, err)
	}

	missingCfg := cfg
	missingCfg.MCPServers = []MCPServerConfig{{Name: "missing-wave3", DisplayName: "Missing", Enabled: true, Transport: "stdio", Command: filepath.Join(t.TempDir(), "missing.exe"), AutoInstall: false}}
	if _, _, _, err := state.prepareMCPServer(ctx, project, missingCfg, AgentAction{Action: "mcp_list_tools", Server: "missing-wave3"}); err == nil || !strings.Contains(strings.ToLower(err.Error()), "not installed") {
		t.Fatalf("missing MCP prepare error=%v", err)
	}
	status := mcpServerStatus(ctx, cfg, project, MCPServerConfig{Name: "bad", Enabled: true, Transport: "mystery"}, false)
	if status.Error == "" || status.Installed {
		t.Fatalf("unknown transport status=%#v", status)
	}
}

func TestMissingToolInstallOfferDeclineAndProjectCommandContext(t *testing.T) {
	cfg := isolateWave3Config(t)
	project := cfg.LastProject
	state := NewAppState(cfg, NewOllamaClient())
	defer state.Close()

	if _, _, installed, err := state.offerInstallMissingTool(context.Background(), project, cfg, nil); err != nil || installed {
		t.Fatalf("nil missing installed=%v err=%v", installed, err)
	}
	unsupported := &ToolNotFoundError{Info: ToolInfo{Name: "unsupported-wave3", DisplayName: "Unsupported"}}
	if _, _, installed, err := state.offerInstallMissingTool(context.Background(), project, cfg, unsupported); err == nil || installed {
		t.Fatalf("unsupported missing installed=%v err=%v", installed, err)
	}

	type offerResult struct {
		detail    string
		installed bool
		err       error
	}
	resultCh := make(chan offerResult, 1)
	go func() {
		_, detail, installed, err := state.offerInstallMissingTool(context.Background(), project, cfg, &ToolNotFoundError{
			Info:   ToolInfo{Name: "adb", DisplayName: "Android Debug Bridge"},
			Detail: "searched fixture paths",
		})
		resultCh <- offerResult{detail: detail, installed: installed, err: err}
	}()
	pending := waitForPendingWave3(t, state)
	pending.Result <- ApprovalDecision{Approved: false}
	result := <-resultCh
	if result.err != nil || result.installed || !strings.Contains(strings.ToLower(result.detail), "declined") {
		t.Fatalf("declined offer detail=%q installed=%v err=%v", result.detail, result.installed, result.err)
	}

	commandsDir := filepath.Join(project, ".localcode", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(commandsDir, "review.md")
	commandBody := "---\ndescription: Review the selected project\n---\nCheck {{project}} in {{cwd}} with {{args}}.\n"
	if err := os.WriteFile(commandPath, []byte(commandBody), 0o600); err != nil {
		t.Fatal(err)
	}
	contextText := projectCommandsContext(project)
	if !strings.Contains(contextText, "/review") || !strings.Contains(contextText, "Review the selected project") {
		t.Fatalf("project command context=%q", contextText)
	}
	expanded, cmd, ok, err := expandSlashCommandPrompt(project, "/review security")
	if err != nil || !ok || cmd == nil || !strings.Contains(expanded, "security") || !strings.Contains(expanded, filepath.Base(project)) {
		t.Fatalf("expanded=%q cmd=%#v ok=%v err=%v", expanded, cmd, ok, err)
	}
	if _, err := loadProjectCommandFromFile(projectCommandRoot{Path: commandsDir, Scope: "project"}, commandsDir, "bad"); err == nil {
		t.Fatal("directory command path must fail")
	}
	empty := filepath.Join(commandsDir, "empty.md")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProjectCommandFromFile(projectCommandRoot{Path: commandsDir, Scope: "project"}, empty, "empty"); err == nil {
		t.Fatal("empty command file must fail")
	}
}
