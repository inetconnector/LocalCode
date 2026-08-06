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

func TestCoverageFinalMCPGitPowerShellAndArguments(t *testing.T) {
	isolateCoverageEnv(t)
	cfg := defaultConfig()
	cfg.Language = "de"
	cfg.PreferredLanguage = "de"
	cfg.CommandTimeout = 2
	cfg.NetworkEnabled = false
	project := t.TempDir()

	// Remaining generic argument conversion branches.
	if boolArg(map[string]any{"x": "not-bool"}, "x", true) != true {
		t.Fatal("invalid bool fallback")
	}
	if intArg(map[string]any{"x": json.Number("bad")}, "x", 11) != 11 ||
		intArg(map[string]any{"x": "bad"}, "x", 12) != 12 ||
		intArg(map[string]any{"x": true}, "x", 13) != 13 {
		t.Fatal("invalid int fallback")
	}
	if got := stringSliceArg(map[string]any{"x": []string{"a", "b"}}, "x"); len(got) != 2 {
		t.Fatal(got)
	}
	if got := stringSliceArg(map[string]any{"x": 3}, "x"); got != nil {
		t.Fatal(got)
	}
	_ = stringArg(map[string]any{"x": true}, "x")
	_ = builtinTools("filesystem", cfg)
	_ = builtinTools("powershell", cfg)
	_ = builtinTools("git", cfg)

	// Make PowerShell resolution deterministic on every host. The test binary
	// deliberately exits with an argument error, which exercises process,
	// stderr and failure handling without requiring PowerShell to be installed.
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ToolOverrides = map[string]string{"powershell": self, "pwsh": self}
	if got := powershellExecutable(project, cfg); got == "" {
		t.Fatal("PowerShell override was not resolved")
	}
	psCases := []struct {
		name string
		args map[string]any
	}{
		{"powershell_run", map[string]any{}},
		{"powershell_run", map[string]any{"script": "Write-Output ok", "timeout_seconds": 1}},
		{"powershell_get_command", map[string]any{}},
		{"powershell_get_command", map[string]any{"name": "Get-Item"}},
		{"powershell_get_help", map[string]any{}},
		{"powershell_get_help", map[string]any{"name": "Get-Item"}},
		{"powershell_get_help", map[string]any{"name": "Get-Item", "online": true}},
		{"unknown", map[string]any{}},
	}
	for _, tc := range psCases {
		_, _ = executePowerShellMCPTool(context.Background(), cfg, project, tc.name, tc.args)
	}
	_, _ = runPowerShell(context.Background(), self, project, "Write-Output ok", cfg)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runPowerShell(cancelled, self, project, "x", cfg); err == nil {
		t.Fatal("cancelled PowerShell invocation should fail")
	}

	// Real local Git repository: all MCP dispatch branches are exercised.
	ctx, cancelGit := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelGit()
	if _, err := executeGitMCPTool(ctx, cfg, project, "git_init", nil); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	_, _ = runGit(ctx, project, []string{"config", "user.name", "Coverage Test"}, cfg)
	_, _ = runGit(ctx, project, []string{"config", "user.email", "coverage@example.invalid"}, cfg)
	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitCases := []struct {
		name string
		args map[string]any
	}{
		{"git_status", nil},
		{"git_diff", map[string]any{"staged": false, "path": "tracked.txt"}},
		{"git_add", map[string]any{"paths": []any{"tracked.txt"}}},
		{"git_diff", map[string]any{"staged": true}},
		{"git_commit", map[string]any{}},
		{"git_commit", map[string]any{"message": "test: initial", "stage_all": true}},
		{"git_log", map[string]any{"max_count": -1}},
		{"git_log", map[string]any{"max_count": 500}},
		{"git_branch", nil},
		{"git_branch", map[string]any{"name": "coverage-branch"}},
		{"git_checkout", nil},
		{"git_checkout", map[string]any{"target": "coverage-created", "create": true}},
		{"git_checkout", map[string]any{"target": "coverage-branch"}},
		{"git_show", nil},
		{"git_show", map[string]any{"object": "HEAD"}},
		{"git_pull", nil},
		{"git_pull", map[string]any{"remote": "origin", "branch": "main"}},
		{"git_push", nil},
		{"git_push", map[string]any{"set_upstream": true, "remote": "origin", "branch": "coverage-branch"}},
		{"unknown", nil},
	}
	for _, tc := range gitCases {
		_, _ = executeGitMCPTool(ctx, cfg, project, tc.name, tc.args)
	}
	for _, preset := range []string{"git", "powershell"} {
		for _, name := range builtinToolNames(preset, cfg) {
			// Invalid or empty arguments intentionally cover built-in dispatch
			// and validation without causing external side effects.
			_, _ = executeBuiltinMCPTool(ctx, cfg, project, preset, name, map[string]any{})
		}
	}
}

