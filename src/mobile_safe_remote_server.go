// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	mobileRemoteApprovalMaxBody = 16 << 10
	mobileRemoteEngineMaxBody   = 4 << 10
)

type mobileSafeRemoteHandler struct {
	remote *RemoteServer
}

func (h mobileSafeRemoteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/remote/api/approve" {
		limited := http.MaxBytesReader(w, r.Body, mobileRemoteApprovalMaxBody)
		body, err := io.ReadAll(limited)
		if err != nil {
			http.Error(w, "invalid approval request", http.StatusBadRequest)
			return
		}
		var req struct {
			Decision string `json:"decision"`
		}
		if json.Unmarshal(body, &req) == nil && strings.EqualFold(strings.TrimSpace(req.Decision), "global") {
			http.Error(w, "global approval persistence is not available from the mobile remote", http.StatusForbidden)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	h.remote.ServeHTTP(w, r)
}

func newMobileSafeRemoteServer(state *AppState) *RemoteServer {
	remote := NewRemoteServer(state)
	remote.mux.HandleFunc("/remote/api/project-action", remote.withAuth(remote.handleRemoteProjectAction))
	remote.mux.HandleFunc("/remote/api/project-delete-preview", remote.withAuth(remote.handleRemoteProjectDeletePreview))
	remote.mux.HandleFunc("/remote/api/project-quarantine", remote.withAuth(remote.handleRemoteProjectQuarantineList))
	remote.mux.HandleFunc("/remote/api/project-quarantine-action", remote.withAuth(remote.handleRemoteProjectQuarantineAction))
	remote.mux.HandleFunc("/remote/api/editing-engine", remote.withAuth(remote.handleRemoteEditingEngine))
	return remote
}

func (s *RemoteServer) handleRemoteEditingEngine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, mobileRemoteEngineMaxBody)
	var req struct {
		Engine string `json:"engine"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid editing engine request", http.StatusBadRequest)
		return
	}
	requested := strings.ToLower(strings.TrimSpace(req.Engine))
	engine := normalizeEditingEngine(requested)
	if requested == "" || engine != requested {
		http.Error(w, "unknown editing engine", http.StatusBadRequest)
		return
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if s.state.Running {
		http.Error(w, "agent is running", http.StatusConflict)
		return
	}
	cfg := s.state.Config
	if !codingEngineEnabled(cfg, engine) {
		http.Error(w, "editing engine is disabled", http.StatusConflict)
		return
	}
	updated := cfg
	updated.EditingEngine = engine
	updated = normalizeConfig(updated)
	if err := saveConfig(updated); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.state.Config = updated
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":           true,
		"engine":       engine,
		"display_name": codingEngineDisplayName(engine),
	})
}

func mobileSafeRemoteHTTPHandler(remote *RemoteServer) http.Handler {
	return mobileSafeRemoteHandler{remote: remote}
}

func startMobileSafeRemoteHTTPServer(state *AppState, cfg Config) ([]string, error) {
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
	remote := newMobileSafeRemoteServer(state)
	server := &http.Server{
		Handler:           mobileSafeRemoteHTTPHandler(remote),
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

func startMobileSafeProductionRemoteServer(state *AppState, cfg Config) ([]string, error) {
	if !cfg.RemoteEnabled {
		return nil, nil
	}
	bindHost := strings.TrimSpace(cfg.RemoteBindHost)
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	if !remoteRequiresTLS(bindHost) {
		return startMobileSafeRemoteHTTPServer(state, cfg)
	}

	port := cfg.RemotePort
	if port <= 0 {
		port = 32146
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, strconv.Itoa(port)))
	if err != nil && port != 0 {
		ln, err = net.Listen("tcp", net.JoinHostPort(bindHost, "0"))
	}
	if err != nil {
		return nil, err
	}
	_, actualPortText, splitErr := net.SplitHostPort(ln.Addr().String())
	if splitErr != nil {
		_ = ln.Close()
		return nil, splitErr
	}
	actualPort, err := strconv.Atoi(actualPortText)
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("invalid remote listen port %q: %w", actualPortText, err)
	}
	if err := ensureRemoteFirewallRule(actualPort); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("LAN Remote firewall rule: %w", err)
	}
	pair, fingerprint, err := ensureRemoteTLSCertificate(bindHost)
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("LAN Remote TLS certificate: %w", err)
	}
	urls := secureRemoteURLsForListener(ln.Addr(), bindHost)
	state.mu.Lock()
	state.RemoteListenAddr = ln.Addr().String()
	state.RemoteURLs = append([]string(nil), urls...)
	state.mu.Unlock()

	remote := newMobileSafeRemoteServer(state)
	registerRemoteDiscoveryRoute(remote, fingerprint, urls)
	server := &http.Server{
		Handler:           mobileSafeRemoteHTTPHandler(remote),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(os.Stderr, "remote https: ", log.LstdFlags),
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{pair},
		},
	}
	tlsListener := tls.NewListener(ln, server.TLSConfig)
	if err := startRemoteMDNS(actualPort, bindHost, fingerprint, urls); err != nil {
		log.Printf("LocalCode Remote mDNS advertisement unavailable: %v", err)
	}
	go func() {
		if err := server.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			log.Printf("remote HTTPS server error: %v", err)
		}
	}()
	return urls, nil
}
