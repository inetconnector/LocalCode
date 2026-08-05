// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleAiderStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	_ = writeJSON(w, aiderStatus(ctx, cfg))
}

func (s *Server) handleAiderSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := readJSONPermissive(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	project := s.state.Project
	running := s.state.Running
	s.state.mu.RUnlock()
	if running {
		http.Error(w, "agent is running", http.StatusConflict)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Minute)
	defer cancel()
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "install", "repair", "update":
		status, detail, err := installAider(ctx, cfg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "detail": detail, "error": err.Error()})
			return
		}
		cfg.AiderExecutable = status.Executable
		cfg.AiderVersion = status.ExpectedVersion
		cfg = normalizeConfig(cfg)
		if err := saveConfig(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.state.mu.Lock()
		s.state.Config = cfg
		s.state.mu.Unlock()
		_ = writeJSON(w, map[string]any{"ok": true, "status": status, "detail": detail, "settings": cfg})
	case "test":
		status := aiderStatus(ctx, cfg)
		if !status.Installed {
			w.WriteHeader(http.StatusConflict)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "error": status.Error})
			return
		}
		if strings.TrimSpace(project) == "" {
			project = cfg.RootProjectDir
		}
		model := cfg.AiderMainModel
		if strings.TrimSpace(model) == "" {
			model = s.state.Model
		}
		output, err := runAiderUtility(ctx, project, "repo-map", model, "settings-test", cfg)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = writeJSON(w, map[string]any{"ok": false, "status": status, "detail": output, "error": err.Error()})
			return
		}
		_ = writeJSON(w, map[string]any{"ok": true, "status": status, "detail": truncateText(output, 30000)})
	default:
		http.Error(w, "unsupported action", http.StatusBadRequest)
	}
}

func (s *Server) handleAiderUndo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	project := s.state.Project
	backup := s.state.LastAiderBackup
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
		http.Error(w, "no Aider backup is available for this project", http.StatusConflict)
		return
	}
	detail, err := restoreAiderBackup(project, backup)
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "detail": detail})
		return
	}
	s.state.AddEvent(UIEvent{Type: "action_done", Message: localizeConfigText(cfg, "Letzte Aider-Änderung zurückgesetzt", "Last Aider edit restored"), Detail: detail, Action: "aider_undo"})
	_ = writeJSON(w, map[string]any{"ok": true, "detail": detail})
}
