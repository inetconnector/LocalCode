// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const runJournalSchemaVersion = 1

var (
	runJournalFileMu         sync.Mutex
	runJournalSecretPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^\s]+`),
		regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|passwd|secret)\s*[:=]\s*)[^\s,;]+`),
		regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+\-/=]{8,}`),
	}
)

type RunJournalEvent struct {
	At      time.Time `json:"at"`
	Type    string    `json:"type"`
	Action  string    `json:"action,omitempty"`
	Path    string    `json:"path,omitempty"`
	Message string    `json:"message,omitempty"`
}

type RunRecoveryState struct {
	SchemaVersion int               `json:"schema_version"`
	RunID         string            `json:"run_id"`
	ThreadID      string            `json:"thread_id,omitempty"`
	Project       string            `json:"project"`
	Model         string            `json:"model,omitempty"`
	Task          string            `json:"task,omitempty"`
	Phase         string            `json:"phase"`
	StartedAt     time.Time         `json:"started_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Terminal      bool              `json:"terminal"`
	Outcome       string            `json:"outcome,omitempty"`
	Events        []RunJournalEvent `json:"events,omitempty"`
}

func runJournalPath() string {
	return filepath.Join(appDataDir(), "recovery", "active-run.json")
}

func sanitizeRunJournalText(value string, limit int) string {
	value = strings.TrimSpace(value)
	for _, pattern := range runJournalSecretPatterns {
		value = pattern.ReplaceAllString(value, `${1}[REDACTED]`)
	}
	if limit > 0 && len(value) > limit {
		value = value[:limit] + "…"
	}
	return value
}

func loadRunJournal() (*RunRecoveryState, error) {
	data, err := os.ReadFile(runJournalPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state RunRecoveryState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.SchemaVersion != runJournalSchemaVersion || strings.TrimSpace(state.RunID) == "" {
		return nil, fmt.Errorf("unsupported or incomplete run journal")
	}
	return &state, nil
}

func loadRecoverableRun() *RunRecoveryState {
	state, err := loadRunJournal()
	if err != nil || state == nil || state.Terminal {
		return nil
	}
	if strings.TrimSpace(state.Project) == "" {
		return nil
	}
	if info, statErr := os.Stat(state.Project); statErr != nil || !info.IsDir() {
		return nil
	}
	copy := *state
	copy.Events = append([]RunJournalEvent(nil), state.Events...)
	return &copy
}

func writeRunJournal(state RunRecoveryState) error {
	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	return writeRunJournalUnlocked(state)
}

func writeRunJournalUnlocked(state RunRecoveryState) error {
	state.SchemaVersion = runJournalSchemaVersion
	state.UpdatedAt = time.Now()
	if len(state.Events) > 64 {
		state.Events = append([]RunJournalEvent(nil), state.Events[len(state.Events)-64:]...)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := runJournalPath()
	expected, err := readFileVersion(path)
	if err != nil {
		return err
	}
	return atomicWriteFileIfVersion(path, data, 0o600, expected)
}

func (s *AppState) beginRunJournal(runID, project, model, task, threadID string, startedAt time.Time) {
	state := RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         runID,
		ThreadID:      threadID,
		Project:       filepath.Clean(project),
		Model:         sanitizeRunJournalText(model, 300),
		Task:          sanitizeRunJournalText(task, 6000),
		Phase:         "starting",
		StartedAt:     startedAt,
		UpdatedAt:     startedAt,
		Events: []RunJournalEvent{{
			At:      startedAt,
			Type:    "run_start",
			Message: "Agent run started",
		}},
	}
	_ = writeRunJournal(state)
}

func (s *AppState) updateRunJournal(runID string, mutate func(*RunRecoveryState)) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	state, err := loadRunJournal()
	if err != nil || state == nil || state.RunID != runID || state.Terminal {
		return
	}
	mutate(state)
	_ = writeRunJournalUnlocked(*state)
}

func (s *AppState) journalRunTask(runID, task string) {
	task = sanitizeRunJournalText(task, 6000)
	s.updateRunJournal(runID, func(state *RunRecoveryState) {
		state.Task = task
	})
}

func (s *AppState) journalRunPhase(runID, phase string) {
	phase = sanitizeRunJournalText(phase, 100)
	s.updateRunJournal(runID, func(state *RunRecoveryState) {
		state.Phase = phase
		state.Events = append(state.Events, RunJournalEvent{At: time.Now(), Type: "phase", Message: phase})
	})
}

func shouldJournalRunEvent(ev UIEvent) bool {
	// Keep only recovery-relevant checkpoints durable. The original task is
	// already stored separately, action_running/action_done capture tool
	// progress, and raw tool results are deliberately excluded from the journal.
	// Persisting user, agent_step and tool_result events therefore added atomic
	// disk writes and more free text without improving crash reconciliation.
	switch ev.Type {
	case "approval_required", "approval", "approval_rule", "action_running", "action_done", "tool_error", "error", "warning", "final", "question", "context_compacted", "verification", "recovery":
		return true
	default:
		return false
	}
}

