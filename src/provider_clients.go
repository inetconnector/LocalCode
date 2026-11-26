// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ToolDefinition describes a callable tool using JSON Schema parameters.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolCall represents a native structured tool invocation returned by a model.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ArgumentsMap decodes the raw JSON arguments into a map.
func (tc ToolCall) ArgumentsMap() (map[string]any, error) {
	raw := bytes.TrimSpace(tc.Arguments)
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// UsageMetrics captures token accounting including prompt caching and reasoning tokens.
type UsageMetrics struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	TotalTokens         int `json:"total_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
	ReasoningTokens     int `json:"reasoning_tokens,omitempty"`
}

// ProviderMessage is the provider-agnostic conversation message representation.
type ProviderMessage struct {
	Role       string     `json:"role"` // system | user | assistant | tool
	Content    string     `json:"content,omitempty"`
	Thinking   string     `json:"thinking,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"tool_name,omitempty"`
	Images     []string   `json:"images,omitempty"`
}

// InferenceRequest configures a single completion or streaming call.
type InferenceRequest struct {
	Model             string            `json:"model"`
	SystemPrompt      string            `json:"system_prompt,omitempty"`
	Messages          []ProviderMessage `json:"messages"`
	Tools             []ToolDefinition  `json:"tools,omitempty"`
	MaxTokens         int               `json:"max_tokens,omitempty"`
	Temperature       *float64          `json:"temperature,omitempty"`
	ReasoningEffort   string            `json:"reasoning_effort,omitempty"` // low | medium | high
	ThinkingBudget    int               `json:"thinking_budget,omitempty"`
	EnablePromptCache *bool             `json:"enable_prompt_cache,omitempty"`
	UseResponsesAPI   bool              `json:"use_responses_api,omitempty"`
	JSONSchema        any               `json:"json_schema,omitempty"`
	ContextLength     int               `json:"context_length,omitempty"`
	ExtraHeaders      map[string]string `json:"extra_headers,omitempty"`
}

// StreamDelta represents an incremental streaming event emitted by a provider.
type StreamDelta struct {
	Text          string        `json:"text,omitempty"`
	Thinking      string        `json:"thinking,omitempty"`
	ToolCallDelta *ToolCall     `json:"tool_call_delta,omitempty"`
	Done          bool          `json:"done,omitempty"`
	Usage         *UsageMetrics `json:"usage,omitempty"`
}

