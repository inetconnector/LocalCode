package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	state *AppState
	mux   *http.ServeMux
}

func NewServer(state *AppState) *Server {
	s := &Server{state: state, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	sub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("/", http.FileServer(http.FS(sub)))
	s.mux.HandleFunc("/api/ping", s.handlePing)
	s.mux.HandleFunc("/api/status", s.handleStatus)
	s.mux.HandleFunc("/api/projects", s.handleProjects)
	s.mux.HandleFunc("/api/select-project", s.handleSelectProject)
	s.mux.HandleFunc("/api/root", s.handleRoot)
	s.mux.HandleFunc("/api/browse-root", s.handleBrowseRoot)
	s.mux.HandleFunc("/api/reset-root", s.handleResetRoot)
	s.mux.HandleFunc("/api/chat", s.handleChat)
	s.mux.HandleFunc("/api/threads", s.handleThreads)
	s.mux.HandleFunc("/api/new-chat", s.handleNewChat)
	s.mux.HandleFunc("/api/select-chat", s.handleSelectChat)
	s.mux.HandleFunc("/api/archive-chat", s.handleArchiveChat)
	s.mux.HandleFunc("/api/stop", s.handleStop)
	s.mux.HandleFunc("/api/force-stop", s.handleForceStop)
	s.mux.HandleFunc("/api/approve", s.handleApprove)
	s.mux.HandleFunc("/api/snapshot", s.handleSnapshot)
	s.mux.HandleFunc("/api/events", s.handleEvents)
	s.mux.HandleFunc("/api/shutdown", s.handleShutdown)
	s.mux.HandleFunc("/api/settings", s.handleSettings)
	s.mux.HandleFunc("/api/mcp/test", s.handleMCPTest)
	s.mux.HandleFunc("/api/open-terminal", s.handleOpenTerminal)
	s.mux.HandleFunc("/api/terminal-command", s.handleTerminalCommand)
	s.mux.HandleFunc("/api/open-project", s.handleOpenProject)
	s.mux.HandleFunc("/api/git-overview", s.handleGitOverview)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cache-Control", "no-store")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"app": "LocalCodex", "version": version})
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
	st := Status{
		Version:        version,
		OllamaOnline:   err == nil,
		OllamaURL:      s.state.Ollama.BaseURL,
		Models:         models,
		SelectedModel:  s.state.Model,
		GPU:            detectGPU(),
		RootDir:        s.state.Config.RootProjectDir,
		Project:        s.state.Project,
		Running:        s.state.Running,
		GitAvailable:   gitAvailable(),
		MCPCount:       enabledMCPCount(s.state.Config),
		RunID:          s.state.RunID,
		RunPhase:       s.state.RunPhase,
		RunStartedAt:   s.state.RunStartedAt,
		LastProgressAt: s.state.LastProgressAt,
	}
	s.state.mu.RUnlock()
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
	root := s.state.Config.RootProjectDir
	s.state.mu.RUnlock()
	entries, err := os.ReadDir(root)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	projects := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			projects = append(projects, filepath.Join(root, e.Name()))
		}
	}
	sort.Slice(projects, func(i, j int) bool { return strings.ToLower(projects[i]) < strings.ToLower(projects[j]) })
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"root": root, "projects": projects})
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
	s.state.selectProjectThread(full)
	s.state.mu.Lock()
	s.state.Config.LastProject = full
	cfg := s.state.Config
	s.state.mu.Unlock()
	_ = saveConfig(cfg)
	_ = ensureProjectDocs(full, cfg)
	s.state.UpdateProjectState("Projekt ausgewählt")
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "project": full})
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
	running := s.state.Running
	s.state.mu.RUnlock()
	if running {
		http.Error(w, "Agent läuft gerade", 409)
		return
	}
	selected, err := selectDirectory(initial)
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
	if err := s.state.StartAgent(req.Message, req.Model, attachments); err != nil {
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
		ID      string `json:"id"`
		Approve bool   `json:"approve"`
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
	select {
	case pending.Result <- req.Approve:
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
	var pending any
	if s.state.Pending != nil {
		p := s.state.Pending
		pending = UIEvent{ID: p.ID, Type: "approval_required", Message: p.Action.Message, Action: p.Action.Action, Path: p.Action.Path, Command: p.Action.Command, Preview: p.Preview, Timestamp: time.Now()}
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
		_ = writeJSON(w, cfg)
	case http.MethodPost:
		var patch map[string]json.RawMessage
		if err := readJSONPermissive(r.Body, &patch); err != nil {
			http.Error(w, "Ungültige Einstellungsdaten: "+err.Error(), 400)
			return
		}
		s.state.mu.Lock()
		current := s.state.Config
		baseData, _ := json.Marshal(current)
		var merged map[string]json.RawMessage
		_ = json.Unmarshal(baseData, &merged)
		for key, value := range patch {
			merged[key] = value
		}
		mergedData, err := json.Marshal(merged)
		if err != nil {
			s.state.mu.Unlock()
			http.Error(w, "Einstellungen konnten nicht zusammengeführt werden: "+err.Error(), 400)
			return
		}
		var incoming Config
		if err := json.Unmarshal(mergedData, &incoming); err != nil {
			s.state.mu.Unlock()
			http.Error(w, "Einstellungen enthalten ungültige Werte: "+err.Error(), 400)
			return
		}
		incoming.RootProjectDir = current.RootProjectDir
		incoming.LastProject = current.LastProject
		incoming.LastModel = current.LastModel
		incoming.Port = current.Port
		cfg := normalizeConfig(incoming)
		s.state.Config = cfg
		if cfg.OllamaURL != "" {
			s.state.Ollama.BaseURL = cfg.OllamaURL
		} else {
			s.state.Ollama.BaseURL = firstOllamaCandidate()
		}
		s.state.Ollama.ContextLength = cfg.ContextLength
		s.state.mu.Unlock()
		if err := saveConfig(cfg); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.state.UpdateProjectState("Einstellungen geändert")
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
	s.state.mu.RUnlock()
	results := map[string]any{}
	for _, server := range cfg.MCPServers {
		if !server.Enabled || (req.Name != "" && !strings.EqualFold(req.Name, server.Name)) {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(server.TimeoutSec)*time.Second)
		out, err := mcpCall(ctx, cfg, server.Name, "tools/list", map[string]any{})
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
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
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
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
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
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	cfg := s.state.Config
	s.state.mu.RUnlock()
	if project == "" {
		http.Error(w, "Kein Projekt ausgewählt", 400)
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = cfg.DefaultOpenTarget
	}
	if err := openProjectTarget(project, target); err != nil {
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
	enabled := s.state.Config.GitEnabled
	s.state.mu.RUnlock()
	if project == "" {
		http.Error(w, "Kein Projekt ausgewählt", 400)
		return
	}
	if !enabled || !gitAvailable() {
		http.Error(w, "Git ist nicht verfügbar oder deaktiviert", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	status, statusErr := runGit(ctx, project, []string{"status", "--short", "--branch"})
	diff, diffErr := runGit(ctx, project, []string{"diff", "--stat"})
	logText, logErr := runGit(ctx, project, []string{"log", "-5", "--pretty=format:%h  %s  (%cr)"})
	branch, _ := runGit(ctx, project, []string{"branch", "--show-current"})
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
	// qwen2.5-coder:14b is the most reliable installed model for LocalCodex's
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
