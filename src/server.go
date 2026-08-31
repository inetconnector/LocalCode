// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	state           *AppState
	mux             *http.ServeMux
	selectDirectory func(initial, language string) (string, error)
}

var launchTaskWindow = openBrowser

func NewServer(state *AppState) *Server {
	s := &Server{state: state, mux: http.NewServeMux(), selectDirectory: selectDirectory}
	s.routes()
	return s
}

func (s *Server) routes() {
	sub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/", http.FileServer(http.FS(sub)))
	s.mux.HandleFunc("/api/ping", s.handlePing)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/mission-recovery", s.handleMissionRecovery)
	s.mux.HandleFunc("/api/mission-recovery/continue", s.handleMissionRecoveryContinue)
	s.mux.HandleFunc("/api/projects", s.handleProjects)
	s.mux.HandleFunc("/api/select-project", s.handleSelectProject)
	s.mux.HandleFunc("/api/project-action", s.handleProjectAction)
	s.mux.HandleFunc("/api/project-delete-preview", s.handleProjectDeletePreview)
	s.mux.HandleFunc("/api/project-quarantine", s.handleProjectQuarantineList)
	s.mux.HandleFunc("/api/project-quarantine-action", s.handleProjectQuarantineAction)
	s.mux.HandleFunc("/api/root", s.handleRoot)
	s.mux.HandleFunc("/api/browse-root", s.handleBrowseRoot)
	s.mux.HandleFunc("/api/reset-root", s.handleResetRoot)
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/threads", s.handleThreads)
	s.mux.HandleFunc("/api/new-chat", s.handleNewChat)
	s.mux.HandleFunc("/api/select-chat", s.handleSelectChat)
	s.mux.HandleFunc("/api/archive-chat", s.handleArchiveChat)
	s.mux.HandleFunc("/api/rename-chat", s.handleRenameChat)
	s.mux.HandleFunc("/api/duplicate-chat", s.handleDuplicateChat)
	s.mux.HandleFunc("/api/delete-chat", s.handleDeleteChat)
	s.mux.HandleFunc("/api/open-chat-window", s.handleOpenChatWindow)
	s.mux.HandleFunc("/api/stop", s.handleStop)
	s.mux.HandleFunc("/api/force-stop", s.handleForceStop)
	s.mux.HandleFunc("/api/approve", s.handleApprove)
	s.mux.HandleFunc("/api/snapshot", s.handleSnapshot)
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/shutdown", s.handleShutdown)
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/remote/pairing", s.handleRemotePairing)
	s.mux.HandleFunc("/api/remote/devices", s.handleRemoteDevices)
	s.mux.HandleFunc("/api/remote/revoke", s.handleRemoteRevoke)
	s.mux.HandleFunc("/api/mcp/test", s.handleMCPTest)
	s.mux.HandleFunc("/api/mcp/status", s.handleMCPStatus)
	s.mux.HandleFunc("/api/mcp/setup", s.handleMCPSetup)
	s.mux.HandleFunc("/api/open-terminal", s.handleOpenTerminal)
	s.mux.HandleFunc("/api/terminal-command", s.handleTerminalCommand)
	s.mux.HandleFunc("/api/open-project", s.handleOpenProject)
	s.mux.HandleFunc("/api/git-overview", s.handleGitOverview)
	s.mux.HandleFunc("/api/tools", s.handleTools)
	s.mux.HandleFunc("/api/tools/diagnose", s.handleToolDiagnose)
	s.mux.HandleFunc("/api/engines/status", s.handleCodingEngineStatus)
	s.mux.HandleFunc("/api/engines/setup", s.handleCodingEngineSetup)
	s.mux.HandleFunc("/api/engines/undo", s.handleCodingEngineUndo)
	s.mux.HandleFunc("/api/aider/status", s.handleAiderStatus)
	s.mux.HandleFunc("/api/aider/setup", s.handleAiderSetup)
	s.mux.HandleFunc("/api/aider/undo", s.handleAiderUndo)
	s.mux.HandleFunc("/api/mission/recovery", s.handleMissionRecovery)
	s.mux.HandleFunc("/api/mission/recovery/continuation", s.handleMissionRecoveryContinuation)
	s.mux.HandleFunc("/api/mission/knowledge", s.handleMissionKnowledge)
	s.mux.HandleFunc("/api/computemesh/status", s.handleComputeMeshStatus)
	s.mux.HandleFunc("/api/computemesh/autodetect", s.handleComputeMeshAutoDetect)
	s.mux.HandleFunc("/api/computemesh/test", s.handleComputeMeshTest)
	s.mux.HandleFunc("/api/doctor", s.handleDoctor)
}