func TestCoverageFinalAgentParsingDispatchAndErrors(t *testing.T) {
	base := isolateCoverageEnv(t)
	root := filepath.Join(base, "root")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "file.txt"), []byte("alpha needle alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.ApprovalMode = "dangerous"
	cfg.NetworkEnabled = false
	cfg.WebSearchProvider = "disabled"
	cfg.AutoResearchToolHelp = false
	cfg.EditingEngine = "native"
	cfg.AiderEnabled = false
	cfg.AutoStateUpdate = false
	cfg.CreateProjectDocs = false
	cfg.CommandTimeout = 1
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Transport != "builtin" {
			cfg.MCPServers[i].Enabled = false
		}
	}
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project
	state.Model = "test-model"
	state.selectProjectThread(project)
	t.Cleanup(state.Close)

	parsing := []struct {
		content string
		ok      bool
	}{
		{`{"action":"finish","message":"done"}`, true},
		{"```json\n{\"action\":\"project_info\"}\n```", true},
		{"prefix {\"action\":\"list_files\",\"message\":\"x\"} suffix", true},
		{`{"message":"missing"}`, false},
		{`not json`, false},
		{`{`, false},
	}
	for _, tc := range parsing {
		a, err := parseAgentAction(tc.content)
		if tc.ok && (err != nil || a.Action == "" || a.Message == "") {
			t.Fatalf("parse %q => %+v %v", tc.content, a, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("expected parse error for %q", tc.content)
		}
	}
	_ = mustJSON(map[string]any{"x": 1})

	ctx := context.Background()
	errorActions := []AgentAction{
		{Action: "discover_tool", Tool: "definitely-not-installed-coverage-tool", Message: "missing"},
		{Action: "read_file", Path: "missing.txt", Message: "read missing"},
		{Action: "list_files", Path: "../outside", Message: "escape"},
		{Action: "search_text", Query: "", Message: "empty search"},
		{Action: "git", Args: []string{"reset", "--hard"}, Message: "blocked git"},
		{Action: "web_search", Query: "query", Message: "offline search"},
		{Action: "web_fetch", URL: "https://example.invalid", Message: "offline fetch"},
		{Action: "mcp_read_resource", Server: "filesystem", URI: "http://bad", Message: "bad resource"},
		{Action: "mcp_get_prompt", Server: "filesystem", PromptName: "missing", Message: "bad prompt"},
		{Action: "unsupported-final", Message: "bad"},
	}
	for _, a := range errorActions {
		result, wait := state.handleAgentAction(ctx, project, a)
		if wait {
			t.Fatalf("unexpected wait for %s", a.Action)
		}
		if !strings.Contains(result, "ERROR:") {
			t.Fatalf("expected error for %s: %s", a.Action, result)
		}
	}

	// Approval/preview failures and execution failures.
	approvedCases := []AgentAction{
		{Action: "replace_text", Path: "file.txt", OldText: "alpha", NewText: "beta", Message: "ambiguous replace"},
		{Action: "delete_file", Path: "missing.txt", Message: "missing delete"},
		{Action: "run_command", Command: "", Message: "empty command"},
		{Action: "run_tool", Tool: "", Message: "empty tool"},
		{Action: "mcp_call_tool", Server: "missing", Tool: "x", Message: "missing MCP"},
	}
	for _, a := range approvedCases {
		result, _ := state.performApproved(ctx, project, a)
		if !strings.Contains(result, "ERROR:") && !strings.Contains(result, "fehlt") {
			t.Fatalf("expected failure for %s: %s", a.Action, result)
		}
	}

	// Direct approved dispatcher branches that can safely fail.
	for _, a := range []AgentAction{
		{Action: "build_project"},
		{Action: "deploy_android"},
		{Action: "web_search", Query: "q"},
		{Action: "web_fetch", URL: "https://example.invalid"},
		{Action: "mcp_call_tool", Server: "filesystem", Tool: "read_text_file", Arguments: map[string]any{"path": "file.txt"}},
		{Action: "git_commit", CommitMessage: "test: nothing", StageAll: true},
	} {
		_, _ = executeAction(ctx, project, cfg, a)
	}

	// Cancellation branch of interactive approval.
	strict := cfg
	strict.ApprovalMode = "strict"
	strict.ApprovalRules = nil
	state.mu.Lock()
	state.Config = strict
	state.mu.Unlock()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if ok, err := state.requestApprovalWithPreview(cancelled, project, AgentAction{Action: "run_command", Command: "echo x", Message: "cancel"}, "preview"); err == nil || ok {
		t.Fatalf("cancel approval ok=%v err=%v", ok, err)
	}
}

