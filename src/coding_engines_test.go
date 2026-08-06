// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fakeExternalEngine(t *testing.T, kind string) string {
	t.Helper()
	name := kind
	body := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "` + kind + ` 1.2.3"; exit 0; fi
if [ "$1" = "auth" ]; then echo "logged in"; exit 0; fi
printf "engine changed\n" > engine-output.txt
echo "engine completed"
exit 0`
	if runtime.GOOS == "windows" {
		name += ".cmd"
		body = `@echo off
if "%1"=="--version" (echo ` + kind + ` 1.2.3& exit /b 0)
if "%1"=="auth" (echo logged in& exit /b 0)
echo engine changed>engine-output.txt
echo engine completed
exit /b 0`
	}
	path := filepath.Join(t.TempDir(), name)
	writeTestExecutable(t, path, body)
	return path
}

func TestCodingEngineConfigurationAndArguments(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 8, EditingEngine: "CLAUDE", ClaudeCodePermissionMode: "invalid", ClaudeCodeMaxTurns: 0})
	if cfg.SchemaVersion != 9 || cfg.EditingEngine != editingEngineClaude || cfg.ClaudeCodePermissionMode != "acceptEdits" || cfg.ClaudeCodeMaxTurns != 24 {
		t.Fatalf("normalized config = %#v", cfg)
	}
	if normalizeEditingEngine("unknown") != editingEngineAider || codingEngineDisplayName("opencode") != "OpenCode" {
		t.Fatal("engine normalization/display failed")
	}
	if !isCodingEngineAction("engine_edit") || !isCodingEngineAction("aider_test") || isCodingEngineAction("read_file") {
		t.Fatal("engine action classification failed")
	}

	cfg.ClaudeCodeModel = "sonnet"
	cfg.ClaudeCodePermissionMode = "acceptEdits"
	cfg.ClaudeCodeMaxTurns = 7
	args := buildClaudeCodeArgs("fix it", selectedEngineModel(cfg, "fallback"), "edit", "thread 1", cfg)
	joined := strings.Join(args, " ")
	for _, wanted := range []string{"-p", "--model sonnet", "--permission-mode acceptEdits", "--max-turns 7", "fix it"} {
		if !strings.Contains(joined, wanted) {
			t.Fatalf("Claude args %q missing %q", joined, wanted)
		}
	}
	plan := strings.Join(buildClaudeCodeArgs("inspect", "sonnet", "repo-map", "", cfg), " ")
	if !strings.Contains(plan, "--permission-mode plan") {
		t.Fatalf("repo-map Claude args = %q", plan)
	}

	cfg.EditingEngine = editingEngineOpenCode
	cfg.OpenCodeModel = ""
	cfg.OllamaDefaultModel = "qwen2.5-coder:14b"
	cfg.OpenCodeAgent = "build"
	cfg.OpenCodeAutoApprove = true
	if got := selectedEngineModel(cfg, "qwen2.5-coder:14b"); got != "ollama/qwen2.5-coder:14b" {
		t.Fatalf("OpenCode model = %q", got)
	}
	openArgs := strings.Join(buildOpenCodeArgs("fix it", selectedEngineModel(cfg, "qwen2.5-coder:14b"), "edit", cfg), " ")
	for _, wanted := range []string{"run", "--agent build", "--model ollama/qwen2.5-coder:14b", "--auto", "fix it"} {
		if !strings.Contains(openArgs, wanted) {
			t.Fatalf("OpenCode args %q missing %q", openArgs, wanted)
		}
	}
}

func TestClaudeInstallerCommandAndOpenCodeOllamaEnvironment(t *testing.T) {
	stable := claudeInstallPowerShell("stable")
	versioned := claudeInstallPowerShell("2.1.211")
	if !strings.Contains(stable, "install.ps1") || !strings.HasSuffix(stable, "'stable'") {
		t.Fatalf("stable installer command = %q", stable)
	}
	if !strings.HasSuffix(versioned, "'2.1.211'") {
		t.Fatalf("version installer command = %q", versioned)
	}

	cfg := defaultConfig()
	cfg.OllamaURL = "http://127.0.0.1:11434/"
	env := openCodeCommandEnvironment(cfg, "ollama/qwen2.5-coder:14b")
	joined := strings.Join(env, "\n")
	for _, wanted := range []string{
		"OPENCODE_DISABLE_AUTOUPDATE=1",
		"OLLAMA_HOST=http://127.0.0.1:11434",
		"OPENCODE_CONFIG_CONTENT=",
		`"baseURL":"http://127.0.0.1:11434/v1"`,
		`"qwen2.5-coder:14b"`,
		`"npm":"@ai-sdk/openai-compatible"`,
	} {
		if !strings.Contains(joined, wanted) {
			t.Fatalf("OpenCode environment missing %q: %s", wanted, joined)
		}
	}
	remote := strings.Join(openCodeCommandEnvironment(cfg, "anthropic/claude-sonnet-4"), "\n")
	if strings.Contains(remote, "OPENCODE_CONFIG_CONTENT=") {
		t.Fatalf("remote provider unexpectedly received Ollama config: %s", remote)
	}
	if got := ollamaOpenAIBaseURL("http://localhost:11434/v1/"); got != "http://localhost:11434/v1" {
		t.Fatalf("normalized Ollama URL = %q", got)
	}
}

func TestClaudeAndOpenCodeStatusAndExecution(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "main.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.NetworkEnabled = false
	cfg.CommandTimeout = 30

	claude := fakeExternalEngine(t, "claude")
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeExecutable = claude
	status := codingEngineStatus(context.Background(), cfg, editingEngineClaude)
	if !status.Installed || !status.Authenticated || !strings.Contains(status.Version, "1.2.3") || status.Executable == "" {
		t.Fatalf("Claude status = %#v", status)
	}
	result, err := runCodingEngine(context.Background(), project, "change the project", "", "thread", "edit", cfg)
	if err != nil {
		t.Fatalf("Claude run failed: %v\n%+v", err, result)
	}
	if len(result.ChangedFiles) == 0 || result.BackupDir == "" || !strings.Contains(result.Output, "engine completed") {
		t.Fatalf("Claude result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(project, "engine-output.txt")); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(project, "engine-output.txt")); err != nil {
		t.Fatal(err)
	}
	opencode := fakeExternalEngine(t, "opencode")
	cfg.EditingEngine = editingEngineOpenCode
	cfg.OpenCodeExecutable = opencode
	cfg.OpenCodeModel = "ollama/qwen2.5-coder:14b"
	status = codingEngineStatus(context.Background(), cfg, editingEngineOpenCode)
	if !status.Installed || !status.Authenticated || !strings.Contains(status.Version, "1.2.3") {
		t.Fatalf("OpenCode status = %#v", status)
	}
	result, err = runCodingEngine(context.Background(), project, "change it", "", "thread", "edit", cfg)
	if err != nil || result.Engine != editingEngineOpenCode || result.BackupDir == "" {
		t.Fatalf("OpenCode result = %+v, err=%v", result, err)
	}

	cfg.EditingEngine = editingEngineNative
	status = codingEngineStatus(context.Background(), cfg, editingEngineNative)
	if !status.Installed || !status.Authenticated || status.Executable != "embedded" {
		t.Fatalf("native status = %#v", status)
	}
	if _, err := runCodingEngine(context.Background(), project, "x", "", "", "edit", cfg); err == nil {
		t.Fatal("native subprocess should be rejected")
	}
}

func TestCodingEngineHTTPStatusAndTest(t *testing.T) {
	project := t.TempDir()
	executable := fakeExternalEngine(t, "opencode")
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.EditingEngine = editingEngineOpenCode
	cfg.OpenCodeExecutable = executable
	cfg.OpenCodeModel = "ollama/qwen2.5-coder:14b"
	cfg.NetworkEnabled = false
	state := &AppState{Config: cfg, Project: project, Model: "qwen2.5-coder:14b", Ollama: NewOllamaClient()}
	server := NewServer(state)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/engines/status?engine=opencode", nil)
	req.Host = "127.0.0.1"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status HTTP %d: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Selected string               `json:"selected"`
		Status   CodingEngineStatus   `json:"status"`
		Engines  []CodingEngineStatus `json:"engines"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Selected != editingEngineOpenCode || !payload.Status.Installed || len(payload.Engines) != 4 {
		t.Fatalf("payload = %#v", payload)
	}

	body := strings.NewReader(`{"action":"test","engine":"opencode"}`)
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/engines/setup", body)
	req.Host = "127.0.0.1"
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("test HTTP %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCodingEngineCancellationReturns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered by generic Windows process-tree cancellation tests")
	}
	path := filepath.Join(t.TempDir(), "claude")
	writeTestExecutable(t, path, "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo claude 1; exit 0; fi\nif [ \"$1\" = \"auth\" ]; then echo logged; exit 0; fi\nsleep 30\n")
	cfg := defaultConfig()
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeExecutable = path
	project := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := runCodingEngine(ctx, project, "wait", "", "", "edit", cfg)
	if err == nil || time.Since(started) > 6*time.Second {
		t.Fatalf("cancellation err=%v duration=%v", err, time.Since(started))
	}
}

