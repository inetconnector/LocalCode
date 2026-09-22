// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	remotePairingTTL         = 3 * time.Minute
	remotePairingMaxAttempts = 5
	remotePairingMaxBody     = 8 << 10
)

type RemoteServer struct {
	state *AppState
	mux   *http.ServeMux
}

func NewRemoteServer(state *AppState) *RemoteServer {
	s := &RemoteServer{state: state, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *RemoteServer) routes() {
	s.mux.HandleFunc("/", s.handleRemotePage)
	s.mux.HandleFunc("/remote", s.handleRemotePage)
	s.mux.HandleFunc("/remote/", s.handleRemotePage)
	s.mux.HandleFunc("/remote/api/ping", s.handlePing)
	s.mux.HandleFunc("/remote/api/pair", s.handlePair)
	s.mux.HandleFunc("/remote/api/pair-status", s.handlePairStatus)
	s.mux.HandleFunc("/remote/api/unpair", s.withAuth(s.handleUnpair))
	s.mux.HandleFunc("/remote/api/status", s.withAuth(s.handleStatus))
	s.mux.HandleFunc("/remote/api/projects", s.withAuth(s.handleProjects))
	s.mux.HandleFunc("/remote/api/select-project", s.withAuth(s.handleSelectProject))
	s.mux.HandleFunc("/remote/api/threads", s.withAuth(s.handleThreads))
	s.mux.HandleFunc("/remote/api/new-chat", s.withAuth(s.handleNewChat))
	s.mux.HandleFunc("/remote/api/select-chat", s.withAuth(s.handleSelectChat))
	s.mux.HandleFunc("/remote/api/snapshot", s.withAuth(s.handleSnapshot))
	s.mux.HandleFunc("/remote/api/chat", s.withAuth(s.handleChat))
	s.mux.HandleFunc("/remote/api/approve", s.withAuth(s.handleApprove))
	s.mux.HandleFunc("/remote/api/stop", s.withAuth(s.handleStop))
	s.mux.HandleFunc("/remote/api/event-ticket", s.withAuth(s.handleEventTicket))
	s.mux.HandleFunc("/remote/api/events", s.withStreamTicket(s.handleEvents))
}

func (s *RemoteServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(self), microphone=(self), geolocation=(), payment=(), usb=()")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; connect-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if origin := r.Header.Get("Origin"); origin != "" && !sameRequestOrigin(origin, r.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		fetchSite := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site")))
		if fetchSite != "" && fetchSite != "same-origin" && fetchSite != "none" {
			http.Error(w, "forbidden fetch site", http.StatusForbidden)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

func sameRequestOrigin(origin, requestHost string) bool {
	parsed, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return equalHostPort(parsed.Scheme, parsed.Host, requestHost)
}

func equalHostPort(scheme, left, right string) bool {
	lh, lp := splitOptionalPort(left)
	rh, rp := splitOptionalPort(right)
	lp = effectiveOriginPort(scheme, lp)
	rp = effectiveOriginPort(scheme, rp)
	if lp != rp {
		return false
	}
	return strings.EqualFold(strings.Trim(lh, "[]"), strings.Trim(rh, "[]"))
}

func effectiveOriginPort(scheme, port string) string {
	if strings.TrimSpace(port) != "" {
		return strings.TrimSpace(port)
	}
	switch strings.ToLower(strings.TrimSpace(scheme)) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func splitOptionalPort(value string) (string, string) {
	value = strings.TrimSpace(value)
	if host, port, err := net.SplitHostPort(value); err == nil {
		return host, port
	}
	return strings.Trim(value, "[]"), ""
}

func (s *RemoteServer) handleRemotePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/remote" && r.URL.Path != "/remote/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFS.ReadFile("static/remote.html")
	if err != nil {
		http.Error(w, "remote app not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(data)
}

func (s *RemoteServer) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"app": "LocalCode Remote", "version": version, "pairing": s.pairingOpen()})
}

