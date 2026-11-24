// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestServerChatRunsSSECompactStream(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.Language = "en"

	state := NewAppState(cfg, newOfflineTestOllamaClient(t))
	defer state.Close()

	srv := httptest.NewServer(NewServer(state))
	defer srv.Close()

	body := strings.NewReader(`{"schema":"chat.run.events.v1","prompt":"/compact SSE live check","turn_id":"turn-sse-1"}`)
	resp, err := srv.Client().Post(srv.URL+"/api/chat/runs", "application/json", body)
	if err != nil {
		t.Fatalf("POST /api/chat/runs failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d want 200 (body=%s)", resp.StatusCode, raw)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type=%q want text/event-stream", ct)
	}

	var events []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			events = append(events, strings.TrimSpace(strings.TrimPrefix(line, "event:")))
		}
	}
	joined := strings.Join(events, ",")
	for _, want := range []string{"run.started", "answer.final", "run.completed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing SSE event %q in stream [%s]", want, joined)
		}
	}
}

func TestExecuteCommandOSSandboxLiveWiring(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.OSSandboxEnabled = true
	cfg.NetworkEnabled = false
	cfg.CommandTimeout = 30

	t.Setenv("OPENAI_API_KEY", "sk-secret-must-be-stripped-in-sandbox")

	// Harmless command succeeds inside Win32 Job Object / Unix OS sandbox and strips secrets.
	out, err := executeCommand(context.Background(), project, "echo SANDBOX_WIRED_OK", cfg)
	if err != nil {
		t.Fatalf("executeCommand harmless failed: %v (out=%s)", err, out)
	}
	if !strings.Contains(out, "SANDBOX_WIRED_OK") || !strings.Contains(out, "audit=") {
		t.Fatalf("unexpected sandbox output: %s", out)
	}

	// Secret env var must not leak into child process environment.
	outEnv, errEnv := executeCommand(context.Background(), project, "echo %OPENAI_API_KEY% $env:OPENAI_API_KEY", cfg)
	if errEnv == nil && strings.Contains(outEnv, "sk-secret-must-be-stripped-in-sandbox") {
		t.Fatalf("secret API key leaked inside sandboxed executeCommand: %s", outEnv)
	}

	// Protected credential path (~/.ssh/id_rsa) must be blocked fail-closed.
	_, errSSH := executeCommand(context.Background(), project, "type ~/.ssh/id_rsa", cfg)
	if errSSH == nil {
		t.Fatalf("expected sandboxed executeCommand to block ~/.ssh/id_rsa")
	}
}