func TestCoverageFinalServerInvalidPayloadsAndMethodBranches(t *testing.T) {
	base := isolateCoverageEnv(t)
	root := filepath.Join(base, "projects")
	project := filepath.Join(root, "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.NetworkEnabled = false
	cfg.AutoStateUpdate = false
	cfg.CreateProjectDocs = false
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Transport != "builtin" {
			cfg.MCPServers[i].Enabled = false
		}
	}
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project
	state.Model = "test-model"
	state.selectProjectThread(project)
	t.Cleanup(state.Close)
	server := NewServer(state)

	postEndpoints := []string{
		"/api/project-action", "/api/select-project", "/api/root", "/api/browse-root", "/api/reset-root",
		"/api/chat", "/api/new-chat", "/api/select-chat", "/api/archive-chat", "/api/rename-chat",
		"/api/duplicate-chat", "/api/delete-chat", "/api/open-chat-window", "/api/stop", "/api/force-stop",
		"/api/approve", "/api/settings", "/api/mcp/test", "/api/open-terminal", "/api/terminal-command",
		"/api/open-project", "/api/tools/diagnose", "/api/mcp/setup",
	}
	jsonEndpoints := []string{
		"/api/project-action", "/api/select-project", "/api/root", "/api/chat", "/api/new-chat",
		"/api/select-chat", "/api/archive-chat", "/api/rename-chat", "/api/duplicate-chat",
		"/api/delete-chat", "/api/open-chat-window", "/api/approve", "/api/settings", "/api/mcp/test",
		"/api/open-terminal", "/api/terminal-command", "/api/open-project", "/api/tools/diagnose", "/api/mcp/setup",
	}
	for _, path := range jsonEndpoints {
		rr := serveCoverageRequest(t, server, http.MethodPost, path, json.RawMessage("{"))
		if rr.Code == http.StatusOK {
			t.Fatalf("invalid JSON unexpectedly accepted by %s: %s", path, rr.Body.String())
		}
	}

	// Valid JSON with missing/invalid values reaches deeper validation paths.
	requests := []struct {
		path string
		body any
	}{
		{"/api/project-action", map[string]any{}},
		{"/api/select-project", map[string]any{}},
		{"/api/root", map[string]any{}},
		{"/api/chat", map[string]any{}},
		{"/api/new-chat", map[string]any{"project": filepath.Join(base, "outside")}},
		{"/api/select-chat", map[string]any{"id": "missing"}},
		{"/api/archive-chat", map[string]any{"id": "missing"}},
		{"/api/rename-chat", map[string]any{"id": "missing", "title": ""}},
		{"/api/duplicate-chat", map[string]any{"id": "missing"}},
		{"/api/delete-chat", map[string]any{"id": "missing"}},
		{"/api/open-chat-window", map[string]any{}},
		{"/api/approve", map[string]any{}},
		{"/api/mcp/test", map[string]any{}},
		{"/api/open-terminal", map[string]any{}},
		{"/api/terminal-command", map[string]any{"command": "echo x", "path": filepath.Join(base, "outside")}},
		{"/api/open-project", map[string]any{}},
		{"/api/tools/diagnose", map[string]any{}},
		{"/api/mcp/setup", map[string]any{}},
	}
	for _, tc := range requests {
		rr := serveCoverageRequest(t, server, http.MethodPost, tc.path, tc.body)
		if rr.Code == 0 {
			t.Fatalf("no response from %s", tc.path)
		}
	}

	// Method-not-allowed paths for write-only endpoints and unknown route.
	for _, path := range postEndpoints {
		rr := serveCoverageRequest(t, server, http.MethodPut, path, map[string]any{})
		if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusNotFound {
			t.Fatalf("PUT %s=%d body=%s", path, rr.Code, rr.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/does-not-exist", nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown route=%d", rr.Code)
	}
}

func TestCoverageFinalContinuationNormalizationAndHints(t *testing.T) {
	questions := []struct {
		q string
		a string
	}{
		{"Soll Git initialisiert werden?", "ja"},
		{"Ist das Gerät verbunden?", "das gerät ist verbunden"},
		{"Soll ADB installiert werden?", "adb installieren"},
		{"Soll fortgefahren werden?", "nein abbrechen"},
		{"Welches Modell verwenden?", "modell qwen"},
		{"Alte Rückfrage", "suche die neuesten Nachrichten im Internet"},
		{"", "ja"},
		{"Frage", ""},
		{"Eine längere Frage zu einem völlig anderen Thema", "dies ist eine ausführliche neue Aufgabe ohne jeden Bezug und mit sehr vielen Worten"},
	}
	for _, tc := range questions {
		_ = likelyContinuationAnswer(tc.q, tc.a)
	}

	gitCases := []AgentAction{
		{Action: "git", Message: "commit changes"},
		{Action: "git", Message: "show status"},
		{Action: "git", Message: "show diff"},
		{Action: "git", Message: "Git initialisieren"},
		{Action: "git", Message: "push changes"},
		{Action: "git", Message: "pull changes"},
		{Action: "git", Message: "add .gitignore"},
		{Action: "git", Message: "stage all files"},
		{Action: "git", Message: "something generic"},
		{Action: "git_commit"},
		{Action: "git_commit", CommitMessage: "fix: supplied"},
		{Action: "web_search"},
		{Action: "web_search", Query: "current Go release"},
	}
	for _, a := range gitCases {
		normalized := normalizeAgentAction(a, "implement coverage tests")
		if normalized.Action == "" {
			t.Fatal("normalization removed action")
		}
	}

	avoidanceCases := []struct {
		task     string
		question string
	}{
		{"analyze source", "Soll ich Git initialisieren?"},
		{"create git repository", "Soll ich Git initialisieren?"},
		{"deploy android", "Bitte bestätigen Sie, dass Sie den Debug-APK bereits erfolgreich gebaut haben"},
		{"deploy android", "Stellen Sie sicher, dass ADB installiert ist"},
		{"run", "Möchten Sie den Befehl manuell eingeben oder erneut versuchen?"},
		{"other", "Eine normale echte Nutzerfrage"},
		{"other", ""},
	}
	for _, tc := range avoidanceCases {
		_, _ = blockedAvoidanceQuestion(tc.task, tc.question)
	}
	for _, task := range []string{
		"verteile die App auf das Handy",
		"deploy android",
		"kompiliere das Projekt",
		"build project",
		"suche im Internet die neuesten Nachrichten",
		"analysiere das Projekt",
		"schreibe einen Text",
	} {
		_ = taskAutomationHint(task)
	}
}

func TestCoverageFinalMCPStatusConnectionMatrix(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.CommandTimeout = 1
	cfg.NetworkEnabled = false

	if path, ok := commandAvailable("", project); ok || path != "" {
		t.Fatal(path, ok)
	}
	file := filepath.Join(project, "tool")
	if err := os.WriteFile(file, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if path, ok := commandAvailable(file, project); !ok || path != file {
		t.Fatal(path, ok)
	}
	if _, ok := commandAvailable(project, project); ok {
		t.Fatal("directory must not be executable command")
	}
	if path, ok := commandAvailable("definitely-missing-command", project); ok || path == "" {
		t.Fatal(path, ok)
	}

	var session string
	mcpHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method, _ := payload["method"].(string)
		switch method {
		case "initialize":
			session = "coverage-session"
			w.Header().Set("Mcp-Session-Id", session)
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2026-07-28"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"beta"},{"name":"alpha"}]}}`))
		default:
			http.Error(w, "unknown", http.StatusNotFound)
		}
	}))
	defer mcpHTTP.Close()

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ToolOverrides = map[string]string{"powershell": self, "pwsh": self, "git": self}
	servers := []MCPServerConfig{
		{Name: "filesystem", Preset: "filesystem", Enabled: true, Transport: "builtin"},
		{Name: "powershell", Preset: "powershell", Enabled: true, Transport: "builtin"},
		{Name: "git", Preset: "git", Enabled: true, Transport: "builtin"},
		{Name: "http-off", Enabled: false, Transport: "http", URL: mcpHTTP.URL},
		{Name: "http-on", Enabled: true, Transport: "streamable-http", URL: mcpHTTP.URL, TimeoutSec: 1},
		{Name: "http-empty", Enabled: true, Transport: "http"},
		{Name: "stdio-disabled", Enabled: false, Transport: "stdio", Command: self},
		{Name: "stdio-missing", Enabled: true, Transport: "stdio", Command: filepath.Join(project, "missing")},
		{Name: "bad", Enabled: true, Transport: "unknown"},
	}
	cfg.MCPServers = servers
	for _, server := range servers {
		connect := server.Name == "http-on"
		status := mcpServerStatus(context.Background(), cfg, project, server, connect)
		if status.Name != server.Name {
			t.Fatalf("status mismatch: %+v", status)
		}
		if server.Name == "http-on" && (!status.Connected || status.ToolCount != 2) {
			t.Fatalf("HTTP status not connected: %+v session=%s", status, session)
		}
	}
	statuses := allMCPStatuses(context.Background(), cfg, project, false)
	if len(statuses) != len(servers) {
		t.Fatalf("statuses=%d", len(statuses))
	}
	if findMCPServerIndex(cfg, " HTTP-ON ") < 0 || findMCPServerIndex(cfg, "missing-name") != -1 {
		t.Fatal("server index")
	}

	// Connection succeeds but tools/list has malformed result.
	badHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		switch payload["method"] {
		case "initialize":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2026-07-28"}}`))
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		default:
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":"not-an-object"}`))
		}
	}))
	defer badHTTP.Close()
	badStatus := mcpServerStatus(context.Background(), cfg, project, MCPServerConfig{Name: "bad-result", Enabled: true, Transport: "http", URL: badHTTP.URL, TimeoutSec: 1}, true)
	if badStatus.Connected || badStatus.Error == "" {
		t.Fatalf("expected malformed result error: %+v", badStatus)
	}
}