func (s *RemoteServer) pairingOpen() bool {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	return s.state.RemotePairing != nil && time.Now().Before(s.state.RemotePairing.ExpiresAt)
}

func remoteTokenHash(token string) string {
	sum := sha256.Sum256([]byte("localcode-remote-token\x00" + token))
	return hex.EncodeToString(sum[:])
}

func remotePairingHash(code string) string {
	sum := sha256.Sum256([]byte("localcode-remote-pairing\x00" + strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func secureCompareHex(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if len(left) != len(right) || left == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func randomRemoteToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func randomPairingCode() (string, error) {
	// rand.Int avoids the modulo bias of byte%10 while keeping a human-entered
	// numeric code. Eight digits plus a five-attempt cap makes online guessing
	// impractical during the short pairing window.
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%08d", n.Int64()), nil
}

func remoteDeviceName(name string) string {
	name = strings.Join(strings.Fields(strings.TrimSpace(name)), " ")
	if name == "" {
		return "Phone"
	}
	if runes := []rune(name); len(runes) > 80 {
		name = strings.TrimSpace(string(runes[:80]))
	}
	return name
}

func (s *RemoteServer) handlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, remotePairingMaxBody)
	var req struct {
		Code       string `json:"code"`
		DeviceName string `json:"device_name"`
		Auto       bool   `json:"auto"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid pairing request", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Code) != "" {
		token, device, err := s.state.PairRemoteDevice(req.Code, req.DeviceName)
		if err != nil {
			// Keep failures deliberately indistinguishable to avoid turning the pair
			// endpoint into an oracle for code state.
			http.Error(w, "invalid or expired pairing code", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "token": token, "device": remoteDeviceView(device)})
		return
	}

	if req.Auto {
		s.state.mu.RLock()
		autoPairEnabled := s.state.Config.RemoteAutoPair
		s.state.mu.RUnlock()

		if autoPairEnabled {
			token, device, err := s.state.DirectPairRemoteDevice(req.DeviceName)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = writeJSON(w, map[string]any{"ok": true, "token": token, "device": remoteDeviceView(device)})
			return
		}

		clientIP := clientIPFromRequest(r)
		pending, err := s.state.CreatePendingRemotePairing(req.DeviceName, clientIP)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "pending": true, "request_id": pending.ID, "expires_at": pending.ExpiresAt})
		return
	}

	http.Error(w, "invalid or expired pairing code", http.StatusForbidden)
}

func (s *RemoteServer) handlePairStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	reqID := strings.TrimSpace(r.URL.Query().Get("request_id"))
	if reqID == "" {
		http.Error(w, "request_id required", http.StatusBadRequest)
		return
	}
	pending := s.state.GetPendingRemotePairing(reqID)
	if pending == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": false, "status": "expired", "error": "pairing request expired or not found"})
		return
	}

	s.state.mu.RLock()
	status := pending.Status
	token := pending.Token
	device := pending.Device
	doneCh := pending.DoneCh
	expiresAt := pending.ExpiresAt
	s.state.mu.RUnlock()

	if status == "approved" {
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "status": "approved", "token": token, "device": remoteDeviceView(device)})
		return
	}
	if status == "rejected" {
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": false, "status": "rejected", "rejected": true, "error": "pairing rejected by desktop"})
		return
	}
	if time.Now().After(expiresAt) {
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": false, "status": "expired", "expired": true, "error": "pairing request expired"})
		return
	}

	timeout := time.NewTimer(25 * time.Second)
	defer timeout.Stop()

	select {
	case <-r.Context().Done():
		return
	case <-timeout.C:
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "status": "pending", "request_id": reqID})
		return
	case <-doneCh:
		s.state.mu.RLock()
		status = pending.Status
		token = pending.Token
		device = pending.Device
		s.state.mu.RUnlock()
		if status == "approved" {
			w.Header().Set("Content-Type", "application/json")
			_ = writeJSON(w, map[string]any{"ok": true, "status": "approved", "token": token, "device": remoteDeviceView(device)})
			return
		}
		if status == "rejected" {
			w.Header().Set("Content-Type", "application/json")
			_ = writeJSON(w, map[string]any{"ok": false, "status": "rejected", "rejected": true, "error": "pairing rejected by desktop"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": false, "status": status, "expired": status == "expired", "error": "pairing request " + status})
		return
	}
}

func clientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func (s *RemoteServer) handleUnpair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimSpace(r.Header.Get("X-LocalCode-Remote-Token"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	if err := s.state.RevokeRemoteToken(token); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "unpaired": true})
}

func (s *RemoteServer) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.Header.Get("X-LocalCode-Remote-Token"))
		if token == "" {
			auth := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				token = strings.TrimSpace(auth[7:])
			}
		}
		if !s.state.RemoteTokenValid(token) {
			http.Error(w, "remote token required", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *RemoteServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	status := map[string]any{
		"app":                 "LocalCode Remote",
		"version":             version,
		"project":             s.state.Project,
		"root_dir":            cfg.RootProjectDir,
		"model":               s.state.Model,
		"running":             s.state.Running,
		"running_projects":    s.state.GetRunningProjectsLocked(),
		"active_runs_count":   len(s.state.ActiveRuns),
		"pending":             s.state.Pending != nil,
		"current_thread":      s.state.CurrentThread,
		"run_id":              s.state.RunID,
		"run_phase":           s.state.RunPhase,
		"remote_urls":         append([]string(nil), s.state.RemoteURLs...),
		"editing_engine":      cfg.EditingEngine,
		"approval_mode":       cfg.ApprovalMode,
		"resolved_language":   resolvedLanguage(cfg),
		"paired_device_count": len(cfg.RemoteDevices),
	}
	s.state.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, status)
}

func (s *RemoteServer) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	projects, err := listProjects(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"root": cfg.RootProjectDir, "projects": projects})
}

func (s *RemoteServer) handleSelectProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	root := s.state.Config.RootProjectDir
	running := s.state.Running
	s.state.mu.RUnlock()
	if running {
		http.Error(w, "agent is running", http.StatusConflict)
		return
	}
	full, err := ensureWithinRoot(root, req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		http.Error(w, "project directory not found", http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	if err := ensureProjectDocs(full, cfg); err != nil {
		http.Error(w, localizeConfigText(cfg, "Projektdokumentation konnte nicht vorbereitet werden: ", "Project documentation could not be prepared: ")+err.Error(), http.StatusInternalServerError)
		return
	}
	cfg, err = s.state.mutateConfig(func(next *Config) error {
		next.LastProject = full
		return nil
	})
	if err != nil {
		http.Error(w, localizeConfigText(cfg, "Projektauswahl konnte nicht gespeichert werden: ", "Project selection could not be saved: ")+err.Error(), http.StatusInternalServerError)
		return
	}
	s.state.selectProjectThread(full)
	s.state.UpdateProjectState(localizeConfigText(cfg, "Projekt ausgewählt", "Project selected"))
	w.Header().Set("Content-Type", "application/json")
	if err := writeJSON(w, map[string]any{"ok": true, "project": full}); err != nil {
		log.Printf("writing remote project selection response failed: %v", err)
	}
}

func (s *RemoteServer) handleThreads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	current := s.state.CurrentThread
	s.state.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"threads": s.state.threadSummaries(), "current": current})
}

func (s *RemoteServer) handleNewChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Project string `json:"project"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t, err := s.state.NewChat(req.Project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "thread": t})
}

func (s *RemoteServer) handleSelectChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.SelectChat(req.ID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *RemoteServer) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	requestedThread := strings.TrimSpace(r.URL.Query().Get("thread_id"))
	s.state.mu.RLock()
	events := append([]UIEvent(nil), s.state.Events...)
	project := s.state.Project
	model := s.state.Model
	currentThread := s.state.CurrentThread
	running := s.state.Running
	runID := s.state.RunID
	runPhase := s.state.RunPhase
	runStartedAt := s.state.RunStartedAt
	lastProgressAt := s.state.LastProgressAt
	runningProjects := s.state.GetRunningProjectsLocked()
	if requestedThread != "" {
		if t := s.state.Threads[requestedThread]; t != nil && !t.Archived {
			events = append([]UIEvent(nil), t.Events...)
			project = t.Project
			if t.Model != "" {
				model = t.Model
			}
			currentThread = requestedThread
			if activeRun := s.state.getActiveRunForThreadLocked(requestedThread); activeRun != nil {
				running = true
				runID = activeRun.ID
				runPhase = activeRun.Phase
				runStartedAt = activeRun.StartedAt
				lastProgressAt = activeRun.LastProgressAt
			} else if s.state.CurrentThread != requestedThread {
				running = false
				runID = ""
				runPhase = "idle"
				runStartedAt = time.Time{}
				lastProgressAt = time.Time{}
			}
		}
	}
	var pending any
	if s.state.Pending != nil && (requestedThread == "" || s.state.CurrentThread == requestedThread) {
		p := s.state.Pending
		pending = UIEvent{ID: p.ID, ThreadID: s.state.CurrentThread, Type: "approval_required", Message: p.Action.Message, Action: p.Action.Action, Path: p.Action.Path, Command: p.Action.Command, Preview: p.Preview, Timestamp: time.Now()}
	}
	s.state.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"events": events, "project": project, "model": model, "running": running, "running_projects": runningProjects, "pending": pending, "current_thread": currentThread, "run_id": runID, "run_phase": runPhase, "run_started_at": runStartedAt, "last_progress_at": lastProgressAt})
}

