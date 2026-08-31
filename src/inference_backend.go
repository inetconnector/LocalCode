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

// LlamaCppBackend implements InferenceRunner for standalone llama-server / llama.cpp
type LlamaCppBackend struct {
	URL       string
	AuthToken string
	HTTP      *http.Client
}

func NewLlamaCppBackend(rawURL, authToken string) *LlamaCppBackend {
	u := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if u == "" {
		u = "http://127.0.0.1:8080"
	}
	return &LlamaCppBackend{
		URL:       u,
		AuthToken: strings.TrimSpace(authToken),
		HTTP:      &http.Client{Timeout: 60 * time.Second},
	}
}

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

func (l *LlamaCppBackend) Chat(ctx context.Context, model string, messages []OllamaMessage, opts map[string]any) (string, error) {
	type openAIMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type openAIReq struct {
		Model       string          `json:"model"`
		Messages    []openAIMessage `json:"messages"`
		Temperature float64         `json:"temperature,omitempty"`
		Stream      bool            `json:"stream"`
	}

	oaiMsgs := make([]openAIMessage, 0, len(messages))
	for _, m := range messages {
		oaiMsgs = append(oaiMsgs, openAIMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	payload := openAIReq{
		Model:       model,
		Messages:    oaiMsgs,
		Temperature: 0.2,
		Stream:      false,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.URL+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if l.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+l.AuthToken)
	}

	resp, err := l.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama.cpp HTTP %d: %s", resp.StatusCode, truncateText(string(body), 1000))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("invalid llama.cpp completion response: %w", err)
	}

	if out.Error != nil && out.Error.Message != "" {
		return "", errors.New(out.Error.Message)
	}

	if len(out.Choices) == 0 {
		return "", errors.New("llama.cpp returned empty choices")
	}

	content := strings.TrimSpace(out.Choices[0].Message.Content)
	if obj := extractJSONObject(content); obj != "" {
		return obj, nil
	}

	return content, nil
}
