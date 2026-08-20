// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"strings"
)

const remoteProjectQuarantineMaxBody = 16 << 10

func (s *RemoteServer) handleRemoteProjectQuarantineList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries, err := s.state.ListProjectQuarantine()
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
	if req.Action != "restore" && req.Action != "purge" {
		http.Error(w, "remote quarantine action is not allowed", http.StatusForbidden)
		return
	}

	entry, err := s.state.ProjectQuarantineAction(req.Action, req.ID, req.Confirmation)
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