func loopbackRequestHost(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	} else if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		host = strings.Trim(value, "[]")
	} else if i := strings.LastIndex(value, ":"); i > 0 && !strings.Contains(value[:i], ":") {
		host = value[:i]
	}
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func trustedBrowserOrigin(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "null" {
		return value == ""
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	return loopbackRequestHost(parsed.Host)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), payment=(), usb=()")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; connect-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	if !loopbackRequestHost(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if origin := r.Header.Get("Origin"); origin != "" && !trustedBrowserOrigin(origin) {
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

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"app": "LocalCode", "version": version, "license": "Apache-2.0"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	models, err := s.state.Ollama.Tags(ctx)
	s.state.mu.RLock()
	cfg := s.state.Config
	st := Status{
		Version:            version,
		EditingEngine:      cfg.EditingEngine,
		OllamaOnline:       err == nil,
		OllamaURL:          s.state.Ollama.BaseURL,
		Models:             models,
		SelectedModel:      s.state.Model,
		GPU:                detectGPU(),
		RootDir:            cfg.RootProjectDir,
		Project:            s.state.Project,
		Running:            s.state.Running,
		GitAvailable:       gitAvailable(s.state.Project, cfg),
		MCPCount:           enabledMCPCount(cfg),
		RunID:              s.state.RunID,
		RunPhase:           s.state.RunPhase,
		RunStartedAt:       s.state.RunStartedAt,
		LastProgressAt:     s.state.LastProgressAt,
		ResolvedLanguage:   resolvedLanguage(cfg),
		SystemLanguage:     detectSystemLanguage(),
		SupportedLanguages: append([]string(nil), supportedLanguages...),
	}
	s.state.mu.RUnlock()
	engineStatus := selectedCodingEngineStatus(ctx, cfg)
	st.EngineInstalled = engineStatus.Installed
	st.EngineVersion = engineStatus.Version
	st.EngineExecutable = engineStatus.Executable
	st.EngineAuthenticated = engineStatus.Authenticated
	st.EngineError = engineStatus.Error
	if cfg.ComputeMeshEnabled {
		meshStatus := CheckComputeMeshStatus(ctx, cfg)
		st.ComputeMeshOnline = meshStatus.Online
		st.ComputeMeshURL = meshStatus.URL
		st.ComputeMeshNode = meshStatus.NodeID
		st.ComputeMeshGPU = meshStatus.GPU
		st.ComputeMeshVRAM = meshStatus.VRAMPool
		st.ComputeMeshKeyMasked = meshStatus.ActiveKeyMasked
		st.ComputeMeshError = meshStatus.Error
		if meshStatus.Online && len(meshStatus.Models) > 0 {
			if len(st.Models) == 0 {
				st.Models = meshStatus.Models
			} else {
				seen := map[string]bool{}
				for _, m := range st.Models {
					seen[m.Name] = true
				}
				for _, m := range meshStatus.Models {
					if !seen[m.Name] {
						st.Models = append(st.Models, m)
						seen[m.Name] = true
					}
				}
			}
		}
	}
	if err != nil {
		st.OllamaError = err.Error()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, st)
}

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	projects, err := listProjects(cfg)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"root": cfg.RootProjectDir, "projects": projects, "hidden_projects": listHiddenProjects(cfg)})
}

