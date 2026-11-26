// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
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

func newWave17TestState(t *testing.T) *AppState {
	t.Helper()
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(func() { state.Close() })
	return state
}

func TestRemoteTunnelStateAndHTTPHandlers(t *testing.T) {
	stopRemoteMobileTunnel()
	t.Cleanup(stopRemoteMobileTunnel)

	if u := currentRemoteTunnelURL(); u != "" {
		t.Fatalf("expected empty tunnel URL when stopped, got %q", u)
	}
	_ = findCloudflaredBinary()

	state := newWave17TestState(t)
	srv := NewServer(state)

	// GET when stopped
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/remote/tunnel", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"running":false`) {
		t.Fatalf("GET tunnel status=%d body=%s", rr.Code, rr.Body.String())
	}

	// PUT -> 405
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "http://127.0.0.1/api/remote/tunnel", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT tunnel status=%d want 405", rr.Code)
	}

	// DELETE -> 200 running:false
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/remote/tunnel", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"running":false`) {
		t.Fatalf("DELETE tunnel status=%d body=%s", rr.Code, rr.Body.String())
	}

	// Simulate an already-running tunnel process so GET & POST reuse active URL without spawning cloudflared
	remoteTunnelMu.Lock()
	remoteTunnelCmd = &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	remoteTunnelURL = "https://mock-unit-test.trycloudflare.com"
	remoteTunnelMu.Unlock()
	t.Cleanup(func() {
		remoteTunnelMu.Lock()
		remoteTunnelCmd = nil
		remoteTunnelURL = ""
		remoteTunnelMu.Unlock()
	})

	if got := currentRemoteTunnelURL(); got != "https://mock-unit-test.trycloudflare.com" {
		t.Fatalf("expected active mock URL, got %q", got)
	}

	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/remote/tunnel", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"running":true`) || !strings.Contains(rr.Body.String(), `mock-unit-test.trycloudflare.com/remote`) {
		t.Fatalf("GET active tunnel status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/remote/tunnel", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"deep_link":"localcode://pair?`) {
		t.Fatalf("POST active tunnel status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAutoRiskClassifierGitCommandsAndHelpers(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.ApprovalMode = "balanced"

	policy := RiskPolicyFromConfig(cfg)
	gitCases := []struct {
		args []string
		want RiskLevel
	}{
		{nil, RiskSafeRead},
		{[]string{"status"}, RiskSafeRead},
		{[]string{"diff", "--stat"}, RiskSafeRead},
		{[]string{"tag"}, RiskSafeRead},
		{[]string{"tag", "-d", "v1.0.0"}, RiskSensitiveMutation},
		{[]string{"branch"}, RiskSafeRead},
		{[]string{"branch", "-D", "old"}, RiskSensitiveMutation},
		{[]string{"add", "main.go"}, RiskSafeLocalEdit},
		{[]string{"commit", "-m", "ok"}, RiskSafeLocalEdit},
		{[]string{"push", "--force", "origin", "main"}, RiskSensitiveMutation},
		{[]string{"reset", "--hard", "HEAD~1"}, RiskSensitiveMutation},
		{[]string{"push", "origin", "main"}, RiskSensitiveMutation},
	}
	for _, tc := range gitCases {
		res := ClassifyActionRisk(project, AgentAction{Action: "git", Args: tc.args}, policy)
		if res.Level != tc.want {
			t.Fatalf("git %v: risk=%s want=%s (reason=%s)", tc.args, res.Level, tc.want, res.Reason)
		}
	}

	for _, cmd := range []string{
		"go test ./...",
		"gofmt -l .",
		"ruff check .",
		"cargo test --lib",
		"dotnet test",
		"mvn test",
		"npm test",
		"npm run lint",
		"npx tsc --noEmit",
		"python -m pytest -q",
		"ctest --output-on-failure",
	} {
		if !isBoundedLocalTestCommand(cmd) {
			t.Fatalf("expected %q to be bounded local test command", cmd)
		}
	}
	for _, cmd := range []string{"", "npm run deploy", "go test && rm -rf /", "curl https://example.com"} {
		if isBoundedLocalTestCommand(cmd) {
			t.Fatalf("expected %q NOT to be bounded local test command", cmd)
		}
	}

	for _, cmd := range []string{"ls -la", "pwd", "grep foo bar.go", "echo hello"} {
		if !isSafeReadCommand(cmd) {
			t.Fatalf("expected %q to be safe read command", cmd)
		}
	}
	for _, cmd := range []string{"", "ls | sh", "rm -rf tmp"} {
		if isSafeReadCommand(cmd) {
			t.Fatalf("expected %q NOT to be safe read command", cmd)
		}
	}

	if got := normalizeRiskApprovalMode("  DANGEROUS "); got != "dangerous" {
		t.Fatalf("normalizeRiskApprovalMode=%q", got)
	}
	if got := normalizeRiskApprovalMode("unknown-mode"); got != "balanced" {
		t.Fatalf("normalizeRiskApprovalMode fallback=%q", got)
	}
	if line := reconstructActionCommandLine(AgentAction{Action: "run_tool", Tool: "go", Args: []string{"test", "./..."}}); line != "go test ./..." {
		t.Fatalf("reconstructActionCommandLine=%q", line)
	}
	if target := primaryActionTarget(AgentAction{Action: "write_file", Path: "src/app.go"}); target != "src/app.go" {
		t.Fatalf("primaryActionTarget=%q", target)
	}
}

func TestBrowserAutomationHelpersAndFallbacks(t *testing.T) {
	cfg := defaultConfig()
	project := t.TempDir()

	if _, err := validateBrowserURL(""); err == nil {
		t.Fatal("empty URL must fail")
	}
	if _, err := validateBrowserURL("ftp://example.com"); err == nil {
		t.Fatal("ftp scheme must fail")
	}
	u, err := validateBrowserURL("example.com/path")
	if err != nil || u.Scheme != "https" {
		t.Fatalf("bare host normalization failed: u=%v err=%v", u, err)
	}

	inspectOut, err := BrowserInspect(context.Background(), cfg, project, "#main")
	if err != nil || !strings.Contains(inspectOut, "BROWSER INSPECT") {
		t.Fatalf("BrowserInspect out=%q err=%v", inspectOut, err)
	}
	if _, err := BrowserClick(context.Background(), cfg, project, ""); err == nil {
		t.Fatal("empty selector click must fail")
	}
	if _, err := BrowserClick(context.Background(), cfg, project, "#btn"); err == nil {
		t.Fatal("click without Playwright MCP must fail")
	}
	if _, err := BrowserType(context.Background(), cfg, project, "", "hi"); err == nil {
		t.Fatal("empty selector type must fail")
	}
	if _, err := BrowserType(context.Background(), cfg, project, "#in", "hi"); err == nil {
		t.Fatal("type without Playwright MCP must fail")
	}

	sampleHTML := `<!doctype html><html><head><title>Demo Page</title></head><body>
		<h1 id="title">Main Heading</h1>
		<p>First paragraph text.</p>
		<a href="/docs" class="nav">Docs Link</a>
		<button id="go">Submit</button>
		<input type="text" name="query" placeholder="Search"/>
		<ul><li>Item One</li><li>Item Two</li></ul>
		<table><tr><th>Col</th></tr><tr><td>Val</td></tr></table>
	</body></html>`
	summary := parseHTMLToSummary("https://example.com", sampleHTML, 200)
	if summary.Title != "Demo Page" || len(summary.Elements) == 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	extracted := extractStructuredContent(sampleHTML, "h1")
	if !strings.Contains(extracted, "Main Heading") {
		t.Fatalf("expected heading in extracted content: %q", extracted)
	}
	extractedAll := extractStructuredContent(sampleHTML, "")
	if !strings.Contains(extractedAll, "First paragraph text.") {
		t.Fatalf("expected paragraph in full extraction: %q", extractedAll)
	}
	if sel := selectorFromAttrs("button", map[string]string{"id": "login-btn", "class": "primary"}); sel != "#login-btn" {
		t.Fatalf("selectorFromAttrs=%q", sel)
	}
	if sel := selectorFromAttrs("input", map[string]string{"name": "query"}); sel != `input[name='query']` {
		t.Fatalf("selectorFromAttrs name=%q", sel)
	}
	if sel := selectorFromAttrs("div", map[string]string{}); sel != "div" {
		t.Fatalf("selectorFromAttrs bare=%q", sel)
	}
}

func TestWindowsPipeFallbackPTY(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only pipe fallback test")
	}
	p, err := startWindowsPipeFallback(t.TempDir(), 80, 24)
	if err != nil {
		t.Fatalf("startWindowsPipeFallback: %v", err)
	}
	defer func() { _ = p.Close() }()

	if p.BackendName() != "pipe-fallback" {
		t.Fatalf("BackendName=%q", p.BackendName())
	}
	if err := p.Resize(100, 30); err != nil {
		t.Fatalf("Resize err=%v", err)
	}
	if _, err := p.Write([]byte("echo pipe_ok_marker\r\nexit 0\r\n")); err != nil {
		t.Fatalf("Write err=%v", err)
	}

	buf := make([]byte, 4096)
	var out strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, rerr := p.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if rerr != nil {
			break
		}
	}
	code, _ := p.Wait()
	if code != 0 || !strings.Contains(out.String(), "pipe_ok_marker") {
		t.Fatalf("pipe fallback code=%d out=%q", code, out.String())
	}
}

