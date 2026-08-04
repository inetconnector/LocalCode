package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type OllamaClient struct {
	BaseURL       string
	HTTP          *http.Client
	ContextLength int
}

type OllamaMessage struct {
	Role     string   `json:"role"`
	Content  string   `json:"content"`
	Thinking string   `json:"thinking,omitempty"`
	Images   []string `json:"images,omitempty"`
}

type OllamaChatRequest struct {
	Model     string          `json:"model"`
	Messages  []OllamaMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	Format    any             `json:"format,omitempty"`
	Think     any             `json:"think,omitempty"`
	KeepAlive string          `json:"keep_alive,omitempty"`
	Options   map[string]any  `json:"options,omitempty"`
}

type OllamaChatResponse struct {
	Message         OllamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	Error           string        `json:"error,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
}

type ollamaTagsResponse struct {
	Models []struct {
		Name       string    `json:"name"`
		Size       int64     `json:"size"`
		ModifiedAt time.Time `json:"modified_at"`
	} `json:"models"`
}

type ollamaShowResponse struct {
	Capabilities []string `json:"capabilities"`
}

type ollamaPullRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

func NewOllamaClient() *OllamaClient {
	return &OllamaClient{
		BaseURL:       firstOllamaCandidate(),
		HTTP:          &http.Client{Timeout: 8 * time.Minute},
		ContextLength: 32768,
	}
}

func firstOllamaCandidate() string {
	candidates := ollamaCandidates()
	if len(candidates) > 0 {
		return candidates[0]
	}
	return "http://127.0.0.1:11434"
}

func ollamaCandidates() []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(raw string) {
		base := normalizeOllamaBaseURL(raw)
		if base == "" || seen[base] {
			return
		}
		seen[base] = true
		out = append(out, base)
	}

	add(os.Getenv("OLLAMA_HOST"))
	add("http://127.0.0.1:11434")
	add("http://localhost:11434")
	return out
}

func normalizeOllamaBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	hostname := strings.TrimSpace(u.Hostname())
	port := u.Port()
	if port == "" {
		port = "11434"
	}
	if hostname == "0.0.0.0" || hostname == "::" || hostname == "[::]" || hostname == "" {
		hostname = "127.0.0.1"
	}
	u.Scheme = "http"
	u.Host = hostname + ":" + port
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

func (o *OllamaClient) Discover(ctx context.Context) error {
	var lastErr error
	seen := map[string]bool{}
	candidates := append([]string{normalizeOllamaBaseURL(o.BaseURL)}, ollamaCandidates()...)
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		if _, err := o.tagsAt(ctx, candidate); err == nil {
			o.BaseURL = candidate
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("keine Ollama-Adresse gefunden")
	}
	return lastErr
}

func (o *OllamaClient) Tags(ctx context.Context) ([]ModelInfo, error) {
	return o.tagsAt(ctx, o.BaseURL)
}

func (o *OllamaClient) tagsAt(ctx context.Context, baseURL string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return nil, fmt.Errorf("ollama tags: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tr ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	models := make([]ModelInfo, 0, len(tr.Models))
	for _, m := range tr.Models {
		models = append(models, ModelInfo{Name: m.Name, Size: m.Size, ModifiedAt: m.ModifiedAt.Format(time.RFC3339)})
	}
	return models, nil
}

func (o *OllamaClient) Show(ctx context.Context, model string) ([]string, error) {
	data, err := json.Marshal(map[string]any{"model": model, "verbose": false})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/api/show", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama show %s: HTTP %d: %s", model, resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1200))
	}
	var out ollamaShowResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Capabilities, nil
}

func hasCapability(capabilities []string, wanted string) bool {
	for _, capability := range capabilities {
		if strings.EqualFold(strings.TrimSpace(capability), wanted) {
			return true
		}
	}
	return false
}

func visionPreference(name string) int {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "qwen3-vl"):
		return 100
	case strings.Contains(name, "gemma3") || strings.Contains(name, "gemma4"):
		return 90
	case strings.Contains(name, "minicpm-v"):
		return 80
	case strings.Contains(name, "llava"):
		return 70
	case strings.Contains(name, "moondream"):
		return 60
	default:
		return 10
	}
}

func (o *OllamaClient) FindVisionModel(ctx context.Context) (string, error) {
	models, err := o.Tags(ctx)
	if err != nil {
		return "", err
	}
	type candidate struct {
		name  string
		score int
	}
	candidates := make([]candidate, 0, len(models))
	for _, model := range models {
		capCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		capabilities, showErr := o.Show(capCtx, model.Name)
		cancel()
		if showErr == nil && hasCapability(capabilities, "vision") {
			candidates = append(candidates, candidate{name: model.Name, score: visionPreference(model.Name)})
		}
	}
	if len(candidates) == 0 {
		return "", errors.New("kein installiertes Ollama-Modell mit Vision-Fähigkeit gefunden")
	}
	best := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.score > best.score {
			best = candidate
		}
	}
	return best.name, nil
}

func (o *OllamaClient) Pull(ctx context.Context, model string) error {
	data, err := json.Marshal(ollamaPullRequest{Model: model, Stream: false})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/api/pull", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 45 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama pull %s: HTTP %d: %s", model, resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1600))
	}
	var result struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if len(strings.TrimSpace(string(body))) > 0 {
		_ = json.Unmarshal(body, &result)
	}
	if result.Error != "" {
		return errors.New(result.Error)
	}
	return nil
}

func (o *OllamaClient) EnsureVisionModel(ctx context.Context) (string, bool, error) {
	if model, err := o.FindVisionModel(ctx); err == nil {
		return model, false, nil
	}
	const fallback = "gemma3:4b"
	if err := o.Pull(ctx, fallback); err != nil {
		return "", false, fmt.Errorf("kein Vision-Modell installiert und %s konnte nicht geladen werden: %w", fallback, err)
	}
	capabilities, err := o.Show(ctx, fallback)
	if err != nil {
		return "", true, err
	}
	if !hasCapability(capabilities, "vision") {
		return "", true, fmt.Errorf("%s wurde geladen, meldet aber keine Vision-Fähigkeit", fallback)
	}
	return fallback, true, nil
}

func (o *OllamaClient) DescribeImages(ctx context.Context, model, userTask string, images []Attachment) (string, error) {
	encoded := make([]string, 0, len(images))
	names := make([]string, 0, len(images))
	for _, image := range images {
		encoded = append(encoded, image.Data)
		names = append(names, image.Name)
	}
	prompt := "Analysiere die beigefügten Bilder präzise für einen Softwareentwicklungs-Agenten. " +
		"Beschreibe sichtbare Benutzeroberflächen, Fehlermeldungen, Quellcode, Diagramme, Dateinamen, Zustände und relevante Details. " +
		"Lies sichtbaren Text möglichst vollständig. Erfinde nichts. Formuliere eine sachliche deutschsprachige Bildanalyse.\n\n" +
		"Dateien: " + strings.Join(names, ", ") + "\nAufgabe des Nutzers: " + strings.TrimSpace(userTask)
	request := OllamaChatRequest{
		Model:  model,
		Stream: false,
		Messages: []OllamaMessage{{
			Role:    "user",
			Content: prompt,
			Images:  encoded,
		}},
		KeepAlive: "30m",
		Options: map[string]any{
			"temperature": 0.1,
			"num_ctx":     12288,
			"num_predict": 3072,
		},
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Vision-Modell HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 2000))
	}
	var out OllamaChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("ungültige Vision-Antwort: %w", err)
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	content := strings.TrimSpace(out.Message.Content)
	if content == "" {
		return "", fmt.Errorf("Vision-Modell %s lieferte keine Bildanalyse", model)
	}
	return content, nil
}

func (o *OllamaClient) EnsureRunning(ctx context.Context) error {
	if err := o.Discover(ctx); err == nil {
		return nil
	}
	path := findOllamaExecutable()
	if path == "" {
		return errors.New("Ollama wurde nicht gefunden. Bitte Ollama unter Windows installieren und starten")
	}
	if err := startOllamaDetached(path); err != nil {
		return fmt.Errorf("Ollama konnte nicht gestartet werden: %w", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
		probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := o.Discover(probeCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("Ollama antwortet nicht. Geprüfte Adressen: %s; letzter Fehler: %w", strings.Join(ollamaCandidates(), ", "), lastErr)
}

func thinkingModeForModel(model string) any {
	name := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(name, "gpt-oss") {
		// Ollama documents GPT-OSS as requiring a reasoning level rather than a bool.
		return "low"
	}
	return false
}

func contextForModel(model string) int {
	name := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(name, "gpt-oss") {
		return 16384
	}
	return 24576
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var probe any
	if json.Unmarshal([]byte(text), &probe) == nil {
		return text
	}
	for start := strings.Index(text, "{"); start >= 0; {
		depth := 0
		inString := false
		escaped := false
		for i := start; i < len(text); i++ {
			c := text[i]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if c == '\\' {
					escaped = true
					continue
				}
				if c == '"' {
					inString = false
				}
				continue
			}
			switch c {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := strings.TrimSpace(text[start : i+1])
					if json.Unmarshal([]byte(candidate), &probe) == nil {
						return candidate
					}
				}
			}
		}
		next := strings.Index(text[start+1:], "{")
		if next < 0 {
			break
		}
		start += next + 1
	}
	return ""
}

func (o *OllamaClient) Chat(ctx context.Context, model string, messages []OllamaMessage, schema any) (string, error) {
	request := OllamaChatRequest{
		Model:     model,
		Messages:  messages,
		Stream:    false,
		Format:    schema,
		Think:     thinkingModeForModel(model),
		KeepAlive: "30m",
		Options: map[string]any{
			"temperature": 0.1,
			"top_p":       0.9,
			"num_ctx": func() int {
				if o.ContextLength >= 4096 {
					return o.ContextLength
				}
				return contextForModel(model)
			}(),
			"num_predict": 8192,
		},
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/api/chat", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 2000))
	}
	var out OllamaChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("ungültige Ollama-Antwort: %w; Body: %s", err, truncateText(string(body), 2000))
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	if content := extractJSONObject(out.Message.Content); content != "" {
		return content, nil
	}
	// Some thinking models can place the structured final action in message.thinking.
	// We never expose the reasoning trace; only a syntactically complete JSON object is accepted.
	if content := extractJSONObject(out.Message.Thinking); content != "" {
		return content, nil
	}
	return "", fmt.Errorf(
		"Ollama lieferte keine verwertbare strukturierte Antwort (Modell=%s, done_reason=%s, content=%d Zeichen, thinking=%d Zeichen, prompt_tokens=%d, output_tokens=%d)",
		model, out.DoneReason, len(strings.TrimSpace(out.Message.Content)), len(strings.TrimSpace(out.Message.Thinking)), out.PromptEvalCount, out.EvalCount,
	)
}