func (s *Server) handleProjectAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Value  string `json:"value"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	project, err := s.state.ProjectAction(req.Path, req.Action, req.Value)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "project": project})
}

func (s *Server) handleSelectProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	root := s.state.Config.RootProjectDir
	running := s.state.Running
	s.state.mu.RUnlock()
	if running {
		http.Error(w, "agent is running", 409)
		return
	}
	full, err := ensureWithinRoot(root, req.Path)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		http.Error(w, "project directory not found", 400)
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
		log.Printf("writing project selection response failed: %v", err)
	}
}

func (s *Server) applyRoot(root string) (string, error) {
	full, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("Projektwurzel nicht gefunden: %s", full)
	}

	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	if s.state.Running {
		return "", fmt.Errorf("Agent läuft gerade")
	}
	cfg := s.state.Config
	cfg.RootProjectDir = full
	cfg.LastProject = ""
	cfg = normalizeConfig(cfg)
	s.state.Config = cfg
	s.state.Project = ""
	s.state.CurrentThread = ""
	s.state.Events = nil
	s.state.Pending = nil
	s.state.Continuation = nil
	if err := saveConfig(cfg); err != nil {
		return "", err
	}
	return cfg.RootProjectDir, nil
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	full, err := s.applyRoot(req.Path)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "root": full})
}

func (s *Server) handleBrowseRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	s.state.mu.RLock()
	initial := s.state.Config.RootProjectDir
	language := resolvedLanguage(s.state.Config)
	running := s.state.Running
	s.state.mu.RUnlock()
	if running {
		http.Error(w, "Agent läuft gerade", 409)
		return
	}
	selected, err := s.selectDirectory(initial, language)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if selected == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "cancelled": true})
		return
	}
	full, err := s.applyRoot(selected)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "root": full})
}

func (s *Server) handleResetRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	full, err := s.applyRoot(preferredProjectRoot())
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "root": full})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
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
		http.Error(w, err.Error(), 400)
		return
	}
	attachments, err := validateAttachments(req.Attachments)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.state.StartAgentForThread(req.Message, req.Model, attachments, req.Project, req.ThreadID); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleThreads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	s.state.mu.RLock()
	current := s.state.CurrentThread
	s.state.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"threads": s.state.threadSummaries(), "current": current})
}

func (s *Server) handleNewChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Project string `json:"project"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	t, err := s.state.NewChat(req.Project)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "thread": t})
}