// InferenceResponse is the final accumulated response from a provider call.
type InferenceResponse struct {
	Content    string       `json:"content"`
	Thinking   string       `json:"thinking,omitempty"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	StopReason string       `json:"stop_reason,omitempty"`
	Usage      UsageMetrics `json:"usage"`
	Model      string       `json:"model,omitempty"`
	Provider   string       `json:"provider,omitempty"`
}

// NativeInferenceProvider is the unified interface for frontier and local providers.
type NativeInferenceProvider interface {
	Name() string
	Complete(ctx context.Context, req InferenceRequest) (*InferenceResponse, error)
	Stream(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error)
	SupportsNativeTools() bool
	SupportsPromptCaching() bool
}

func providerMessagesFromOllama(messages []OllamaMessage) (string, []ProviderMessage) {
	var sysParts []string
	out := make([]ProviderMessage, 0, len(messages))
	for _, m := range messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			if strings.TrimSpace(m.Content) != "" {
				sysParts = append(sysParts, m.Content)
			}
			continue
		}
		pm := ProviderMessage{
			Role:       m.Role,
			Content:    m.Content,
			Thinking:   m.Thinking,
			ToolCallID: m.ToolCallID,
			ToolName:   m.ToolName,
			Images:     append([]string(nil), m.Images...),
		}
		for i, tc := range m.ToolCalls {
			argsBytes, err := json.Marshal(tc.Function.Arguments)
			if err != nil || len(argsBytes) == 0 {
				argsBytes = []byte("{}")
			}
			id := strings.TrimSpace(tc.ID)
			if id == "" {
				id = fmt.Sprintf("call_ollama_%d", i+1)
			}
			pm.ToolCalls = append(pm.ToolCalls, ToolCall{
				ID:        id,
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(argsBytes),
			})
		}
		out = append(out, pm)
	}
	return strings.Join(sysParts, "\n\n"), out
}

func normalizeRawJSONObject(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return json.RawMessage(`{}`)
	}
	if json.Valid(trimmed) {
		return trimmed
	}
	// If the model emitted a JSON-encoded string containing JSON, try to unwrap it.
	var unquoted string
	if json.Unmarshal(trimmed, &unquoted) == nil && json.Valid([]byte(strings.TrimSpace(unquoted))) {
		return json.RawMessage(strings.TrimSpace(unquoted))
	}
	wrapped, _ := json.Marshal(map[string]any{"raw": string(trimmed)})
	return json.RawMessage(wrapped)
}

func decodeJSONObjectMap(raw json.RawMessage) map[string]any {
	norm := normalizeRawJSONObject(raw)
	var out map[string]any
	if err := json.Unmarshal(norm, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func defaultProviderHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Minute}
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		529: // Anthropic overloaded
		return true
	default:
		return false
	}
}

func retryDelayFromResponse(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if ra := strings.TrimSpace(resp.Header.Get("Retry-After")); ra != "" {
			if sec, err := strconv.Atoi(ra); err == nil && sec > 0 && sec <= 30 {
				return time.Duration(sec) * time.Second
			}
		}
	}
	delay := time.Duration(300*(1<<attempt)) * time.Millisecond
	if delay > 4*time.Second {
		delay = 4 * time.Second
	}
	return delay
}

func executeProviderHTTP(ctx context.Context, client *http.Client, maxRetries int, buildReq func() (*http.Request, error)) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = defaultProviderHTTPClient()
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			if attempt < maxRetries {
				timer := time.NewTimer(retryDelayFromResponse(nil, attempt))
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
				}
				continue
			}
			return nil, lastErr
		}
		if isRetryableStatus(resp.StatusCode) && attempt < maxRetries {
			delay := retryDelayFromResponse(resp, attempt)
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

type sseEvent struct {
	Event string
	Data  string
}

func readSSEStream(ctx context.Context, r io.Reader, handle func(ev sseEvent) error) error {
	scanner := bufio.NewScanner(io.LimitReader(r, 64<<20))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var currentEvent string
	var dataLines []string

	flush := func() error {
		if currentEvent == "" && len(dataLines) == 0 {
			return nil
		}
		ev := sseEvent{
			Event: strings.TrimSpace(currentEvent),
			Data:  strings.Join(dataLines, "\n"),
		}
		currentEvent = ""
		dataLines = dataLines[:0]
		if ev.Data == "" && ev.Event == "" {
			return nil
		}
		return handle(ev)
	}

	for scanner.Scan() {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			// SSE comment / keepalive
			continue
		}
		if strings.HasPrefix(line, "event:") {
			currentEvent = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			val := strings.TrimPrefix(line, "data:")
			if strings.HasPrefix(val, " ") {
				val = val[1:]
			}
			dataLines = append(dataLines, val)
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	if err := flush(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func promptCacheEnabled(req InferenceRequest, defaultOn bool) bool {
	if req.EnablePromptCache != nil {
		return *req.EnablePromptCache
	}
	return defaultOn
}

func thinkingBudgetForEffort(effort string, explicitBudget int) int {
	if explicitBudget > 0 {
		return explicitBudget
	}
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return 4096
	case "medium":
		return 10240
	case "high":
		return 24576
	default:
		return 0
	}
}

// ---------------------------------------------------------------------------
// 1. AnthropicProvider (/v1/messages, Prompt Caching, Thinking, Native Tools)
// ---------------------------------------------------------------------------

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicAPIVersion     = "2023-06-01"
)

type AnthropicProvider struct {
	BaseURL            string
	APIKey             string
	HTTP               *http.Client
	MaxRetries         int
	DefaultPromptCache bool
	ExtraHeaders       map[string]string
}

func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultAnthropicBaseURL
	}
	return &AnthropicProvider{
		BaseURL:            base,
		APIKey:             strings.TrimSpace(apiKey),
		HTTP:               defaultProviderHTTPClient(),
		MaxRetries:         3,
		DefaultPromptCache: true,
	}
}

func (p *AnthropicProvider) Name() string                { return "anthropic" }
func (p *AnthropicProvider) SupportsNativeTools() bool   { return true }
func (p *AnthropicProvider) SupportsPromptCaching() bool { return true }

func (p *AnthropicProvider) buildPayload(req InferenceRequest, stream bool) (map[string]any, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("anthropic: model is required")
	}
	useCache := promptCacheEnabled(req, p.DefaultPromptCache)
	ephemeral := map[string]any{"type": "ephemeral"}

	// Separate system prompt if embedded inside Messages as well.
	systemParts := make([]string, 0, 2)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		systemParts = append(systemParts, req.SystemPrompt)
	}

	type anthropicMsg struct {
		Role    string           `json:"role"`
		Content []map[string]any `json:"content"`
	}
	var msgs []anthropicMsg
	appendMsg := func(role string, blocks []map[string]any) {
		if len(blocks) == 0 {
			return
		}
		if len(msgs) > 0 && msgs[len(msgs)-1].Role == role {
			msgs[len(msgs)-1].Content = append(msgs[len(msgs)-1].Content, blocks...)
			return
		}
		msgs = append(msgs, anthropicMsg{Role: role, Content: blocks})
	}

	for _, m := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				systemParts = append(systemParts, m.Content)
			}
		case "tool":
			block := map[string]any{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     m.Content,
			}
			appendMsg("user", []map[string]any{block})
		case "assistant":
			var blocks []map[string]any
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": decodeJSONObjectMap(tc.Arguments),
				})
			}
			appendMsg("assistant", blocks)
		default:
			var blocks []map[string]any
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}
			appendMsg("user", blocks)
		}
	}

	// Attach rolling conversation cache breakpoint on the latest user turn's last content block.
	if useCache {
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "user" && len(msgs[i].Content) > 0 {
				msgs[i].Content[len(msgs[i].Content)-1]["cache_control"] = ephemeral
				break
			}
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 8192
	}
	budget := thinkingBudgetForEffort(req.ReasoningEffort, req.ThinkingBudget)
	if budget > 0 && maxTokens <= budget {
		maxTokens = budget + 4096
	}

	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTokens,
		"messages":   msgs,
		"stream":     stream,
	}

	if len(systemParts) > 0 {
		sysBlock := map[string]any{
			"type": "text",
			"text": strings.Join(systemParts, "\n\n"),
		}
		if useCache {
			sysBlock["cache_control"] = ephemeral
		}
		payload["system"] = []map[string]any{sysBlock}
	}

	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for i, t := range req.Tools {
			schema := t.Parameters
			if schema == nil {
				schema = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			entry := map[string]any{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": schema,
			}
			if useCache && i == len(req.Tools)-1 {
				entry["cache_control"] = ephemeral
			}
			tools = append(tools, entry)
		}
		payload["tools"] = tools
	}

	if budget > 0 {
		payload["thinking"] = map[string]any{
			"type":          "enabled",
			"budget_tokens": budget,
		}
	} else if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}

	return payload, nil
}

func (p *AnthropicProvider) doRequest(ctx context.Context, req InferenceRequest, stream bool) (*http.Response, error) {
	payload, err := p.buildPayload(req, stream)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1/messages"
	return executeProviderHTTP(ctx, p.HTTP, p.MaxRetries, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
		if p.APIKey != "" {
			httpReq.Header.Set("x-api-key", p.APIKey)
		}
		for k, v := range p.ExtraHeaders {
			httpReq.Header.Set(k, v)
		}
		for k, v := range req.ExtraHeaders {
			httpReq.Header.Set(k, v)
		}
		return httpReq, nil
	})
}

func (p *AnthropicProvider) Complete(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	resp, err := p.doRequest(ctx, req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}
	var parsed struct {
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Thinking string          `json:"thinking"`
			ID       string          `json:"id"`
			Name     string          `json:"name"`
			Input    json.RawMessage `json:"input"`
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("anthropic decode response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("anthropic error (%s): %s", parsed.Error.Type, parsed.Error.Message)
	}
	var textBuf, thinkBuf strings.Builder
	var toolCalls []ToolCall
	for _, blk := range parsed.Content {
		switch blk.Type {
		case "text":
			textBuf.WriteString(blk.Text)
		case "thinking":
			thinkBuf.WriteString(blk.Thinking)
		case "tool_use":
			toolCalls = append(toolCalls, ToolCall{
				ID:        blk.ID,
				Name:      blk.Name,
				Arguments: normalizeRawJSONObject(blk.Input),
			})
		}
	}
	usage := UsageMetrics{
		InputTokens:         parsed.Usage.InputTokens,
		OutputTokens:        parsed.Usage.OutputTokens,
		TotalTokens:         parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		CacheCreationTokens: parsed.Usage.CacheCreationInputTokens,
		CacheReadTokens:     parsed.Usage.CacheReadInputTokens,
	}
	return &InferenceResponse{
		Content:    textBuf.String(),
		Thinking:   thinkBuf.String(),
		ToolCalls:  toolCalls,
		StopReason: parsed.StopReason,
		Usage:      usage,
		Model:      parsed.Model,
		Provider:   p.Name(),
	}, nil
}

func (p *AnthropicProvider) Stream(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	resp, err := p.doRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return nil, fmt.Errorf("anthropic stream HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}

	type activeBlock struct {
		kind    string
		id      string
		name    string
		jsonBuf strings.Builder
	}
	blocks := make(map[int]*activeBlock)
	var textBuf, thinkBuf strings.Builder
	var toolCalls []ToolCall
	out := &InferenceResponse{Model: req.Model, Provider: p.Name()}

	err = readSSEStream(ctx, resp.Body, func(ev sseEvent) error {
		if strings.TrimSpace(ev.Data) == "" || strings.TrimSpace(ev.Data) == "[DONE]" {
			return nil
		}
		var envelope struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message *struct {
				Model string `json:"model"`
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					OutputTokens             int `json:"output_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message,omitempty"`
			ContentBlock *struct {
				Type  string          `json:"type"`
				ID    string          `json:"id"`
				Name  string          `json:"name"`
				Text  string          `json:"text"`
				Input json.RawMessage `json:"input"`
			} `json:"content_block,omitempty"`
			Delta *struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta,omitempty"`
			Usage *struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage,omitempty"`
			Error *struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(ev.Data), &envelope); err != nil {
			return nil
		}
		evType := envelope.Type
		if evType == "" {
			evType = ev.Event
		}
		switch evType {
		case "error":
			if envelope.Error != nil {
				return fmt.Errorf("anthropic stream error (%s): %s", envelope.Error.Type, envelope.Error.Message)
			}
			return errors.New("anthropic stream error")
		case "message_start":
			if envelope.Message != nil {
				if envelope.Message.Model != "" {
					out.Model = envelope.Message.Model
				}
				out.Usage.InputTokens = envelope.Message.Usage.InputTokens
				out.Usage.OutputTokens = envelope.Message.Usage.OutputTokens
				out.Usage.CacheCreationTokens = envelope.Message.Usage.CacheCreationInputTokens
				out.Usage.CacheReadTokens = envelope.Message.Usage.CacheReadInputTokens
			}
		case "content_block_start":
			if envelope.ContentBlock != nil {
				blk := &activeBlock{
					kind: envelope.ContentBlock.Type,
					id:   envelope.ContentBlock.ID,
					name: envelope.ContentBlock.Name,
				}
				if envelope.ContentBlock.Type == "text" && envelope.ContentBlock.Text != "" {
					textBuf.WriteString(envelope.ContentBlock.Text)
					if onDelta != nil {
						onDelta(StreamDelta{Text: envelope.ContentBlock.Text})
					}
				}
				blocks[envelope.Index] = blk
			}
		case "content_block_delta":
			if envelope.Delta == nil {
				return nil
			}
			switch envelope.Delta.Type {
			case "text_delta":
				textBuf.WriteString(envelope.Delta.Text)
				if onDelta != nil && envelope.Delta.Text != "" {
					onDelta(StreamDelta{Text: envelope.Delta.Text})
				}
			case "thinking_delta":
				thinkBuf.WriteString(envelope.Delta.Thinking)
				if onDelta != nil && envelope.Delta.Thinking != "" {
					onDelta(StreamDelta{Thinking: envelope.Delta.Thinking})
				}
			case "input_json_delta":
				if blk, ok := blocks[envelope.Index]; ok {
					blk.jsonBuf.WriteString(envelope.Delta.PartialJSON)
				}
			}
		case "content_block_stop":
			if blk, ok := blocks[envelope.Index]; ok {
				if blk.kind == "tool_use" {
					tc := ToolCall{
						ID:        blk.id,
						Name:      blk.name,
						Arguments: normalizeRawJSONObject(json.RawMessage(blk.jsonBuf.String())),
					}
					toolCalls = append(toolCalls, tc)
					if onDelta != nil {
						tcCopy := tc
						onDelta(StreamDelta{ToolCallDelta: &tcCopy})
					}
				}
				delete(blocks, envelope.Index)
			}
		case "message_delta":
			if envelope.Delta != nil && envelope.Delta.StopReason != "" {
				out.StopReason = envelope.Delta.StopReason
			}
			if envelope.Usage != nil && envelope.Usage.OutputTokens > 0 {
				out.Usage.OutputTokens = envelope.Usage.OutputTokens
			}
		case "message_stop":
			out.Usage.TotalTokens = out.Usage.InputTokens + out.Usage.OutputTokens
			if onDelta != nil {
				u := out.Usage
				onDelta(StreamDelta{Done: true, Usage: &u})
			}
			return io.EOF
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(blocks) > 0 {
		idxs := make([]int, 0, len(blocks))
		for idx := range blocks {
			idxs = append(idxs, idx)
		}
		sort.Ints(idxs)
		for _, idx := range idxs {
			blk := blocks[idx]
			if blk != nil && blk.kind == "tool_use" {
				toolCalls = append(toolCalls, ToolCall{
					ID:        blk.id,
					Name:      blk.name,
					Arguments: normalizeRawJSONObject(json.RawMessage(blk.jsonBuf.String())),
				})
			}
		}
	}
	out.Content = textBuf.String()
	out.Thinking = thinkBuf.String()
	out.ToolCalls = toolCalls
	out.Usage.TotalTokens = out.Usage.InputTokens + out.Usage.OutputTokens
	return out, nil
}