func (s *RemoteServer) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Message     string       `json:"message"`
		Model       string       `json:"model"`
		Project     string       `json:"project,omitempty"`
		ThreadID    string       `json:"thread_id,omitempty"`
		Attachments []Attachment `json:"attachments"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	attachments, err := validateAttachments(req.Attachments)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.state.StartAgentForThread(req.Message, req.Model, attachments, req.Project, req.ThreadID); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *RemoteServer) handleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID       string `json:"id"`
		Approve  bool   `json:"approve"`
		Decision string `json:"decision"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	pending := s.state.Pending
	s.state.mu.RUnlock()
	if pending == nil || pending.ID != req.ID {
		http.Error(w, "pending action not found", http.StatusNotFound)
		return
	}
	decision := ApprovalDecision{Approved: req.Approve}
	switch strings.ToLower(strings.TrimSpace(req.Decision)) {
	case "project":
		decision.Approved = true
		decision.Persist = true
		decision.Scope = "project"
	case "global":
		decision.Approved = true
		decision.Persist = true
		decision.Scope = "global"
	case "once", "":
		decision.Approved = req.Approve
	case "reject":
		decision.Approved = false
	default:
		http.Error(w, "invalid approval decision", http.StatusBadRequest)
		return
	}
	select {
	case pending.Result <- decision:
	default:
		http.Error(w, "approval already handled", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *RemoteServer) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	wasRunning := s.state.StopAgent()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "was_running": wasRunning})
}

