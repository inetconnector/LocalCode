// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventID int, eventType string, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["schema"]; !ok {
		payload["schema"] = "chat.run.events.v1"
	}
	payload["type"] = eventType
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", eventID, eventType, encoded)
	flusher.Flush()
}

func resolveProjectForRun(state *AppState, requested string) string {
	cand := strings.TrimSpace(requested)
	if cand == "" {
		state.mu.RLock()
		cand = strings.TrimSpace(state.Project)
		if cand == "" {
			cand = strings.TrimSpace(state.Config.RootProjectDir)
		}
		state.mu.RUnlock()
	}
	if cand != "" {
		if info, err := os.Stat(cand); err == nil && info.IsDir() {
			return cand
		}
	}
	return ""
}

// handleChatRunsSSE exposes POST /api/chat/runs using the unified
// `chat.run.events.v1` SSE schema expected by NomaMica Studio.
func (s *Server) handleChatRunsSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var req struct {
		Schema    string `json:"schema"`
		Prompt    string `json:"prompt"`
		TurnID    string `json:"turn_id"`
		SessionID string `json:"session_id"`
		Project   string `json:"project"`
		Model     string `json:"model"`
	}
	if err := readJSON(io.LimitReader(r.Body, 256*1024), &req); err != nil {
		http.Error(w, "invalid JSON request: "+err.Error(), http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	project := resolveProjectForRun(s.state, req.Project)
	if project == "" {
		http.Error(w, "no valid project directory configured", http.StatusBadRequest)
		return
	}

	turnID := strings.TrimSpace(req.TurnID)
	if turnID == "" {
		turnID = "turn-" + newID()
	}

	ch := s.state.Subscribe()
	defer s.state.Unsubscribe(ch)

	if err := s.state.StartAgentForThread(prompt, strings.TrimSpace(req.Model), nil, project, ""); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	s.state.mu.RLock()
	runID := s.state.RunID
	model := s.state.Model
	s.state.mu.RUnlock()
	if runID == "" {
		runID = "run-" + newID()
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	seq := 1
	writeSSEEvent(w, flusher, seq, "run.started", map[string]any{
		"runId":  runID,
		"turnId": turnID,
		"model":  model,
	})
	seq++

	maxWait := time.NewTimer(5 * time.Minute)
	defer maxWait.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	sawFinal := false
	for {
		select {
		case <-r.Context().Done():
			return
		case <-maxWait.C:
			writeSSEEvent(w, flusher, seq, "run.failed", map[string]any{
				"runId":  runID,
				"turnId": turnID,
				"error":  "run stream timed out",
			})
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-ch:
			switch ev.Type {
			case "action_running":
				toolName := ev.Action
				if toolName == "" {
					toolName = ev.Message
				}
				writeSSEEvent(w, flusher, seq, "tool.started", map[string]any{
					"runId":      runID,
					"turnId":     turnID,
					"toolCallId": ev.ID,
					"tool":       toolName,
					"args":       map[string]any{"path": ev.Path, "command": ev.Command},
				})
				seq++
			case "action_done", "tool_result":
				writeSSEEvent(w, flusher, seq, "tool.completed", map[string]any{
					"runId":      runID,
					"turnId":     turnID,
					"toolCallId": ev.ID,
					"output":     truncateText(ev.Detail, 4000),
				})
				seq++
			case "approval_required":
				writeSSEEvent(w, flusher, seq, "approval.requested", map[string]any{
					"approvalId": ev.ID,
					"tool":       ev.Action,
					"command":    ev.Command,
					"detail":     ev.Detail,
				})
				seq++
			case "progress", "agent_step", "context_compacted":
				delta := strings.TrimSpace(ev.Message)
				if ev.Detail != "" {
					delta += "\n" + truncateText(ev.Detail, 1000)
				}
				if delta != "" {
					writeSSEEvent(w, flusher, seq, "answer.delta", map[string]any{
						"runId":  runID,
						"turnId": turnID,
						"delta":  delta + "\n",
					})
					seq++
				}
			case "error":
				msg := strings.TrimSpace(ev.Message + " " + ev.Detail)
				writeSSEEvent(w, flusher, seq, "answer.delta", map[string]any{
					"runId":  runID,
					"turnId": turnID,
					"delta":  "\n**Error:** " + msg + "\n",
				})
				seq++
				writeSSEEvent(w, flusher, seq, "run.failed", map[string]any{
					"runId":  runID,
					"turnId": turnID,
					"error":  msg,
				})
				return
			case "final", "question":
				if ev.Message != "" {
					writeSSEEvent(w, flusher, seq, "answer.delta", map[string]any{
						"runId":  runID,
						"turnId": turnID,
						"delta":  ev.Message,
					})
					seq++
				}
				writeSSEEvent(w, flusher, seq, "answer.final", map[string]any{
					"runId":  runID,
					"turnId": turnID,
					"text":   ev.Message,
				})
				seq++
				writeSSEEvent(w, flusher, seq, "run.completed", map[string]any{
					"runId":  runID,
					"turnId": turnID,
				})
				sawFinal = true
				return
			case "status":
				if sawFinal {
					return
				}
			}
		}
	}
}

// buildProvidersStatusPayload returns active provider capabilities and boolean key availability.
// Raw API keys or secrets are NEVER included in responses.
func buildProvidersStatusPayload(state *AppState) map[string]any {
	state.mu.RLock()
	cfg := state.Config
	model := state.Model
	state.mu.RUnlock()
	if model != "" {
		cfg.LastModel = model
	}

	active := defaultProviderRouter.Resolve(cfg, model)
	return map[string]any{
		"active_provider":         active.Name(),
		"supports_native_tools":   active.SupportsNativeTools(),
		"supports_prompt_caching": active.SupportsPromptCaching(),
		"configured_mode":         cfg.InferenceProvider,
		"prompt_caching_enabled":  cfg.PromptCachingEnabled,
		"reasoning_effort":        cfg.ReasoningEffort,
		"os_sandbox_enabled":      cfg.OSSandboxEnabled || strings.EqualFold(cfg.SandboxMode, "os"),
		"key_configured": map[string]bool{
			"anthropic": configEnvLookup(cfg, "ANTHROPIC_API_KEY") != "",
			"openai":    configEnvLookup(cfg, "OPENAI_API_KEY") != "",
			"gemini":    configEnvLookup(cfg, "GEMINI_API_KEY", "GOOGLE_API_KEY") != "",
			"llamacpp":  configEnvLookup(cfg, "LOCALCODE_LLAMACPP_URL") != "" || strings.Contains(strings.ToLower(cfg.OllamaURL), "llama"),
			"ollama":    true,
		},
	}
}

// handleProvidersStatus returns active provider capabilities and boolean key availability.
// Raw API keys or secrets are NEVER included in responses.
func (s *Server) handleProvidersStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, buildProvidersStatusPayload(s.state))
}

