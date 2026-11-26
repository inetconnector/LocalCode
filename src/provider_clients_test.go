// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnthropicProviderPromptCacheAndToolUseSSE(t *testing.T) {
	var capturedBody map[string]any
	var capturedVersion string
	var capturedKey string
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			http.NotFound(w, r)
			return
		}
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, `{"type":"error","error":{"type":"rate_limit_error","message":"try again"}}`, http.StatusTooManyRequests)
			return
		}
		capturedVersion = r.Header.Get("anthropic-version")
		capturedKey = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		events := []string{
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":1200,\"output_tokens\":0,\"cache_creation_input_tokens\":800,\"cache_read_input_tokens\":400}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Inspecting repo...\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"Reading file now.\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":2,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_01\",\"name\":\"read_file\",\"input\":{}}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"main.go\\\"}\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":2}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":85}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		}
		for _, ev := range events {
			_, _ = w.Write([]byte(ev))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	provider := NewAnthropicProvider("sk-ant-test", srv.URL)
	req := InferenceRequest{
		Model:           "claude-sonnet-4-5",
		SystemPrompt:    "You are LocalCode.",
		ReasoningEffort: "low",
		Messages: []ProviderMessage{
			{Role: "user", Content: "Inspect main.go"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "toolu_prev", Name: "list_files", Arguments: json.RawMessage(`{"path":"."}`)}}},
			{Role: "tool", ToolCallID: "toolu_prev", Content: "main.go\nutil.go"},
			{Role: "user", Content: "Now read main.go"},
		},
		Tools: []ToolDefinition{
			{Name: "list_files", Description: "List project files", Parameters: map[string]any{"type": "object"}},
			{Name: "read_file", Description: "Read a file", Parameters: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}},
		},
	}

	var streamedText strings.Builder
	var streamedToolCalls []ToolCall
	resp, err := provider.Stream(context.Background(), req, func(d StreamDelta) {
		streamedText.WriteString(d.Text)
		if d.ToolCallDelta != nil {
			streamedToolCalls = append(streamedToolCalls, *d.ToolCallDelta)
		}
	})
	if err != nil {
		t.Fatalf("Anthropic Stream failed: %v", err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("expected 1 retry after 429 (2 attempts), got %d", attempts.Load())
	}
	if capturedVersion != "2023-06-01" || capturedKey != "sk-ant-test" {
		t.Fatalf("unexpected headers: version=%q key=%q", capturedVersion, capturedKey)
	}

	// Verify automatic ephemeral cache_control on system, last tool, and rolling conversation breakpoint.
	sysBlocks, _ := capturedBody["system"].([]any)
	if len(sysBlocks) != 1 {
		t.Fatalf("expected 1 system block, got %#v", capturedBody["system"])
	}
	if cc, _ := sysBlocks[0].(map[string]any)["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("expected ephemeral cache_control on system prompt, got %#v", sysBlocks[0])
	}
	tools, _ := capturedBody["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if cc, _ := tools[1].(map[string]any)["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("expected ephemeral cache_control on last tool, got %#v", tools[1])
	}
	msgs, _ := capturedBody["messages"].([]any)
	lastMsg, _ := msgs[len(msgs)-1].(map[string]any)
	lastBlocks, _ := lastMsg["content"].([]any)
	if cc, _ := lastBlocks[len(lastBlocks)-1].(map[string]any)["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
		t.Fatalf("expected rolling ephemeral cache_control on latest user message, got %#v", lastBlocks)
	}
	if thinking, _ := capturedBody["thinking"].(map[string]any); thinking["type"] != "enabled" || thinking["budget_tokens"].(float64) < 4096 {
		t.Fatalf("expected thinking effort budget enabled, got %#v", capturedBody["thinking"])
	}

	if resp.Content != "Reading file now." || streamedText.String() != "Reading file now." {
		t.Fatalf("unexpected text content: %q (streamed %q)", resp.Content, streamedText.String())
	}
	if resp.Thinking != "Inspecting repo..." {
		t.Fatalf("unexpected thinking: %q", resp.Thinking)
	}
	if len(resp.ToolCalls) != 1 || len(streamedToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got resp=%+v streamed=%+v", resp.ToolCalls, streamedToolCalls)
	}
	args, err := resp.ToolCalls[0].ArgumentsMap()
	if err != nil || args["path"] != "main.go" || resp.ToolCalls[0].ID != "toolu_01" || resp.ToolCalls[0].Name != "read_file" {
		t.Fatalf("unexpected tool call: %+v err=%v", resp.ToolCalls[0], err)
	}
	if resp.Usage.CacheCreationTokens != 800 || resp.Usage.CacheReadTokens != 400 || resp.Usage.OutputTokens != 85 {
		t.Fatalf("unexpected usage metrics: %+v", resp.Usage)
	}
}