func (s *RemoteServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Cache-Control", "no-cache")
	ch := s.state.Subscribe()
	defer s.state.Unsubscribe(ch)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *AppState) StartRemotePairing() (string, time.Time, []string, error) {
	code, err := randomPairingCode()
	if err != nil {
		return "", time.Time{}, nil, err
	}
	expires := time.Now().Add(remotePairingTTL)
	s.mu.Lock()
	s.RemotePairing = &RemotePairingState{CodeHash: remotePairingHash(code), ExpiresAt: expires}
	urls := append([]string(nil), s.RemoteURLs...)
	listenAddr := s.RemoteListenAddr
	s.mu.Unlock()
	if len(urls) == 0 {
		port := 32146
		if _, portStr, err := net.SplitHostPort(listenAddr); err == nil {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
				port = p
			}
		}
		for _, ip := range activeLANIPv4Addresses() {
			urls = append(urls, fmt.Sprintf("https://%s:%d/remote", ip, port))
		}
		if len(urls) == 0 {
			urls = []string{fmt.Sprintf("https://127.0.0.1:%d/remote", port)}
		}
	}
	return code, expires, urls, nil
}

func (s *AppState) PairRemoteDevice(code, deviceName string) (string, RemoteDevice, error) {
	codeHash := remotePairingHash(strings.TrimSpace(code))
	now := time.Now()

	// Validate and consume the pairing window before minting a long-lived token.
	// A small failed-attempt budget prevents online brute force on the LAN.
	s.mu.Lock()
	pairing := s.RemotePairing
	if pairing == nil || now.After(pairing.ExpiresAt) {
		s.RemotePairing = nil
		s.mu.Unlock()
		return "", RemoteDevice{}, fmt.Errorf("invalid or expired pairing code")
	}
	if !secureCompareHex(pairing.CodeHash, codeHash) {
		pairing.FailedAttempts++
		if pairing.FailedAttempts >= remotePairingMaxAttempts {
			s.RemotePairing = nil
		}
		s.mu.Unlock()
		return "", RemoteDevice{}, fmt.Errorf("invalid or expired pairing code")
	}
	s.RemotePairing = nil
	s.mu.Unlock()

	token, err := randomRemoteToken()
	if err != nil {
		return "", RemoteDevice{}, err
	}
	device := RemoteDevice{ID: newID(), Name: remoteDeviceName(deviceName), TokenHash: remoteTokenHash(token), PairedAt: now, LastSeenAt: now}
	if _, err := s.mutateConfig(func(cfg *Config) error {
		cfg.RemoteDevices = append(cfg.RemoteDevices, device)
		return nil
	}); err != nil {
		return "", RemoteDevice{}, err
	}
	return token, device, nil
}