// ---------------------------------------------------------------------------
// 2. OpenAIProvider (/v1/responses + /v1/chat/completions, Parallel Tools)
// ---------------------------------------------------------------------------

const defaultOpenAIBaseURL = "https://api.openai.com"

type OpenAIProvider struct {
	BaseURL         string
	APIKey          string
	HTTP            *http.Client
	MaxRetries      int
	UseResponsesAPI bool
	ExtraHeaders    map[string]string
}

func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultOpenAIBaseURL
	}
	return &OpenAIProvider{
		BaseURL:    base,
		APIKey:     strings.TrimSpace(apiKey),
		HTTP:       defaultProviderHTTPClient(),
		MaxRetries: 3,
	}
}

func (p *OpenAIProvider) Name() string                { return "openai" }
func (p *OpenAIProvider) SupportsNativeTools() bool   { return true }
func (p *OpenAIProvider) SupportsPromptCaching() bool { return true }

func (p *OpenAIProvider) useResponses(req InferenceRequest) bool {
	return req.UseResponsesAPI || p.UseResponsesAPI
}

func (p *OpenAIProvider) buildChatCompletionsPayload(req InferenceRequest, stream bool) (map[string]any, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("openai: model is required")
	}
	msgs := make([]map[string]any, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.SystemPrompt})
	}
	for _, m := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "tool":
			entry := map[string]any{
				"role":         "tool",
				"tool_call_id": m.ToolCallID,
				"content":      m.Content,
			}
			if m.ToolName != "" {
				entry["name"] = m.ToolName
			}
			msgs = append(msgs, entry)
		case "assistant":
			entry := map[string]any{"role": "assistant"}
			if m.Content != "" || len(m.ToolCalls) == 0 {
				entry["content"] = m.Content
			}
			if len(m.ToolCalls) > 0 {
				tcs := make([]map[string]any, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					tcs = append(tcs, map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": string(normalizeRawJSONObject(tc.Arguments)),
						},
					})
				}
				entry["tool_calls"] = tcs
			}
			msgs = append(msgs, entry)
		default:
			msgs = append(msgs, map[string]any{
				"role":    role,
				"content": m.Content,
			})
		}
	}

	payload := map[string]any{
		"model":    req.Model,
		"messages": msgs,
		"stream":   stream,
	}
	if stream {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			params := t.Parameters
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  params,
				},
			})
		}
		payload["tools"] = tools
		payload["parallel_tool_calls"] = true
	}
	if effort := strings.ToLower(strings.TrimSpace(req.ReasoningEffort)); effort != "" {
		payload["reasoning_effort"] = effort
	}
	if req.MaxTokens > 0 {
		payload["max_completion_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil && strings.TrimSpace(req.ReasoningEffort) == "" {
		payload["temperature"] = *req.Temperature
	}
	return payload, nil
}

func (p *OpenAIProvider) buildResponsesPayload(req InferenceRequest, stream bool) (map[string]any, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("openai: model is required")
	}
	var sysParts []string
	if strings.TrimSpace(req.SystemPrompt) != "" {
		sysParts = append(sysParts, req.SystemPrompt)
	}
	inputItems := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				sysParts = append(sysParts, m.Content)
			}
		case "tool":
			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
		case "assistant":
			if strings.TrimSpace(m.Content) != "" {
				inputItems = append(inputItems, map[string]any{
					"type":    "message",
					"role":    "assistant",
					"content": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				inputItems = append(inputItems, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Name,
					"arguments": string(normalizeRawJSONObject(tc.Arguments)),
				})
			}
		default:
			inputItems = append(inputItems, map[string]any{
				"type":    "message",
				"role":    "user",
				"content": m.Content,
			})
		}
	}

	payload := map[string]any{
		"model":  req.Model,
		"input":  inputItems,
		"stream": stream,
	}
	if len(sysParts) > 0 {
		payload["instructions"] = strings.Join(sysParts, "\n\n")
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			params := t.Parameters
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, map[string]any{
				"type":        "function",
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			})
		}
		payload["tools"] = tools
		payload["parallel_tool_calls"] = true
	}
	if effort := strings.ToLower(strings.TrimSpace(req.ReasoningEffort)); effort != "" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	if req.MaxTokens > 0 {
		payload["max_output_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil && strings.TrimSpace(req.ReasoningEffort) == "" {
		payload["temperature"] = *req.Temperature
	}
	return payload, nil
}

