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
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OllamaClient struct {
	BaseURL               string
	HTTP                  *http.Client
	ContextLength         int
	AuthToken             string
	APIKey                string
	CustomHeaders         map[string]string
	NomaSupermodelEnabled bool
}

func (o *OllamaClient) applyAuth(req *http.Request) {
	if o == nil || req == nil {
		return
	}
	token := strings.TrimSpace(o.AuthToken)
	if token == "" {
		token = strings.TrimSpace(o.APIKey)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if o.NomaSupermodelEnabled {
		req.Header.Set("X-Noma-Supermodel", "1")
	} else {
		req.Header.Set("X-Noma-Supermodel", "0")
	}
	for k, v := range o.CustomHeaders {
		req.Header.Set(k, v)
	}
}

type OllamaToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Index     int            `json:"index,omitempty"`
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type OllamaMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	Thinking   string           `json:"thinking,omitempty"`
	Images     []string         `json:"images,omitempty"`
	ToolCalls  []OllamaToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolName   string           `json:"tool_name,omitempty"`
}

type OllamaChatRequest struct {
	Model      string           `json:"model"`
	Messages   []OllamaMessage  `json:"messages"`
	Stream     bool             `json:"stream"`
	Format     any              `json:"format,omitempty"`
	Tools      []map[string]any `json:"tools,omitempty"`
	Think      any              `json:"think,omitempty"`
	KeepAlive  string           `json:"keep_alive,omitempty"`
	Supermodel *bool            `json:"supermodel,omitempty"`
	Options    map[string]any   `json:"options,omitempty"`
}

type OllamaChatResponse struct {
	Message         OllamaMessage `json:"message"`
	Done            bool          `json:"done"`
	DoneReason      string        `json:"done_reason,omitempty"`
	Error           string        `json:"error,omitempty"`
	PromptEvalCount int           `json:"prompt_eval_count,omitempty"`
	EvalCount       int           `json:"eval_count,omitempty"`
	EvalDuration    int64         `json:"eval_duration,omitempty"`
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
		HTTP:          &http.Client{Timeout: 15 * time.Minute},
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
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "http" && scheme != "https" {
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
	u.Scheme = scheme
	u.Host = net.JoinHostPort(hostname, port)
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
	primary, err := o.tagsAt(ctx, o.BaseURL)
	if !isLocalHostURL(o.BaseURL) {
		localEndpoints := []string{"http://127.0.0.1:11434", "http://127.0.0.1:8080"}
		seen := map[string]bool{}
		merged := make([]ModelInfo, 0, len(primary)+8)
		for _, m := range primary {
			if !seen[m.Name] {
				seen[m.Name] = true
				merged = append(merged, m)
			}
		}
		for _, ep := range localEndpoints {
			if localModels, localErr := o.tagsAt(ctx, ep); localErr == nil {
				for _, m := range localModels {
					if !seen[m.Name] {
						seen[m.Name] = true
						merged = append(merged, m)
					}
				}
			}
		}
		if len(merged) > 0 {
			return merged, nil
		}
	}
	return primary, err
}

func (o *OllamaClient) tagsAt(ctx context.Context, baseURL string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	o.applyAuth(req)
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
	o.applyAuth(req)
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
	name = strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(name, "-vl") || strings.Contains(name, "vision"):
		return 100
	case strings.Contains(name, "multimodal") || strings.Contains(name, "vlm") || strings.Contains(name, "ocr"):
		return 85
	default:
		return 50
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
	return o.PullWithProgress(ctx, model, nil)
}

func (o *OllamaClient) PullWithProgress(ctx context.Context, model string, progress func(status string, completed, total int64)) error {
	data, err := json.Marshal(ollamaPullRequest{Model: model, Stream: progress != nil})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/api/pull", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	o.applyAuth(req)
	client := &http.Client{Timeout: 2 * time.Hour}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return fmt.Errorf("ollama pull %s: HTTP %d: %s", model, resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 1600))
	}
	if progress != nil {
		type pullProgressResponse struct {
			Status    string `json:"status"`
			Error     string `json:"error"`
			Completed int64  `json:"completed"`
			Total     int64  `json:"total"`
		}
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 64*1024), 4<<20)
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var update pullProgressResponse
			if err := json.Unmarshal(line, &update); err != nil {
				return fmt.Errorf("invalid Ollama pull progress for %s: %w", model, err)
			}
			if strings.TrimSpace(update.Error) != "" {
				return errors.New(update.Error)
			}
			progress(update.Status, update.Completed, update.Total)
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
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
	fallback := strings.TrimSpace(os.Getenv("LOCALCODE_VISION_MODEL"))
	if fallback == "" {
		fallback = strings.TrimSpace(os.Getenv("OLLAMA_VISION_MODEL"))
	}
	if fallback == "" {
		return "", false, errors.New("kein Vision-Modell in Ollama installiert; installiere ein Vision-fähiges Modell über 'ollama pull <vision-modell>' oder setze LOCALCODE_VISION_MODEL")
	}
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
	describeCtx := 12288
	if isLocalHostURL(o.BaseURL) {
		describeCtx = computeDynamicLocalContext(Config{
			ContextLength: o.ContextLength,
			OllamaURL:     o.BaseURL,
		}, detectLocalVRAMMB())
	}
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
			"num_ctx":     describeCtx,
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
	o.applyAuth(req)
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

