// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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

type pushRegisterRequest struct {
	DeviceID    string `json:"device_id"`
	Provider    string `json:"provider"`
	Endpoint    string `json:"endpoint"`
	DeviceToken string `json:"device_token"`
	ProjectID   string `json:"project_id"`
	BearerToken string `json:"bearer_token"`
	APNsTopic   string `json:"apns_topic"`
	APNsSandbox bool   `json:"apns_sandbox"`
}

func handlePushRegisterHTTP(state *AppState, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req pushRegisterRequest
	if err := readJSON(io.LimitReader(r.Body, 16*1024), &req); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	deviceID := strings.TrimSpace(req.DeviceID)
	if !safePushIdentifierRe.MatchString(deviceID) {
		http.Error(w, "invalid device_id", http.StatusBadRequest)
		return
	}
	providerRaw := strings.ToLower(strings.TrimSpace(req.Provider))
	if providerRaw == "" {
		providerRaw = string(PushProviderUnifiedPush)
	}
	provider := PushProvider(providerRaw)

	target := PushTarget{
		Provider:    provider,
		DeviceID:    deviceID,
		Endpoint:    strings.TrimSpace(req.Endpoint),
		DeviceToken: strings.TrimSpace(req.DeviceToken),
		ProjectID:   strings.TrimSpace(req.ProjectID),
		BearerToken: strings.TrimSpace(req.BearerToken),
		APNsTopic:   strings.TrimSpace(req.APNsTopic),
		APNsSandbox: req.APNsSandbox,
	}

	switch provider {
	case PushProviderUnifiedPush:
		validated, err := validatePushEndpointURL(target.Endpoint)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		target.Endpoint = validated
	case PushProviderFCMv1:
		if target.BearerToken == "" || target.DeviceToken == "" {
			http.Error(w, "fcm_v1 requires bearer_token and device_token", http.StatusBadRequest)
			return
		}
		if target.Endpoint != "" {
			validated, err := validatePushEndpointURL(target.Endpoint)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			target.Endpoint = validated
		} else if target.ProjectID == "" {
			http.Error(w, "fcm_v1 requires project_id or endpoint", http.StatusBadRequest)
			return
		}
	case PushProviderAPNs:
		if target.BearerToken == "" || target.DeviceToken == "" || target.APNsTopic == "" {
			http.Error(w, "apns requires bearer_token, device_token, and apns_topic", http.StatusBadRequest)
			return
		}
		if target.Endpoint != "" {
			validated, err := validatePushEndpointURL(target.Endpoint)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			target.Endpoint = validated
		}
	default:
		http.Error(w, fmt.Sprintf("unsupported push provider %q", providerRaw), http.StatusBadRequest)
		return
	}

	state.mu.Lock()
	if state.PushTargets == nil {
		state.PushTargets = make(map[string]PushTarget)
	}
	state.PushTargets[deviceID] = target
	count := len(state.PushTargets)
	state.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":               true,
		"device_id":        deviceID,
		"provider":         string(provider),
		"registered_count": count,
	})
}

func (s *Server) handleRegisterPushEndpoint(w http.ResponseWriter, r *http.Request) {
	handlePushRegisterHTTP(s.state, w, r)
}

func (s *Server) handleNotifyApprovalPush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ApprovalID  string `json:"approval_id"`
		ProjectName string `json:"project_name"`
		DeviceID    string `json:"device_id"`
	}
	if err := readJSON(io.LimitReader(r.Body, 16*1024), &req); err != nil && err != io.EOF {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.state.mu.RLock()
	approvalID := strings.TrimSpace(req.ApprovalID)
	if approvalID == "" && s.state.Pending != nil {
		approvalID = s.state.Pending.ID
	}
	projectName := strings.TrimSpace(req.ProjectName)
	if projectName == "" {
		projectName = s.state.Project
	}
	deviceFilter := strings.TrimSpace(req.DeviceID)
	targets := make([]PushTarget, 0, len(s.state.PushTargets))
	if deviceFilter != "" {
		if t, ok := s.state.PushTargets[deviceFilter]; ok {
			targets = append(targets, t)
		}
	} else {
		for _, t := range s.state.PushTargets {
			targets = append(targets, t)
		}
	}
	s.state.mu.RUnlock()

	if !safePushIdentifierRe.MatchString(approvalID) {
		http.Error(w, "valid approval_id is required (or an active pending approval)", http.StatusBadRequest)
		return
	}
	if deviceFilter != "" && len(targets) == 0 {
		http.Error(w, "push device_id not registered", http.StatusNotFound)
		return
	}

	mgr, err := s.state.EnsureLockscreenTokenManager()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dispatcher, err := NewPushNotificationDispatcher(mgr, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	delivered := 0
	failed := 0
	errMsgs := []string{}
	ev := ApprovalPushEvent{
		ApprovalID:  approvalID,
		ProjectName: projectName,
		Timestamp:   time.Now().UTC(),
	}
	for _, target := range targets {
		if _, err := dispatcher.SendApprovalNotification(r.Context(), target, ev); err != nil {
			failed++
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", target.DeviceID, err))
			continue
		}
		delivered++
	}

	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":          failed == 0,
		"approval_id": approvalID,
		"delivered":   delivered,
		"failed":      failed,
		"errors":      errMsgs,
	})
}