func (p *OpenAIProvider) doRequest(ctx context.Context, path string, payload map[string]any, extraHeaders map[string]string) (*http.Response, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + path
	return executeProviderHTTP(ctx, p.HTTP, p.MaxRetries, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if p.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
		}
		for k, v := range p.ExtraHeaders {
			httpReq.Header.Set(k, v)
		}
		for k, v := range extraHeaders {
			httpReq.Header.Set(k, v)
		}
		return httpReq, nil
	})
}

func (p *OpenAIProvider) Complete(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	if p.useResponses(req) {
		return p.completeResponses(ctx, req)
	}
	return p.completeChatCompletions(ctx, req)
}

func (p *OpenAIProvider) completeChatCompletions(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	payload, err := p.buildChatCompletionsPayload(req, false)
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequest(ctx, "/v1/chat/completions", payload, req.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai chat HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}
	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CompletionTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"completion_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("openai chat decode: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, errors.New(parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, errors.New("openai chat returned empty choices")
	}
	ch := parsed.Choices[0]
	tcs := make([]ToolCall, 0, len(ch.Message.ToolCalls))
	for _, tc := range ch.Message.ToolCalls {
		tcs = append(tcs, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: normalizeRawJSONObject(json.RawMessage(tc.Function.Arguments)),
		})
	}
	return &InferenceResponse{
		Content:    ch.Message.Content,
		Thinking:   ch.Message.ReasoningContent,
		ToolCalls:  tcs,
		StopReason: ch.FinishReason,
		Usage: UsageMetrics{
			InputTokens:     parsed.Usage.PromptTokens,
			OutputTokens:    parsed.Usage.CompletionTokens,
			TotalTokens:     parsed.Usage.TotalTokens,
			CacheReadTokens: parsed.Usage.PromptTokensDetails.CachedTokens,
			ReasoningTokens: parsed.Usage.CompletionTokensDetails.ReasoningTokens,
		},
		Model:    parsed.Model,
		Provider: p.Name(),
	}, nil
}

