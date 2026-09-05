// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCoverageBoostHelpers(t *testing.T) {
	// 1. desktopMissionRecoveryDebugString
	snap := DesktopMissionRecoverySnapshot{
		RunID:               "r-test-1",
		MissionID:           "m-test-1",
		ReconciliationState: "matched",
		Tasks:               []DesktopMissionRecoveryTask{{TaskID: "t-1"}},
	}
	debugStr := desktopMissionRecoveryDebugString(snap)
	if !strings.Contains(debugStr, "r-test-1") || !strings.Contains(debugStr, "m-test-1") {
		t.Fatalf("unexpected debugStr: %s", debugStr)
	}

	// 2. agentImmediateRepeat helpers
	cfg := defaultConfig()
	cfg.Language = "de"
	msgDe := agentImmediateRepeatMessage(cfg)
	if !strings.Contains(msgDe, "Identische") {
		t.Fatalf("unexpected msgDe: %s", msgDe)
	}
	cfg.Language = "en"
	msgEn := agentImmediateRepeatMessage(cfg)
	if !strings.Contains(msgEn, "Identical") {
		t.Fatalf("unexpected msgEn: %s", msgEn)
	}

	act := AgentAction{Action: "read_file", Path: "main.go"}
	detail := agentImmediateRepeatDetail(cfg, act)
	if !strings.Contains(detail, "read_file") {
		t.Fatalf("unexpected detail: %s", detail)
	}

	hint1 := agentImmediateRepeatHint(cfg, 1)
	hint2 := agentImmediateRepeatHint(cfg, 2)
	if !strings.Contains(hint1, "SYSTEM NOTICE") || !strings.Contains(hint2, "Do not ask") {
		t.Fatalf("unexpected hints: hint1=%s hint2=%s", hint1, hint2)
	}

	// 3. agentLoopBlockDetail
	detailCycle := agentLoopBlockDetail(cfg, agentLoopBlockCycle, act)
	detailFail := agentLoopBlockDetail(cfg, agentLoopBlockRepeatedFailure, act)
	detailOutcome := agentLoopBlockDetail(cfg, agentLoopBlockRepeatedOutcome, act)
	if detailCycle == "" || detailFail == "" || detailOutcome == "" {
		t.Fatal("empty loop block detail")
	}

	// 4. StagnationTracker Reset
	tracker := &StagnationTracker{fingerprints: map[string][]string{}}
	tracker.RecordAttempt("task-1", "err-1")
	tracker.Reset("task-1")
	if count, _ := tracker.RecordAttempt("task-1", "err-2"); count != 1 {
		t.Fatalf("expected count 1 after Reset, got %d", count)
	}

	// 5. escapePowerShellString
	escaped := escapePowerShellString(`echo "hello world"`)
	if escaped != `echo \"hello world\"` {
		t.Fatalf("unexpected escaped string: %s", escaped)
	}

	// 6. worktreeRegistry get
	reg := &worktreeRegistry{worktrees: map[string]*AgentWorktreeWorkspace{}}
	wt := &AgentWorktreeWorkspace{
		MainProject:  "C:/Projects/Test",
		WorktreePath: "C:/Projects/Test/.worktrees/wt-1",
		BranchName:   "feature/wt-1",
		TaskID:       "task-123",
		Active:       true,
	}
	reg.register(wt)
	gotWt, ok := reg.get("C:/Projects/Test", "task-123")
	if !ok || gotWt.BranchName != "feature/wt-1" {
		t.Fatalf("worktreeRegistry.get failed: got=%#v ok=%v", gotWt, ok)
	}
	reg.unregister(wt)
	_, okAfter := reg.get("C:/Projects/Test", "task-123")
	if okAfter {
		t.Fatal("expected worktree to be unregistered")
	}

	// 7. LlamaCppBackend BaseURL and type
	backend := NewLlamaCppBackend("http://localhost:8080", "secret-token")
	if backend.BaseURL() != "http://localhost:8080" {
		t.Fatalf("BaseURL=%s want http://localhost:8080", backend.BaseURL())
	}
	if backend.BackendType() != InferenceBackendLlamaCpp {
		t.Fatalf("BackendType=%s want llama.cpp", backend.BackendType())
	}
}

