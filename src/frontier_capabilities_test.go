// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFrontierTestState(t *testing.T, cfg Config) *AppState {
	t.Helper()
	st := NewAppState(cfg, newOfflineTestOllamaClient(t))
	t.Cleanup(func() { st.Close() })
	return st
}

func TestDiffHunkDecisionsEndpointPersistsAndValidates(t *testing.T) {
	ws := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = ws
	state := newFrontierTestState(t, cfg)
	srv := NewServer(state)

	// 1. Reject invalid status fail-closed
	badReq := httptest.NewRequest(http.MethodPost, "/api/diff/hunk", strings.NewReader(`{"diffId":"d1","hunkKey":"h1","status":"invalid"}`))
	badReq.Host = "127.0.0.1:32145"
	badRec := httptest.NewRecorder()
	srv.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 on invalid status, got %d", badRec.Code)
	}

	// 2. Accept valid hunk decision (camelCase from Studio)
	postReq := httptest.NewRequest(http.MethodPost, "/api/diff/hunk", strings.NewReader(`{"diffId":"diff-42","hunkKey":"file.go:1","status":"accepted","comment":"LGTM"}`))
	postReq.Host = "127.0.0.1:32145"
	postRec := httptest.NewRecorder()
	srv.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on POST /api/diff/hunk, got %d: %s", postRec.Code, postRec.Body.String())
	}

	// 3. Query persisted decisions via GET /api/diff/hunks
	getReq := httptest.NewRequest(http.MethodGet, "/api/diff/hunks?diff_id=diff-42", nil)
	getReq.Host = "127.0.0.1:32145"
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on GET /api/diff/hunks, got %d", getRec.Code)
	}
	var payload struct {
		OK        bool               `json:"ok"`
		Decisions []DiffHunkDecision `json:"decisions"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.OK || len(payload.Decisions) != 1 || payload.Decisions[0].Status != "accepted" || payload.Decisions[0].Comment != "LGTM" {
		t.Fatalf("unexpected persisted hunk decisions: %+v", payload)
	}
}

func TestChatRunsSSEEnforcesWorkspaceContainment(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := newFrontierTestState(t, cfg)
	srv := NewServer(state)

	// Escape attempt outside RootProjectDir must fail closed with 400
	body, _ := json.Marshal(map[string]any{
		"prompt":  "hello",
		"project": outside,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/chat/runs", bytes.NewReader(body))
	req.Host = "127.0.0.1:32145"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for outside project path, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRelayPairAndAttachPeerStartsLiveBridge(t *testing.T) {
	// Mock zero-knowledge relay endpoint that accepts poll requests cleanly
	mockRelay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer mockRelay.Close()

	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	state := newFrontierTestState(t, cfg)
	srv := NewServer(state)

	// 1. Issue pairing offer
	pairBody, _ := json.Marshal(map[string]string{"relay_url": mockRelay.URL})
	pairReq := httptest.NewRequest(http.MethodPost, "/api/relay/pair", bytes.NewReader(pairBody))
	pairReq.Host = "127.0.0.1:32145"
	pairRec := httptest.NewRecorder()
	srv.ServeHTTP(pairRec, pairReq)
	if pairRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /api/relay/pair, got %d: %s", pairRec.Code, pairRec.Body.String())
	}
	var pairResp struct {
		OK        bool   `json:"ok"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(pairRec.Body.Bytes(), &pairResp); err != nil || !pairResp.OK || pairResp.SessionID == "" {
		t.Fatalf("invalid pair response: %s", pairRec.Body.String())
	}

	// Verify private key is retained in memory (not discarded)
	state.mu.RLock()
	hasPriv := state.RelayPairing != nil && state.RelayPairing.privateKey != nil && len(state.RelayPairing.authSecret) > 0
	state.mu.RUnlock()
	if !hasPriv {
		t.Fatal("expected ephemeral private key and authSecret to be retained in memory")
	}

	// 2. Attach mobile peer public key & verify live bridge starts and stops cleanly
	mobileKP, err := GenerateRelayKeyPair()
	if err != nil {
		t.Fatalf("generate mobile keypair: %v", err)
	}
	attachBody, _ := json.Marshal(map[string]string{
		"session_id":      pairResp.SessionID,
		"peer_public_b64": base64.RawURLEncoding.EncodeToString(mobileKP.Public.Bytes()),
	})
	attachReq := httptest.NewRequest(http.MethodPost, "/api/relay/attach-peer", bytes.NewReader(attachBody))
	attachReq.Host = "127.0.0.1:32145"
	attachRec := httptest.NewRecorder()
	srv.ServeHTTP(attachRec, attachReq)
	if attachRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /api/relay/attach-peer, got %d: %s", attachRec.Code, attachRec.Body.String())
	}

	state.mu.Lock()
	tunnelLive := state.RelayPairing != nil && state.RelayPairing.TunnelLive && state.RelayPairing.bridge != nil
	if state.RelayPairing != nil && state.RelayPairing.bridge != nil {
		state.RelayPairing.bridge.Stop()
	}
	state.mu.Unlock()
	if !tunnelLive {
		t.Fatal("expected live RelayWorkstationBridge after attach-peer")
	}
}