func (p *OpenAIProvider) completeResponses(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	payload, err := p.buildResponsesPayload(req, false)
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequest(ctx, "/v1/responses", payload, req.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai responses HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}
	var parsed struct {
		Model      string `json:"model"`
		Status     string `json:"status"`
		OutputText string `json:"output_text"`
		Output     []struct {
			Type      string `json:"type"`
			CallID    string `json:"call_id"`
			ID        string `json:"id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
			Content   []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Summary []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"summary"`
		} `json:"output"`
		Usage struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			TotalTokens        int `json:"total_tokens"`
			InputTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"input_tokens_details"`
			OutputTokensDetails struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("openai responses decode: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, errors.New(parsed.Error.Message)
	}
	var textBuf, thinkBuf strings.Builder
	var tcs []ToolCall
	for _, item := range parsed.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" || c.Type == "text" {
					textBuf.WriteString(c.Text)
				}
			}
		case "reasoning":
			for _, s := range item.Summary {
				thinkBuf.WriteString(s.Text)
			}
		case "function_call":
			callID := item.CallID
			if callID == "" {
				callID = item.ID
			}
			tcs = append(tcs, ToolCall{
				ID:        callID,
				Name:      item.Name,
				Arguments: normalizeRawJSONObject(json.RawMessage(item.Arguments)),
			})
		}
	}
	content := textBuf.String()
	if content == "" && parsed.OutputText != "" {
		content = parsed.OutputText
	}
	return &InferenceResponse{
		Content:    content,
		Thinking:   thinkBuf.String(),
		ToolCalls:  tcs,
		StopReason: parsed.Status,
		Usage: UsageMetrics{
			InputTokens:     parsed.Usage.InputTokens,
			OutputTokens:    parsed.Usage.OutputTokens,
			TotalTokens:     parsed.Usage.TotalTokens,
			CacheReadTokens: parsed.Usage.InputTokensDetails.CachedTokens,
			ReasoningTokens: parsed.Usage.OutputTokensDetails.ReasoningTokens,
		},
		Model:    parsed.Model,
		Provider: p.Name(),
	}, nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	if p.useResponses(req) {
		return p.streamResponses(ctx, req, onDelta)
	}
	return p.streamChatCompletions(ctx, req, onDelta)
}

func (p *OpenAIProvider) streamChatCompletions(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	payload, err := p.buildChatCompletionsPayload(req, true)
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequest(ctx, "/v1/chat/completions", payload, req.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return nil, fmt.Errorf("openai chat stream HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}

	type accTool struct {
		id   string
		name string
		args strings.Builder
	}
	toolMap := make(map[int]*accTool)
	var textBuf, thinkBuf strings.Builder
	out := &InferenceResponse{Model: req.Model, Provider: p.Name()}

	err = readSSEStream(ctx, resp.Body, func(ev sseEvent) error {
		data := strings.TrimSpace(ev.Data)
		if data == "" {
			return nil
		}
		if data == "[DONE]" {
			return io.EOF
		}
		var chunk struct {
			Model   string `json:"model"`
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Delta        struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens        int `json:"prompt_tokens"`
				CompletionTokens    int `json:"completion_tokens"`
				TotalTokens         int `json:"total_tokens"`
				PromptTokensDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
				CompletionTokensDetails struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage,omitempty"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return errors.New(chunk.Error.Message)
		}
		if chunk.Model != "" {
			out.Model = chunk.Model
		}
		if len(chunk.Choices) > 0 {
			ch := chunk.Choices[0]
			if ch.FinishReason != "" {
				out.StopReason = ch.FinishReason
			}
			if ch.Delta.Content != "" {
				textBuf.WriteString(ch.Delta.Content)
				if onDelta != nil {
					onDelta(StreamDelta{Text: ch.Delta.Content})
				}
			}
			if ch.Delta.ReasoningContent != "" {
				thinkBuf.WriteString(ch.Delta.ReasoningContent)
				if onDelta != nil {
					onDelta(StreamDelta{Thinking: ch.Delta.ReasoningContent})
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				acc, ok := toolMap[tc.Index]
				if !ok {
					acc = &accTool{}
					toolMap[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.args.WriteString(tc.Function.Arguments)
				}
			}
		}
		if chunk.Usage != nil {
			out.Usage = UsageMetrics{
				InputTokens:     chunk.Usage.PromptTokens,
				OutputTokens:    chunk.Usage.CompletionTokens,
				TotalTokens:     chunk.Usage.TotalTokens,
				CacheReadTokens: chunk.Usage.PromptTokensDetails.CachedTokens,
				ReasoningTokens: chunk.Usage.CompletionTokensDetails.ReasoningTokens,
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	indices := make([]int, 0, len(toolMap))
	for idx := range toolMap {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		acc := toolMap[idx]
		tc := ToolCall{
			ID:        acc.id,
			Name:      acc.name,
			Arguments: normalizeRawJSONObject(json.RawMessage(acc.args.String())),
		}
		if tc.ID == "" {
			tc.ID = fmt.Sprintf("call_oai_%d", idx+1)
		}
		out.ToolCalls = append(out.ToolCalls, tc)
		if onDelta != nil {
			tcCopy := tc
			onDelta(StreamDelta{ToolCallDelta: &tcCopy})
		}
	}
	out.Content = textBuf.String()
	out.Thinking = thinkBuf.String()
	if onDelta != nil {
		u := out.Usage
		onDelta(StreamDelta{Done: true, Usage: &u})
	}
	return out, nil
}

func (p *OpenAIProvider) streamResponses(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	payload, err := p.buildResponsesPayload(req, true)
	if err != nil {
		return nil, err
	}
	resp, err := p.doRequest(ctx, "/v1/responses", payload, req.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return nil, fmt.Errorf("openai responses stream HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}

	type activeCall struct {
		callID  string
		name    string
		args    strings.Builder
		emitted bool
	}
	callsByItemID := make(map[string]*activeCall)
	callOrder := make([]string, 0)
	var textBuf, thinkBuf strings.Builder
	out := &InferenceResponse{Model: req.Model, Provider: p.Name()}

	err = readSSEStream(ctx, resp.Body, func(ev sseEvent) error {
		data := strings.TrimSpace(ev.Data)
		if data == "" || data == "[DONE]" {
			return nil
		}
		var chunk struct {
			Type      string `json:"type"`
			Delta     string `json:"delta"`
			ItemID    string `json:"item_id"`
			Arguments string `json:"arguments"`
			Item      *struct {
				ID        string `json:"id"`
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"item,omitempty"`
			Response *struct {
				Model  string `json:"model"`
				Status string `json:"status"`
				Usage  *struct {
					InputTokens        int `json:"input_tokens"`
					OutputTokens       int `json:"output_tokens"`
					TotalTokens        int `json:"total_tokens"`
					InputTokensDetails struct {
						CachedTokens int `json:"cached_tokens"`
					} `json:"input_tokens_details"`
					OutputTokensDetails struct {
						ReasoningTokens int `json:"reasoning_tokens"`
					} `json:"output_tokens_details"`
				} `json:"usage,omitempty"`
			} `json:"response,omitempty"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil
		}
		evType := chunk.Type
		if evType == "" {
			evType = ev.Event
		}
		switch evType {
		case "error":
			if chunk.Error != nil && chunk.Error.Message != "" {
				return errors.New(chunk.Error.Message)
			}
			return errors.New("openai responses stream error")
		case "response.output_text.delta":
			textBuf.WriteString(chunk.Delta)
			if onDelta != nil && chunk.Delta != "" {
				onDelta(StreamDelta{Text: chunk.Delta})
			}
		case "response.reasoning_summary_text.delta":
			thinkBuf.WriteString(chunk.Delta)
			if onDelta != nil && chunk.Delta != "" {
				onDelta(StreamDelta{Thinking: chunk.Delta})
			}
		case "response.output_item.added":
			if chunk.Item != nil && chunk.Item.Type == "function_call" {
				key := chunk.Item.ID
				if key == "" {
					key = chunk.Item.CallID
				}
				callID := chunk.Item.CallID
				if callID == "" {
					callID = key
				}
				ac := &activeCall{callID: callID, name: chunk.Item.Name}
				if chunk.Item.Arguments != "" {
					ac.args.WriteString(chunk.Item.Arguments)
				}
				callsByItemID[key] = ac
				callOrder = append(callOrder, key)
			}
		case "response.function_call_arguments.delta":
			if ac, ok := callsByItemID[chunk.ItemID]; ok {
				ac.args.WriteString(chunk.Delta)
			}
		case "response.function_call_arguments.done":
			if ac, ok := callsByItemID[chunk.ItemID]; ok && !ac.emitted {
				raw := chunk.Arguments
				if raw == "" {
					raw = ac.args.String()
				}
				tc := ToolCall{
					ID:        ac.callID,
					Name:      ac.name,
					Arguments: normalizeRawJSONObject(json.RawMessage(raw)),
				}
				out.ToolCalls = append(out.ToolCalls, tc)
				ac.emitted = true
				if onDelta != nil {
					tcCopy := tc
					onDelta(StreamDelta{ToolCallDelta: &tcCopy})
				}
			}
		case "response.completed":
			if chunk.Response != nil {
				if chunk.Response.Model != "" {
					out.Model = chunk.Response.Model
				}
				if chunk.Response.Status != "" {
					out.StopReason = chunk.Response.Status
				}
				if chunk.Response.Usage != nil {
					out.Usage = UsageMetrics{
						InputTokens:     chunk.Response.Usage.InputTokens,
						OutputTokens:    chunk.Response.Usage.OutputTokens,
						TotalTokens:     chunk.Response.Usage.TotalTokens,
						CacheReadTokens: chunk.Response.Usage.InputTokensDetails.CachedTokens,
						ReasoningTokens: chunk.Response.Usage.OutputTokensDetails.ReasoningTokens,
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, key := range callOrder {
		ac := callsByItemID[key]
		if ac != nil && !ac.emitted {
			tc := ToolCall{
				ID:        ac.callID,
				Name:      ac.name,
				Arguments: normalizeRawJSONObject(json.RawMessage(ac.args.String())),
			}
			out.ToolCalls = append(out.ToolCalls, tc)
			if onDelta != nil {
				tcCopy := tc
				onDelta(StreamDelta{ToolCallDelta: &tcCopy})
			}
		}
	}
	out.Content = textBuf.String()
	out.Thinking = thinkBuf.String()
	if onDelta != nil {
		u := out.Usage
		onDelta(StreamDelta{Done: true, Usage: &u})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 3. GeminiProvider (/v1beta/models/{model}:streamGenerateContent?alt=sse)
// ---------------------------------------------------------------------------

const defaultGeminiBaseURL = "https://generativelanguage.googleapis.com"

type GeminiProvider struct {
	BaseURL      string
	APIKey       string
	HTTP         *http.Client
	MaxRetries   int
	ExtraHeaders map[string]string
}

func NewGeminiProvider(apiKey, baseURL string) *GeminiProvider {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultGeminiBaseURL
	}
	return &GeminiProvider{
		BaseURL:    base,
		APIKey:     strings.TrimSpace(apiKey),
		HTTP:       defaultProviderHTTPClient(),
		MaxRetries: 3,
	}
}

func (p *GeminiProvider) Name() string                { return "gemini" }
func (p *GeminiProvider) SupportsNativeTools() bool   { return true }
func (p *GeminiProvider) SupportsPromptCaching() bool { return true }

func (p *GeminiProvider) buildPayload(req InferenceRequest) (map[string]any, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("gemini: model is required")
	}
	var sysParts []string
	if strings.TrimSpace(req.SystemPrompt) != "" {
		sysParts = append(sysParts, req.SystemPrompt)
	}

	contents := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		switch role {
		case "system":
			if strings.TrimSpace(m.Content) != "" {
				sysParts = append(sysParts, m.Content)
			}
		case "tool":
			name := strings.TrimSpace(m.ToolName)
			if name == "" {
				name = "tool"
			}
			contents = append(contents, map[string]any{
				"role": "user",
				"parts": []map[string]any{{
					"functionResponse": map[string]any{
						"name":     name,
						"response": map[string]any{"result": m.Content},
					},
				}},
			})
		case "assistant", "model":
			parts := make([]map[string]any, 0, len(m.ToolCalls)+1)
			if m.Content != "" {
				parts = append(parts, map[string]any{"text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": tc.Name,
						"args": decodeJSONObjectMap(tc.Arguments),
					},
				})
			}
			if len(parts) > 0 {
				contents = append(contents, map[string]any{
					"role":  "model",
					"parts": parts,
				})
			}
		default:
			contents = append(contents, map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": m.Content}},
			})
		}
	}

	payload := map[string]any{
		"contents": contents,
	}
	if len(sysParts) > 0 {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": strings.Join(sysParts, "\n\n")}},
		}
	}
	if len(req.Tools) > 0 {
		decls := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			decl := map[string]any{
				"name":        t.Name,
				"description": t.Description,
			}
			if t.Parameters != nil {
				decl["parameters"] = t.Parameters
			}
			decls = append(decls, decl)
		}
		payload["tools"] = []map[string]any{{"functionDeclarations": decls}}
	}

	genCfg := map[string]any{}
	if req.MaxTokens > 0 {
		genCfg["maxOutputTokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		genCfg["temperature"] = *req.Temperature
	}
	if budget := thinkingBudgetForEffort(req.ReasoningEffort, req.ThinkingBudget); budget > 0 {
		genCfg["thinkingConfig"] = map[string]any{
			"thinkingBudget": budget,
		}
	}
	if len(genCfg) > 0 {
		payload["generationConfig"] = genCfg
	}
	return payload, nil
}

func (p *GeminiProvider) doRequest(ctx context.Context, req InferenceRequest, stream bool) (*http.Response, error) {
	payload, err := p.buildPayload(req)
	if err != nil {
		return nil, err
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	modelID := strings.TrimPrefix(strings.TrimSpace(req.Model), "models/")
	action := ":generateContent"
	if stream {
		action = ":streamGenerateContent?alt=sse"
	}
	endpoint := strings.TrimRight(p.BaseURL, "/") + "/v1beta/models/" + url.PathEscape(modelID) + action
	return executeProviderHTTP(ctx, p.HTTP, p.MaxRetries, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if p.APIKey != "" {
			httpReq.Header.Set("x-goog-api-key", p.APIKey)
		}
		for k, v := range p.ExtraHeaders {
			httpReq.Header.Set(k, v)
		}
		for k, v := range req.ExtraHeaders {
			httpReq.Header.Set(k, v)
		}
		return httpReq, nil
	})
}

type geminiResponseChunk struct {
	Candidates []struct {
		FinishReason string `json:"finishReason"`
		Content      struct {
			Role  string `json:"role"`
			Parts []struct {
				Text         string `json:"text"`
				Thought      bool   `json:"thought"`
				FunctionCall *struct {
					ID   string          `json:"id,omitempty"`
					Name string          `json:"name"`
					Args json.RawMessage `json:"args"`
				} `json:"functionCall,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (p *GeminiProvider) Complete(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	resp, err := p.doRequest(ctx, req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}
	var parsed geminiResponseChunk
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("gemini decode: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, fmt.Errorf("gemini error (%s): %s", parsed.Error.Status, parsed.Error.Message)
	}
	out := &InferenceResponse{Model: req.Model, Provider: p.Name()}
	if len(parsed.Candidates) > 0 {
		cand := parsed.Candidates[0]
		out.StopReason = cand.FinishReason
		var textBuf, thinkBuf strings.Builder
		for i, part := range cand.Content.Parts {
			if part.Thought && part.Text != "" {
				thinkBuf.WriteString(part.Text)
			} else if part.Text != "" {
				textBuf.WriteString(part.Text)
			}
			if part.FunctionCall != nil {
				id := strings.TrimSpace(part.FunctionCall.ID)
				if id == "" {
					id = fmt.Sprintf("call_gemini_%d", i+1)
				}
				out.ToolCalls = append(out.ToolCalls, ToolCall{
					ID:        id,
					Name:      part.FunctionCall.Name,
					Arguments: normalizeRawJSONObject(part.FunctionCall.Args),
				})
			}
		}
		out.Content = textBuf.String()
		out.Thinking = thinkBuf.String()
	}
	if parsed.UsageMetadata != nil {
		out.Usage = UsageMetrics{
			InputTokens:     parsed.UsageMetadata.PromptTokenCount,
			OutputTokens:    parsed.UsageMetadata.CandidatesTokenCount,
			TotalTokens:     parsed.UsageMetadata.TotalTokenCount,
			CacheReadTokens: parsed.UsageMetadata.CachedContentTokenCount,
			ReasoningTokens: parsed.UsageMetadata.ThoughtsTokenCount,
		}
	}
	return out, nil
}

func (p *GeminiProvider) Stream(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	resp, err := p.doRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return nil, fmt.Errorf("gemini stream HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1500))
	}

	var textBuf, thinkBuf strings.Builder
	out := &InferenceResponse{Model: req.Model, Provider: p.Name()}
	callSeq := 0

	err = readSSEStream(ctx, resp.Body, func(ev sseEvent) error {
		data := strings.TrimSpace(ev.Data)
		if data == "" || data == "[DONE]" {
			return nil
		}
		var chunk geminiResponseChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return fmt.Errorf("gemini stream error (%s): %s", chunk.Error.Status, chunk.Error.Message)
		}
		if len(chunk.Candidates) > 0 {
			cand := chunk.Candidates[0]
			if cand.FinishReason != "" {
				out.StopReason = cand.FinishReason
			}
			for _, part := range cand.Content.Parts {
				if part.Thought && part.Text != "" {
					thinkBuf.WriteString(part.Text)
					if onDelta != nil {
						onDelta(StreamDelta{Thinking: part.Text})
					}
				} else if part.Text != "" {
					textBuf.WriteString(part.Text)
					if onDelta != nil {
						onDelta(StreamDelta{Text: part.Text})
					}
				}
				if part.FunctionCall != nil {
					callSeq++
					id := strings.TrimSpace(part.FunctionCall.ID)
					if id == "" {
						id = fmt.Sprintf("call_gemini_%d", callSeq)
					}
					tc := ToolCall{
						ID:        id,
						Name:      part.FunctionCall.Name,
						Arguments: normalizeRawJSONObject(part.FunctionCall.Args),
					}
					out.ToolCalls = append(out.ToolCalls, tc)
					if onDelta != nil {
						tcCopy := tc
						onDelta(StreamDelta{ToolCallDelta: &tcCopy})
					}
				}
			}
		}
		if chunk.UsageMetadata != nil {
			out.Usage = UsageMetrics{
				InputTokens:     chunk.UsageMetadata.PromptTokenCount,
				OutputTokens:    chunk.UsageMetadata.CandidatesTokenCount,
				TotalTokens:     chunk.UsageMetadata.TotalTokenCount,
				CacheReadTokens: chunk.UsageMetadata.CachedContentTokenCount,
				ReasoningTokens: chunk.UsageMetadata.ThoughtsTokenCount,
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out.Content = textBuf.String()
	out.Thinking = thinkBuf.String()
	if onDelta != nil {
		u := out.Usage
		onDelta(StreamDelta{Done: true, Usage: &u})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 4. OllamaNativeProvider (Native Tool Calling + Flat JSON Fallback)
// ---------------------------------------------------------------------------

type OllamaNativeProvider struct {
	Client *OllamaClient
}

func NewOllamaNativeProvider(client *OllamaClient) *OllamaNativeProvider {
	if client == nil {
		client = NewOllamaClient()
	}
	return &OllamaNativeProvider{Client: client}
}

func (p *OllamaNativeProvider) Name() string                { return "ollama" }
func (p *OllamaNativeProvider) SupportsNativeTools() bool   { return true }
func (p *OllamaNativeProvider) SupportsPromptCaching() bool { return false }

func (p *OllamaNativeProvider) toOllamaMessages(req InferenceRequest) []OllamaMessage {
	msgs := make([]OllamaMessage, 0, len(req.Messages)+1)
	if strings.TrimSpace(req.SystemPrompt) != "" {
		msgs = append(msgs, OllamaMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		om := OllamaMessage{
			Role:       m.Role,
			Content:    m.Content,
			Thinking:   m.Thinking,
			Images:     append([]string(nil), m.Images...),
			ToolCallID: m.ToolCallID,
			ToolName:   m.ToolName,
		}
		for i, tc := range m.ToolCalls {
			otc := OllamaToolCall{ID: tc.ID, Type: "function"}
			otc.Function.Index = i
			otc.Function.Name = tc.Name
			otc.Function.Arguments = decodeJSONObjectMap(tc.Arguments)
			om.ToolCalls = append(om.ToolCalls, otc)
		}
		msgs = append(msgs, om)
	}
	return msgs
}

func (p *OllamaNativeProvider) toOllamaTools(tools []ToolDefinition) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			},
		})
	}
	return out
}

func (p *OllamaNativeProvider) buildOptions(req InferenceRequest) map[string]any {
	effectiveCtx := req.ContextLength
	if effectiveCtx < 4096 && p.Client != nil {
		effectiveCtx = p.Client.ContextLength
	}
	if effectiveCtx < 4096 {
		effectiveCtx = contextForModel(req.Model)
	}
	if p.Client != nil && isLocalHostURL(p.Client.BaseURL) {
		effectiveCtx = computeDynamicLocalContext(Config{
			ContextLength: effectiveCtx,
			OllamaURL:     p.Client.BaseURL,
		}, detectLocalVRAMMB())
	}
	numPredict := req.MaxTokens
	if numPredict <= 0 {
		numPredict = computeDynamicNumPredict(effectiveCtx)
	}
	temp := 0.1
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	return map[string]any{
		"temperature":    temp,
		"top_p":          0.9,
		"repeat_penalty": 1.08,
		"num_ctx":        effectiveCtx,
		"num_predict":    numPredict,
	}
}

func ollamaErrorIndicatesNoToolSupport(status int, body string) bool {
	if status != http.StatusBadRequest && status != http.StatusInternalServerError && status != http.StatusUnprocessableEntity {
		return false
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "does not support tools") ||
		strings.Contains(lower, "tools is not supported") ||
		strings.Contains(lower, "unsupported parameter: tools")
}

func (p *OllamaNativeProvider) Complete(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	return p.run(ctx, req, false, nil)
}

func (p *OllamaNativeProvider) Stream(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	return p.run(ctx, req, true, onDelta)
}

func (p *OllamaNativeProvider) run(ctx context.Context, req InferenceRequest, stream bool, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	if p.Client == nil {
		return nil, errors.New("ollama native provider: client is nil")
	}
	msgs := p.toOllamaMessages(req)
	tools := p.toOllamaTools(req.Tools)
	opts := p.buildOptions(req)

	chatReq := OllamaChatRequest{
		Model:     req.Model,
		Messages:  msgs,
		Stream:    stream,
		Tools:     tools,
		Think:     thinkingModeForModel(req.Model),
		KeepAlive: "30m",
		Options:   opts,
	}
	if len(tools) == 0 && req.JSONSchema != nil {
		chatReq.Format = fastOllamaFormat(req.JSONSchema)
	}

	resp, err := p.postChat(ctx, chatReq, req.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		_ = resp.Body.Close()
		bodyText := strings.TrimSpace(string(body))
		// Transparent fallback to flat JSON format if the model does not support Ollama native tools.
		if len(tools) > 0 && ollamaErrorIndicatesNoToolSupport(resp.StatusCode, bodyText) {
			fallbackContent, fbErr := p.Client.ChatStream(ctx, req.Model, msgs, req.JSONSchema, func(acc string, _ int, _ float64) {
				if onDelta != nil {
					onDelta(StreamDelta{Text: acc})
				}
			})
			if fbErr != nil {
				return nil, fbErr
			}
			if onDelta != nil {
				onDelta(StreamDelta{Done: true})
			}
			return &InferenceResponse{
				Content:    fallbackContent,
				StopReason: "stop",
				Model:      req.Model,
				Provider:   p.Name(),
			}, nil
		}
		return nil, fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, truncateText(bodyText, 1500))
	}
	defer resp.Body.Close()

	out := &InferenceResponse{Model: req.Model, Provider: p.Name()}
	if stream {
		var textBuf, thinkBuf strings.Builder
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, 16<<20))
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var chunk OllamaChatResponse
			if json.Unmarshal([]byte(line), &chunk) != nil {
				continue
			}
			if chunk.Error != "" {
				return nil, errors.New(chunk.Error)
			}
			if chunk.Message.Content != "" {
				textBuf.WriteString(chunk.Message.Content)
				if onDelta != nil {
					onDelta(StreamDelta{Text: chunk.Message.Content})
				}
			}
			if chunk.Message.Thinking != "" {
				thinkBuf.WriteString(chunk.Message.Thinking)
				if onDelta != nil {
					onDelta(StreamDelta{Thinking: chunk.Message.Thinking})
				}
			}
			for i, otc := range chunk.Message.ToolCalls {
				argsBytes, _ := json.Marshal(otc.Function.Arguments)
				id := strings.TrimSpace(otc.ID)
				if id == "" {
					id = fmt.Sprintf("call_ollama_%d", len(out.ToolCalls)+i+1)
				}
				tc := ToolCall{
					ID:        id,
					Name:      otc.Function.Name,
					Arguments: normalizeRawJSONObject(argsBytes),
				}
				out.ToolCalls = append(out.ToolCalls, tc)
				if onDelta != nil {
					tcCopy := tc
					onDelta(StreamDelta{ToolCallDelta: &tcCopy})
				}
			}
			if chunk.Done {
				out.StopReason = chunk.DoneReason
				out.Usage = UsageMetrics{
					InputTokens:  chunk.PromptEvalCount,
					OutputTokens: chunk.EvalCount,
					TotalTokens:  chunk.PromptEvalCount + chunk.EvalCount,
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		out.Content = textBuf.String()
		out.Thinking = thinkBuf.String()
		if onDelta != nil {
			u := out.Usage
			onDelta(StreamDelta{Done: true, Usage: &u})
		}
	} else {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		if err != nil {
			return nil, err
		}
		var parsed OllamaChatResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("ollama decode: %w", err)
		}
		if parsed.Error != "" {
			return nil, errors.New(parsed.Error)
		}
		out.Content = parsed.Message.Content
		out.Thinking = parsed.Message.Thinking
		out.StopReason = parsed.DoneReason
		out.Usage = UsageMetrics{
			InputTokens:  parsed.PromptEvalCount,
			OutputTokens: parsed.EvalCount,
			TotalTokens:  parsed.PromptEvalCount + parsed.EvalCount,
		}
		for i, otc := range parsed.Message.ToolCalls {
			argsBytes, _ := json.Marshal(otc.Function.Arguments)
			id := strings.TrimSpace(otc.ID)
			if id == "" {
				id = fmt.Sprintf("call_ollama_%d", i+1)
			}
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:        id,
				Name:      otc.Function.Name,
				Arguments: normalizeRawJSONObject(argsBytes),
			})
		}
	}

	// If no native tool_calls were returned and flat JSON was embedded, normalize cleanly.
	if len(out.ToolCalls) == 0 && len(tools) > 0 {
		if obj := extractJSONObject(out.Content); obj != "" {
			out.Content = obj
		} else if obj := extractJSONObject(out.Thinking); obj != "" {
			out.Content = obj
		}
	}
	return out, nil
}