func TestServerProvidersStatusNoSecretLeak(t *testing.T) {
	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	cfg.InferenceProvider = "anthropic"
	cfg.PromptCachingEnabled = true
	cfg.ReasoningEffort = "high"

	const sentinelAnthropic = "sk-ant-test-secret-never-return-999"
	t.Setenv("ANTHROPIC_API_KEY", sentinelAnthropic)

	state := NewAppState(cfg, newOfflineTestOllamaClient(t))
	defer state.Close()

	srv := httptest.NewServer(NewServer(state))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/api/providers/status")
	if err != nil {
		t.Fatalf("GET /api/providers/status failed: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
	bodyStr := string(raw)
	if strings.Contains(bodyStr, sentinelAnthropic) {
		t.Fatalf("secret API key leaked in /api/providers/status response!")
	}

	var parsed struct {
		ActiveProvider        string          `json:"active_provider"`
		SupportsNativeTools   bool            `json:"supports_native_tools"`
		SupportsPromptCaching bool            `json:"supports_prompt_caching"`
		ConfiguredMode        string          `json:"configured_mode"`
		KeyConfigured         map[string]bool `json:"key_configured"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if parsed.ActiveProvider != "anthropic" || !parsed.SupportsNativeTools || !parsed.SupportsPromptCaching {
		t.Fatalf("unexpected provider status: %+v", parsed)
	}
	if !parsed.KeyConfigured["anthropic"] {
		t.Fatalf("expected key_configured.anthropic=true")
	}
}

func TestRelayPairAndLockscreenApproveEndToEnd(t *testing.T) {
	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	state := NewAppState(cfg, newOfflineTestOllamaClient(t))
	defer state.Close()

	desktopSrv := httptest.NewServer(NewServer(state))
	defer desktopSrv.Close()

	remoteSrv := httptest.NewServer(NewRemoteServer(state))
	defer remoteSrv.Close()

	// 1. Pair E2EE Relay & query status (public metadata only)
	pairResp, err := desktopSrv.Client().Post(desktopSrv.URL+"/api/relay/pair", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST /api/relay/pair: %v", err)
	}
	pairBody, _ := io.ReadAll(pairResp.Body)
	_ = pairResp.Body.Close()
	if pairResp.StatusCode != http.StatusOK {
		t.Fatalf("/api/relay/pair status=%d body=%s", pairResp.StatusCode, pairBody)
	}
	if !strings.Contains(string(pairBody), "localcode://relay-pair") || !strings.Contains(string(pairBody), "fingerprint") {
		t.Fatalf("unexpected relay pair payload: %s", pairBody)
	}

	statusResp, err := desktopSrv.Client().Get(desktopSrv.URL + "/api/relay/status")
	if err != nil {
		t.Fatalf("GET /api/relay/status: %v", err)
	}
	statusBody, _ := io.ReadAll(statusResp.Body)
	_ = statusResp.Body.Close()
	if !strings.Contains(string(statusBody), `"active":true`) {
		t.Fatalf("expected active relay session, got: %s", statusBody)
	}

	// 2. Reject "global" lockscreen token issuance fail-closed
	badTokResp, err := desktopSrv.Client().Post(
		desktopSrv.URL+"/api/push/lockscreen-token",
		"application/json",
		strings.NewReader(`{"approval_id":"ap-lock-1","decision":"global","device_id":"dev-1"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = badTokResp.Body.Close()
	if badTokResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("global lockscreen token status=%d want 400", badTokResp.StatusCode)
	}

	// 3. Issue valid single-use "once" lockscreen token
	tokResp, err := desktopSrv.Client().Post(
		desktopSrv.URL+"/api/push/lockscreen-token",
		"application/json",
		strings.NewReader(`{"approval_id":"ap-lock-1","decision":"once","device_id":"dev-1"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	tokBody, _ := io.ReadAll(tokResp.Body)
	_ = tokResp.Body.Close()
	if tokResp.StatusCode != http.StatusOK {
		t.Fatalf("issue token status=%d body=%s", tokResp.StatusCode, tokBody)
	}
	var tokParsed struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(tokBody, &tokParsed); err != nil || tokParsed.Token == "" {
		t.Fatalf("invalid token payload: %s", tokBody)
	}

	// 4. Create paired mobile bearer token & pending approval
	bearerRaw := make([]byte, 32)
	_, _ = rand.Read(bearerRaw)
	bearer := "tok-" + hex.EncodeToString(bearerRaw)
	now := time.Now()
	state.mu.Lock()
	state.Config.RemoteDevices = []RemoteDevice{{
		ID:         "dev-1",
		Name:       "Pixel Lockscreen",
		TokenHash:  remoteTokenHash(bearer),
		PairedAt:   now,
		LastSeenAt: now,
	}}
	pending := &PendingAction{
		ID:     "ap-lock-1",
		Result: make(chan ApprovalDecision, 1),
	}
	state.Pending = pending
	state.mu.Unlock()

	postApprove := func() *http.Response {
		reqBody, _ := json.Marshal(map[string]string{
			"token":       tokParsed.Token,
			"approval_id": "ap-lock-1",
			"device_id":   "dev-1",
		})
		req, _ := http.NewRequest(http.MethodPost, remoteSrv.URL+"/remote/api/lockscreen-approve", strings.NewReader(string(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := remoteSrv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	r1 := postApprove()
	b1, _ := io.ReadAll(r1.Body)
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusOK || !strings.Contains(string(b1), `"persist":false`) {
		t.Fatalf("lockscreen approve status=%d body=%s", r1.StatusCode, b1)
	}

	dec := <-pending.Result
	if !dec.Approved || dec.Persist {
		t.Fatalf("unexpected ApprovalDecision: %+v", dec)
	}

	// 5. Replaying same token must fail closed with 403 Forbidden
	r2 := postApprove()
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusForbidden {
		t.Fatalf("replay lockscreen token status=%d want 403", r2.StatusCode)
	}
}

func TestRewindSlashCommandRestoresCheckpoint(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	project := filepath.Join(root, "demo-rewind")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(project, "app.txt")
	if err := os.WriteFile(targetFile, []byte("original-state-v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = project
	cfg.Language = "en"

	state := NewAppState(cfg, newOfflineTestOllamaClient(t))
	defer state.Close()

	th := newThread(project, "qwen2.5-coder:14b")
	state.mu.Lock()
	state.Project = project
	state.Threads[th.ID] = th
	state.CurrentThread = th.ID
	state.mu.Unlock()

	cp, err := state.CreateTurnCheckpoint(th.ID, project, "Edit app.txt")
	if err != nil || cp == nil {
		t.Fatalf("CreateTurnCheckpoint failed: cp=%v err=%v", cp, err)
	}
	state.AddEvent(UIEvent{Type: "user", Message: "Edit app.txt", CheckpointID: cp.ID})

	if err := os.WriteFile(targetFile, []byte("mutated-state-v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := state.StartAgentForThread("/rewind", "qwen2.5-coder:14b", nil, project, th.ID); err != nil {
		t.Fatalf("StartAgentForThread(/rewind) failed: %v", err)
	}

	gotBytes, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotBytes) != "original-state-v1" {
		t.Fatalf("file content after /rewind = %q want %q", string(gotBytes), "original-state-v1")
	}

	state.mu.RLock()
	running := state.Running
	activeRuns := len(state.ActiveRuns)
	lastSummary := state.LastSummary
	state.mu.RUnlock()
	if running || activeRuns != 0 {
		t.Fatalf("expected idle state after /rewind, got running=%v activeRuns=%d", running, activeRuns)
	}
	if !strings.Contains(lastSummary, cp.ID) {
		t.Fatalf("expected /rewind summary to mention checkpoint %s, got %q", cp.ID, lastSummary)
	}
}

func TestRemoteSteerAndProvidersStatusEndpoints(t *testing.T) {
	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	cfg.InferenceProvider = "openai"
	const sentinelKey = "sk-proj-never-leak-remote-999"
	t.Setenv("OPENAI_API_KEY", sentinelKey)

	state := NewAppState(cfg, newOfflineTestOllamaClient(t))
	defer state.Close()

	bearerRaw := make([]byte, 32)
	_, _ = rand.Read(bearerRaw)
	bearer := "tok-" + hex.EncodeToString(bearerRaw)
	now := time.Now()
	th := newThread(cfg.RootProjectDir, "gpt-5")
	state.mu.Lock()
	state.Config.RemoteDevices = []RemoteDevice{{
		ID:         "dev-steer-1",
		Name:       "Galaxy S25",
		TokenHash:  remoteTokenHash(bearer),
		PairedAt:   now,
		LastSeenAt: now,
	}}
	state.Threads[th.ID] = th
	state.CurrentThread = th.ID
	state.Running = true
	state.RunID = "run-steer-1"
	state.RunPhase = "running"
	state.mu.Unlock()
	state.beginAgentSteering("run-steer-1")

	remoteSrv := httptest.NewServer(NewRemoteServer(state))
	defer remoteSrv.Close()

	// 1. GET /remote/api/providers-status (no secret leak)
	provReq, _ := http.NewRequest(http.MethodGet, remoteSrv.URL+"/remote/api/providers-status", nil)
	provReq.Header.Set("Authorization", "Bearer "+bearer)
	provResp, err := remoteSrv.Client().Do(provReq)
	if err != nil {
		t.Fatal(err)
	}
	provBody, _ := io.ReadAll(provResp.Body)
	_ = provResp.Body.Close()
	if provResp.StatusCode != http.StatusOK || !strings.Contains(string(provBody), `"active_provider":"openai"`) {
		t.Fatalf("GET /remote/api/providers-status status=%d body=%s", provResp.StatusCode, provBody)
	}
	if strings.Contains(string(provBody), sentinelKey) {
		t.Fatal("/remote/api/providers-status leaked raw API key")
	}

	// 2. POST /remote/api/steer during active run -> 202 Accepted & queued in mailbox
	steerReq, _ := http.NewRequest(http.MethodPost, remoteSrv.URL+"/remote/api/steer", strings.NewReader(`{"message":"Switch to table-driven tests"}`))
	steerReq.Header.Set("Content-Type", "application/json")
	steerReq.Header.Set("Authorization", "Bearer "+bearer)
	steerResp, err := remoteSrv.Client().Do(steerReq)
	if err != nil {
		t.Fatal(err)
	}
	steerBody, _ := io.ReadAll(steerResp.Body)
	_ = steerResp.Body.Close()
	if steerResp.StatusCode != http.StatusAccepted || !strings.Contains(string(steerBody), `"state":"queued"`) {
		t.Fatalf("POST /remote/api/steer status=%d body=%s", steerResp.StatusCode, steerBody)
	}

	drained := state.drainAgentSteering("run-steer-1")
	if len(drained) != 1 || drained[0].Message != "Switch to table-driven tests" {
		t.Fatalf("unexpected drained steering messages: %+v", drained)
	}
}

func TestHooksV2PreToolUseVetoInAgentLoop(t *testing.T) {
	project := t.TempDir()
	hookDir := filepath.Join(project, ".localcode")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vetoCmd := "echo 'policy veto: write_file blocked' >&2; exit 2"
	if runtime.GOOS == "windows" {
		vetoCmd = "[Console]::Error.WriteLine('policy veto: write_file blocked'); exit 2"
	}
	hooksJSON, err := json.Marshal(map[string]any{
		"hooks": []HookDefinition{{
			Name:        "block-writes",
			Event:       HookEventPreToolUse,
			Type:        "command",
			Command:     vetoCmd,
			ToolMatcher: "write_file",
			TimeoutSec:  10,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), hooksJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	defs, err := LoadProjectHooksV2(project)
	if err != nil || len(defs) != 1 {
		t.Fatalf("LoadProjectHooksV2 defs=%+v err=%v", defs, err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	_, runErr := RunHooksV2(context.Background(), project, cfg, defs, HookPayload{
		Event:   HookEventPreToolUse,
		Project: project,
		Action:  AgentAction{Action: "write_file", Path: "blocked.txt", Message: "write"},
	})
	if _, isVeto := IsHookVetoError(runErr); !isVeto {
		t.Fatalf("expected *HookVetoError from PreToolUse exit 2, got %v", runErr)
	}
}

func TestAgentLoopRoutesThroughNativeFrontierProviderAndTranslatesToolCalls(t *testing.T) {
	var seenCacheControl bool
	var seenNativeTools bool
	anthropicMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		if strings.Contains(bodyStr, `"cache_control"`) {
			seenCacheControl = true
		}
		if strings.Contains(bodyStr, `"tools"`) && strings.Contains(bodyStr, `"apply_patch"`) {
			seenNativeTools = true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-opus-4-1\",\"usage\":{\"input_tokens\":42}}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"read_file\",\"input\":{}}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\\\"README.md\\\",\\\"message\\\":\\\"Inspect README\\\"}\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		_, _ = io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":12}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer anthropicMock.Close()

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-local-test-key")
	t.Setenv("ANTHROPIC_BASE_URL", anthropicMock.URL)

	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	cfg.InferenceProvider = "anthropic"
	cfg.PromptCachingEnabled = true
	cfg.LastModel = "claude-opus-4-1"

	state := NewAppState(cfg, newOfflineTestOllamaClient(t))
	defer state.Close()

	action, usedModel, err := state.nextAgentAction(context.Background(), "claude-opus-4-1", []OllamaMessage{
		{Role: "system", Content: "You are a coding assistant."},
		{Role: "user", Content: "Read README.md"},
	})
	if err != nil {
		t.Fatalf("nextAgentAction via Anthropic provider failed: %v", err)
	}
	if usedModel != "claude-opus-4-1" {
		t.Fatalf("usedModel=%q want claude-opus-4-1", usedModel)
	}
	if action.Action != "read_file" || action.Path != "README.md" {
		t.Fatalf("translated native tool call mismatch: %+v", action)
	}
	if !seenCacheControl {
		t.Fatalf("expected Anthropic request to include prompt caching cache_control")
	}
	if !seenNativeTools {
		t.Fatalf("expected Anthropic request to include native tool definitions (incl. apply_patch)")
	}
}

func TestVerifyFrameEndpointsDetectBlackScreenAndPassRichUI(t *testing.T) {
	cfg := defaultConfig()
	cfg.RootProjectDir = t.TempDir()
	state := NewAppState(cfg, newOfflineTestOllamaClient(t))
	defer state.Close()

	srv := NewServer(state)

	// 1. Pure black 32x32 PNG -> fails with BLANK_OR_BLACK_SCREEN
	var blackBuf bytes.Buffer
	blackImg := image.NewRGBA(image.Rect(0, 0, 32, 32))
	if err := png.Encode(&blackBuf, blackImg); err != nil {
		t.Fatal(err)
	}
	blackReqPayload, _ := json.Marshal(map[string]string{
		"image_base64": "data:image/png;base64," + base64.StdEncoding.EncodeToString(blackBuf.Bytes()),
	})
	recBlack := httptest.NewRecorder()
	reqBlack := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/vision/verify-frame", bytes.NewReader(blackReqPayload))
	reqBlack.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(recBlack, reqBlack)
	if recBlack.Code != http.StatusOK || !strings.Contains(recBlack.Body.String(), `"passed":false`) || !strings.Contains(recBlack.Body.String(), "BLANK_OR_BLACK_SCREEN") {
		t.Fatalf("black frame check mismatch code=%d body=%s", recBlack.Code, recBlack.Body.String())
	}

	// 2. Rich high-contrast UI PNG -> passes
	richImg := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			c := color.RGBA{R: 18, G: 24, B: 38, A: 255}
			if y >= 4 && y < 12 && x >= 4 && x < 44 {
				c = color.RGBA{R: 56, G: 189, B: 248, A: 255}
			} else if y >= 16 && y < 28 && x >= 4 && x < 24 {
				c = color.RGBA{R: 241, G: 245, B: 249, A: 255}
			}
			richImg.SetRGBA(x, y, c)
		}
	}
	var richBuf bytes.Buffer
	if err := png.Encode(&richBuf, richImg); err != nil {
		t.Fatal(err)
	}
	richReqPayload, _ := json.Marshal(map[string]string{
		"image_base64": base64.StdEncoding.EncodeToString(richBuf.Bytes()),
	})
	recRich := httptest.NewRecorder()
	reqRich := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:32145/api/vision/verify-frame", bytes.NewReader(richReqPayload))
	reqRich.Header.Set("Content-Type", "application/json")
	srv.ServeHTTP(recRich, reqRich)
	if recRich.Code != http.StatusOK || !strings.Contains(recRich.Body.String(), `"passed":true`) {
		t.Fatalf("rich UI frame check mismatch code=%d body=%s", recRich.Code, recRich.Body.String())
	}

	// 3. Authenticated Mobile Remote endpoint /remote/api/verify-frame
	token, _, err := state.DirectPairRemoteDevice("Galaxy S25")
	if err != nil {
		t.Fatal(err)
	}
	remoteSrv := NewRemoteServer(state)
	recRemote := httptest.NewRecorder()
	reqRemote := httptest.NewRequest(http.MethodPost, "https://127.0.0.1:32146/remote/api/verify-frame", bytes.NewReader(richReqPayload))
	reqRemote.Header.Set("Content-Type", "application/json")
	reqRemote.Header.Set("Authorization", "Bearer "+token)
	remoteSrv.ServeHTTP(recRemote, reqRemote)
	if recRemote.Code != http.StatusOK || !strings.Contains(recRemote.Body.String(), `"passed":true`) {
		t.Fatalf("remote verify-frame mismatch code=%d body=%s", recRemote.Code, recRemote.Body.String())
	}
}