func fastOllamaFormat(schema any) any {
	if schema == nil {
		return nil
	}
	if s, ok := schema.(string); ok {
		return s
	}
	if m, ok := schema.(map[string]any); ok {
		if t, ok := m["type"].(string); ok && t == "object" {
			// Complex multi-property JSON schemas trigger llama.cpp's single-threaded CPU
			// GBNF grammar sampler over the full token vocabulary (150k+ logits),
			// dropping GPU utilization to 0% and causing 100x slowdowns. Ollama's built-in
			// "json" format runs at full GPU speed while Go strictly validates all fields.
			return "json"
		}
	}
	return schema
}

type retryTemperatureKey struct{}

func withRetryTemperature(ctx context.Context, temp float64) context.Context {
	if temp <= 0 {
		return ctx
	}
	return context.WithValue(ctx, retryTemperatureKey{}, temp)
}

func temperatureForContext(ctx context.Context, fallback float64) float64 {
	if ctx != nil {
		if v, ok := ctx.Value(retryTemperatureKey{}).(float64); ok && v > 0 {
			return v
		}
	}
	return fallback
}

func (o *OllamaClient) Chat(ctx context.Context, model string, messages []OllamaMessage, schema any) (string, error) {
	return o.ChatStream(ctx, model, messages, schema, nil)
}

const minLocalContextTokens = 8192