func (p *OllamaNativeProvider) postChat(ctx context.Context, chatReq OllamaChatRequest, extraHeaders map[string]string) (*http.Response, error) {
	data, err := json.Marshal(chatReq)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(p.Client.BaseURL, "/") + "/api/chat"
	return executeProviderHTTP(ctx, p.Client.HTTP, 3, func() (*http.Request, error) {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		p.Client.applyAuth(httpReq)
		for k, v := range extraHeaders {
			httpReq.Header.Set(k, v)
		}
		return httpReq, nil
	})
}

// ---------------------------------------------------------------------------
// 5. ProviderRouter & ResolveInferenceProvider(cfg)
// ---------------------------------------------------------------------------

type ProviderRouter struct {
	mu        sync.RWMutex
	overrides map[string]NativeInferenceProvider
}

func NewProviderRouter() *ProviderRouter {
	return &ProviderRouter{
		overrides: make(map[string]NativeInferenceProvider),
	}
}

func (r *ProviderRouter) Register(name string, provider NativeInferenceProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.overrides == nil {
		r.overrides = make(map[string]NativeInferenceProvider)
	}
	r.overrides[strings.ToLower(strings.TrimSpace(name))] = provider
}

func configEnvLookup(cfg Config, keys ...string) string {
	for _, k := range keys {
		if cfg.EnvironmentVars != nil {
			if v := strings.TrimSpace(cfg.EnvironmentVars[k]); v != "" {
				return v
			}
		}
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func (r *ProviderRouter) Resolve(cfg Config, model string) NativeInferenceProvider {
	if strings.TrimSpace(model) == "" {
		model = strings.TrimSpace(cfg.LastModel)
	}
	if strings.TrimSpace(model) == "" {
		model = strings.TrimSpace(cfg.OllamaDefaultModel)
	}
	lowerModel := strings.ToLower(strings.TrimSpace(model))
	explicit := strings.ToLower(configEnvLookup(cfg, "LOCALCODE_PROVIDER"))
	if explicit == "" {
		cfgProv := strings.ToLower(strings.TrimSpace(cfg.InferenceProvider))
		if cfgProv != "" && cfgProv != "auto" {
			explicit = cfgProv
		}
	}

	providerKey := ""
	switch explicit {
	case "anthropic", "claude":
		providerKey = "anthropic"
	case "openai":
		providerKey = "openai"
	case "gemini", "google":
		providerKey = "gemini"
	case "llamacpp", "llama.cpp", "llama-server":
		providerKey = "llamacpp"
	case "ollama":
		providerKey = "ollama"
	}

	if providerKey == "" {
		switch {
		case strings.HasPrefix(lowerModel, "anthropic/") || strings.HasPrefix(lowerModel, "claude-"):
			if configEnvLookup(cfg, "ANTHROPIC_API_KEY") != "" || strings.HasPrefix(lowerModel, "anthropic/") {
				providerKey = "anthropic"
			}
		case strings.HasPrefix(lowerModel, "openai/") ||
			(strings.HasPrefix(lowerModel, "gpt-") && !strings.HasPrefix(lowerModel, "gpt-oss")) ||
			strings.HasPrefix(lowerModel, "o1-") || strings.HasPrefix(lowerModel, "o3-") || strings.HasPrefix(lowerModel, "o4-"):
			if configEnvLookup(cfg, "OPENAI_API_KEY") != "" || strings.HasPrefix(lowerModel, "openai/") {
				providerKey = "openai"
			}
		case strings.HasPrefix(lowerModel, "gemini/") || strings.HasPrefix(lowerModel, "google/") || strings.HasPrefix(lowerModel, "gemini-"):
			if configEnvLookup(cfg, "GEMINI_API_KEY", "GOOGLE_API_KEY") != "" || strings.HasPrefix(lowerModel, "gemini/") || strings.HasPrefix(lowerModel, "google/") {
				providerKey = "gemini"
			}
		case strings.HasPrefix(lowerModel, "llamacpp/") || strings.HasPrefix(lowerModel, "llama.cpp/"):
			providerKey = "llamacpp"
		case configEnvLookup(cfg, "LOCALCODE_LLAMACPP_URL") != "":
			providerKey = "llamacpp"
		case strings.Contains(strings.ToLower(cfg.OllamaURL), "llama") ||
			(isLocalHostURL(cfg.OllamaURL) && strings.Contains(cfg.OllamaURL, ":8080") && !cfg.ComputeMeshEnabled):
			providerKey = "llamacpp"
		default:
			providerKey = "ollama"
		}
	}

	if providerKey == "" {
		providerKey = "ollama"
	}

	if r != nil {
		r.mu.RLock()
		custom, ok := r.overrides[providerKey]
		r.mu.RUnlock()
		if ok && custom != nil {
			return custom
		}
	}

	switch providerKey {
	case "anthropic":
		return NewAnthropicProvider(
			configEnvLookup(cfg, "ANTHROPIC_API_KEY"),
			configEnvLookup(cfg, "ANTHROPIC_BASE_URL"),
		)
	case "openai":
		p := NewOpenAIProvider(
			configEnvLookup(cfg, "OPENAI_API_KEY"),
			configEnvLookup(cfg, "OPENAI_BASE_URL"),
		)
		if strings.EqualFold(configEnvLookup(cfg, "OPENAI_USE_RESPONSES_API"), "true") ||
			configEnvLookup(cfg, "OPENAI_USE_RESPONSES_API") == "1" {
			p.UseResponsesAPI = true
		}
		return p
	case "gemini":
		return NewGeminiProvider(
			configEnvLookup(cfg, "GEMINI_API_KEY", "GOOGLE_API_KEY"),
			configEnvLookup(cfg, "GEMINI_BASE_URL"),
		)
	case "llamacpp":
		u := configEnvLookup(cfg, "LOCALCODE_LLAMACPP_URL")
		if u == "" {
			u = cfg.OllamaURL
		}
		token := configEnvLookup(cfg, "LOCALCODE_LLAMACPP_API_KEY", "LLAMACPP_API_KEY")
		return NewLlamaCppBackend(u, token)
	default:
		client := NewOllamaClient()
		if strings.TrimSpace(cfg.OllamaURL) != "" {
			client.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.OllamaURL), "/")
		}
		if cfg.ContextLength >= 4096 {
			client.ContextLength = cfg.ContextLength
		}
		if cfg.ComputeMeshEnabled && strings.TrimSpace(cfg.ComputeMeshAPIKey) != "" {
			client.APIKey = strings.TrimSpace(cfg.ComputeMeshAPIKey)
		}
		return NewOllamaNativeProvider(client)
	}
}

var defaultProviderRouter = NewProviderRouter()

// ResolveInferenceProvider resolves the appropriate NativeInferenceProvider
// for the active configuration (Anthropic, OpenAI, Gemini, llama.cpp, or Ollama).
func ResolveInferenceProvider(cfg Config) NativeInferenceProvider {
	return defaultProviderRouter.Resolve(cfg, cfg.LastModel)
}
