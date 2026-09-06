// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type agentSteeringMessage struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// This bounded mailbox is transient input, never execution/recovery authority.
// Existing run budgets are retained. Tool execution is not abruptly interrupted.
type agentSteeringState struct {
	RunID       string
	Accepting   bool
	Queue       []agentSteeringMessage
	Seen        map[string]string
	Bytes       int
	ModelCancel context.CancelFunc
}

func (s *Server) handleSteer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ThreadID string `json:"thread_id"`
		RunID    string `json:"run_id"`
		ID       string `json:"id"`
		Message  string `json:"message"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 40000))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		s.ideError(w, "Ungültiger Folgehinweis", "Invalid steering request", http.StatusBadRequest)
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		s.ideError(w, "Ungültiger Folgehinweis", "Invalid steering request", http.StatusBadRequest)
		return
	}
	if err := s.state.queueAgentSteering(req.ThreadID, req.RunID, req.ID, req.Message); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = writeJSON(w, map[string]any{"ok": true, "id": req.ID, "state": "queued"})
}

func (s *AppState) queueAgentSteering(threadID, runID, id, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	mailbox := &s.Steering
	if !s.Running || s.RunID != runID || s.CurrentThread != threadID || mailbox.RunID != runID || !mailbox.Accepting || s.RunPhase == "cancelling" {
		return fmt.Errorf("%s", localizeConfigText(s.Config, "Die Aufgabe nimmt gerade keine Hinweise an. Nach Abschluss erneut senden.", "This task is not accepting steering right now. Send again after completion."))
	}
	message = strings.TrimSpace(message)
	if id == "" || len(id) > 128 || message == "" || len(message) > 32768 {
		return fmt.Errorf("%s", localizeConfigText(s.Config, "Ungültiger Hinweis (maximal 32 KiB).", "Invalid steering message (maximum 32 KiB)."))
	}
	if previous, exists := mailbox.Seen[id]; exists {
		if previous != message {
			return fmt.Errorf("%s", localizeConfigText(s.Config, "Hinweis-ID wurde bereits mit anderem Inhalt verwendet.", "Steering ID was already used with different content."))
		}
		return nil
	}
	if len(mailbox.Queue) >= 16 || len(mailbox.Seen) >= 64 || mailbox.Bytes+len(message) > 131072 {
		return fmt.Errorf("%s", localizeConfigText(s.Config, "Hinweisbudget dieses Laufs erreicht. Eine neue Aufgabe starten.", "Steering budget for this run reached. Start a new task."))
	}
	mailbox.Queue = append(mailbox.Queue, agentSteeringMessage{ID: id, Message: message})
	mailbox.Seen[id] = message
	mailbox.Bytes += len(message)
	if mailbox.ModelCancel != nil {
		mailbox.ModelCancel()
	}
	// An approval wait has not executed the proposed tool yet. A new user
	// instruction invalidates that proposal; reject it and replan first.
	if s.Pending != nil {
		select {
		case s.Pending.Result <- ApprovalDecision{Approved: false}:
		default:
		}
	}
	return nil
}

func (s *AppState) beginAgentSteering(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.RunID == runID && s.Running {
		s.Steering = agentSteeringState{RunID: runID, Accepting: true, Seen: make(map[string]string)}
	}
}

func (s *AppState) endAgentSteering(runID string) {
	s.mu.Lock()
	var remaining []agentSteeringMessage
	if s.Steering.RunID == runID {
		remaining = s.Steering.Queue
		s.Steering = agentSteeringState{}
	}
	cfg := s.Config
	current := s.RunID == runID
	s.mu.Unlock()
	if current {
		for _, item := range remaining {
			s.AddEvent(UIEvent{Type: "warning", Action: "steering_undelivered", Message: localizeConfigText(cfg, "Hinweis wegen Laufende nicht mehr verarbeitet. Bitte erneut senden.", "Steering was not processed before the run ended. Please send again."), Detail: item.Message})
		}
	}
}

func (s *AppState) drainAgentSteering(runID string) []agentSteeringMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Steering.RunID != runID || s.RunID != runID {
		return nil
	}
	items := s.Steering.Queue
	s.Steering.Queue = nil
	return items
}

func (s *AppState) setSteeringModelCancel(runID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Steering.RunID == runID {
		s.Steering.ModelCancel = cancel
		if cancel != nil && len(s.Steering.Queue) > 0 {
			cancel()
		}
	}
}

// Linearization point: a queued correction wins over an unstarted action.
// Closing admission for terminal actions prevents accepted-but-lost late input.
func (s *AppState) admitActionAfterSteering(runID string, terminal bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Steering.RunID != runID {
		return true
	}
	if len(s.Steering.Queue) > 0 {
		return false
	}
	if terminal {
		s.Steering.Accepting = false
	}
	return true
}
