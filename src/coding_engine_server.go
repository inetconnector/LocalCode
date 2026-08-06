// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"time"
)

type codingEngineSetupRequest struct {
	Action string `json:"action"`
	Engine string `json:"engine,omitempty"`
}

func requestedCodingEngine(r *http.Request, cfg Config, explicit string) string {
	engine := strings.TrimSpace(explicit)
	if engine == "" {
		engine = strings.TrimSpace(r.URL.Query().Get("engine"))
	}
	if engine == "" {
		engine = cfg.EditingEngine
	}
	return normalizeEditingEngine(engine)
}

func (s *Server) handleCodingEngineStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	selected := requestedCodingEngine(r, cfg, "")
	statuses := make([]CodingEngineStatus, 0, 4)
	selectedStatus := CodingEngineStatus{}
	for _, engine := range []string{editingEngineAider, editingEngineClaude, editingEngineOpenCode, editingEngineNative} {
		status := codingEngineStatus(ctx, cfg, engine)
		statuses = append(statuses, status)
		if engine == selected {
			selectedStatus = status
		}
	}
	_ = writeJSON(w, map[string]any{
		"selected": selected,
		"status":   selectedStatus,
		"engines":  statuses,
	})
}

func engineLoginCommand(executable, engine string) string {
	args := []string{"auth", "login"}
	if runtime.GOOS == "windows" {
		return buildWindowsCommandLine(executable, args)
	}
	return quoteArgs(append([]string{executable}, args...))
}

func (s *Server) handleCodingEngineSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req codingEngineSetupRequest
	if err := readJSONPermissive(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	project := s.state.Project
	running := s.state.Running
	model := s.state.Model
	s.state.mu.RUnlock()
	if running {
		http.Error(w, "agent is running", http.StatusConflict)
		return
	}
	if strings.TrimSpace(project) == "" {
		project = cfg.RootProjectDir
	}
	engine := requestedCodingEngine(r, cfg, req.Engine)
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Minute)
	defer cancel()
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "install", "repair", "update":
		status, updated, detail, err := installCodingEngine(ctx, project, engine, cfg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "detail": detail, "error": err.Error()})
			return
		}
		updated.EditingEngine = engine
		updated = normalizeConfig(updated)
		if err := saveConfig(updated); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.state.mu.Lock()
		s.state.Config = updated
		s.state.mu.Unlock()
		_ = writeJSON(w, map[string]any{"ok": true, "status": status, "detail": detail, "settings": updated})
	case "login":
		status := codingEngineStatus(ctx, cfg, engine)
		if engine == editingEngineAider || engine == editingEngineNative {
			_ = writeJSON(w, map[string]any{"ok": true, "status": status, "detail": status.DisplayName + " requires no separate CLI login from LocalCode."})
			return
		}
		if !status.Installed || status.Executable == "" {
			w.WriteHeader(http.StatusConflict)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "error": status.Error})
			return
		}
		if err := openInteractiveTerminal(project, engineLoginCommand(status.Executable, engine), cfg); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "error": err.Error()})
			return
		}
		_ = writeJSON(w, map[string]any{"ok": true, "status": status, "detail": status.DisplayName + " login opened in an interactive terminal. Complete the login there, then run Status again."})
	case "test":
		status := codingEngineStatus(ctx, cfg, engine)
		if !status.Installed {
			w.WriteHeader(http.StatusConflict)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "error": status.Error})
			return
		}
		testCfg := cfg
		testCfg.EditingEngine = engine
		result, err := runCodingEngine(ctx, project, "Analyze the repository and report its architecture without modifying files.", model, "settings-test", "repo-map", testCfg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "detail": formatCodingEngineResult(result, cfg), "error": err.Error()})
			return
		}
		_ = writeJSON(w, map[string]any{"ok": true, "status": codingEngineStatus(ctx, testCfg, engine), "detail": truncateText(formatCodingEngineResult(result, cfg), 30000)})
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
	}
}

func (s *Server) handleCodingEngineUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	backup := s.state.LastEngineBackup
	if backup == "" {
		backup = s.state.LastAiderBackup
	}
	running := s.state.Running
	cfg := s.state.Config
	s.state.mu.RUnlock()
	if running {
		http.Error(w, "agent is running", http.StatusConflict)
		return
	}
	if strings.TrimSpace(project) == "" {
		http.Error(w, "no project selected", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(backup) == "" {
		backup = latestAiderBackup(project)
	}
	if strings.TrimSpace(backup) == "" {
		http.Error(w, "no editing-engine backup is available for this project", http.StatusConflict)
		return
	}
	detail, err := restoreAiderBackup(project, backup)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "detail": detail})
		return
	}
	s.state.AddEvent(UIEvent{Type: "action_done", Message: localizeConfigText(cfg, "Letzte Engine-Änderung zurückgesetzt", "Last editing-engine change restored"), Detail: detail, Action: "engine_undo"})
	_ = writeJSON(w, map[string]any{"ok": true, "detail": detail})
}