var (
	localVRAMOnce   sync.Once
	localVRAMCached int
	gpuVRAMPattern  = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(mib|mb|gib|gb)\b`)
)

func parseGPUVRAMMB(gpuDesc string) int {
	m := gpuVRAMPattern.FindStringSubmatch(strings.TrimSpace(gpuDesc))
	if len(m) < 3 {
		return 0
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil || val <= 0 {
		return 0
	}
	switch strings.ToLower(m[2]) {
	case "gib", "gb":
		return int(val * 1024)
	default:
		return int(val)
	}
}

func detectLocalVRAMMB() int {
	if raw := strings.TrimSpace(os.Getenv("LOCALCODE_DETECTED_VRAM_MB")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	localVRAMOnce.Do(func() {
		localVRAMCached = parseGPUVRAMMB(detectGPU())
	})
	return localVRAMCached
}

func hardwareLocalContextLimit(vramMB int) int {
	switch {
	case vramMB <= 0:
		return 12288
	case vramMB < 8192:
		return 8192
	case vramMB < 20480:
		// 8–16 GB GPUs: cap at 12288 so 14B Q4 + Flash Attention + Q8_0 KV cache
		// fits 100% in dedicated VRAM without WDDM shared-memory PCIe paging.
		return 12288
	default:
		return 32768
	}
}

// computeDynamicLocalContext replaces the legacy 6144 token cap with a
// hardware/VRAM-aware dynamic context ceiling configurable via cfg.ContextLength
// and LOCALCODE_MAX_LOCAL_CTX.
func computeDynamicLocalContext(cfg Config, vramMB int) int {
	hwCap := hardwareLocalContextLimit(vramMB)
	rawEnv := strings.TrimSpace(os.Getenv("LOCALCODE_MAX_LOCAL_CTX"))
	if rawEnv == "" && cfg.EnvironmentVars != nil {
		rawEnv = strings.TrimSpace(cfg.EnvironmentVars["LOCALCODE_MAX_LOCAL_CTX"])
	}
	if rawEnv != "" {
		if envMax, err := strconv.Atoi(rawEnv); err == nil && envMax > 0 {
			hwCap = envMax
		}
	}
	base := cfg.ContextLength
	if base <= 0 {
		base = hwCap
	}
	effective := base
	if effective > hwCap {
		effective = hwCap
	}
	if effective < minLocalContextTokens {
		effective = minLocalContextTokens
	}
	return effective
}

// computeDynamicNumPredict scales local output generation capacity dynamically
// as min(max(4096, effectiveCtx/4), 16384).
func computeDynamicNumPredict(effectiveCtx int) int {
	n := effectiveCtx / 4
	if n < 4096 {
		n = 4096
	}
	if n > 16384 {
		n = 16384
	}
	return n
}

func (o *OllamaClient) ChatStream(ctx context.Context, model string, messages []OllamaMessage, schema any, onDelta func(accumulated string, tokens int, tps float64)) (string, error) {
	streamMode := onDelta != nil
	effectiveCtx := o.ContextLength
	if effectiveCtx < 4096 {
		effectiveCtx = contextForModel(model)
	}
	if isLocalHostURL(o.BaseURL) {
		effectiveCtx = computeDynamicLocalContext(Config{
			ContextLength: effectiveCtx,
			OllamaURL:     o.BaseURL,
		}, detectLocalVRAMMB())
	}
	numPredict := computeDynamicNumPredict(effectiveCtx)
	supermodelFlag := o.NomaSupermodelEnabled
	request := OllamaChatRequest{
		Model:      model,
		Messages:   messages,
		Stream:     streamMode,
		Format:     fastOllamaFormat(schema),
		Think:      thinkingModeForModel(model),
		KeepAlive:  "30m",
		Supermodel: &supermodelFlag,
		Options: map[string]any{
			"temperature":    temperatureForContext(ctx, 0.1),
			"top_p":          0.9,
			"repeat_penalty": 1.08,
			"num_ctx":        effectiveCtx,
			"num_predict":    numPredict,
		},
	}
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	var resp *http.Response
	var body []byte
	var out OllamaChatResponse
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/api/chat", bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		o.applyAuth(req)
		resp, err = o.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(time.Duration(350*(attempt+1)) * time.Millisecond):
				}
				continue
			}
			return "", err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			_, _ = io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(450*(attempt+1)) * time.Millisecond):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			_ = resp.Body.Close()
			if err != nil {
				return "", err
			}
			// If remote/cluster gateway returned an error (e.g. 404 model not found), attempt transparent fallback to local nodes (ComputeMesh 8080 or Ollama 11434)
			if !isLocalHostURL(o.BaseURL) {
				fallbackCandidates := []string{"http://127.0.0.1:8080", "http://127.0.0.1:11434"}
				for _, fb := range fallbackCandidates {
					if strings.TrimRight(strings.ToLower(fb), "/") == strings.TrimRight(strings.ToLower(o.BaseURL), "/") {
						continue
					}
					localReq, localErr := http.NewRequestWithContext(ctx, http.MethodPost, fb+"/api/chat", bytes.NewReader(data))
					if localErr == nil {
						localReq.Header.Set("Content-Type", "application/json")
						localClient := &http.Client{Timeout: 8 * time.Minute}
						if localResp, localDoErr := localClient.Do(localReq); localDoErr == nil {
							localBody, _ := io.ReadAll(io.LimitReader(localResp.Body, 8<<20))
							_ = localResp.Body.Close()
							if localResp.StatusCode == http.StatusOK {
								var localOut OllamaChatResponse
								if json.Unmarshal(localBody, &localOut) == nil && localOut.Error == "" {
									if content := extractJSONObject(localOut.Message.Content); content != "" {
										return content, nil
									}
									if content := extractJSONObject(localOut.Message.Thinking); content != "" {
										return content, nil
									}
									if localOut.Message.Content != "" {
										return strings.TrimSpace(localOut.Message.Content), nil
									}
								}
							}
						}
					}
				}
			}
			return "", fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 2000))
		}
		if streamMode {
			var contentBuf strings.Builder
			var thinkingBuf strings.Builder
			scanner := bufio.NewScanner(io.LimitReader(resp.Body, 16<<20))
			scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
			firstTokenAt := time.Time{}
			lastEmit := time.Time{}
			chunks := 0
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				var chunk OllamaChatResponse
				if uErr := json.Unmarshal([]byte(line), &chunk); uErr != nil {
					continue
				}
				if chunk.Error != "" {
					out.Error = chunk.Error
					break
				}
				if chunk.Message.Content != "" {
					if firstTokenAt.IsZero() {
						firstTokenAt = time.Now()
					}
					contentBuf.WriteString(chunk.Message.Content)
					chunks++
				}
				if chunk.Message.Thinking != "" {
					if firstTokenAt.IsZero() {
						firstTokenAt = time.Now()
					}
					thinkingBuf.WriteString(chunk.Message.Thinking)
				}
				if chunk.Done {
					out.Done = true
					out.DoneReason = chunk.DoneReason
					out.PromptEvalCount = chunk.PromptEvalCount
					out.EvalCount = chunk.EvalCount
					out.EvalDuration = chunk.EvalDuration
				}
				now := time.Now()
				if lastEmit.IsZero() || now.Sub(lastEmit) >= 250*time.Millisecond || chunk.Done {
					tps := 0.0
					if chunk.Done && chunk.EvalDuration > 0 && chunk.EvalCount > 0 {
						tps = float64(chunk.EvalCount) / (float64(chunk.EvalDuration) / 1e9)
					} else if !firstTokenAt.IsZero() && chunks > 1 {
						elapsed := now.Sub(firstTokenAt).Seconds()
						if elapsed > 0.05 {
							tps = float64(chunks) / elapsed
						}
					}
					onDelta(contentBuf.String(), chunks, tps)
					lastEmit = now
				}
			}
			scanErr := scanner.Err()
			_ = resp.Body.Close()
			if scanErr != nil && ctx.Err() != nil {
				return "", ctx.Err()
			}
			out.Message.Content = contentBuf.String()
			out.Message.Thinking = thinkingBuf.String()
		} else {
			body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
			_ = resp.Body.Close()
			if err != nil {
				return "", err
			}
			if err := json.Unmarshal(body, &out); err != nil {
				return "", fmt.Errorf("ungültige Ollama-Antwort: %w; Body: %s", err, truncateText(string(body), 2000))
			}
		}
		break
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
	if trimmed := strings.TrimSpace(out.Message.Content); out.DoneReason == "length" && strings.HasPrefix(trimmed, "{") && strings.Contains(trimmed, `"action"`) {
		return trimmed, nil
	}
	return "", fmt.Errorf(
		"Ollama lieferte keine verwertbare strukturierte Antwort (Modell=%s, done_reason=%s, content=%d Zeichen, thinking=%d Zeichen, prompt_tokens=%d, output_tokens=%d)",
		model, out.DoneReason, len(strings.TrimSpace(out.Message.Content)), len(strings.TrimSpace(out.Message.Thinking)), out.PromptEvalCount, out.EvalCount,
	)
}

func (o *OllamaClient) ChatRaw(ctx context.Context, model string, messages []OllamaMessage, opts map[string]any) (string, error) {
	if o == nil {
		return "", errors.New("Ollama client is nil")
	}
	payload := map[string]any{
		"model":    model,
		"messages": messages,
		"stream":   false,
	}
	if len(opts) > 0 {
		payload["options"] = opts
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var resp *http.Response
	var body []byte
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(o.BaseURL, "/")+"/api/chat", bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		o.applyAuth(req)
		resp, err = o.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(time.Duration(350*(attempt+1)) * time.Millisecond):
				}
				continue
			}
			return "", err
		}
		body, err = io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(450*(attempt+1)) * time.Millisecond):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, truncateText(strings.TrimSpace(string(body)), 2000))
		}
		break
	}
	var out OllamaChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("ungültige Ollama-Antwort: %w; Body: %s", err, truncateText(string(body), 2000))
	}
	if out.Error != "" {
		return "", errors.New(out.Error)
	}
	return strings.TrimSpace(out.Message.Content), nil
}

func isLocalHostURL(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	return strings.Contains(u, "127.0.0.1") || strings.Contains(u, "localhost") || strings.Contains(u, "[::1]")
}