func (s *AppState) DirectPairRemoteDevice(deviceName string) (string, RemoteDevice, error) {
	now := time.Now()
	token, err := randomRemoteToken()
	if err != nil {
		return "", RemoteDevice{}, err
	}
	device := RemoteDevice{ID: newID(), Name: remoteDeviceName(deviceName), TokenHash: remoteTokenHash(token), PairedAt: now, LastSeenAt: now}
	if _, err := s.mutateConfig(func(cfg *Config) error {
		cfg.RemoteDevices = append(cfg.RemoteDevices, device)
		return nil
	}); err != nil {
		return "", RemoteDevice{}, err
	}
	return token, device, nil
}

func (s *AppState) CreatePendingRemotePairing(deviceName, clientIP string) (*PendingRemotePairing, error) {
	now := time.Now()
	reqID := newID()
	pending := &PendingRemotePairing{
		ID:         reqID,
		DeviceName: remoteDeviceName(deviceName),
		ClientIP:   clientIP,
		CreatedAt:  now,
		ExpiresAt:  now.Add(60 * time.Second),
		Status:     "pending",
		DoneCh:     make(chan struct{}),
	}
	s.mu.Lock()
	if s.PendingRemotePairings == nil {
		s.PendingRemotePairings = make(map[string]*PendingRemotePairing)
	}
	for id, p := range s.PendingRemotePairings {
		if now.After(p.ExpiresAt) || p.Status != "pending" {
			delete(s.PendingRemotePairings, id)
		}
	}
	s.PendingRemotePairings[reqID] = pending
	s.mu.Unlock()

	data, _ := json.Marshal(map[string]any{
		"id":          reqID,
		"device_name": pending.DeviceName,
		"client_ip":   clientIP,
		"created_at":  now,
		"expires_at":  pending.ExpiresAt,
	})
	s.Broadcast(UIEvent{
		Type:      "remote_pairing_request",
		Message:   pending.DeviceName,
		Action:    reqID,
		Path:      clientIP,
		Detail:    string(data),
		Timestamp: now,
	})
	return pending, nil
}

