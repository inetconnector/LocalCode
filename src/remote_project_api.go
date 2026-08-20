// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"strings"
)

const remoteProjectActionMaxBody = 16 << 10

func (s *RemoteServer) handleRemoteProjectDeletePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		http.Error(w, "project path is required", http.StatusBadRequest)
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
	preview, err := inspectProjectDelete(root, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, preview)
}

func (s *RemoteServer) handleRemoteProjectAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, remoteProjectActionMaxBody)
	var req struct {
		Path   string `json:"path"`
		Action string `json:"action"`
		Value  string `json:"value"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid project action request", http.StatusBadRequest)
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	switch req.Action {
	case "create_folder", "create_project", "delete_empty", "delete_recursive":
	default:
		http.Error(w, "remote project action is not allowed", http.StatusForbidden)
		return
	}
	project, err := s.state.ProjectAction(req.Path, req.Action, req.Value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "project": project})
}