// handleRelayStatus returns public metadata for any active E2EE relay pairing.
func (s *Server) handleRelayStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	rp := s.state.RelayPairing
	s.state.mu.RUnlock()

	now := time.Now().UTC()
	if rp == nil || now.After(rp.ExpiresAt) {
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"active": false})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"active":      true,
		"session_id":  rp.SessionID,
		"relay_url":   rp.RelayURL,
		"fingerprint": rp.Fingerprint,
		"expires_at":  rp.ExpiresAt,
	})
}

// handleRelayPair starts an E2EE pairing offer (X25519 + HKDF-SHA256 + fingerprint).
func (s *Server) handleRelayPair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RelayURL string `json:"relay_url"`
	}
	if err := readJSON(io.LimitReader(r.Body, 16*1024), &req); err != nil && err != io.EOF {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	relayURL := strings.TrimSpace(req.RelayURL)
	if relayURL == "" {
		relayURL = "https://relay.inetconnector.com"
	}

	kp, err := GenerateRelayKeyPair()
	if err != nil {
		http.Error(w, "generate relay keys: "+err.Error(), http.StatusInternalServerError)
		return
	}
	sid, err := GenerateRelaySessionID()
	if err != nil {
		http.Error(w, "generate relay session: "+err.Error(), http.StatusInternalServerError)
		return
	}
	secret, err := GenerateRelayAuthSecret()
	if err != nil {
		http.Error(w, "generate relay auth secret: "+err.Error(), http.StatusInternalServerError)
		return
	}
	pairingURI, fp, err := GenerateRelayPairingURI(relayURL, sid, kp.Public, secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	exp := now.Add(15 * time.Minute)
	stateMeta := &ActiveRelayPairingState{
		SessionID:   sid,
		RelayURL:    relayURL,
		PairingURI:  pairingURI,
		Fingerprint: fp,
		CreatedAt:   now,
		ExpiresAt:   exp,
	}
	s.state.mu.Lock()
	s.state.RelayPairing = stateMeta
	s.state.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":          true,
		"session_id":  sid,
		"relay_url":   relayURL,
		"pairing_uri": pairingURI,
		"fingerprint": fp,
		"expires_at":  exp,
	})
}