func TestOpenAIProviderParallelToolCallsAndCachedTokensSSE(t *testing.T) {
	var chatReq map[string]any
	var responsesReq map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-oai-test" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_ = json.NewDecoder(r.Body).Decode(&chatReq)
			chunks := []string{
				`data: {"model":"o4-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Checking two files."}}]}`,
				`data: {"model":"o4-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"read_file","arguments":"{\"path\":"}}]}}]}`,
				`data: {"model":"o4-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"b.go\"}"}}]}}]}`,
				`data: {"model":"o4-mini","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a.go\"}"}}]}}]}`,
				`data: {"model":"o4-mini","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":900,"completion_tokens":150,"total_tokens":1050,"prompt_tokens_details":{"cached_tokens":640},"completion_tokens_details":{"reasoning_tokens":90}}}`,
				`data: [DONE]`,
			}
			for _, c := range chunks {
				fmt.Fprintf(w, "%s\n\n", c)
			}
		case "/v1/responses":
			_ = json.NewDecoder(r.Body).Decode(&responsesReq)
			events := []string{
				"event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Running responses tool.\"}\n\n",
				"event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"fc_1\",\"type\":\"function_call\",\"call_id\":\"call_resp_1\",\"name\":\"search_text\"}}\n\n",
				"event: response.function_call_arguments.delta\ndata: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"{\\\"query\\\":\\\"TODO\\\"}\"}\n\n",
				"event: response.function_call_arguments.done\ndata: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"fc_1\",\"arguments\":\"{\\\"query\\\":\\\"TODO\\\"}\"}\n\n",
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1000,\"output_tokens\":200,\"total_tokens\":1200,\"input_tokens_details\":{\"cached_tokens\":750},\"output_tokens_details\":{\"reasoning_tokens\":120}}}}\n\n",
			}
			for _, ev := range events {
				_, _ = w.Write([]byte(ev))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := NewOpenAIProvider("sk-oai-test", srv.URL)
	baseReq := InferenceRequest{
		Model:           "o4-mini",
		SystemPrompt:    "System rules.",
		ReasoningEffort: "medium",
		Messages:        []ProviderMessage{{Role: "user", Content: "Read a.go and b.go"}},
		Tools:           []ToolDefinition{{Name: "read_file", Description: "Read file", Parameters: map[string]any{"type": "object"}}},
	}

	// 1. /v1/chat/completions SSE with parallel tool calls + cached_tokens
	resp, err := p.Stream(context.Background(), baseReq, nil)
	if err != nil {
		t.Fatalf("OpenAI chat completions Stream failed: %v", err)
	}
	if chatReq["parallel_tool_calls"] != true || chatReq["reasoning_effort"] != "medium" {
		t.Fatalf("unexpected OpenAI chat request options: %#v", chatReq)
	}
	if len(resp.ToolCalls) != 2 {
		t.Fatalf("expected 2 parallel tool calls, got %+v", resp.ToolCalls)
	}
	args0, _ := resp.ToolCalls[0].ArgumentsMap()
	args1, _ := resp.ToolCalls[1].ArgumentsMap()
	if args0["path"] != "a.go" || args1["path"] != "b.go" {
		t.Fatalf("unexpected parallel tool args: %+v / %+v", args0, args1)
	}
	if resp.Usage.CacheReadTokens != 640 || resp.Usage.ReasoningTokens != 90 {
		t.Fatalf("unexpected OpenAI cached/reasoning tokens: %+v", resp.Usage)
	}

	// 2. /v1/responses SSE
	respReq := baseReq
	respReq.Model = "gpt-5"
	respReq.UseResponsesAPI = true
	resp2, err := p.Stream(context.Background(), respReq, nil)
	if err != nil {
		t.Fatalf("OpenAI responses Stream failed: %v", err)
	}
	if responsesReq["instructions"] != "System rules." {
		t.Fatalf("expected instructions in /v1/responses payload, got %#v", responsesReq)
	}
	if resp2.Content != "Running responses tool." || len(resp2.ToolCalls) != 1 || resp2.ToolCalls[0].Name != "search_text" {
		t.Fatalf("unexpected /v1/responses output: %+v", resp2)
	}
	if resp2.Usage.CacheReadTokens != 750 || resp2.Usage.ReasoningTokens != 120 {
		t.Fatalf("unexpected /v1/responses usage: %+v", resp2.Usage)
	}
}

func TestGeminiProviderFunctionCallAndThinkingSSE(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	var capturedAPIKey string
	var capturedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedAPIKey = r.Header.Get("x-goog-api-key")
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)

		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Planning fix","thought":true}]}}]}`,
			`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Applying patch now."}]}}]}`,
			`data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"apply_patch","args":{"patch":"*** Begin Patch\n*** End Patch"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":50000,"candidatesTokenCount":320,"totalTokenCount":50320,"cachedContentTokenCount":42000,"thoughtsTokenCount":180}}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "%s\n\n", c)
		}
	}))
	defer srv.Close()

	p := NewGeminiProvider("AIza-test-key", srv.URL)
	resp, err := p.Stream(context.Background(), InferenceRequest{
		Model:          "gemini-2.5-pro",
		SystemPrompt:   "You are a coding agent.",
		ThinkingBudget: 8192,
		Messages:       []ProviderMessage{{Role: "user", Content: "Fix the bug"}},
		Tools: []ToolDefinition{{
			Name:        "apply_patch",
			Description: "Apply unified patch",
			Parameters:  map[string]any{"type": "object", "properties": map[string]any{"patch": map[string]any{"type": "string"}}},
		}},
	}, nil)
	if err != nil {
		t.Fatalf("Gemini Stream failed: %v", err)
	}
	if capturedPath != "/v1beta/models/gemini-2.5-pro:streamGenerateContent" || capturedQuery != "alt=sse" || capturedAPIKey != "AIza-test-key" {
		t.Fatalf("unexpected Gemini endpoint/auth: path=%q query=%q key=%q", capturedPath, capturedQuery, capturedAPIKey)
	}
	genCfg, _ := capturedBody["generationConfig"].(map[string]any)
	thinkingCfg, _ := genCfg["thinkingConfig"].(map[string]any)
	if thinkingCfg["thinkingBudget"].(float64) != 8192 {
		t.Fatalf("expected thinkingConfig.thinkingBudget=8192, got %#v", capturedBody["generationConfig"])
	}
	if resp.Thinking != "Planning fix" || resp.Content != "Applying patch now." {
		t.Fatalf("unexpected Gemini content/thinking: %+v", resp)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "apply_patch" {
		t.Fatalf("expected Gemini functionCall apply_patch, got %+v", resp.ToolCalls)
	}
	if resp.Usage.CacheReadTokens != 42000 || resp.Usage.ReasoningTokens != 180 || resp.Usage.InputTokens != 50000 {
		t.Fatalf("unexpected Gemini usage: %+v", resp.Usage)
	}
}

func TestLlamaCppBackendCachePromptAndNativeTools(t *testing.T) {
	var capturedPayloads []map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedPayloads = append(capturedPayloads, body)

		if stream, _ := body["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"model\":\"qwen2.5-coder:14b\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_llama_1\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\\\"go.mod\\\"}\"}}]}}]}\n\n")
			fmt.Fprint(w, "data: {\"model\":\"qwen2.5-coder:14b\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}],\"usage\":{\"prompt_tokens\":600,\"completion_tokens\":25,\"total_tokens\":625},\"timings\":{\"cache_n\":512}}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "qwen2.5-coder:14b",
			"choices": []map[string]any{{
				"finish_reason": "tool_calls",
				"message": map[string]any{
					"role":    "assistant",
					"content": "",
					"tool_calls": []map[string]any{{
						"id":   "call_llama_sync",
						"type": "function",
						"function": map[string]any{
							"name":      "list_files",
							"arguments": `{"path":"src"}`,
						},
					}},
				},
			}},
			"usage":   map[string]any{"prompt_tokens": 400, "completion_tokens": 20, "total_tokens": 420},
			"timings": map[string]any{"cache_n": 320},
		})
	}))
	defer srv.Close()

	backend := NewLlamaCppBackend(srv.URL, "local-secret")
	if !backend.SupportsPromptCaching() || !backend.SupportsNativeTools() {
		t.Fatal("LlamaCppBackend should advertise prompt caching and native tools")
	}

	req := InferenceRequest{
		Model:        "qwen2.5-coder:14b",
		SystemPrompt: "Local agent prompt",
		Messages:     []ProviderMessage{{Role: "user", Content: "List src"}},
		Tools:        []ToolDefinition{{Name: "list_files", Description: "List files"}, {Name: "read_file", Description: "Read file"}},
	}

	resp, err := backend.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("LlamaCpp Complete failed: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "list_files" || resp.Usage.CacheReadTokens != 320 {
		t.Fatalf("unexpected LlamaCpp Complete response: %+v", resp)
	}

	streamResp, err := backend.Stream(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("LlamaCpp Stream failed: %v", err)
	}
	if len(streamResp.ToolCalls) != 1 || streamResp.ToolCalls[0].Name != "read_file" || streamResp.Usage.CacheReadTokens != 512 {
		t.Fatalf("unexpected LlamaCpp Stream response: %+v", streamResp)
	}

	if len(capturedPayloads) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(capturedPayloads))
	}
	for i, pl := range capturedPayloads {
		if pl["cache_prompt"] != true {
			t.Fatalf("payload #%d missing cache_prompt=true: %#v", i, pl)
		}
		if tools, ok := pl["tools"].([]any); !ok || len(tools) != 2 {
			t.Fatalf("payload #%d missing tools: %#v", i, pl["tools"])
		}
	}
}

func TestProviderRouterResolveAndOllamaFallback(t *testing.T) {
	// Ensure routing resolves every provider cleanly from config/env.
	cfgAnthropic := defaultConfig()
	cfgAnthropic.LastModel = "claude-sonnet-4-5"
	cfgAnthropic.EnvironmentVars = map[string]string{"ANTHROPIC_API_KEY": "sk-ant-1"}
	if p := ResolveInferenceProvider(cfgAnthropic); p.Name() != "anthropic" {
		t.Fatalf("expected anthropic provider, got %q", p.Name())
	}

	cfgOpenAI := defaultConfig()
	cfgOpenAI.LastModel = "gpt-5"
	cfgOpenAI.EnvironmentVars = map[string]string{"OPENAI_API_KEY": "sk-oai-1"}
	if p := ResolveInferenceProvider(cfgOpenAI); p.Name() != "openai" {
		t.Fatalf("expected openai provider, got %q", p.Name())
	}

	cfgGemini := defaultConfig()
	cfgGemini.LastModel = "gemini-2.5-pro"
	cfgGemini.EnvironmentVars = map[string]string{"GEMINI_API_KEY": "AIza-1"}
	if p := ResolveInferenceProvider(cfgGemini); p.Name() != "gemini" {
		t.Fatalf("expected gemini provider, got %q", p.Name())
	}

	cfgLlama := defaultConfig()
	cfgLlama.LastModel = "qwen2.5-coder:14b"
	cfgLlama.EnvironmentVars = map[string]string{"LOCALCODE_LLAMACPP_URL": "http://127.0.0.1:8080"}
	if p := ResolveInferenceProvider(cfgLlama); p.Name() != "llamacpp" {
		t.Fatalf("expected llamacpp provider, got %q", p.Name())
	}

	// Test Ollama native tool calling fallback to flat JSON when model does not support tools.
	var callCount atomic.Int32
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n == 1 {
			http.Error(w, `{"error":"registry.ollama.ai/library/legacy:latest does not support tools"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"message":{"role":"assistant","content":"{\"action\":\"finish\",\"message\":\"done via flat json\"}"},"done":true,"done_reason":"stop"}`)
	}))
	defer ollamaSrv.Close()

	oc := NewOllamaClient()
	oc.BaseURL = ollamaSrv.URL
	ollamaProvider := NewOllamaNativeProvider(oc)
	resp, err := ollamaProvider.Complete(context.Background(), InferenceRequest{
		Model:      "legacy:latest",
		Messages:   []ProviderMessage{{Role: "user", Content: "hi"}},
		Tools:      []ToolDefinition{{Name: "read_file", Description: "Read"}},
		JSONSchema: actionSchema,
	})
	if err != nil {
		t.Fatalf("Ollama fallback failed: %v", err)
	}
	if !strings.Contains(resp.Content, `"finish"`) || callCount.Load() != 2 {
		t.Fatalf("expected flat JSON fallback after tool error, got calls=%d content=%q", callCount.Load(), resp.Content)
	}
}

func TestDynamicLocalContextAndNumPredict(t *testing.T) {
	t.Setenv("LOCALCODE_MAX_LOCAL_CTX", "")

	cfg := defaultConfig()
	cfg.ContextLength = 65536

	cases := []struct {
		name        string
		vramMB      int
		cfgCtx      int
		wantCtx     int
		wantPredict int
	}{
		{"unknown_vram", 0, 65536, 12288, 4096},
		{"under_8gb", 6144, 65536, 8192, 4096},
		{"8gb_tier", 8192, 65536, 12288, 4096},
		{"12gb_to_20gb_tier", 16384, 65536, 12288, 4096},
		{"20gb_plus_tier", 24576, 65536, 32768, 8192},
		{"configured_base_respected", 24576, 16384, 16384, 4096},
		{"min_floor_8192", 16384, 4096, 8192, 4096},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := cfg
			c.ContextLength = tc.cfgCtx
			gotCtx := computeDynamicLocalContext(c, tc.vramMB)
			if gotCtx != tc.wantCtx {
				t.Fatalf("computeDynamicLocalContext(cfgCtx=%d, vramMB=%d) = %d, want %d", tc.cfgCtx, tc.vramMB, gotCtx, tc.wantCtx)
			}
			gotPredict := computeDynamicNumPredict(gotCtx)
			if gotPredict != tc.wantPredict {
				t.Fatalf("computeDynamicNumPredict(%d) = %d, want %d", gotCtx, gotPredict, tc.wantPredict)
			}
		})
	}

	// Verify LOCALCODE_MAX_LOCAL_CTX override
	t.Setenv("LOCALCODE_MAX_LOCAL_CTX", "20480")
	if got := computeDynamicLocalContext(cfg, 24576); got != 20480 {
		t.Fatalf("expected LOCALCODE_MAX_LOCAL_CTX=20480 override, got %d", got)
	}
	t.Setenv("LOCALCODE_MAX_LOCAL_CTX", "")

	// Verify parseGPUVRAMMB
	if mb := parseGPUVRAMMB("NVIDIA GeForce RTX 4080, 16376 MiB"); mb != 16376 {
		t.Fatalf("parseGPUVRAMMB MiB = %d, want 16376", mb)
	}
	if mb := parseGPUVRAMMB("RTX 4090 (24 GB)"); mb != 24576 {
		t.Fatalf("parseGPUVRAMMB GB = %d, want 24576", mb)
	}

	// Verify OllamaClient.ChatStream actually sends dynamic num_ctx & num_predict (no 6144/2400 cap!)
	t.Setenv("LOCALCODE_DETECTED_VRAM_MB", "16384")
	var capturedOpts map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Options map[string]any `json:"options"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		capturedOpts = body.Options
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{
			Message: OllamaMessage{Role: "assistant", Content: `{"action":"finish","message":"ok"}`},
			Done:    true,
		})
	}))
	defer srv.Close()

	oc := NewOllamaClient()
	oc.BaseURL = srv.URL
	oc.ContextLength = 32768
	if _, err := oc.Chat(context.Background(), "qwen2.5-coder:14b", []OllamaMessage{{Role: "user", Content: "hi"}}, actionSchema); err != nil {
		t.Fatalf("Ollama Chat failed: %v", err)
	}
	if numCtx := int(capturedOpts["num_ctx"].(float64)); numCtx != 12288 {
		t.Fatalf("expected dynamic local num_ctx=12288 (VRAM-safe 16GB tier), got %d", numCtx)
	}
	if numPredict := int(capturedOpts["num_predict"].(float64)); numPredict != 4096 {
		t.Fatalf("expected dynamic local num_predict=4096 (no 2400 cap), got %d", numPredict)
	}
}