func (s *Server) handleSelectChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.state.SelectChat(req.ID); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleArchiveChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ID       string `json:"id"`
		Archived bool   `json:"archived"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.state.ArchiveChat(req.ID, req.Archived); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleRenameChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.state.RenameChat(req.ID, req.Title); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleDuplicateChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	thread, err := s.state.DuplicateChat(req.ID)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "thread": thread})
}

func (s *Server) handleDeleteChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := s.state.DeleteChat(req.ID); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleOpenChatWindow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	thread := s.state.Threads[req.ID]
	s.state.mu.RUnlock()
	if thread == nil || thread.Archived {
		http.Error(w, "Chat nicht gefunden", 404)
		return
	}
	windowURL := "http://" + r.Host + "/?thread=" + url.QueryEscape(req.ID)
	if err := launchTaskWindow(windowURL); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "url": windowURL})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	wasRunning := s.state.StopAgent()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "was_running": wasRunning})
}

func (s *Server) handleForceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	wasRunning := s.state.ForceStopAgent()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "was_running": wasRunning})
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		ID       string `json:"id"`
		Approve  bool   `json:"approve"`
		Decision string `json:"decision"` // reject | once | project | global
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	pending := s.state.Pending
	s.state.mu.RUnlock()
	if pending == nil || pending.ID != req.ID {
		http.Error(w, "pending action not found", 404)
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
		http.Error(w, "invalid approval decision", 400)
		return
	}
	select {
	case pending.Result <- decision:
	default:
		http.Error(w, "approval already handled", 409)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
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

func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
	go func() {
		time.Sleep(300 * time.Millisecond)
		s.state.Close()
		os.Exit(0)
	}()
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
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

func enabledMCPCount(cfg Config) int {
	count := 0
	for _, server := range cfg.MCPServers {
		if server.Enabled {
			count++
		}
	}
	return count
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.state.mu.RLock()
		cfg := s.state.Config
		s.state.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, publicSettingsConfig(cfg))
	case http.MethodPost:
		var patch map[string]json.RawMessage
		if err := readJSONPermissive(r.Body, &patch); err != nil {
			http.Error(w, "Ungültige Einstellungsdaten: "+err.Error(), 400)
			return
		}
		var previous Config
		cfg, err := s.state.mutateConfig(func(current *Config) error {
			previous = *current
			baseData, err := json.Marshal(*current)
			if err != nil {
				return fmt.Errorf("Einstellungen konnten nicht serialisiert werden: %w", err)
			}
			var merged map[string]json.RawMessage
			if err := json.Unmarshal(baseData, &merged); err != nil {
				return fmt.Errorf("Einstellungen konnten nicht zusammengeführt werden: %w", err)
			}
			for key, value := range patch {
				merged[key] = value
			}
			mergedData, err := json.Marshal(merged)
			if err != nil {
				return fmt.Errorf("Einstellungen konnten nicht zusammengeführt werden: %w", err)
			}
			var incoming Config
			if err := json.Unmarshal(mergedData, &incoming); err != nil {
				return fmt.Errorf("Einstellungen enthalten ungültige Werte: %w", err)
			}
			incoming.RootProjectDir = current.RootProjectDir
			incoming.LastProject = current.LastProject
			incoming.LastModel = current.LastModel
			incoming.Port = current.Port
			incoming.RemoteDevices = append([]RemoteDevice(nil), current.RemoteDevices...)
			*current = incoming
			return nil
		})
		if err != nil {
			if isConfigMutationError(err) {
				http.Error(w, err.Error(), http.StatusBadRequest)
			} else {
				http.Error(w, localizeConfigText(previous, "Einstellungen konnten nicht gespeichert werden: ", "Settings could not be saved: ")+err.Error(), http.StatusInternalServerError)
			}
			return
		}
		s.state.mu.Lock()
		if cfg.OllamaURL != "" {
			s.state.Ollama.BaseURL = cfg.OllamaURL
		} else {
			s.state.Ollama.BaseURL = firstOllamaCandidate()
		}
		s.state.Ollama.ContextLength = cfg.ContextLength
		s.state.mu.Unlock()
		defaultMCPManager.Close()
		s.state.UpdateProjectState(localizeConfigText(cfg, "Einstellungen geändert", "Settings changed"))
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "settings": cfg})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) handleMCPTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	project := s.state.Project
	s.state.mu.RUnlock()
	results := map[string]any{}
	for _, server := range cfg.MCPServers {
		if !server.Enabled || (req.Name != "" && !strings.EqualFold(req.Name, server.Name)) {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(server.TimeoutSec)*time.Second)
		out, err := mcpCall(ctx, cfg, project, server.Name, "tools/list", map[string]any{})
		cancel()
		if err != nil {
			results[server.Name] = map[string]any{"ok": false, "error": err.Error()}
		} else {
			results[server.Name] = map[string]any{"ok": true, "tools": json.RawMessage(out)}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"results": results})
}

func (s *Server) handleOpenTerminal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Command string `json:"command"`
		Path    string `json:"path"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
	if strings.TrimSpace(req.Path) != "" {
		resolved, err := ensureWithinRoot(cfg.RootProjectDir, req.Path)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		project = resolved
	}
	if project == "" {
		http.Error(w, "Kein Projekt ausgewählt", 400)
		return
	}
	if strings.TrimSpace(req.Command) != "" {
		if err := commandBlocked(cfg, req.Command); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
	}
	if err := openInteractiveTerminal(project, req.Command, cfg); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleTerminalCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Command string `json:"command"`
		Path    string `json:"path"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
	if strings.TrimSpace(req.Path) != "" {
		resolved, err := ensureWithinRoot(cfg.RootProjectDir, req.Path)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		project = resolved
	}
	if project == "" {
		http.Error(w, "Kein Projekt ausgewählt", 400)
		return
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		http.Error(w, "Befehl fehlt", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.CommandTimeout)*time.Second)
	defer cancel()
	out, err := runProjectCommand(ctx, project, command, cfg)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": err == nil, "output": out, "error": errorText(err)})
}

func (s *Server) handleOpenProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		Target string `json:"target"`
		Path   string `json:"path"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
	if strings.TrimSpace(req.Path) != "" {
		resolved, err := ensureWithinRoot(cfg.RootProjectDir, req.Path)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		project = resolved
	}
	if project == "" {
		http.Error(w, "Kein Projekt ausgewählt", 400)
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = cfg.DefaultOpenTarget
	}
	if err := openProjectTarget(project, target, cfg); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) handleGitOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	enabled := cfg.GitEnabled
	s.state.mu.RUnlock()
	if project == "" {
		http.Error(w, "Kein Projekt ausgewählt", 400)
		return
	}
	if !enabled || !gitAvailable(project, cfg) {
		http.Error(w, "Git ist nicht verfügbar oder deaktiviert", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	status, statusErr := runGit(ctx, project, []string{"status", "--short", "--branch"}, cfg)
	diff, diffErr := runGit(ctx, project, []string{"diff", "--stat"}, cfg)
	logText, logErr := runGit(ctx, project, []string{"log", "-5", "--pretty=format:%h  %s  (%cr)"}, cfg)
	branch, _ := runGit(ctx, project, []string{"branch", "--show-current"}, cfg)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"branch": strings.TrimSpace(branch),
		"status": strings.TrimSpace(status),
		"diff":   strings.TrimSpace(diff),
		"log":    strings.TrimSpace(logText),
		"errors": []string{errorText(statusErr), errorText(diffErr), errorText(logErr)},
	})
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func chooseDefaultModel(models []ModelInfo, previous string) string {
	// qwen2.5-coder:14b is the most reliable installed model for LocalCode's
	// strict structured-action loop. Older versions preferred gpt-oss:20b,
	// which can return a thinking trace without a final structured action.
	for _, m := range models {
		if m.Name == "qwen2.5-coder:14b" && strings.HasPrefix(strings.ToLower(previous), "gpt-oss") {
			return m.Name
		}
	}
	if previous != "" {
		for _, m := range models {
			if m.Name == previous {
				return previous
			}
		}
	}
	preferred := []string{"qwen2.5-coder:14b", "qwen2.5-coder:7b", "gpt-oss:20b"}
	for _, p := range preferred {
		for _, m := range models {
			if m.Name == p {
				return p
			}
		}
	}
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.Name), "coder") {
			return m.Name
		}
	}
	if len(models) > 0 {
		return models[0].Name
	}
	return ""
}