func TestCoverageFinalProjectCatalogEdgeCases(t *testing.T) {
	isolateCoverageEnv(t)
	root := t.TempDir()
	alpha := filepath.Join(root, "Alpha")
	beta := filepath.Join(root, "Beta")
	hiddenDot := filepath.Join(root, ".hidden")
	for _, dir := range []string{alpha, beta, hiddenDot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "not-project.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = alpha
	cfg.ProjectAliases = map[string]string{alpha: "Zeta", beta: "Alpha alias", filepath.Join(root, "missing"): "Missing"}
	cfg.PinnedProjects = []string{alpha}
	cfg.HiddenProjects = []string{beta, filepath.Join(root, "missing"), t.TempDir()}
	projects, err := listProjects(cfg)
	if err != nil || len(projects) != 1 || !projects[0].Pinned {
		t.Fatalf("projects=%+v err=%v", projects, err)
	}
	hidden := listHiddenProjects(cfg)
	if len(hidden) != 1 || hidden[0].Path != beta {
		t.Fatalf("hidden=%+v", hidden)
	}
	badCfg := cfg
	badCfg.RootProjectDir = filepath.Join(root, "missing-root")
	if _, err := listProjects(badCfg); err == nil {
		t.Fatal("expected missing root")
	}
	_ = projectDisplayName(cfg, string(filepath.Separator))
	_ = projectDisplayName(cfg, "")
	_ = projectListWithout([]string{alpha, beta}, alpha)

	state := NewAppState(cfg, NewOllamaClient())
	state.Project = alpha
	state.selectProjectThread(alpha)
	t.Cleanup(state.Close)
	if _, err := state.ProjectAction("", "pin", ""); err == nil {
		t.Fatal("empty path")
	}
	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()
	if _, err := state.ProjectAction(alpha, "pin", ""); err == nil {
		t.Fatal("running")
	}
	state.mu.Lock()
	state.Running = false
	state.mu.Unlock()
	if _, err := state.ProjectAction(filepath.Join(root, "missing"), "pin", ""); err == nil {
		t.Fatal("missing project")
	}
	if _, err := state.ProjectAction(t.TempDir(), "pin", ""); err == nil {
		t.Fatal("outside root")
	}
	if _, err := state.ProjectAction(alpha, "rename", strings.Repeat("x", 121)); err == nil {
		t.Fatal("long alias")
	}
	if summary, err := state.ProjectAction(alpha, "rename", filepath.Base(alpha)); err != nil || summary.Name != "Alpha" {
		t.Fatalf("rename reset=%+v err=%v", summary, err)
	}
	if _, err := state.ProjectAction(alpha, "unsupported", ""); err == nil {
		t.Fatal("unsupported action")
	}
	if _, err := state.ProjectAction(alpha, "remove", ""); err != nil {
		t.Fatal(err)
	}
	state.mu.RLock()
	selected := state.Project
	last := state.Config.LastProject
	state.mu.RUnlock()
	if selected != "" || last != "" {
		t.Fatalf("removed current project still selected project=%q last=%q", selected, last)
	}
}

func TestCoverageFinalStandardProfileFallbacks(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCALCODE_USER_HOME", "")
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	} else {
		t.Setenv("USERPROFILE", "")
		t.Setenv("HOME", home)
	}
	if got := userProfileDir(); filepath.Clean(got) != filepath.Clean(home) {
		t.Fatalf("user profile fallback = %q, want %q", got, home)
	}

	t.Setenv("LOCALCODE_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("LOCALCODE_CACHE_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	configBase := userConfigBaseDir()
	cacheBase := userCacheBaseDir()
	if strings.TrimSpace(configBase) == "" || strings.TrimSpace(cacheBase) == "" {
		t.Fatalf("empty standard directories: config=%q cache=%q", configBase, cacheBase)
	}
}

func TestCoverageFinalConfigAndFileFailureBranches(t *testing.T) {
	base := isolateCoverageEnv(t)
	t.Setenv("LOCALCODE_CONFIG_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "xdg-config"))
	if !strings.Contains(userConfigBaseDir(), "xdg-config") {
		t.Fatal(userConfigBaseDir())
	}
	t.Setenv("LOCALCODE_CACHE_HOME", "")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "xdg-cache"))
	if !strings.Contains(userCacheBaseDir(), "xdg-cache") {
		t.Fatal(userCacheBaseDir())
	}

	source := filepath.Join(base, "source.txt")
	target := filepath.Join(base, "nested", "target.txt")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyFileIfMissing(source, target); err != nil {
		t.Fatal(err)
	}
	if err := copyFileIfMissing(filepath.Join(base, "missing"), target); err != nil {
		t.Fatal("existing target should short-circuit", err)
	}
	if err := copyFileIfMissing(filepath.Join(base, "missing"), filepath.Join(base, "other")); err == nil {
		t.Fatal("missing source")
	}
	if err := copyDirIfMissing(filepath.Join(base, "missing-dir"), filepath.Join(base, "copy")); err == nil {
		t.Fatal("missing directory")
	}
	if err := os.MkdirAll(filepath.Join(base, "tree", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "tree", "sub", "x.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyDirIfMissing(filepath.Join(base, "tree"), filepath.Join(base, "tree-copy")); err != nil {
		t.Fatal(err)
	}
	if pathWithin("", target) || pathWithin(base, "") || !pathWithin(base, base) || !pathWithin(base, target) || pathWithin(base, filepath.Dir(base)) {
		t.Fatal("pathWithin matrix")
	}

	project := filepath.Join(base, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeProjectFile(project, "../escape.txt", "x"); err == nil {
		t.Fatal("escape write")
	}
	binary := filepath.Join(project, "binary.bin")
	if err := os.WriteFile(binary, []byte{0, 1, 0, 2}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeProjectFile(project, "binary.bin", "text"); err == nil {
		t.Fatal("binary overwrite")
	}
	dir := filepath.Join(project, "dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := deleteProjectFile(project, "dir"); err == nil {
		t.Fatal("directory deletion")
	}
	if _, err := deleteProjectFile(project, "missing.txt"); err == nil {
		t.Fatal("missing deletion")
	}
	if _, err := deleteProjectFile(project, "../escape.txt"); err == nil {
		t.Fatal("escape deletion")
	}
}

func TestCoverageFinalCancelledInstallOffers(t *testing.T) {
	base := isolateCoverageEnv(t)
	project := filepath.Join(base, "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	emptyBin := filepath.Join(base, "empty-bin")
	if err := os.MkdirAll(emptyBin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", emptyBin)
	cfg := defaultConfig()
	cfg.RootProjectDir = base
	cfg.LastProject = project
	cfg.AiderEnabled = true
	cfg.AiderAutoInstall = false
	cfg.AiderExecutable = filepath.Join(base, "missing-aider")
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project
	t.Cleanup(state.Close)

	if _, _, installed, err := state.offerInstallAider(context.Background(), project, cfg); err == nil || installed {
		t.Fatalf("disabled auto-install should return typed error, installed=%v err=%v", installed, err)
	}
	cfg.AiderAutoInstall = true
	state.mu.Lock()
	state.Config = cfg
	state.mu.Unlock()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, installed, err := state.offerInstallAider(cancelled, project, cfg); err == nil || installed {
		t.Fatalf("cancelled Aider approval installed=%v err=%v", installed, err)
	}

	missing := &ToolNotFoundError{Info: ToolInfo{Name: "git", DisplayName: "Git", InstallSupported: true}, Detail: "not found in test paths"}
	cancelledTool, cancelTool := context.WithCancel(context.Background())
	cancelTool()
	if _, _, installed, err := state.offerInstallMissingTool(cancelledTool, project, cfg, missing); err == nil || installed {
		t.Fatalf("cancelled tool approval installed=%v err=%v", installed, err)
	}

	if minInt(9, 2) != 2 || minInt(1, 2) != 1 {
		t.Fatal("minInt")
	}
	if got := tailStrings([]string{"a"}, 2); len(got) != 1 {
		t.Fatal(got)
	}
	if minDuration(time.Second, 2*time.Second) != time.Second || minDuration(3*time.Second, 2*time.Second) != 2*time.Second {
		t.Fatal("minDuration")
	}
	if timeDurationSeconds(0) != 90*time.Second || timeDurationSeconds(2) != 2*time.Second {
		t.Fatal("timeDurationSeconds")
	}
}

func TestCoverageFinalMCPStdioInternalBookkeeping(t *testing.T) {
	for _, payload := range []map[string]any{
		{},
		{"id": int64(1)},
		{"id": int(2)},
		{"id": float64(3)},
		{"id": "bad"},
	} {
		_, _ = requestID(payload)
	}
	s := &mcpStdioSession{
		pending: map[int64]chan mcpPendingResult{
			1: make(chan mcpPendingResult, 1),
			2: make(chan mcpPendingResult, 1),
		},
		done: make(chan struct{}),
	}
	if s.isClosed() {
		t.Fatal("new session closed")
	}
	s.removePending(1)
	if _, ok := s.pending[1]; ok {
		t.Fatal("pending request was not removed")
	}
	s.readStderr(strings.NewReader("first\nsecond\n"))
	if text := s.stderrText(); !strings.Contains(text, "first") || !strings.Contains(text, "second") {
		t.Fatal(text)
	}
	close(s.done)
	if !s.isClosed() {
		t.Fatal("closed session reported open")
	}
	if err := s.write(map[string]any{"x": 1}); err == nil {
		t.Fatal("write to closed session")
	}
}
