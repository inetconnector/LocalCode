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

func TestComputeMeshAutoDetectCredentialsAllSources(t *testing.T) {
	// Clear env vars first so file discovery paths execute
	for _, k := range []string{
		"COMPUTEMESH_API_KEY", "COMPUTEMESH_PROVIDER_KEY", "COMPUTEMESH_KEY",
		"COMPUTEMESH_AUTH_TOKEN", "COMPUTEMESH_URL", "COMPUTEMESH_NODE_URL",
	} {
		t.Setenv(k, "")
	}

	home := t.TempDir()
	appData := t.TempDir()
	localAppData := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", appData)
	t.Setenv("LOCALAPPDATA", localAppData)

	dotMesh := filepath.Join(home, ".computemesh")
	_ = os.MkdirAll(dotMesh, 0o700)

	// 1. JSON variants in ~/.computemesh/
	jsonVariants := []struct {
		file string
		body string
		want string
	}{
		{"provider_config.json", `{"provider_key":"pk-1","gateway_url":"https://gw1.example","provider_account":"acct-1","local_node_id":"node-1","local_node_url":"http://127.0.0.1:8081"}`, "pk-1"},
		{"node_registry.json", `{"api_key":"ak-2","url":"https://gw2.example","node_id":"node-2"}`, "ak-2"},
		{"credentials.json", `{"token":"tk-3"}`, "tk-3"},
		{"config.json", `{"auth_token":"at-4"}`, "at-4"},
		{"config.json", `{"key":"k-5"}`, "k-5"},
		{"config.json", `{"provider":{"api_key":"pak-6","account":"acct-6"}}`, "pak-6"},
		{"config.json", `{"provider":{"key":"pk-7"}}`, "pk-7"},
	}
	for _, tc := range jsonVariants {
		// clean previous files
		entries, _ := os.ReadDir(dotMesh)
		for _, e := range entries {
			_ = os.Remove(filepath.Join(dotMesh, e.Name()))
		}
		_ = os.WriteFile(filepath.Join(dotMesh, tc.file), []byte(tc.body), 0o600)
		apiKey, _, _, _, _, src := AutoDetectComputeMeshCredentials()
		if apiKey != tc.want || src == "" {
			t.Fatalf("variant %s: apiKey=%q want=%q src=%q", tc.file, apiKey, tc.want, src)
		}
	}

	// 2. Plain text token file fallback
	entries, _ := os.ReadDir(dotMesh)
	for _, e := range entries {
		_ = os.Remove(filepath.Join(dotMesh, e.Name()))
	}
	_ = os.WriteFile(filepath.Join(dotMesh, "token.txt"), []byte("txt-token-8\n"), 0o600)
	if apiKey, _, _, _, _, _ := AutoDetectComputeMeshCredentials(); apiKey != "txt-token-8" {
		t.Fatalf("txt fallback apiKey=%q", apiKey)
	}
	_ = os.Remove(filepath.Join(dotMesh, "token.txt"))

	// 3. APPDATA/ComputeMesh fallback
	cmAppDir := filepath.Join(appData, "ComputeMesh")
	_ = os.MkdirAll(cmAppDir, 0o700)
	_ = os.WriteFile(filepath.Join(cmAppDir, "credentials.json"), []byte(`{"api_key":"appdata-key-9"}`), 0o600)
	_, _, _, _, _, _ = AutoDetectComputeMeshCredentials()

	// 4. Env var precedence
	t.Setenv("COMPUTEMESH_API_KEY", "env-key-10")
	t.Setenv("COMPUTEMESH_URL", "https://custom-gw.example")
	t.Setenv("COMPUTEMESH_NODE_URL", "http://127.0.0.1:9099")
	apiKey, gw, _, _, nodeURL, src := AutoDetectComputeMeshCredentials()
	if apiKey != "env-key-10" || gw != "https://custom-gw.example" || nodeURL != "http://127.0.0.1:9099" || !strings.Contains(src, "COMPUTEMESH_API_KEY") {
		t.Fatalf("env precedence failed: key=%q gw=%q node=%q src=%q", apiKey, gw, nodeURL, src)
	}
}

