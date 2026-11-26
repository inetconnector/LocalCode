// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type InferenceBackendType string

const (
	InferenceBackendComputeMesh InferenceBackendType = "computemesh"
	InferenceBackendOllama      InferenceBackendType = "ollama"
	InferenceBackendLlamaCpp    InferenceBackendType = "llamacpp"
	InferenceBackendOpenAIComp  InferenceBackendType = "openai_compatible"
)

type InferenceRunner interface {
	BackendType() InferenceBackendType
	BaseURL() string
	Tags(ctx context.Context) ([]ModelInfo, error)
	Chat(ctx context.Context, model string, messages []OllamaMessage, opts map[string]any) (string, error)
	Health(ctx context.Context) (online bool, latencyMs int64, err error)
}

// LlamaCppBackend implements InferenceRunner and NativeInferenceProvider for
// standalone llama-server / llama.cpp with KV-cache prompt reuse and native tool calling.
type LlamaCppBackend struct {
	URL        string
	AuthToken  string
	HTTP       *http.Client
	MaxRetries int
}

func NewLlamaCppBackend(rawURL, authToken string) *LlamaCppBackend {
	u := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if u == "" {
		u = "http://127.0.0.1:8080"
	}
	return &LlamaCppBackend{
		URL:        u,
		AuthToken:  strings.TrimSpace(authToken),
		HTTP:       &http.Client{Timeout: 5 * time.Minute},
		MaxRetries: 3,
	}
}

func (l *LlamaCppBackend) Name() string                { return "llamacpp" }
func (l *LlamaCppBackend) SupportsNativeTools() bool   { return true }
func (l *LlamaCppBackend) SupportsPromptCaching() bool { return true }

func (l *LlamaCppBackend) BackendType() InferenceBackendType {
	return InferenceBackendLlamaCpp
}

func (l *LlamaCppBackend) BaseURL() string {
	return l.URL
}

func (l *LlamaCppBackend) Health(ctx context.Context) (bool, int64, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL+"/health", nil)
	if err != nil {
		return false, 0, err
	}
	if l.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+l.AuthToken)
	}
	resp, err := l.HTTP.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		// Fallback to /v1/models
		reqModels, _ := http.NewRequestWithContext(ctx, http.MethodGet, l.URL+"/v1/models", nil)
		if reqModels != nil {
			if l.AuthToken != "" {
				reqModels.Header.Set("Authorization", "Bearer "+l.AuthToken)
			}
			if respModels, errModels := l.HTTP.Do(reqModels); errModels == nil {
				defer respModels.Body.Close()
				return respModels.StatusCode == http.StatusOK, time.Since(start).Milliseconds(), nil
			}
		}
		return false, latency, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK, latency, nil
}

func (l *LlamaCppBackend) Tags(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if l.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+l.AuthToken)
	}

	resp, err := l.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llama.cpp models returned HTTP %d", resp.StatusCode)
	}

	var data struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	models := make([]ModelInfo, 0, len(data.Data))
	for _, m := range data.Data {
		modTime := time.Unix(m.Created, 0).Format(time.RFC3339)
		if m.Created == 0 {
			modTime = time.Now().Format(time.RFC3339)
		}
		models = append(models, ModelInfo{
			Name:       m.ID,
			ModifiedAt: modTime,
		})
	}
	return models, nil
}

func (l *LlamaCppBackend) buildChatPayload(req InferenceRequest, stream bool) map[string]any {
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

	temp := 0.2
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	cachePrompt := promptCacheEnabled(req, true)

	payload := map[string]any{
		"model":        req.Model,
		"messages":     msgs,
		"temperature":  temp,
		"stream":       stream,
		"cache_prompt": cachePrompt,
	}
	if stream {
		payload["stream_options"] = map[string]any{"include_usage": true}
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
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
	return payload
}

func (l *LlamaCppBackend) postCompletions(ctx context.Context, payload map[string]any, extraHeaders map[string]string) (*http.Response, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(l.URL, "/") + "/v1/chat/completions"
	return executeProviderHTTP(ctx, l.HTTP, l.MaxRetries, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if l.AuthToken != "" {
			req.Header.Set("Authorization", "Bearer "+l.AuthToken)
		}
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		return req, nil
	})
}