// handleIssueLockscreenToken issues a single-use HMAC-signed lockscreen token
// restricted strictly to "once" or "reject" decisions.
func (s *Server) handleIssueLockscreenToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ApprovalID string `json:"approval_id"`
		Decision   string `json:"decision"`
		DeviceID   string `json:"device_id"`
	}
	if err := readJSON(io.LimitReader(r.Body, 16*1024), &req); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	mgr, err := s.state.EnsureLockscreenTokenManager()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token, expiresAt, err := mgr.IssueActionToken(
		req.ApprovalID,
		LockscreenActionDecision(strings.ToLower(strings.TrimSpace(req.Decision))),
		req.DeviceID,
		time.Now().UTC(),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":          true,
		"token":       token,
		"approval_id": strings.TrimSpace(req.ApprovalID),
		"decision":    strings.ToLower(strings.TrimSpace(req.Decision)),
		"device_id":   strings.TrimSpace(req.DeviceID),
		"expires_at":  expiresAt,
	})
}

const maxVerifyFrameBytes = 8 * 1024 * 1024

// inspectUploadedImageFrame runs deterministic pure-Go screen-health checks
// (blank/black screen & uniform low-contrast frame detection) for uploaded PNG/JPEG frames.
func inspectUploadedImageFrame(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("image payload is empty")
	}
	if len(raw) > maxVerifyFrameBytes {
		return nil, fmt.Errorf("image payload exceeds %d byte limit", maxVerifyFrameBytes)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("unsupported or corrupted image frame: %w", err)
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("image has zero dimensions")
	}

	// Deterministic stride sampling for large frames (<= 256x256 samples).
	stepX := 1
	if w > 256 {
		stepX = w / 256
	}
	stepY := 1
	if h > 256 {
		stepY = h / 256
	}

	var sumLuma, sumSqLuma float64
	var samples, nonDark int
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			r := float64(r16>>8) / 255.0
			g := float64(g16>>8) / 255.0
			b := float64(b16>>8) / 255.0
			luma := 0.2126*r + 0.7152*g + 0.0722*b
			sumLuma += luma
			sumSqLuma += luma * luma
			if luma > 0.04 {
				nonDark++
			}
			samples++
		}
	}
	meanLuma := sumLuma / float64(samples)
	variance := (sumSqLuma / float64(samples)) - (meanLuma * meanLuma)
	if variance < 0 {
		variance = 0
	}
	stdLuma := math.Sqrt(variance)
	nonDarkRatio := float64(nonDark) / float64(samples)

	issues := []string{}
	if nonDarkRatio < 0.01 && stdLuma < 0.015 {
		issues = append(issues, "BLANK_OR_BLACK_SCREEN")
	} else if stdLuma < 0.012 {
		issues = append(issues, "UNIFORM_LOW_CONTRAST_FRAME")
	}

	return map[string]any{
		"schema":         "localcode.visual_frame_check.v1",
		"passed":         len(issues) == 0,
		"width":          w,
		"height":         h,
		"format":         format,
		"mean_luma":      math.Round(meanLuma*10000) / 10000,
		"luma_stddev":    math.Round(stdLuma*10000) / 10000,
		"non_dark_ratio": math.Round(nonDarkRatio*10000) / 10000,
		"issue_codes":    issues,
	}, nil
}

func decodeBase64ImagePayload(encoded string) ([]byte, error) {
	s := strings.TrimSpace(encoded)
	if s == "" {
		return nil, fmt.Errorf("image_base64 is required")
	}
	if strings.HasPrefix(s, "data:") {
		idx := strings.Index(s, ",")
		if idx < 0 {
			return nil, fmt.Errorf("malformed data URL")
		}
		s = s[idx+1:]
	}
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func handleVerifyFrameHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ImageBase64 string `json:"image_base64"`
	}
	// Base64 expands 3 bytes -> 4 chars, plus JSON envelope overhead.
	if err := readJSON(io.LimitReader(r.Body, (maxVerifyFrameBytes*4)/3+65536), &req); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	raw, err := decodeBase64ImagePayload(req.ImageBase64)
	if err != nil {
		http.Error(w, "invalid image_base64: "+err.Error(), http.StatusBadRequest)
		return
	}
	report, err := inspectUploadedImageFrame(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, report)
}

func (s *Server) handleVerifyFrame(w http.ResponseWriter, r *http.Request) {
	handleVerifyFrameHTTP(w, r)
}