func TestLoopbackAndTrustedBrowserOrigin(t *testing.T) {
	// loopbackRequestHost
	if !loopbackRequestHost("localhost") || !loopbackRequestHost("localhost:8080") || !loopbackRequestHost("127.0.0.1:9000") || !loopbackRequestHost("::1") || !loopbackRequestHost("[::1]:80") {
		t.Error("loopbackRequestHost failed on valid loopback hosts")
	}
	if loopbackRequestHost("") || loopbackRequestHost("example.com") || loopbackRequestHost("192.168.1.100") {
		t.Error("loopbackRequestHost allowed non-loopback hosts")
	}

	// trustedBrowserOrigin
	if !trustedBrowserOrigin("") || !trustedBrowserOrigin("http://localhost:8080") || !trustedBrowserOrigin("http://127.0.0.1:3000") {
		t.Error("trustedBrowserOrigin rejected valid origins")
	}
	if trustedBrowserOrigin("null") || trustedBrowserOrigin("http://evil.com") || trustedBrowserOrigin("https://example.com:8080") || trustedBrowserOrigin("invalid-scheme://localhost") {
		t.Error("trustedBrowserOrigin allowed untrusted origins")
	}
}

func TestServerBasicEndpointsDirect(t *testing.T) {
	cfg := defaultConfig()
	appState := &AppState{
		Config: cfg,
		Ollama: &OllamaClient{BaseURL: "http://127.0.0.1:11434"},
	}
	server := NewServer(appState)

	// handlePing
	{
		req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
		w := httptest.NewRecorder()
		server.handlePing(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handlePing GET code=%d want 200", w.Code)
		}

		postReq := httptest.NewRequest(http.MethodPost, "/api/ping", nil)
		postW := httptest.NewRecorder()
		server.handlePing(postW, postReq)
		if postW.Code != http.StatusMethodNotAllowed {
			t.Fatalf("handlePing POST code=%d want 405", postW.Code)
		}
	}

	// handleProjects
	{
		req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
		w := httptest.NewRecorder()
		server.handleProjects(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleProjects GET code=%d want 200", w.Code)
		}

		postReq := httptest.NewRequest(http.MethodPost, "/api/projects", nil)
		postW := httptest.NewRecorder()
		server.handleProjects(postW, postReq)
		if postW.Code != http.StatusMethodNotAllowed {
			t.Fatalf("handleProjects POST code=%d want 405", postW.Code)
		}
	}

	// handleDoctor
	{
		req := httptest.NewRequest(http.MethodGet, "/api/doctor", nil)
		w := httptest.NewRecorder()
		server.handleDoctor(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleDoctor GET code=%d want 200", w.Code)
		}

		postReq := httptest.NewRequest(http.MethodPost, "/api/doctor", nil)
		postW := httptest.NewRecorder()
		server.handleDoctor(postW, postReq)
		if postW.Code != http.StatusMethodNotAllowed {
			t.Fatalf("handleDoctor POST code=%d want 405", postW.Code)
		}
	}

	// handleCodingEngineStatus
	{
		req := httptest.NewRequest(http.MethodGet, "/api/engines/status", nil)
		w := httptest.NewRecorder()
		server.handleCodingEngineStatus(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleCodingEngineStatus GET code=%d want 200", w.Code)
		}
	}

	// handleTools & handleToolDiagnose
	{
		req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
		w := httptest.NewRecorder()
		server.handleTools(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleTools GET code=%d want 200", w.Code)
		}

		diagReq := httptest.NewRequest(http.MethodPost, "/api/tools/diagnose", strings.NewReader(`{"tool":"git"}`))
		diagReq.Header.Set("Content-Type", "application/json")
		diagW := httptest.NewRecorder()
		server.handleToolDiagnose(diagW, diagReq)
		if diagW.Code != http.StatusOK {
			t.Fatalf("handleToolDiagnose POST code=%d want 200", diagW.Code)
		}
	}

	// handleComputeMeshStatus & handleComputeMeshTest
	{
		req := httptest.NewRequest(http.MethodGet, "/api/computemesh/status", nil)
		w := httptest.NewRecorder()
		server.handleComputeMeshStatus(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleComputeMeshStatus GET code=%d want 200", w.Code)
		}

		testReq := httptest.NewRequest(http.MethodPost, "/api/computemesh/test", strings.NewReader(`{"url":"http://127.0.0.1:11434"}`))
		testReq.Header.Set("Content-Type", "application/json")
		testW := httptest.NewRecorder()
		server.handleComputeMeshTest(testW, testReq)
		if testW.Code != http.StatusOK {
			t.Fatalf("handleComputeMeshTest POST code=%d want 200", testW.Code)
		}
	}

	// handleADBDevices
	{
		req := httptest.NewRequest(http.MethodGet, "/api/adb/devices", nil)
		w := httptest.NewRecorder()
		server.handleADBDevices(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleADBDevices GET code=%d want 200", w.Code)
		}
	}
}