func (s *AppState) GetPendingRemotePairing(id string) *PendingRemotePairing {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.PendingRemotePairings == nil {
		return nil
	}
	return s.PendingRemotePairings[id]
}

func (s *AppState) DecidePendingRemotePairing(id string, approve bool) error {
	s.mu.Lock()
	if s.PendingRemotePairings == nil {
		s.mu.Unlock()
		return fmt.Errorf("pairing request not found")
	}
	pending, ok := s.PendingRemotePairings[id]
	if !ok || pending == nil {
		s.mu.Unlock()
		return fmt.Errorf("pairing request not found")
	}
	if time.Now().After(pending.ExpiresAt) {
		pending.Status = "expired"
		delete(s.PendingRemotePairings, id)
		s.mu.Unlock()
		select {
		case <-pending.DoneCh:
		default:
			close(pending.DoneCh)
		}
		return fmt.Errorf("pairing request expired")
	}
	if pending.Status != "pending" {
		s.mu.Unlock()
		return fmt.Errorf("pairing request already resolved")
	}
	deviceName := pending.DeviceName
	doneCh := pending.DoneCh
	s.mu.Unlock()

	if !approve {
		s.mu.Lock()
		pending.Status = "rejected"
		s.mu.Unlock()
		select {
		case <-doneCh:
		default:
			close(doneCh)
		}
		return nil
	}

	token, device, err := s.DirectPairRemoteDevice(deviceName)
	if err != nil {
		return err
	}
	s.mu.Lock()
	pending.Status = "approved"
	pending.Token = token
	pending.Device = device
	s.mu.Unlock()
	select {
	case <-doneCh:
	default:
		close(doneCh)
	}
	return nil
}

func (s *AppState) RemoteTokenValid(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	hash := remoteTokenHash(token)
	now := time.Now()

	s.mu.RLock()
	var matched RemoteDevice
	found := false
	for _, device := range s.Config.RemoteDevices {
		if secureCompareHex(device.TokenHash, hash) {
			matched = device
			found = true
			break
		}
	}
	s.mu.RUnlock()
	if !found {
		return false
	}
	if remoteDeviceExpired(matched, now) {
		// Expired credentials are invalid immediately. Best-effort pruning
		// keeps config state bounded without making authentication depend on
		// a successful cleanup write.
		if _, err := s.mutateConfig(func(cfg *Config) error {
			out := cfg.RemoteDevices[:0]
			for _, device := range cfg.RemoteDevices {
				if strings.EqualFold(device.ID, matched.ID) && secureCompareHex(device.TokenHash, hash) && remoteDeviceExpired(device, now) {
					continue
				}
				out = append(out, device)
			}
			cfg.RemoteDevices = out
			return nil
		}); err != nil {
			log.Printf("pruning expired remote device failed: %v", err)
		}
		return false
	}

	if matched.LastSeenAt.IsZero() || now.Sub(matched.LastSeenAt) > time.Minute {
		if _, err := s.mutateConfig(func(cfg *Config) error {
			for i := range cfg.RemoteDevices {
				device := &cfg.RemoteDevices[i]
				if !strings.EqualFold(device.ID, matched.ID) || !secureCompareHex(device.TokenHash, hash) {
					continue
				}
				if remoteDeviceExpired(*device, now) {
					return fmt.Errorf("remote device expired")
				}
				device.LastSeenAt = now
				return nil
			}
			return fmt.Errorf("remote device not found")
		}); err != nil {
			return false
		}
	}
	return true
}