func startHTTPServer(state *AppState, requestedPort int) (string, error) {
	if requestedPort <= 0 {
		requestedPort = 32145
	}
	addr := fmt.Sprintf("127.0.0.1:%d", requestedPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
	}
	server := &http.Server{
		Handler:           NewServer(state),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          log.New(os.Stderr, "http: ", log.LstdFlags),
	}
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()
	return "http://" + ln.Addr().String(), nil
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
	withVersion := r.URL.Query().Get("versions") == "1"
	infos := toolInventory(project, cfg, withVersion)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"tools": infos, "project": project})
}

func (s *Server) handleToolDiagnose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Tool string `json:"tool"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Tool) == "" {
		http.Error(w, "tool is required", http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
	info := discoverTool(project, req.Tool, cfg, true)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, info)
}

func (s *Server) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	project := s.state.Project
	s.state.mu.RUnlock()
	connect := r.URL.Query().Get("connect") == "1"
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	statuses := allMCPStatuses(ctx, cfg, project, connect)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"servers": statuses})
}

func (s *Server) handleMCPSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Name   string `json:"name"`
		Action string `json:"action"`
	}
	if err := readJSON(r.Body, &request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	project := s.state.Project
	s.state.mu.RUnlock()
	index := findMCPServerIndex(cfg, request.Name)
	if index < 0 {
		http.Error(w, "MCP server not found", http.StatusNotFound)
		return
	}
	server := cfg.MCPServers[index]
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()
	var detail string
	var err error
	switch strings.ToLower(strings.TrimSpace(request.Action)) {
	case "enable":
		cfg.MCPServers[index].Enabled = true
	case "disable":
		cfg.MCPServers[index].Enabled = false
		defaultMCPManager.ResetServer(server.Name)
	case "reset":
		defaultMCPManager.ResetServer(server.Name)
		detail = localizeConfigText(cfg, "MCP-Sitzung wurde zurückgesetzt.", "MCP session was reset.")
	case "install":
		cfg, detail, err = installMCPDependency(ctx, project, cfg, server)
	case "authenticate":
		if !strings.EqualFold(server.Preset, "github") {
			err = errors.New("authentication is only managed for GitHub MCP")
			break
		}
		gh := discoverTool(project, "gh", cfg, false)
		if !gh.Available {
			cfg, detail, err = installKnownTool(ctx, project, "gh", cfg)
			if err != nil {
				break
			}
			gh = discoverTool(project, "gh", cfg, false)
		}
		if !gh.Available {
			err = errors.New("GitHub CLI is still unavailable")
			break
		}
		if err = openInteractiveTerminal(project, fmt.Sprintf("\"%s\" auth login", gh.Path), cfg); err == nil {
			detail = localizeConfigText(cfg, "GitHub-Anmeldung wurde in einem interaktiven Terminal geöffnet. Schließe die Anmeldung dort ab und klicke anschließend auf Testen.", "GitHub sign-in was opened in an interactive terminal. Complete sign-in there, then click Test.")
		}
	case "test":
		cfg.MCPServers[index].Enabled = true
		status := mcpServerStatus(ctx, cfg, project, cfg.MCPServers[index], true)
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": status.Connected, "status": status})
		return
	default:
		http.Error(w, "unsupported MCP setup action", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error()+"\n\n"+detail, http.StatusInternalServerError)
		return
	}
	cfg = normalizeConfig(cfg)
	if err := saveConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.state.mu.Lock()
	s.state.Config = cfg
	s.state.mu.Unlock()
	defaultMCPManager.ResetServer(server.Name)
	status := mcpServerStatus(ctx, cfg, project, cfg.MCPServers[index], false)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "detail": detail, "status": status, "settings": cfg})
}

func (s *Server) handleMissionRecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	runID := strings.TrimSpace(r.URL.Query().Get("run_id"))
	if runID == "" {
		s.state.mu.RLock()
		if s.state.Recovery != nil {
			runID = strings.TrimSpace(s.state.Recovery.RunID)
		}
		s.state.mu.RUnlock()
	}
	if runID == "" {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}

	snapshot, err := s.state.MissionRecoveryControlSnapshot(runID)
	if err != nil {
		if errors.Is(err, errMissionRecoveryControlActiveRun) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, errMissionRecoveryControlUnavailable) || errors.Is(err, errMissionRecoveryControlInvalidRequest) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, snapshot)
}

func (s *Server) handleMissionRecoveryContinuation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RunID   string `json:"run_id"`
		TaskID  string `json:"task_id"`
		Preview bool   `json:"preview"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.RunID = strings.TrimSpace(req.RunID)
	req.TaskID = strings.TrimSpace(req.TaskID)
	if req.RunID == "" || req.TaskID == "" {
		http.Error(w, "run_id and task_id are required", http.StatusBadRequest)
		return
	}

	if req.Preview {
		materialized, err := s.state.MissionRecoveryContinuationMaterialization(req.RunID, req.TaskID)
		if err != nil {
			if errors.Is(err, errMissionRecoveryControlActiveRun) {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			if errors.Is(err, errMissionRecoveryContinuationUnavailable) || errors.Is(err, errMissionRecoveryContinuationCandidate) || errors.Is(err, errMissionRecoveryContinuationBudget) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "preview": true, "materialization": materialized})
		return
	}

	execution, err := s.state.RunMissionRecoveryContinuation(r.Context(), req.RunID, req.TaskID)
	if err != nil {
		if errors.Is(err, errMissionRecoveryControlActiveRun) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, errMissionRecoveryContinuationUnavailable) || errors.Is(err, errMissionRecoveryContinuationCandidate) || errors.Is(err, errMissionRecoveryContinuationBudget) || errors.Is(err, errMissionRecoveryAdmissionStale) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "preview": false, "execution": execution})
}