func TestFrontierRoutesPushVerifyFrameAndAPKHelpers(t *testing.T) {
	state := newWave17TestState(t)
	srv := NewServer(state)

	// Push endpoint rejects GET/PUT with 405 and accepts DELETE/POST
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/push/register-endpoint", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET push status=%d want 405", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "http://127.0.0.1/api/push/register-endpoint", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT push status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodDelete, "http://127.0.0.1/api/push/register-endpoint", nil))
	if rr.Code != http.StatusMethodNotAllowed && rr.Code != http.StatusOK {
		t.Fatalf("DELETE push status=%d", rr.Code)
	}

	// Verify frame HTTP: 405, bad JSON, valid PNG
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/vision/verify-frame", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET verify-frame status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/vision/verify-frame", strings.NewReader("{bad")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json verify-frame status=%d", rr.Code)
	}

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 200, A: 255})
		}
	}
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	payload, _ := json.Marshal(map[string]any{
		"image_base64": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBuf.Bytes()),
	})
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/vision/verify-frame", bytes.NewReader(payload)))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"width":8`) {
		t.Fatalf("valid verify-frame status=%d body=%s", rr.Code, rr.Body.String())
	}

	// File SHA256 helper
	tmpFile := filepath.Join(t.TempDir(), "sample.bin")
	_ = os.WriteFile(tmpFile, []byte("localcode-sha256-test"), 0o600)
	if hexSum, sz, err := computeFileSHA256Hex(tmpFile); err != nil || len(hexSum) != 64 || sz <= 0 {
		t.Fatalf("computeFileSHA256Hex=%q sz=%d err=%v", hexSum, sz, err)
	}
}