func TestCodingEngineActionsInstallDecisionsAndUndo(t *testing.T) {
	project := t.TempDir()
	original := filepath.Join(project, "original.txt")
	if err := os.WriteFile(original, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	executable := fakeExternalEngine(t, "claude")
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeExecutable = executable
	cfg.CommandTimeout = 30
	state := &AppState{Config: cfg, Project: project, Model: "sonnet", CurrentThread: "thread-1"}

	text, err := state.executeCodingEngineAction(context.Background(), project, cfg, AgentAction{Action: "engine_edit", Task: "make a change", Message: "edit"})
	if err != nil || !strings.Contains(text, "Claude Code") || state.LastEngineBackup == "" {
		t.Fatalf("engine action text=%q backup=%q err=%v", text, state.LastEngineBackup, err)
	}
	if _, err := state.executeCodingEngineAction(context.Background(), project, cfg, AgentAction{Action: "engine_repo_map", Message: "inspect"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.executeCodingEngineAction(context.Background(), project, cfg, AgentAction{Action: "engine_lint", Message: "lint"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.executeCodingEngineAction(context.Background(), project, cfg, AgentAction{Action: "engine_test", Message: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.executeCodingEngineAction(context.Background(), project, cfg, AgentAction{Action: "bogus"}); err == nil {
		t.Fatal("unsupported engine action should fail")
	}

	installedCfg, detail, installed, err := state.offerInstallCodingEngine(context.Background(), project, cfg)
	if err != nil || !installed || installedCfg.ClaudeCodeExecutable == "" || !(strings.Contains(detail, "installed") || strings.Contains(detail, "installiert")) {
		t.Fatalf("installed decision cfg=%#v detail=%q installed=%v err=%v", installedCfg, detail, installed, err)
	}
	missing := cfg
	missing.ClaudeCodeExecutable = filepath.Join(project, "missing-claude")
	missing.ClaudeCodeAutoInstall = false
	_, _, installed, err = state.offerInstallCodingEngine(context.Background(), project, missing)
	var typed *CodingEngineNotInstalledError
	if installed || !errors.As(err, &typed) || typed.Error() == "" {
		t.Fatalf("missing decision installed=%v err=%v", installed, err)
	}
	if codingEngineAutoInstall(cfg, editingEngineClaude) != cfg.ClaudeCodeAutoInstall || codingEngineAutoInstall(cfg, editingEngineNative) {
		t.Fatal("auto-install policy failed")
	}

	if err := os.WriteFile(original, []byte("later manual edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	undo, err := state.executeCodingEngineAction(context.Background(), project, cfg, AgentAction{Action: "engine_undo"})
	if err != nil || undo == "" {
		t.Fatalf("undo=%q err=%v", undo, err)
	}
}

func TestCodingEngineServerNativeSetupAndUndo(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("LOCALCODE_DATA_HOME", t.TempDir())
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "file.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	backup, err := createAiderBackup(project)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotProjectFingerprints(project)
	if err := os.WriteFile(filepath.Join(project, "file.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after := snapshotProjectFingerprints(project)
	changed := changedFingerprintPaths(before, after)
	if err := writeAiderBackupManifest(backup, project, before, after, changed); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.EditingEngine = editingEngineNative
	state := &AppState{Config: cfg, Project: project, Model: "qwen2.5-coder:14b", LastEngineBackup: backup, Ollama: NewOllamaClient()}
	server := NewServer(state)

	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
		req.Host = "127.0.0.1"
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		return rr
	}
	if rr := request(http.MethodPost, "/api/engines/setup", `{"action":"install","engine":"native"}`); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("native install %d: %s", rr.Code, rr.Body.String())
	}
	if rr := request(http.MethodPost, "/api/engines/setup", `{"action":"login","engine":"native"}`); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "no separate CLI login") {
		t.Fatalf("native login %d: %s", rr.Code, rr.Body.String())
	}
	if rr := request(http.MethodPost, "/api/engines/setup", `{"action":"invalid","engine":"native"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid setup = %d", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/engines/undo", `{}`); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("undo %d: %s", rr.Code, rr.Body.String())
	}
	if rr := request(http.MethodGet, "/api/engines/undo", ""); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("undo method = %d", rr.Code)
	}
	if got := engineLoginCommand("tool path", editingEngineClaude); !strings.Contains(got, "auth") || !strings.Contains(got, "login") {
		t.Fatalf("login command = %q", got)
	}
}

func TestCodingEngineInstallDispatchAndRepairWrapper(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.NetworkEnabled = false
	for _, engine := range []string{editingEngineAider, editingEngineClaude, editingEngineOpenCode, editingEngineNative} {
		status, _, _, err := installCodingEngine(context.Background(), project, engine, cfg)
		if engine == editingEngineNative {
			if err != nil || !status.Installed {
				t.Fatalf("native install dispatch: status=%#v err=%v", status, err)
			}
		} else if err == nil {
			t.Fatalf("%s install should be blocked without network", engine)
		}
	}

	executable := fakeExternalEngine(t, "claude")
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeExecutable = executable
	cfg.CommandTimeout = 30
	state := &AppState{Config: cfg, Project: project, Model: "sonnet", CurrentThread: "thread"}
	result, err := state.executeActionWithToolRepair(context.Background(), project, cfg, AgentAction{Action: "engine_edit", Task: "edit"})
	if err != nil || !strings.Contains(result, "Claude Code") {
		t.Fatalf("repair wrapper result=%q err=%v", result, err)
	}

	missing := cfg
	missing.ClaudeCodeExecutable = filepath.Join(project, "missing")
	missing.ClaudeCodeAutoInstall = false
	_, err = state.executeActionWithToolRepair(context.Background(), project, missing, AgentAction{Action: "engine_edit", Task: "edit"})
	if err == nil {
		t.Fatal("missing engine should remain an error when auto-install is disabled")
	}
}

func TestCodingEngineServerValidationAndConflicts(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeExecutable = filepath.Join(project, "missing")
	cfg.NetworkEnabled = false
	state := &AppState{Config: cfg, Project: project, Ollama: NewOllamaClient()}
	server := NewServer(state)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
		req.Host = "127.0.0.1"
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		return rr
	}
	if rr := request(http.MethodGet, "/api/engines/setup", ""); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("setup method = %d", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/engines/setup", "{"); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad JSON = %d", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/engines/setup", `{"action":"login","engine":"claude"}`); rr.Code != http.StatusConflict {
		t.Fatalf("missing login = %d: %s", rr.Code, rr.Body.String())
	}
	if rr := request(http.MethodPost, "/api/engines/setup", `{"action":"test","engine":"claude"}`); rr.Code != http.StatusConflict {
		t.Fatalf("missing test = %d: %s", rr.Code, rr.Body.String())
	}
	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()
	if rr := request(http.MethodPost, "/api/engines/setup", `{"action":"install","engine":"claude"}`); rr.Code != http.StatusConflict {
		t.Fatalf("running conflict = %d", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/engines/undo", `{}`); rr.Code != http.StatusConflict {
		t.Fatalf("undo running conflict = %d", rr.Code)
	}
}

func TestCodingEngineHelperBranches(t *testing.T) {
	cfg := defaultConfig()
	cfg.AiderAutoInstall = false
	cfg.OpenCodeAutoInstall = true
	if codingEngineAutoInstall(cfg, editingEngineAider) || !codingEngineAutoInstall(cfg, editingEngineOpenCode) {
		t.Fatal("engine auto-install branches failed")
	}
	plain := (&CodingEngineNotInstalledError{Status: CodingEngineStatus{DisplayName: "X"}}).Error()
	withDetail := (&CodingEngineNotInstalledError{Status: CodingEngineStatus{DisplayName: "X", Error: "broken"}}).Error()
	if !strings.Contains(plain, "nicht installiert") || !strings.Contains(withDetail, "broken") {
		t.Fatalf("errors: %q / %q", plain, withDetail)
	}
	cfg.AiderMainModel = "aider-model"
	cfg.EditingEngine = editingEngineAider
	if selectedEngineModel(cfg, "fallback") != "aider-model" {
		t.Fatal("Aider model selection failed")
	}
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeModel = ""
	if selectedEngineModel(cfg, "fallback") != "sonnet" {
		t.Fatal("Claude default model failed")
	}
	cfg.EditingEngine = editingEngineOpenCode
	cfg.OpenCodeModel = "provider/model"
	if selectedEngineModel(cfg, "fallback") != "provider/model" {
		t.Fatal("OpenCode explicit model failed")
	}
	project := t.TempDir()
	cfg.AiderLintCommand = "custom-lint"
	cfg.AiderTestCommand = "custom-test"
	for mode, wanted := range map[string]string{"repo-map": "Analyze this repository", "lint": "custom-lint", "test": "custom-test", "edit": "original"} {
		if got := engineTaskForMode(project, "original", mode, cfg); !strings.Contains(got, wanted) {
			t.Fatalf("mode %s: %q", mode, got)
		}
	}
	cfg.OpenCodeAgent = ""
	planArgs := strings.Join(buildOpenCodeArgs("inspect", "", "repo-map", cfg), " ")
	if !strings.Contains(planArgs, "--agent plan") || strings.Contains(planArgs, "--auto") {
		t.Fatalf("plan args = %q", planArgs)
	}
	cfg.ClaudeCodeEnabled = false
	if st := codingEngineStatus(context.Background(), cfg, editingEngineClaude); st.Enabled || st.Error == "" {
		t.Fatalf("disabled status = %#v", st)
	}
}

func TestCodingEngineServerAdditionalErrorBranches(t *testing.T) {
	cfg := defaultConfig()
	cfg.RootProjectDir = ""
	cfg.LastProject = ""
	cfg.EditingEngine = editingEngineClaude
	cfg.ClaudeCodeExecutable = filepath.Join(t.TempDir(), "missing")
	cfg.NetworkEnabled = false
	state := &AppState{Config: cfg, Ollama: NewOllamaClient()}
	server := NewServer(state)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
		req.Host = "127.0.0.1"
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		return rr
	}
	if rr := request(http.MethodPost, "/api/engines/status", `{}`); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status method = %d", rr.Code)
	}
	if rr := request(http.MethodPost, "/api/engines/undo", `{}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("undo without project = %d: %s", rr.Code, rr.Body.String())
	}
	state.mu.Lock()
	state.Project = t.TempDir()
	state.mu.Unlock()
	if rr := request(http.MethodPost, "/api/engines/undo", `{}`); rr.Code != http.StatusConflict {
		t.Fatalf("undo without backup = %d: %s", rr.Code, rr.Body.String())
	}
	if rr := request(http.MethodPost, "/api/engines/setup", `{"action":"install","engine":"claude"}`); rr.Code != http.StatusInternalServerError {
		t.Fatalf("offline install = %d: %s", rr.Code, rr.Body.String())
	}
}