func TestPushRegisterHTTPAllProvidersAndValidation(t *testing.T) {
	state := newWave17TestState(t)
	srv := NewServer(state)

	cases := []struct {
		body string
		want int
	}{
		{`{bad`, http.StatusBadRequest},
		{`{"device_id":"bad id!","provider":"unifiedpush","endpoint":"https://ntfy.sh/ok"}`, http.StatusBadRequest},
		{`{"device_id":"dev-1","provider":"unknown_prov"}`, http.StatusBadRequest},
		{`{"device_id":"dev-1","provider":"fcm_v1"}`, http.StatusBadRequest},
		{`{"device_id":"dev-1","provider":"fcm_v1","bearer_token":"b1","device_token":"t1"}`, http.StatusBadRequest},
		{`{"device_id":"dev-1","provider":"fcm_v1","bearer_token":"b1","device_token":"t1","endpoint":"ftp://invalid.example/x"}`, http.StatusBadRequest},
		{`{"device_id":"dev-1","provider":"fcm_v1","bearer_token":"b1","device_token":"t1","project_id":"proj-1"}`, http.StatusOK},
		{`{"device_id":"dev-2","provider":"fcm_v1","bearer_token":"b2","device_token":"t2","endpoint":"https://fcm.googleapis.com/v1/projects/p/messages:send"}`, http.StatusOK},
		{`{"device_id":"dev-3","provider":"apns","bearer_token":"b3"}`, http.StatusBadRequest},
		{`{"device_id":"dev-3","provider":"apns","bearer_token":"b3","device_token":"t3","apns_topic":"com.test.app","endpoint":"ftp://invalid.example/apns"}`, http.StatusBadRequest},
		{`{"device_id":"dev-3","provider":"apns","bearer_token":"b3","device_token":"t3","apns_topic":"com.test.app","endpoint":"https://api.sandbox.push.apple.com/3/device/abc","apns_sandbox":true}`, http.StatusOK},
		{`{"device_id":"dev-4","endpoint":"https://ntfy.sh/topic-default-provider"}`, http.StatusOK},
	}
	for i, tc := range cases {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/push/register-endpoint", strings.NewReader(tc.body)))
		if rr.Code != tc.want {
			t.Fatalf("case %d (%s): status=%d want=%d body=%s", i, tc.body, rr.Code, tc.want, rr.Body.String())
		}
	}
}

func TestPreflightInferenceBackendAllProviders(t *testing.T) {
	state := newWave17TestState(t)
	for _, k := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "LOCALCODE_LLAMACPP_URL"} {
		t.Setenv(k, "")
	}

	// Anthropic missing + present
	cfg := defaultConfig()
	cfg.InferenceProvider = "anthropic"
	if err := state.preflightInferenceBackend(cfg, "claude-sonnet-4-6"); err == nil {
		t.Fatal("missing ANTHROPIC_API_KEY must fail")
	}
	cfg.EnvironmentVars = map[string]string{"ANTHROPIC_API_KEY": "sk-test-placeholder"}
	if err := state.preflightInferenceBackend(cfg, "claude-sonnet-4-6"); err != nil {
		t.Fatalf("configured ANTHROPIC_API_KEY failed: %v", err)
	}

	// OpenAI missing + present
	cfg = defaultConfig()
	cfg.InferenceProvider = "openai"
	if err := state.preflightInferenceBackend(cfg, "gpt-5"); err == nil {
		t.Fatal("missing OPENAI_API_KEY must fail")
	}
	cfg.EnvironmentVars = map[string]string{"OPENAI_API_KEY": "sk-test-placeholder"}
	if err := state.preflightInferenceBackend(cfg, "gpt-5"); err != nil {
		t.Fatalf("configured OPENAI_API_KEY failed: %v", err)
	}

	// Gemini missing + present
	cfg = defaultConfig()
	cfg.InferenceProvider = "gemini"
	if err := state.preflightInferenceBackend(cfg, "gemini-2.5-pro"); err == nil {
		t.Fatal("missing GEMINI_API_KEY must fail")
	}
	cfg.EnvironmentVars = map[string]string{"GEMINI_API_KEY": "gm-test-placeholder"}
	if err := state.preflightInferenceBackend(cfg, "gemini-2.5-pro"); err != nil {
		t.Fatalf("configured GEMINI_API_KEY failed: %v", err)
	}

	// llama.cpp mock server (200 OK & 500 error)
	llamaStatus := http.StatusOK
	llamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(llamaStatus)
		_, _ = w.Write([]byte(`{"data":[{"id":"local-gguf"}]}`))
	}))
	defer llamaSrv.Close()
	cfg = defaultConfig()
	cfg.InferenceProvider = "llamacpp"
	cfg.EnvironmentVars = map[string]string{"LOCALCODE_LLAMACPP_URL": llamaSrv.URL}
	if err := state.preflightInferenceBackend(cfg, "local-gguf"); err != nil {
		t.Fatalf("healthy llamacpp preflight failed: %v", err)
	}
	llamaStatus = http.StatusInternalServerError
	if err := state.preflightInferenceBackend(cfg, "local-gguf"); err == nil {
		t.Fatal("unhealthy llamacpp preflight must fail")
	}
}

