// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// handleStopTask provides a compare-and-cancel boundary for independent IDE
// windows. The legacy global stop remains available to existing desktop clients.
func (s *Server) handleStopTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadID string `json:"thread_id"`
		RunID    string `json:"run_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		s.ideError(w, "Ungültige Abbruchanfrage", "Invalid stop request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF || strings.TrimSpace(req.ThreadID) == "" || strings.TrimSpace(req.RunID) == "" {
		s.ideError(w, "Ungültige Aufgaben-/Lauf-ID", "Invalid task/run identity", http.StatusBadRequest)
		return
	}
	s.state.mu.Lock()
	if !s.state.Running || s.state.CurrentThread != req.ThreadID || s.state.RunID != req.RunID || s.state.Cancel == nil {
		s.state.mu.Unlock()
		s.ideError(w, "Die aktive Aufgabe hat sich geändert", "The active task has changed", http.StatusConflict)
		return
	}
	s.state.RunPhase = "cancelling"
	s.state.LastProgressAt = time.Now()
	// Context cancellation does not wait for the worker. Keep the identity check
	// and cancellation serialized with completion and admission of another run.
	s.state.Cancel()
	s.state.mu.Unlock()
	s.state.journalRunPhase(req.RunID, "cancelling")
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ok": true})
}

func (s *Server) ideError(w http.ResponseWriter, de, en string, status int) {
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	http.Error(w, localizeConfigText(cfg, de, en), status)
}