func (l *LlamaCppBackend) Complete(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	payload := l.buildChatPayload(req, false)
	resp, err := l.postCompletions(ctx, payload, req.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llama.cpp HTTP %d: %s", resp.StatusCode, truncateText(string(body), 1000))
	}

	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role             string `json:"role"`
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
		} `json:"usage"`
		Timings *struct {
			CacheN     int `json:"cache_n"`
			PromptN    int `json:"prompt_n"`
			PredictedN int `json:"predicted_n"`
		} `json:"timings,omitempty"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("invalid llama.cpp completion response: %w", err)
	}
	if out.Error != nil && out.Error.Message != "" {
		return nil, errors.New(out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return nil, errors.New("llama.cpp returned empty choices")
	}

	ch := out.Choices[0]
	tcs := make([]ToolCall, 0, len(ch.Message.ToolCalls))
	for i, tc := range ch.Message.ToolCalls {
		id := strings.TrimSpace(tc.ID)
		if id == "" {
			id = fmt.Sprintf("call_llamacpp_%d", i+1)
		}
		tcs = append(tcs, ToolCall{
			ID:        id,
			Name:      tc.Function.Name,
			Arguments: normalizeRawJSONObject(json.RawMessage(tc.Function.Arguments)),
		})
	}
	cachedTokens := out.Usage.PromptTokensDetails.CachedTokens
	if cachedTokens == 0 && out.Timings != nil && out.Timings.CacheN > 0 {
		cachedTokens = out.Timings.CacheN
	}
	modelName := out.Model
	if modelName == "" {
		modelName = req.Model
	}
	return &InferenceResponse{
		Content:    ch.Message.Content,
		Thinking:   ch.Message.ReasoningContent,
		ToolCalls:  tcs,
		StopReason: ch.FinishReason,
		Usage: UsageMetrics{
			InputTokens:     out.Usage.PromptTokens,
			OutputTokens:    out.Usage.CompletionTokens,
			TotalTokens:     out.Usage.TotalTokens,
			CacheReadTokens: cachedTokens,
		},
		Model:    modelName,
		Provider: l.Name(),
	}, nil
}

func (l *LlamaCppBackend) Stream(ctx context.Context, req InferenceRequest, onDelta func(StreamDelta)) (*InferenceResponse, error) {
	payload := l.buildChatPayload(req, true)
	resp, err := l.postCompletions(ctx, payload, req.ExtraHeaders)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return nil, fmt.Errorf("llama.cpp stream HTTP %d: %s", resp.StatusCode, truncateText(string(body), 1000))
	}

	type accTool struct {
		id   string
		name string
		args strings.Builder
	}
	toolMap := make(map[int]*accTool)
	var textBuf, thinkBuf strings.Builder
	out := &InferenceResponse{Model: req.Model, Provider: l.Name()}

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
			} `json:"usage,omitempty"`
			Timings *struct {
				CacheN int `json:"cache_n"`
			} `json:"timings,omitempty"`
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
			out.Usage.InputTokens = chunk.Usage.PromptTokens
			out.Usage.OutputTokens = chunk.Usage.CompletionTokens
			out.Usage.TotalTokens = chunk.Usage.TotalTokens
			if chunk.Usage.PromptTokensDetails.CachedTokens > 0 {
				out.Usage.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			}
		}
		if chunk.Timings != nil && chunk.Timings.CacheN > 0 && out.Usage.CacheReadTokens == 0 {
			out.Usage.CacheReadTokens = chunk.Timings.CacheN
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
		id := strings.TrimSpace(acc.id)
		if id == "" {
			id = fmt.Sprintf("call_llamacpp_%d", idx+1)
		}
		tc := ToolCall{
			ID:        id,
			Name:      acc.name,
			Arguments: normalizeRawJSONObject(json.RawMessage(acc.args.String())),
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

func (l *LlamaCppBackend) Chat(ctx context.Context, model string, messages []OllamaMessage, opts map[string]any) (string, error) {
	sysPrompt, providerMsgs := providerMessagesFromOllama(messages)
	req := InferenceRequest{
		Model:        model,
		SystemPrompt: sysPrompt,
		Messages:     providerMsgs,
	}
	if opts != nil {
		if t, ok := opts["temperature"].(float64); ok {
			req.Temperature = &t
		}
		if mt, ok := opts["max_tokens"].(int); ok && mt > 0 {
			req.MaxTokens = mt
		}
		if tools, ok := opts["tools"].([]ToolDefinition); ok && len(tools) > 0 {
			req.Tools = tools
		}
	}
	resp, err := l.Complete(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.ToolCalls) > 0 && strings.TrimSpace(resp.Content) == "" {
		raw, _ := json.Marshal(resp.ToolCalls)
		return string(raw), nil
	}
	content := strings.TrimSpace(resp.Content)
	if obj := extractJSONObject(content); obj != "" {
		return obj, nil
	}
	return content, nil
}