func TestFrontierRepoMapAndPostEditDiagnostics(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "app.py"), []byte("def compute_sum(a, b):\n    return a + b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "bad.json"), []byte(`{"broken": `), 0o644); err != nil {
		t.Fatal(err)
	}

	repoMap := BuildFrontierRepoMap(ws, 10, 4000)
	if !strings.Contains(repoMap, "app.py") || !strings.Contains(repoMap, "compute_sum") {
		t.Fatalf("expected repo map to contain app.py and compute_sum, got:\n%s", repoMap)
	}

	diag := RunPostEditDiagnostics(context.Background(), ws, "bad.json")
	if !strings.Contains(diag, "JSON syntax error") {
		t.Fatalf("expected JSON syntax diagnostic, got: %q", diag)
	}

	cleanDiag := RunPostEditDiagnostics(context.Background(), ws, "app.py")
	if cleanDiag != "" {
		t.Fatalf("expected clean diagnostic for valid app.py, got: %q", cleanDiag)
	}
}

func TestNativeMultiToolParallelQueueAndTextWithToolCalls(t *testing.T) {
	// Mock OpenAI-compatible server that returns explanatory text + 2 parallel tool calls + usage
	callCount := 0
	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if _, hasSchema := reqBody["response_format"]; hasSchema {
			t.Errorf("expected response_format/JSONSchema NOT to be sent alongside native tools")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		chunk := map[string]any{
			"id": "chatcmpl-1",
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"content": "Let me inspect two files in parallel.",
						"tool_calls": []map[string]any{
							{
								"index": 0,
								"id":    "call_1",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": `{"path":"a.txt","message":"Read a"}`,
								},
							},
							{
								"index": 1,
								"id":    "call_2",
								"type":  "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": `{"path":"b.txt","message":"Read b"}`,
								},
							},
						},
					},
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     120,
				"completion_tokens": 35,
				"total_tokens":      155,
			},
		}
		raw, _ := json.Marshal(chunk)
		_, _ = w.Write([]byte("data: " + string(raw) + "\n\ndata: [DONE]\n\n"))
	}))
	defer mockOpenAI.Close()

	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	cfg.InferenceProvider = "openai"
	cfg.EnvironmentVars = map[string]string{
		"OPENAI_API_KEY":  "sk-test-local-mock-key-12345",
		"OPENAI_BASE_URL": mockOpenAI.URL,
	}
	state := newFrontierTestState(t, cfg)

	act1, _, err := state.nextAgentAction(context.Background(), "gpt-4o", []OllamaMessage{{Role: "user", Content: "Read a and b"}})
	if err != nil {
		t.Fatalf("first action failed: %v", err)
	}
	if act1.Action != "read_file" || act1.Path != "a.txt" {
		t.Fatalf("expected first parallel tool call read_file a.txt, got %+v", act1)
	}

	// Second call must drain queued call_2 WITHOUT making a second HTTP request!
	act2, _, err := state.nextAgentAction(context.Background(), "gpt-4o", []OllamaMessage{{Role: "user", Content: "continue"}})
	if err != nil {
		t.Fatalf("second queued parallel action failed: %v", err)
	}
	if act2.Action != "read_file" || act2.Path != "b.txt" {
		t.Fatalf("expected second parallel tool call read_file b.txt, got %+v", act2)
	}
	if callCount != 1 {
		t.Fatalf("expected exactly 1 provider call for 2 parallel tool calls, got %d", callCount)
	}

	state.mu.RLock()
	usage := state.LiveUsage
	state.mu.RUnlock()
	if usage.TotalTokens != 155 || usage.InputTokens != 120 {
		t.Fatalf("expected accumulated usage tokens (120/35/155), got %+v", usage)
	}
}

func TestMicaFFIInProcessThreatClassification(t *testing.T) {
	res := ClassifyThreatWithMICA("Read src/server.go and explain the health route.")
	if !res.Available {
		t.Skipf("mica_ffi.dll not available in this environment: %s", res.Error)
	}
	if !res.IsClean {
		t.Fatalf("expected benign prompt to be clean, got threat_code=%d", res.ThreatCode)
	}
	if res.Version == "" || res.SHA256 == "" {
		t.Fatalf("expected MICA version and SHA256 populated, got %+v", res)
	}
}