func TestMCPServerAllToolsAndProviderClientHelpers(t *testing.T) {
	state := newWave17TestState(t)
	srv := NewServer(state)

	// Method not allowed, parse error, invalid jsonrpc
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/mcp", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mcp status=%d", rr.Code)
	}
	for _, body := range []string{`{bad`, `{"jsonrpc":"1.0","id":1,"method":"ping"}`, `{"jsonrpc":"2.0","id":2,"method":"unknown/method"}`} {
		rr = httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", strings.NewReader(body)))
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"error"`) {
			t.Fatalf("MCP error body=%s status=%d resp=%s", body, rr.Code, rr.Body.String())
		}
	}

	// Build small valid PNG base64 for localcode_verify_frame
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	img.Set(3, 3, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	var pngBuf bytes.Buffer
	_ = png.Encode(&pngBuf, img)
	b64 := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

	// Invoke all 7 MCP tools + unknown tool
	tools := []map[string]any{
		{"name": "localcode_health", "arguments": map[string]any{}},
		{"name": "localcode_providers_status", "arguments": map[string]any{}},
		{"name": "localcode_list_pty_sessions", "arguments": map[string]any{}},
		{"name": "localcode_verify_frame", "arguments": map[string]any{"image_base64": b64}},
		{"name": "localcode_verify_frame", "arguments": map[string]any{"image_base64": "!!!not-base64!!!"}},
		{"name": "localcode_apk_update_info", "arguments": map[string]any{}},
		{"name": "localcode_push_status", "arguments": map[string]any{}},
		{"name": "localcode_relay_status", "arguments": map[string]any{}},
		{"name": "nonexistent_tool", "arguments": map[string]any{}},
	}
	for i, params := range tools {
		raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": i + 10, "method": "tools/call", "params": params})
		rr = httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", bytes.NewReader(raw)))
		if rr.Code != http.StatusOK {
			t.Fatalf("MCP tool %v status=%d", params["name"], rr.Code)
		}
	}

	// Provider client helper coverage
	tc := ToolCall{ID: "c1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.go"}`)}
	if m, err := tc.ArgumentsMap(); err != nil || m["path"] != "a.go" {
		t.Fatalf("ArgumentsMap=%v err=%v", m, err)
	}
	if m, err := (ToolCall{}).ArgumentsMap(); err != nil || len(m) != 0 {
		t.Fatalf("empty ArgumentsMap=%v err=%v", m, err)
	}
	if _, err := (ToolCall{Arguments: json.RawMessage(`{bad`)}).ArgumentsMap(); err == nil {
		t.Fatal("invalid JSON ArgumentsMap must error")
	}

	var otc OllamaToolCall
	otc.Function.Name = "read_file"
	otc.Function.Arguments = map[string]any{"path": "x"}
	sys, pmsgs := providerMessagesFromOllama([]OllamaMessage{
		{Role: "system", Content: "sys prompt"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []OllamaToolCall{otc}},
	})
	if sys != "sys prompt" || len(pmsgs) != 2 || len(pmsgs[1].ToolCalls) != 1 {
		t.Fatalf("providerMessagesFromOllama sys=%q pmsgs=%+v", sys, pmsgs)
	}
	if string(normalizeRawJSONObject(nil)) != "{}" {
		t.Fatal("nil raw json should normalize to {}")
	}
	if got := decodeJSONObjectMap(json.RawMessage(`not-json`)); got["raw"] != "not-json" {
		t.Fatalf("decodeJSONObjectMap fallback=%v", got)
	}

	// Ollama native provider Stream & Complete against mock HTTP server
	attempts := 0
	mockOllama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "test-model",
			"message": map[string]any{
				"role":    "assistant",
				"content": "hello from mock ollama",
			},
			"done":              true,
			"done_reason":       "stop",
			"prompt_eval_count": 10,
			"eval_count":        5,
		})
	}))
	defer mockOllama.Close()

	// Exercise executeProviderHTTP retry on 503 -> 200
	resp, err := executeProviderHTTP(context.Background(), mockOllama.Client(), 1, func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, mockOllama.URL, nil)
	})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("executeProviderHTTP retry err=%v resp=%v", err, resp)
	}
	_ = resp.Body.Close()

	ollamaClient := NewOllamaClient()
	ollamaClient.BaseURL = mockOllama.URL
	provider := NewOllamaNativeProvider(ollamaClient)
	if provider.Name() != "ollama" || !provider.SupportsNativeTools() {
		t.Fatalf("unexpected provider metadata: %s", provider.Name())
	}
	var streamed string
	outResp, err := provider.Stream(context.Background(), InferenceRequest{
		Model:        "test-model",
		SystemPrompt: "be concise",
		Messages:     []ProviderMessage{{Role: "user", Content: "ping"}},
	}, func(d StreamDelta) { streamed += d.Text })
	if err != nil || !strings.Contains(outResp.Content, "hello from mock ollama") || streamed == "" {
		t.Fatalf("OllamaNativeProvider.Stream err=%v resp=%+v streamed=%q", err, outResp, streamed)
	}

	// OpenAI & Gemini payload builders + registry
	temp := 0.2
	oai := NewOpenAIProvider("test-key-placeholder", "https://api.openai.com/v1")
	if _, err := oai.buildChatCompletionsPayload(InferenceRequest{}, false); err == nil {
		t.Fatal("missing model must error")
	}
	richReq := InferenceRequest{
		Model:        "gpt-5",
		SystemPrompt: "sys",
		MaxTokens:    512,
		Temperature:  &temp,
		Tools:        []ToolDefinition{{Name: "read_file", Description: "read"}},
		Messages: []ProviderMessage{
			{Role: "system", Content: "extra sys"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "calling", ToolCalls: []ToolCall{{ID: "tc1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.go"}`)}}},
			{Role: "tool", ToolCallID: "tc1", ToolName: "read_file", Content: "package main"},
		},
	}
	if p, err := oai.buildChatCompletionsPayload(richReq, true); err != nil || p["stream"] != true {
		t.Fatalf("buildChatCompletionsPayload err=%v p=%v", err, p)
	}
	richReq.ReasoningEffort = "medium"
	if p, err := oai.buildResponsesPayload(richReq, true); err != nil || p["model"] != "gpt-5" {
		t.Fatalf("buildResponsesPayload err=%v p=%v", err, p)
	}
	if _, err := oai.buildResponsesPayload(InferenceRequest{}, false); err == nil {
		t.Fatal("missing model in responses must error")
	}
	gem := NewGeminiProvider("test-key-placeholder", "https://generativelanguage.googleapis.com")
	if !gem.SupportsNativeTools() || !gem.SupportsPromptCaching() {
		t.Fatal("expected Gemini capabilities true")
	}
	if p, err := gem.buildPayload(richReq); err != nil || p == nil {
		t.Fatalf("Gemini buildPayload err=%v", err)
	}
	router := NewProviderRouter()
	router.Register("ollama", provider)
	if len(router.overrides) != 1 {
		t.Fatal("expected registered ollama provider override")
	}
}

func TestZeroKnowledgeRelayEdgeCases(t *testing.T) {
	srv := NewZeroKnowledgeRelayServer(ZeroKnowledgeRelayConfig{
		MaxQueueDepth:   2,
		MaxFrameBytes:   1024,
		MaxSessions:     2,
		RateLimitPerSec: 100,
		RateBurst:       100,
	})
	defer srv.Close()

	// Health GET + 405
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/relay/v1/health", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("health status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/relay/v1/health", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("health POST status=%d", rr.Code)
	}
	// Error routes
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/bad", http.StatusNotFound},
		{"/relay/v1/sessions/noslash", http.StatusNotFound},
		{"/relay/v1/sessions/short/send", http.StatusBadRequest},
		{"/relay/v1/sessions/session_identifier_1234567890/unknown", http.StatusNotFound},
	} {
		rr = httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rr.Code != tc.want {
			t.Fatalf("path %s status=%d want=%d", tc.path, rr.Code, tc.want)
		}
	}

	// validateOpaqueEnvelope branches
	sid := "session_identifier_1234567890"
	baseEnv := func() RelayEnvelope {
		return RelayEnvelope{
			Version:    relayProtocolVersion,
			SessionID:  sid,
			SenderRole: string(RelayRoleWorkstation),
			Direction:  relayDirWorkstationToMobile,
			Seq:        1,
			Nonce:      base64.RawURLEncoding.EncodeToString([]byte("123456789012")),
			Ciphertext: base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef_opaque")),
		}
	}
	okEnv := baseEnv()
	if err := validateOpaqueEnvelope(&okEnv, sid); err != nil {
		t.Fatalf("valid envelope failed: %v", err)
	}
	for i, mutate := range []func(*RelayEnvelope){
		func(e *RelayEnvelope) { e.Version = 99 },
		func(e *RelayEnvelope) { e.SessionID = "other_sid_1234567890" },
		func(e *RelayEnvelope) { e.Direction = relayDirMobileToWorkstation },
		func(e *RelayEnvelope) {
			e.SenderRole = string(RelayRoleMobile)
			e.Direction = relayDirWorkstationToMobile
		},
		func(e *RelayEnvelope) { e.SenderRole = "unknown" },
		func(e *RelayEnvelope) { e.Seq = 0 },
		func(e *RelayEnvelope) { e.Nonce = "bad" },
		func(e *RelayEnvelope) { e.Ciphertext = "bad" },
	} {
		env := baseEnv()
		mutate(&env)
		if err := validateOpaqueEnvelope(&env, sid); err == nil {
			t.Fatalf("case %d expected validation error", i)
		}
	}

	// relayClientIP branches
	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	r1.RemoteAddr = "10.1.2.3:8080"
	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	r2.RemoteAddr = "10.1.2.3"
	r3 := httptest.NewRequest(http.MethodGet, "/", nil)
	r3.RemoteAddr = ""
	if relayClientIP(r1) != "10.1.2.3" || relayClientIP(r2) != "10.1.2.3" || relayClientIP(r3) != "unknown" {
		t.Fatal("unexpected relayClientIP result")
	}
}