// dispatchApprovalPushAsync sends Zero-Secret approval push notifications to all
// currently registered in-memory PushTargets whenever an approval becomes pending.
// Runs non-blocking with a strict 5s timeout and spawns zero goroutines if no targets exist.
func (s *AppState) dispatchApprovalPushAsync(approvalID, projectName string) {
	if s == nil || !safePushIdentifierRe.MatchString(strings.TrimSpace(approvalID)) {
		return
	}
	s.mu.RLock()
	if len(s.PushTargets) == 0 {
		s.mu.RUnlock()
		return
	}
	targets := make([]PushTarget, 0, len(s.PushTargets))
	for _, t := range s.PushTargets {
		targets = append(targets, t)
	}
	if strings.TrimSpace(projectName) == "" {
		projectName = s.Project
	}
	s.mu.RUnlock()

	mgr, err := s.EnsureLockscreenTokenManager()
	if err != nil {
		return
	}
	dispatcher, err := NewPushNotificationDispatcher(mgr, nil)
	if err != nil {
		return
	}
	ev := ApprovalPushEvent{
		ApprovalID:  strings.TrimSpace(approvalID),
		ProjectName: projectName,
		Timestamp:   time.Now().UTC(),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, target := range targets {
			_, _ = dispatcher.SendApprovalNotification(ctx, target, ev)
		}
	}()
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

const maxReleaseAPKBytes = 32 * 1024 * 1024

func parseAndroidVersionCode(rawVersion string) (string, int) {
	v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rawVersion), "-debug"))
	if v == "" || v == "dev" {
		// Fall back to VERSION.txt if available near executable or workspace
		for _, candidate := range []string{
			"VERSION.txt",
			filepath.Join("..", "VERSION.txt"),
		} {
			if data, err := os.ReadFile(candidate); err == nil {
				if trimmed := strings.TrimSpace(string(data)); trimmed != "" {
					v = trimmed
					break
				}
			}
		}
	}
	base := v
	if idx := strings.IndexAny(base, "-+"); idx > 0 {
		base = base[:idx]
	}
	parts := strings.Split(base, ".")
	if len(parts) < 3 {
		return v, 0
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || major < 0 || minor < 0 || patch < 0 {
		return v, 0
	}
	return v, major*10000 + minor*100 + patch
}

func resolveReleaseAPKPath() (string, os.FileInfo, error) {
	candidates := []string{}
	if override := strings.TrimSpace(os.Getenv("LOCALCODE_APK_PATH")); override != "" {
		candidates = append(candidates, override)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "LocalCode-Remote.apk"),
			filepath.Join(exeDir, "LocalCode-Remote-debug.apk"),
			filepath.Join(exeDir, "dist", "LocalCode-Remote.apk"),
			filepath.Join(exeDir, "dist", "LocalCode-Remote-debug.apk"),
			filepath.Join(exeDir, "..", "dist", "LocalCode-Remote.apk"),
			filepath.Join(exeDir, "..", "dist", "LocalCode-Remote-debug.apk"),
		)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "dist", "LocalCode-Remote.apk"),
			filepath.Join(cwd, "dist", "LocalCode-Remote-debug.apk"),
			filepath.Join(cwd, "..", "dist", "LocalCode-Remote.apk"),
			filepath.Join(cwd, "..", "dist", "LocalCode-Remote-debug.apk"),
		)
	}
	for _, cand := range candidates {
		cleaned := filepath.Clean(cand)
		if !strings.HasSuffix(strings.ToLower(cleaned), ".apk") {
			continue
		}
		info, err := os.Stat(cleaned)
		if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > maxReleaseAPKBytes {
			continue
		}
		return cleaned, info, nil
	}
	return "", nil, os.ErrNotExist
}

func computeFileSHA256Hex(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxReleaseAPKBytes+1))
	if err != nil {
		return "", 0, err
	}
	if n > maxReleaseAPKBytes {
		return "", 0, fmt.Errorf("APK exceeds %d byte limit", maxReleaseAPKBytes)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func buildAPKUpdateMetadata() map[string]any {
	verStr, verCode := parseAndroidVersionCode(version)
	apkPath, _, err := resolveReleaseAPKPath()
	if err != nil {
		return map[string]any{
			"schema":       "localcode.android_update.v1",
			"available":    false,
			"version":      verStr,
			"version_code": verCode,
		}
	}
	digest, sizeBytes, err := computeFileSHA256Hex(apkPath)
	if err != nil {
		return map[string]any{
			"schema":       "localcode.android_update.v1",
			"available":    false,
			"version":      verStr,
			"version_code": verCode,
		}
	}
	return map[string]any{
		"schema":        "localcode.android_update.v1",
		"available":     true,
		"version":       verStr,
		"version_code":  verCode,
		"sha256":        digest,
		"size_bytes":    sizeBytes,
		"download_path": "/remote/api/apk-update/download",
	}
}
