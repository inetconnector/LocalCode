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
	s.mux.HandleFunc("/remote/api/status", s.withAuth(s.handleStatus))
	s.mux.HandleFunc("/remote/api/projects", s.withAuth(s.handleProjects))
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
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid pairing request", http.StatusBadRequest)
		return
	}
	token, device, err := s.state.PairRemoteDevice(req.Code, req.DeviceName)
	if err != nil {
		// Keep failures deliberately indistinguishable to avoid turning the pair
		// endpoint into an oracle for code state.
		http.Error(w, "invalid or expired pairing code", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "token": token, "device": device})
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
	if requestedThread != "" {
		if t := s.state.Threads[requestedThread]; t != nil && !t.Archived {
			events = append([]UIEvent(nil), t.Events...)
			project = t.Project
			if t.Model != "" {
				model = t.Model
			}
			currentThread = requestedThread
			if s.state.CurrentThread != requestedThread {
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
	_ = writeJSON(w, map[string]any{"events": events, "project": project, "model": model, "running": running, "pending": pending, "current_thread": currentThread, "run_id": runID, "run_phase": runPhase, "run_started_at": runStartedAt, "last_progress_at": lastProgressAt})
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
	s.mu.Unlock()
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
	s.mu.Lock()
	cfg := s.Config
	cfg.RemoteDevices = append(cfg.RemoteDevices, device)
	cfg = normalizeConfig(cfg)
	s.Config = cfg
	if err := saveConfig(cfg); err != nil {
		s.mu.Unlock()
		return "", RemoteDevice{}, err
	}
	s.mu.Unlock()
	return token, device, nil
}

func (s *AppState) RemoteTokenValid(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	hash := remoteTokenHash(token)
	now := time.Now()
	s.mu.Lock()
	for i := range s.Config.RemoteDevices {
		if !secureCompareHex(s.Config.RemoteDevices[i].TokenHash, hash) {
			continue
		}
		if now.Sub(s.Config.RemoteDevices[i].LastSeenAt) > time.Minute {
			s.Config.RemoteDevices[i].LastSeenAt = now
			// Persist synchronously while holding the state lock. This runs at most
			// once per minute per active device and prevents a stale asynchronous
			// whole-config snapshot from overwriting newer settings.
			if err := saveConfig(s.Config); err != nil {
				log.Printf("saving remote-device last-seen failed: %v", err)
			}
		}
		s.mu.Unlock()
		return true
	}
	s.mu.Unlock()
	return false
}

func (s *Server) handleRemotePairing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code, expires, urls, err := s.state.StartRemotePairing()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "code": code, "expires_at": expires, "remote_urls": urls})
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