func TestServerChatEndpointsDirect(t *testing.T) {
	cfg := defaultConfig()
	appState := &AppState{
		Config:  cfg,
		Threads: map[string]*ChatThread{},
	}
	server := NewServer(appState)

	// 1. handleThreads
	{
		req := httptest.NewRequest(http.MethodGet, "/api/threads", nil)
		w := httptest.NewRecorder()
		server.handleThreads(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleThreads GET code=%d want 200", w.Code)
		}
	}

	// 2. handleNewChat
	var threadID string
	{
		req := httptest.NewRequest(http.MethodPost, "/api/new-chat", strings.NewReader(`{"project":"my-proj"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		server.handleNewChat(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("handleNewChat POST code=%d want 200", w.Code)
		}
		appState.mu.RLock()
		threadID = appState.CurrentThread
		appState.mu.RUnlock()
	}

	if threadID != "" {
		// 3. handleRenameChat
		{
			req := httptest.NewRequest(http.MethodPost, "/api/rename-chat", strings.NewReader(`{"id":"`+threadID+`","title":"Renamed Thread"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.handleRenameChat(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("handleRenameChat POST code=%d want 200", w.Code)
			}
		}

		// 4. handleArchiveChat
		{
			req := httptest.NewRequest(http.MethodPost, "/api/archive-chat", strings.NewReader(`{"id":"`+threadID+`","archived":true}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.handleArchiveChat(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("handleArchiveChat POST code=%d want 200", w.Code)
			}
		}

		// 5. handleSelectChat
		{
			req := httptest.NewRequest(http.MethodPost, "/api/select-chat", strings.NewReader(`{"id":"`+threadID+`"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.handleSelectChat(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("handleSelectChat POST code=%d want 200", w.Code)
			}
		}

		// 6. handleDuplicateChat
		{
			req := httptest.NewRequest(http.MethodPost, "/api/duplicate-chat", strings.NewReader(`{"id":"`+threadID+`"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.handleDuplicateChat(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("handleDuplicateChat POST code=%d want 200", w.Code)
			}
		}

		// 7. handleDeleteChat
		{
			req := httptest.NewRequest(http.MethodPost, "/api/delete-chat", strings.NewReader(`{"id":"`+threadID+`"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			server.handleDeleteChat(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("handleDeleteChat POST code=%d want 200", w.Code)
			}
		}
	}
}

func TestComputeMeshAutoDetectHandler(t *testing.T) {
	appState := &AppState{}
	server := httptest.NewServer(NewServer(appState))
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/computemesh/autodetect", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", server.URL)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("handleComputeMeshAutoDetect status=%d", resp.StatusCode)
	}

	// Method not allowed check
	getReq, _ := http.NewRequest(http.MethodGet, server.URL+"/api/computemesh/autodetect", nil)
	getResp, err := server.Client().Do(getReq)
	if err == nil {
		_ = getResp.Body.Close()
		if getResp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected StatusMethodNotAllowed for GET, got: %d", getResp.StatusCode)
		}
	}
}

func TestMissionRecoveryHandlersDirect(t *testing.T) {
	appState := &AppState{}
	server := NewServer(appState)

	// GET /api/mission-recovery (returns 204 when no snapshot available)
	req := httptest.NewRequest(http.MethodGet, "/api/mission-recovery", nil)
	w := httptest.NewRecorder()
	server.handleMissionRecovery(w, req)
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Fatalf("handleMissionRecovery code=%d want 204 or 200", w.Code)
	}

	// POST /api/mission-recovery/continue (empty/invalid body returns 400)
	postReq := httptest.NewRequest(http.MethodPost, "/api/mission-recovery/continue", strings.NewReader("{}"))
	postReq.Header.Set("Content-Type", "application/json")
	postW := httptest.NewRecorder()
	server.handleMissionRecoveryContinue(postW, postReq)
	if postW.Code != http.StatusBadRequest {
		t.Fatalf("handleMissionRecoveryContinue code=%d want 400", postW.Code)
	}
}

func TestInferenceBackendLlamaCppHealthFallback(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	backend := NewLlamaCppBackend(mockServer.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, _, err := backend.Health(ctx)
	if err != nil || !ok {
		t.Fatalf("Health check failed: ok=%v err=%v", ok, err)
	}

	models, err := backend.Tags(ctx)
	if err != nil || len(models) == 0 {
		t.Fatalf("Tags check failed: models=%v err=%v", models, err)
	}
}
