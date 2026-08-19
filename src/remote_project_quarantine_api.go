// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"strings"
)

const remoteProjectQuarantineMaxBody = 16 << 10

func (s *RemoteServer) projectQuarantineRootAndRunning() (string, bool) {
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	return s.state.Config.RootProjectDir, s.state.Running
}

func (s *RemoteServer) handleRemoteProjectQuarantineList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root, _ := s.projectQuarantineRootAndRunning()
	entries, err := listQuarantinedProjects(root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"quarantine": entries})
}

func (s *RemoteServer) handleRemoteProjectQuarantineAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, remoteProjectQuarantineMaxBody)
	var req struct {
		Action       string `json:"action"`
		ID           string `json:"id"`
		Confirmation string `json:"confirmation"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, "invalid quarantine request", http.StatusBadRequest)
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.ID = strings.TrimSpace(req.ID)
	if !validQuarantineID(req.ID) {
		http.Error(w, "invalid quarantine id", http.StatusBadRequest)
		return
	}
	root, running := s.projectQuarantineRootAndRunning()
	if running {
		http.Error(w, "agent is running", http.StatusConflict)
		return
	}

	var (
		entry QuarantinedProject
		err   error
	)
	switch req.Action {
	case "restore":
		entry, err = restoreQuarantinedProject(root, req.ID)
	case "purge":
		entry, err = purgeQuarantinedProject(root, req.ID, req.Confirmation)
	default:
		http.Error(w, "remote quarantine action is not allowed", http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	message := "project restored from quarantine"
	if req.Action == "purge" {
		message = "project permanently purged from quarantine"
	}
	s.state.AddEvent(UIEvent{Type: "action_done", Message: message, Detail: entry.Name, Action: "project_quarantine_" + req.Action})
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true, "action": req.Action, "project": entry})
}