func (s *Server) handleRemotePairing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	listenAddr := s.state.RemoteListenAddr
	cfg := s.state.Config
	s.state.mu.RUnlock()
	if listenAddr == "" || strings.HasPrefix(listenAddr, "127.0.0.1") {
		cfg.RemoteEnabled = true
		cfg.RemoteBindHost = "0.0.0.0"
		if urls, err := startMobileSafeProductionRemoteServer(s.state, cfg); err == nil && len(urls) > 0 {
			_ = saveConfig(cfg)
		}
	}
	code, expires, urls, err := s.state.StartRemotePairing()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.state.mu.RLock()
	fp := s.state.RemoteTLSFingerprint
	s.state.mu.RUnlock()
	pairUrl := ""
	for _, u := range urls {
		if !strings.Contains(u, "127.0.0.1") && !strings.Contains(u, "localhost") {
			pairUrl = u
			break
		}
	}
	if pairUrl == "" && len(urls) > 0 {
		pairUrl = urls[0]
	}
	deepLink := fmt.Sprintf("localcode://pair?url=%s&fp=%s&code=%s", url.QueryEscape(pairUrl), fp, code)
	webLink := fmt.Sprintf("%s#code=%s&fp=%s", pairUrl, code, fp)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":          true,
		"code":        code,
		"expires_at":  expires,
		"remote_urls": urls,
		"fingerprint": fp,
		"deep_link":   deepLink,
		"web_link":    webLink,
	})
}

func (s *Server) handleRemotePairingDecision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		RequestID string `json:"request_id"`
		Approve   bool   `json:"approve"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.state.DecidePendingRemotePairing(req.RequestID, req.Approve); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleRemotePairingPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	now := time.Now()
	pendingList := []map[string]any{}
	if s.state.PendingRemotePairings != nil {
		for _, p := range s.state.PendingRemotePairings {
			if p.Status == "pending" && now.Before(p.ExpiresAt) {
				pendingList = append(pendingList, map[string]any{
					"id":          p.ID,
					"request_id":  p.ID,
					"device_name": p.DeviceName,
					"client_ip":   p.ClientIP,
					"created_at":  p.CreatedAt,
					"expires_at":  p.ExpiresAt,
				})
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "pending": pendingList, "auto_pair": s.state.Config.RemoteAutoPair})
}

func activeLANIPv4Addresses() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := []string{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name + " " + iface.HardwareAddr.String())
		if strings.Contains(name, "virtual") || strings.Contains(name, "wsl") || strings.Contains(name, "loopback") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			value := ip.String()
			if !containsString(out, value) {
				out = append(out, value)
			}
		}
	}
	return out
}

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func remoteURLsForListener(addr net.Addr, bindHost string) []string {
	_, port, _ := net.SplitHostPort(addr.String())
	if port == "" {
		port = "32146"
	}
	bindHost = strings.TrimSpace(bindHost)
	hosts := []string{}
	if bindHost == "" || bindHost == "0.0.0.0" || bindHost == "::" || bindHost == "[::]" {
		hosts = activeLANIPv4Addresses()
	} else {
		hosts = append(hosts, strings.Trim(bindHost, "[]"))
	}
	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1"}
	}
	urls := make([]string, 0, len(hosts))
	for _, host := range hosts {
		urls = append(urls, "http://"+net.JoinHostPort(host, port)+"/remote")
	}
	return urls
}

func startRemoteHTTPServer(state *AppState, cfg Config) ([]string, error) {
	if !cfg.RemoteEnabled {
		return nil, nil
	}
	port := cfg.RemotePort
	if port <= 0 {
		port = 32146
	}
	bindHost := strings.TrimSpace(cfg.RemoteBindHost)
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, fmt.Sprintf("%d", port)))
	if err != nil && port != 0 {
		ln, err = net.Listen("tcp", net.JoinHostPort(bindHost, "0"))
	}
	if err != nil {
		return nil, err
	}
	urls := remoteURLsForListener(ln.Addr(), bindHost)
	state.mu.Lock()
	state.RemoteListenAddr = ln.Addr().String()
	state.RemoteURLs = append([]string(nil), urls...)
	state.mu.Unlock()
	server := &http.Server{
		Handler:           NewRemoteServer(state),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(os.Stderr, "remote http: ", log.LstdFlags),
	}
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("remote HTTP server error: %v", err)
		}
	}()
	return urls, nil
}