func (s *Server) handleMissionKnowledge(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		missionID := strings.TrimSpace(r.URL.Query().Get("mission_id"))
		category := strings.TrimSpace(r.URL.Query().Get("category"))
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		query := strings.TrimSpace(r.URL.Query().Get("query"))

		items, err := s.state.ListMissionKnowledge(missionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var cat MissionKnowledgeCategory
		if category != "" {
			var catErr error
			cat, catErr = normalizeMissionKnowledgeCategory(category)
			if catErr != nil {
				http.Error(w, catErr.Error(), http.StatusBadRequest)
				return
			}
		}
		filtered := filterMissionKnowledge(items, cat, tag, query)
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "knowledge": filtered, "count": len(filtered)})

	case http.MethodPost:
		var req struct {
			MissionID       string                   `json:"mission_id"`
			Category        MissionKnowledgeCategory `json:"category"`
			Title           string                   `json:"title"`
			Summary         string                   `json:"summary"`
			Tags            []string                 `json:"tags"`
			CreatedByTaskID string                   `json:"created_by_task_id"`
			SourcePath      string                   `json:"source_path"`
		}
		if err := readJSON(r.Body, &req); err != nil {
			http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		item := MissionKnowledgeItem{
			Category:        req.Category,
			Title:           req.Title,
			Summary:         req.Summary,
			Tags:            req.Tags,
			CreatedByTaskID: req.CreatedByTaskID,
			SourcePath:      req.SourcePath,
		}
		recorded, err := s.state.RecordMissionKnowledge(req.MissionID, item)
		if err != nil {
			if errors.Is(err, errMissionKnowledgeInvalidCategory) || errors.Is(err, errMissionKnowledgeMissingTitle) || errors.Is(err, errMissionKnowledgeMissingSummary) || errors.Is(err, errMissionKnowledgeLimitExceeded) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if errors.Is(err, errMissionKnowledgeNotFound) {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "item": recorded})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleComputeMeshStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	status := CheckComputeMeshStatus(ctx, cfg)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "status": status})
}

func (s *Server) handleComputeMeshAutoDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	apiKey, gatewayURL, account, nodeID, localNodeURL, source := AutoDetectComputeMeshCredentials()
	s.state.mu.Lock()
	if apiKey != "" {
		s.state.Config.ComputeMeshEnabled = true
		s.state.Config.ComputeMeshAPIKey = apiKey
		if gatewayURL != "" {
			s.state.Config.ComputeMeshURL = gatewayURL
		}
	}
	if localNodeURL != "" {
		s.state.Config.ComputeMeshLocalNodeURL = localNodeURL
	}
	cfg := s.state.Config
	_ = saveConfig(cfg)
	s.state.mu.Unlock()

	ConfigureComputeMeshForAppState(s.state)

	s.state.mu.RLock()
	cfg = s.state.Config
	s.state.mu.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()

	status := CheckComputeMeshStatus(ctx, cfg)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":       true,
		"status":   status,
		"source":   source,
		"account":  account,
		"node_id":  nodeID,
		"detected": apiKey != "",
	})
}

func (s *Server) handleComputeMeshTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		APIKey string `json:"api_key"`
		URL    string `json:"url"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(cfg.ComputeMeshAPIKey)
	}
	if apiKey == "" {
		detectedKey, _, _, _, _, _ := AutoDetectComputeMeshCredentials()
		apiKey = detectedKey
	}

	gatewayURL := strings.TrimSpace(req.URL)
	if gatewayURL == "" {
		gatewayURL = strings.TrimSpace(cfg.ComputeMeshURL)
	}
	if gatewayURL == "" {
		gatewayURL = defaultComputeMeshGatewayURL
	}

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "qwen/qwen2.5-7b-instruct"
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = "Hallo ComputeMesh! Bestätige bitte kurz deine GPU-Bereitschaft."
	}

	client := &OllamaClient{
		BaseURL:   gatewayURL,
		AuthToken: apiKey,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	reply, err := client.ChatRaw(ctx, model, []OllamaMessage{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{
			"ok":    false,
			"error": err.Error(),
			"model": model,
			"url":   gatewayURL,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":       true,
		"response": reply,
		"model":    model,
		"url":      gatewayURL,
	})
}

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	report := RunDoctorDiagnostics(ctx, cfg)
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, report)
}