func TestHandleChatRunsSSEValidationAndOllamaCompleteBranches(t *testing.T) {
	state := newWave17TestState(t)
	state.Config.RootProjectDir = ""
	state.Project = ""
	srv := NewServer(state)

	// Validation branches on POST /api/chat/runs
	for _, tc := range []struct {
		method string
		body   string
		want   int
	}{
		{http.MethodGet, "", http.StatusMethodNotAllowed},
		{http.MethodPost, "{bad", http.StatusBadRequest},
		{http.MethodPost, `{"prompt":"   "}`, http.StatusBadRequest},
		{http.MethodPost, `{"prompt":"hello","project":""}`, http.StatusBadRequest},
	} {
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(tc.method, "http://127.0.0.1/api/chat/runs", strings.NewReader(tc.body)))
		if rr.Code != tc.want {
			t.Fatalf("%s /api/chat/runs (%s): status=%d want=%d body=%s", tc.method, tc.body, rr.Code, tc.want, rr.Body.String())
		}
	}

	// OllamaNativeProvider Complete (non-streaming) with tools, thinking, images & error response
	returnErr := false
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if returnErr {
			http.Error(w, "upstream model error", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-coder:14b",
			"message": map[string]any{
				"role":     "assistant",
				"content":  "complete ok",
				"thinking": "reasoning step",
				"tool_calls": []map[string]any{
					{"id": "oc_1", "function": map[string]any{"name": "read_file", "arguments": map[string]any{"path": "main.go"}}},
				},
			},
			"done":              true,
			"done_reason":       "stop",
			"prompt_eval_count": 25,
			"eval_count":        12,
		})
	}))
	defer mockOllama.Close()

	oc := NewOllamaClient()
	oc.BaseURL = mockOllama.URL
	prov := NewOllamaNativeProvider(oc)
	temp := 0.1
	resp, err := prov.Complete(context.Background(), InferenceRequest{
		Model:         "test-coder:14b",
		SystemPrompt:  "be brief",
		ContextLength: 8192,
		MaxTokens:     256,
		Temperature:   &temp,
		Tools:         []ToolDefinition{{Name: "read_file", Description: "read"}},
		Messages: []ProviderMessage{
			{Role: "user", Content: "check", Images: []string{"AQID"}},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "tc1", Name: "read_file", Arguments: json.RawMessage(`{"path":"x.go"}`)}}},
			{Role: "tool", ToolCallID: "tc1", ToolName: "read_file", Content: "ok"},
		},
	})
	if err != nil || resp.Content != "complete ok" || len(resp.ToolCalls) != 1 || resp.Thinking != "reasoning step" {
		t.Fatalf("OllamaNativeProvider.Complete err=%v resp=%+v", err, resp)
	}
	returnErr = true
	if _, err := prov.Complete(context.Background(), InferenceRequest{Model: "test-coder:14b", Messages: []ProviderMessage{{Role: "user", Content: "fail"}}}); err == nil {
		t.Fatal("400 from mock ollama must return error")
	}

	// RemoteServer handlePairStatus + handleUnpair branches
	rs := NewRemoteServer(state)
	rr := httptest.NewRecorder()
	rs.handlePairStatus(rr, httptest.NewRequest(http.MethodPost, "/remote/api/pair/status", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST pair status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	rs.handlePairStatus(rr, httptest.NewRequest(http.MethodGet, "/remote/api/pair/status", nil))
	if rr.Code != http.StatusBadRequest && rr.Code != http.StatusOK {
		t.Fatalf("GET pair status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	rs.handleUnpair(rr, httptest.NewRequest(http.MethodGet, "/remote/api/unpair", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET unpair status=%d", rr.Code)
	}

	// End-to-end POST /api/chat/runs SSE stream against mock Ollama returning immediate final answer
	proj := t.TempDir()
	sseOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/api/tags") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": "test-coder:14b"}},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/api/chat") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "test-coder:14b",
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"action":"final","message":"SSE completed cleanly"}`,
				},
				"done":        true,
				"done_reason": "stop",
			})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer sseOllama.Close()

	sseState := newWave17TestState(t)
	sseState.Config.OllamaURL = sseOllama.URL
	sseState.Config.OllamaDefaultModel = "test-coder:14b"
	sseState.Config.RootProjectDir = filepath.Dir(proj)
	sseState.Project = proj
	sseState.Ollama.BaseURL = sseOllama.URL
	sseSrv := NewServer(sseState)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reqBody, _ := json.Marshal(map[string]any{
		"prompt":  "say hello",
		"project": proj,
		"model":   "test-coder:14b",
		"turn_id": "turn-sse-1",
	})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/chat/runs", strings.NewReader(string(reqBody))).WithContext(ctx)
	rrSSE := httptest.NewRecorder()
	sseSrv.ServeHTTP(rrSSE, req)
	if rrSSE.Code != http.StatusOK || !strings.Contains(rrSSE.Body.String(), "run.started") {
		t.Fatalf("SSE chat runs status=%d body=%s", rrSSE.Code, rrSSE.Body.String())
	}
}

func TestEnsureRemoteTunnelHandleAgentActionRollbackAndMCPHeaders(t *testing.T) {
	// 1. ensureRemoteMobileTunnel missing binary + fake binary + reuse + cancel
	stopRemoteMobileTunnel()
	origResolver := cloudflaredBinaryResolver
	t.Cleanup(func() {
		cloudflaredBinaryResolver = origResolver
		stopRemoteMobileTunnel()
	})
	cloudflaredBinaryResolver = func() string { return "" }
	if _, err := ensureRemoteMobileTunnel(context.Background(), 32146); err == nil {
		t.Fatal("missing cloudflared binary must error")
	}

	tmpBinDir := t.TempDir()
	var fakeBin string
	if runtime.GOOS == "windows" {
		fakeBin = filepath.Join(tmpBinDir, "fake-cloudflared.cmd")
		script := "@echo off\r\necho https://ci-unit-4g.trycloudflare.com 1>&2\r\nping -n 3 127.0.0.1 >nul\r\n"
		_ = os.WriteFile(fakeBin, []byte(script), 0o700)
	} else {
		fakeBin = filepath.Join(tmpBinDir, "fake-cloudflared.sh")
		script := "#!/bin/sh\necho https://ci-unit-4g.trycloudflare.com >&2\nsleep 2\n"
		_ = os.WriteFile(fakeBin, []byte(script), 0o700)
	}
	cloudflaredBinaryResolver = func() string { return fakeBin }
	gotURL, err := ensureRemoteMobileTunnel(context.Background(), 0)
	if err != nil || gotURL != "https://ci-unit-4g.trycloudflare.com" {
		t.Fatalf("ensureRemoteMobileTunnel err=%v url=%q", err, gotURL)
	}
	// Cached hit
	if reused, err := ensureRemoteMobileTunnel(context.Background(), 32146); err != nil || reused != gotURL {
		t.Fatalf("cached tunnel reuse failed: url=%q err=%v", reused, err)
	}
	stopRemoteMobileTunnel()

	// Context cancellation timeout path with silent mock binary
	var silentBin string
	if runtime.GOOS == "windows" {
		silentBin = filepath.Join(tmpBinDir, "silent-cf.cmd")
		_ = os.WriteFile(silentBin, []byte("@echo off\r\nping -n 4 127.0.0.1 >nul\r\n"), 0o700)
	} else {
		silentBin = filepath.Join(tmpBinDir, "silent-cf.sh")
		_ = os.WriteFile(silentBin, []byte("#!/bin/sh\nsleep 3\n"), 0o700)
	}
	cloudflaredBinaryResolver = func() string { return silentBin }
	shortCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := ensureRemoteMobileTunnel(shortCtx, 32146); err == nil {
		t.Fatal("cancelled tunnel start must fail")
	}

	// 2. handleAgentAction broad dispatch coverage
	state := newWave17TestState(t)
	proj := t.TempDir()
	state.Config.ApprovalMode = "dangerous"
	state.Config.RootProjectDir = filepath.Dir(proj)
	state.Project = proj
	state.EnsurePTYManager().SetAllowedRoot(proj)
	_ = os.WriteFile(filepath.Join(proj, "demo.txt"), []byte("hello world\nsecond line\n"), 0o600)

	actions := []AgentAction{
		{Action: "project_info"},
		{Action: "command_list"},
		{Action: "command_read", Name: "missing-cmd"},
		{Action: "tool_inventory"},
		{Action: "list_files"},
		{Action: "read_file", Path: "demo.txt"},
		{Action: "search_text", Query: "hello"},
		{Action: "create_dir", Path: "sub"},
		{Action: "write_file", Path: "sub/new.txt", Content: "created\n"},
		{Action: "replace_text", Path: "sub/new.txt", OldText: "created", NewText: "updated"},
		{Action: "delete_file", Path: "sub/new.txt"},
		{Action: "pty_list"},
		{Action: "browser_inspect", Target: "#root"},
		{Action: "unknown_nonexistent_action"},
	}
	for _, act := range actions {
		_, _ = state.handleAgentAction(context.Background(), proj, act)
	}

	// 3. rollbackCommittedPatches covering moved (existing + new dest), modified & added files
	rbDir := t.TempDir()
	origFile := filepath.Join(rbDir, "orig.txt")
	movedDest := filepath.Join(rbDir, "moved.txt")
	movedExistingDest := filepath.Join(rbDir, "existing.txt")
	orig2 := filepath.Join(rbDir, "orig2.txt")
	addedFile := filepath.Join(rbDir, "added.txt")
	modFile := filepath.Join(rbDir, "mod.txt")
	_ = os.WriteFile(movedDest, []byte("new-moved"), 0o600)
	_ = os.WriteFile(movedExistingDest, []byte("overwritten"), 0o600)
	_ = os.WriteFile(addedFile, []byte("brand-new"), 0o600)
	_ = os.WriteFile(modFile, []byte("after-edit"), 0o600)

	if err := rollbackCommittedPatches([]committedFileSnapshot{
		{fullPath: modFile, originalExists: true, originalBytes: []byte("before-edit"), originalMode: 0o600},
		{fullPath: addedFile, originalExists: false},
		{fullPath: origFile, originalExists: true, originalBytes: []byte("src1"), originalMode: 0o600, moved: true, moveToFull: movedDest, moveDestExists: false},
		{fullPath: orig2, originalExists: true, originalBytes: []byte("src2"), originalMode: 0o600, moved: true, moveToFull: movedExistingDest, moveDestExists: true, moveDestBytes: []byte("old-dest"), moveDestMode: 0o600},
	}); err != nil {
		t.Fatalf("rollbackCommittedPatches: %v", err)
	}
	if raw, _ := os.ReadFile(modFile); string(raw) != "before-edit" {
		t.Fatalf("modFile not restored: %q", string(raw))
	}
	if _, err := os.Stat(addedFile); !os.IsNotExist(err) {
		t.Fatal("addedFile should have been removed on rollback")
	}
	if raw, _ := os.ReadFile(movedExistingDest); string(raw) != "old-dest" {
		t.Fatalf("movedExistingDest not restored: %q", string(raw))
	}

	// 4. resolveMCPHeaders with GitHub preset + AuthEnv
	t.Setenv("TEST_GH_TOKEN_ENV", "tok_test_value_12345")
	hdrs := resolveMCPHeaders(context.Background(), defaultConfig(), proj, MCPServerConfig{
		Name:    "github",
		Preset:  "github",
		AuthEnv: "TEST_GH_TOKEN_ENV",
		Headers: map[string]string{
			"X-Project":     "${PROJECT}",
			"Authorization": "Bearer ${TEST_GH_TOKEN_ENV}",
		},
	})
	if hdrs["Authorization"] != "Bearer tok_test_value_12345" {
		t.Fatalf("unexpected resolved MCP headers: %+v", hdrs)
	}
}