func (s *AppState) journalRunEvent(runID string, ev UIEvent) {
	if strings.TrimSpace(runID) == "" || !shouldJournalRunEvent(ev) {
		return
	}
	// Store only operational metadata. Tool outputs, commands, attachments and
	// model detail are intentionally excluded so recovery does not become a
	// second secret-bearing transcript.
	event := RunJournalEvent{
		At:      ev.Timestamp,
		Type:    sanitizeRunJournalText(ev.Type, 80),
		Action:  sanitizeRunJournalText(ev.Action, 120),
		Path:    sanitizeRunJournalText(filepath.ToSlash(ev.Path), 1000),
		Message: sanitizeRunJournalText(ev.Message, 1200),
	}
	if event.At.IsZero() {
		event.At = time.Now()
	}
	s.updateRunJournal(runID, func(state *RunRecoveryState) {
		state.Events = append(state.Events, event)
	})
}

func (s *AppState) finishRunJournal(runID, outcome string) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	state, err := loadRunJournal()
	if err != nil || state == nil || state.RunID != runID {
		return
	}
	state.Terminal = true
	state.Phase = "idle"
	state.Outcome = sanitizeRunJournalText(outcome, 200)
	state.Events = append(state.Events, RunJournalEvent{At: time.Now(), Type: "run_end", Message: state.Outcome})
	_ = writeRunJournalUnlocked(*state)
}

func recoveryResumeRequest(value string) bool {
	value = normalizedQuestion(value)
	for _, candidate := range []string{"weiter", "weitermachen", "mach weiter", "fortsetzen", "setze fort", "resume", "continue", "continue task", "resume task"} {
		candidate = normalizedQuestion(candidate)
		if value == candidate || strings.HasPrefix(value, candidate+" ") {
			return true
		}
	}
	return false
}

func (s *AppState) recoveryContextForTask(project, currentTask string) (string, string) {
	s.mu.RLock()
	recovery := s.Recovery
	s.mu.RUnlock()
	if recovery == nil || !strings.EqualFold(filepath.Clean(recovery.Project), filepath.Clean(project)) {
		return "", ""
	}
	current := normalizedQuestion(currentTask)
	original := normalizedQuestion(recovery.Task)
	if current != original && !recoveryResumeRequest(currentTask) {
		return "", ""
	}
	var b strings.Builder
	b.WriteString("RECOVERY CHECKPOINT FROM AN INTERRUPTED LOCALCODE RUN\n")
	fmt.Fprintf(&b, "Previous run: %s\nPhase: %s\nStarted: %s\nOriginal task: %s\n", recovery.RunID, recovery.Phase, recovery.StartedAt.Format(time.RFC3339), recovery.Task)
	b.WriteString("Safety rule: the previous process ended without a terminal checkpoint. Do NOT replay a mutating action or assume it succeeded. Re-read current files/Git state, compare observable postconditions, and only then decide what work remains.\n")
	if len(recovery.Events) > 0 {
		b.WriteString("Last confirmed operational events:\n")
		start := len(recovery.Events) - 12
		if start < 0 {
			start = 0
		}
		for _, event := range recovery.Events[start:] {
			fmt.Fprintf(&b, "- %s %s", event.Type, event.Action)
			if event.Path != "" {
				fmt.Fprintf(&b, " [%s]", event.Path)
			}
			if event.Message != "" {
				fmt.Fprintf(&b, ": %s", event.Message)
			}
			b.WriteByte('\n')
		}
	}
	return b.String(), recovery.Task
}

func (s *AppState) consumeRecoveryContextForTask(project, currentTask string) (string, string) {
	contextText, originalTask := s.recoveryContextForTask(project, currentTask)
	if contextText == "" {
		return "", ""
	}
	s.mu.Lock()
	if s.Recovery != nil && strings.EqualFold(filepath.Clean(s.Recovery.Project), filepath.Clean(project)) {
		s.Recovery = nil
	}
	s.mu.Unlock()
	return contextText, originalTask
}

func recoveryStartupEvent(cfg Config, recovery *RunRecoveryState) UIEvent {
	if recovery == nil {
		return UIEvent{}
	}
	return UIEvent{
		ID:      newID(),
		Type:    "recovery_available",
		Message: localizeConfigText(cfg, "Unterbrochenen Agentenlauf erkannt", "Interrupted agent run detected"),
		Detail: fmt.Sprintf(localizeConfigText(cfg,
			"Phase: %s · Aufgabe: %s\nMit „Weiter“ kann LocalCode den letzten bestätigten Zustand sicher erneut prüfen. Mutierende Aktionen werden nicht blind wiederholt.",
			"Phase: %s · Task: %s\nUse ‘Continue’ to safely re-check the last confirmed state. Mutating actions are never replayed blindly."), recovery.Phase, sanitizeRunJournalText(recovery.Task, 800)),
		Timestamp: time.Now(),
	}
}