func TestCompactMessagesWithFocusAndModelAssistedCompaction(t *testing.T) {
	t.Setenv("LOCALCODE_MAX_LOCAL_CTX", "")
	t.Setenv("LOCALCODE_DETECTED_VRAM_MB", "16384")

	var capturedPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body OllamaChatRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) >= 2 {
			capturedPrompt = body.Messages[1].Content
		}
		summaryJSON, _ := json.Marshal(compactedContext{
			Summary:         "Fokussierte Zusammenfassung auf OAuth2-Token-Refresh.",
			OriginalTask:    "Auth Refactor",
			Decisions:       []string{"PKCE aktiviert"},
			ProjectFacts:    []string{"auth/token.go verwaltet Refresh"},
			FilesRead:       []string{"auth/token.go"},
			FilesChanged:    []string{"auth/token.go"},
			Commands:        []string{"go test ./auth/..."},
			Failures:        []string{},
			OpenItems:       []string{"Retry bei 401 testen"},
			NextRecommended: "Unit-Test für 401-Refresh ergänzen",
		})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{
			Message: OllamaMessage{Role: "assistant", Content: string(summaryJSON)},
			Done:    true,
		})
	}))
	defer srv.Close()

	cfg := defaultConfig()
	cfg.OllamaURL = "http://127.0.0.1:11434"
	cfg.ContextLength = 32768
	cfg.ContextCompactionKeepRecent = 2
	cfg.ModelTimeout = 10

	if eff := effectiveCompactionContextLength(cfg); eff != 12288 {
		t.Fatalf("expected dynamic effectiveCompactionContextLength=12288, got %d", eff)
	}

	oc := NewOllamaClient()
	oc.BaseURL = srv.URL
	oc.ContextLength = 32768

	messages := []OllamaMessage{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Bitte CSS-Farben prüfen"},
		{Role: "assistant", Content: `{"action":"read_file","message":"Lese styles.css"}`},
		{Role: "user", Content: "Wichtiger Hinweis zu auth/token.go OAuth2 Refresh Token Rotation"},
		{Role: "assistant", Content: `{"action":"read_file","message":"Lese auth/token.go"}`},
		{Role: "user", Content: "Aktuelle Frage zu OAuth2"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	compacted, ok := CompactMessagesWithFocus(ctx, messages, "OAuth2 Token Refresh in auth/token.go", cfg, oc, "qwen2.5-coder:14b", "Auth Refactor")
	if !ok {
		t.Fatal("expected CompactMessagesWithFocus to compact messages")
	}
	if !strings.Contains(capturedPrompt, "OAuth2 Token Refresh in auth/token.go") {
		t.Fatalf("expected focus instruction in model compaction prompt, got: %s", capturedPrompt)
	}
	if len(compacted) < 3 {
		t.Fatalf("unexpected compacted message count: %d", len(compacted))
	}
	if !strings.Contains(compacted[1].Content, "Fokussierte Zusammenfassung auf OAuth2-Token-Refresh.") {
		t.Fatalf("expected model-assisted compacted summary, got: %s", compacted[1].Content)
	}
	if !strings.Contains(compacted[1].Content, "Fokus der Komprimierung:\nOAuth2 Token Refresh in auth/token.go") {
		t.Fatalf("expected focus header in compacted state, got: %s", compacted[1].Content)
	}

	// Deterministic fallback with focus (nil client)
	detCompacted, detOK := CompactMessagesWithFocus(ctx, messages, "OAuth2 Rotation", cfg, nil, "", "Auth Refactor")
	if !detOK {
		t.Fatal("expected deterministic CompactMessagesWithFocus to succeed")
	}
	if !strings.Contains(detCompacted[1].Content, "OAuth2 Rotation") || !strings.Contains(detCompacted[1].Content, "auth/token.go") {
		t.Fatalf("expected deterministic focused summary to preserve focus facts, got: %s", detCompacted[1].Content)
	}
}

func TestProviderClientsNonStreamingCompleteAndCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v1/messages":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model":       "claude-sonnet-4-5",
				"stop_reason": "tool_use",
				"content": []map[string]any{
					{"type": "text", "text": "Done."},
					{"type": "tool_use", "id": "tu_1", "name": "read_file", "input": map[string]any{"path": "README.md"}},
				},
				"usage": map[string]any{
					"input_tokens":                500,
					"output_tokens":               40,
					"cache_creation_input_tokens": 300,
					"cache_read_input_tokens":     200,
				},
			})
		case r.URL.Path == "/v1/chat/completions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "gpt-5",
				"choices": []map[string]any{{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"content": "",
						"tool_calls": []map[string]any{{
							"id":   "call_1",
							"type": "function",
							"function": map[string]any{
								"name":      "read_file",
								"arguments": `{"path":"go.mod"}`,
							},
						}},
					},
				}},
				"usage": map[string]any{
					"prompt_tokens":             400,
					"completion_tokens":         30,
					"total_tokens":              430,
					"prompt_tokens_details":     map[string]any{"cached_tokens": 256},
					"completion_tokens_details": map[string]any{"reasoning_tokens": 12},
				},
			})
		case r.URL.Path == "/v1/responses":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model":  "gpt-5",
				"status": "completed",
				"output": []map[string]any{
					{"type": "message", "content": []map[string]any{{"type": "output_text", "text": "Responses OK"}}},
					{"type": "function_call", "call_id": "fc_sync", "name": "list_files", "arguments": `{"path":"."}`},
				},
				"usage": map[string]any{
					"input_tokens":          300,
					"output_tokens":         25,
					"total_tokens":          325,
					"input_tokens_details":  map[string]any{"cached_tokens": 128},
					"output_tokens_details": map[string]any{"reasoning_tokens": 8},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/v1beta/models/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"candidates": []map[string]any{{
					"finishReason": "STOP",
					"content": map[string]any{
						"role": "model",
						"parts": []map[string]any{
							{"text": "Gemini OK"},
							{"functionCall": map[string]any{"name": "read_file", "args": map[string]any{"path": "main.go"}}},
						},
					},
				}},
				"usageMetadata": map[string]any{
					"promptTokenCount":        700,
					"candidatesTokenCount":    50,
					"totalTokenCount":         750,
					"cachedContentTokenCount": 500,
					"thoughtsTokenCount":      20,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Anthropic Complete
	ant := NewAnthropicProvider("k", srv.URL)
	r1, err := ant.Complete(context.Background(), InferenceRequest{Model: "claude-sonnet-4-5", Messages: []ProviderMessage{{Role: "user", Content: "hi"}}})
	if err != nil || len(r1.ToolCalls) != 1 || r1.Usage.CacheReadTokens != 200 {
		t.Fatalf("Anthropic Complete unexpected: resp=%+v err=%v", r1, err)
	}

	// OpenAI Chat Complete & Responses Complete
	oai := NewOpenAIProvider("k", srv.URL)
	r2, err := oai.Complete(context.Background(), InferenceRequest{Model: "gpt-5", Messages: []ProviderMessage{{Role: "user", Content: "hi"}}})
	if err != nil || len(r2.ToolCalls) != 1 || r2.Usage.CacheReadTokens != 256 || r2.Usage.ReasoningTokens != 12 {
		t.Fatalf("OpenAI Chat Complete unexpected: resp=%+v err=%v", r2, err)
	}
	r3, err := oai.Complete(context.Background(), InferenceRequest{Model: "gpt-5", UseResponsesAPI: true, Messages: []ProviderMessage{{Role: "user", Content: "hi"}}})
	if err != nil || r3.Content != "Responses OK" || len(r3.ToolCalls) != 1 || r3.Usage.CacheReadTokens != 128 {
		t.Fatalf("OpenAI Responses Complete unexpected: resp=%+v err=%v", r3, err)
	}

	// Gemini Complete
	gem := NewGeminiProvider("k", srv.URL)
	r4, err := gem.Complete(context.Background(), InferenceRequest{Model: "gemini-2.5-pro", Messages: []ProviderMessage{{Role: "user", Content: "hi"}}})
	if err != nil || r4.Content != "Gemini OK" || len(r4.ToolCalls) != 1 || r4.Usage.CacheReadTokens != 500 {
		t.Fatalf("Gemini Complete unexpected: resp=%+v err=%v", r4, err)
	}

	// Context cancellation stops immediately without hanging
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ant.Complete(canceledCtx, InferenceRequest{Model: "claude-sonnet-4-5", Messages: []ProviderMessage{{Role: "user", Content: "hi"}}}); err == nil {
		t.Fatal("expected canceled context to return error")
	}
}
