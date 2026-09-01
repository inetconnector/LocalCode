// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	missionRecoveryKindReadOnly       = "read-only"
	missionRecoveryRunning            = "running"
	maxMissionRecoveryListItems       = 32
	maxMissionRecoveryObjectiveLength = 4000
	maxMissionRecoveryItemLength      = 1200
)

type MissionRecoveryTaskState struct {
	ID                     string                         `json:"id"`
	ParentID               string                         `json:"parent_id,omitempty"`
	Role                   AgentRole                      `json:"role"`
	Objective              string                         `json:"objective"`
	Dependencies           []string                       `json:"dependencies,omitempty"`
	RequestedCapabilities  []AgentCapability              `json:"requested_capabilities,omitempty"`
	Capabilities           []AgentCapability              `json:"capabilities,omitempty"`
	Model                  string                         `json:"model,omitempty"`
	Budget                 AgentBudget                    `json:"budget"`
	State                  AgentTaskState                 `json:"state"`
	ResourceClass          AgentResourceClass             `json:"resource_class,omitempty"`
	QueuePosition          int                            `json:"queue_position,omitempty"`
	Running                bool                           `json:"running,omitempty"`
	AdmissionBlockedReason string                         `json:"admission_blocked_reason,omitempty"`
	BudgetSnapshot         *AgentBudgetSnapshot           `json:"budget_snapshot,omitempty"`
	Lifecycle              *MissionTaskLifecycle          `json:"lifecycle,omitempty"`
	CompletionEvidence     *MissionTaskCompletionEvidence `json:"completion_evidence,omitempty"`
	Usage                  AgentUsage                     `json:"usage,omitempty"`
}

type MissionRecoveryState struct {
	Kind              string                        `json:"kind"`
	MissionID         string                        `json:"mission_id"`
	ParentTaskID      string                        `json:"parent_task_id,omitempty"`
	Objective         string                        `json:"objective,omitempty"`
	Project           string                        `json:"project"`
	Model             string                        `json:"model,omitempty"`
	Constraints       []string                      `json:"constraints,omitempty"`
	SuccessCriteria   []string                      `json:"success_criteria,omitempty"`
	Budget            AgentBudget                   `json:"budget"`
	State             string                        `json:"state"`
	Reason            string                        `json:"reason,omitempty"`
	BudgetExhaustedBy string                        `json:"budget_exhausted_by,omitempty"`
	Baseline          *MissionProjectBaseline       `json:"baseline,omitempty"`
	Reconciliation    *MissionRestartReconciliation `json:"reconciliation,omitempty"`
	Tasks             []MissionRecoveryTaskState    `json:"tasks"`
	Knowledge         []MissionKnowledgeItem        `json:"knowledge,omitempty"`
	Accounting        *AgentMissionAccounting       `json:"accounting,omitempty"`
	StartedAt         time.Time                     `json:"started_at"`
	UpdatedAt         time.Time                     `json:"updated_at"`
}

func sanitizeMissionRecoveryList(values []string) []string {
	if len(values) > maxMissionRecoveryListItems {
		values = values[:maxMissionRecoveryListItems]
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = sanitizeRunJournalText(value, maxMissionRecoveryItemLength)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func cloneMissionRecoveryState(state *MissionRecoveryState) *MissionRecoveryState {
	if state == nil {
		return nil
	}
	copyState := *state
	copyState.Constraints = append([]string(nil), state.Constraints...)
	copyState.SuccessCriteria = append([]string(nil), state.SuccessCriteria...)
	if len(state.Knowledge) > 0 {
		copyState.Knowledge = append([]MissionKnowledgeItem(nil), state.Knowledge...)
	}
	copyState.Tasks = make([]MissionRecoveryTaskState, len(state.Tasks))
	for index := range state.Tasks {
		copyState.Tasks[index] = state.Tasks[index]
		copyState.Tasks[index].Dependencies = append([]string(nil), state.Tasks[index].Dependencies...)
		copyState.Tasks[index].RequestedCapabilities = append([]AgentCapability(nil), state.Tasks[index].RequestedCapabilities...)
		copyState.Tasks[index].Capabilities = append([]AgentCapability(nil), state.Tasks[index].Capabilities...)
		if state.Tasks[index].BudgetSnapshot != nil {
			budgetCopy := *state.Tasks[index].BudgetSnapshot
			copyState.Tasks[index].BudgetSnapshot = &budgetCopy
		}
		if state.Tasks[index].Lifecycle != nil {
			lifecycleCopy := *state.Tasks[index].Lifecycle
			copyState.Tasks[index].Lifecycle = &lifecycleCopy
		}
		if state.Tasks[index].CompletionEvidence != nil {
			evidenceCopy := *state.Tasks[index].CompletionEvidence
			copyState.Tasks[index].CompletionEvidence = &evidenceCopy
		}
	}
	if state.Baseline != nil {
		baselineCopy := *state.Baseline
		copyState.Baseline = &baselineCopy
	}
	if state.Reconciliation != nil {
		reconciliationCopy := *state.Reconciliation
		reconciliationCopy.Tasks = append([]MissionRecoveryTaskReconciliation(nil), state.Reconciliation.Tasks...)
		copyState.Reconciliation = &reconciliationCopy
	}
	if state.Accounting != nil {
		accountingCopy := *state.Accounting
		copyState.Accounting = &accountingCopy
	}
	return &copyState
}

func missionRecoveryTasksFromGraph(graph AgentTaskGraph) []MissionRecoveryTaskState {
	tasks := make([]MissionRecoveryTaskState, 0, len(graph.Tasks))
	for _, task := range graph.Tasks {
		tasks = append(tasks, MissionRecoveryTaskState{
			ID:                    sanitizeRunJournalText(task.ID, 160),
			ParentID:              sanitizeRunJournalText(task.ParentID, 160),
			Role:                  task.Role,
			Objective:             sanitizeRunJournalText(task.Objective, 2400),
			Dependencies:          append([]string(nil), task.Dependencies...),
			RequestedCapabilities: append([]AgentCapability(nil), task.RequestedCapabilities...),
			Capabilities:          append([]AgentCapability(nil), task.Capabilities...),
			Model:                 sanitizeRunJournalText(task.Model, 300),
			Budget:                task.Budget,
			State:                 task.State,
		})
	}
	return tasks
}

func missionRecoveryTaskLabel(req AgentReadOnlyMissionRequest) string {
	objective := sanitizeRunJournalText(req.Objective, maxMissionRecoveryObjectiveLength)
	if objective != "" {
		return objective
	}
	return fmt.Sprintf("Read-only Mission %s", sanitizeRunJournalText(req.MissionID, 160))
}

func newMissionRecoveryState(req AgentReadOnlyMissionRequest, graph AgentTaskGraph, project, model string, started time.Time) MissionRecoveryState {
	return MissionRecoveryState{
		Kind:            missionRecoveryKindReadOnly,
		MissionID:       sanitizeRunJournalText(req.MissionID, 160),
		ParentTaskID:    sanitizeRunJournalText(req.ParentTaskID, 160),
		Objective:       sanitizeRunJournalText(req.Objective, maxMissionRecoveryObjectiveLength),
		Project:         filepath.Clean(project),
		Model:           sanitizeRunJournalText(model, 300),
		Constraints:     sanitizeMissionRecoveryList(req.Constraints),
		SuccessCriteria: sanitizeMissionRecoveryList(req.SuccessCriteria),
		Budget:          req.Budget,
		State:           missionRecoveryRunning,
		Tasks:           missionRecoveryTasksFromGraph(graph),
		StartedAt:       started,
		UpdatedAt:       started,
	}
}

func applyMissionSchedulerSnapshot(mission *MissionRecoveryState, snapshot AgentSchedulerSnapshot) {
	if mission == nil {
		return
	}
	observedAt := time.Now()
	byID := make(map[string]AgentTaskScheduleSnapshot, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		byID[task.TaskID] = task
	}
	for index := range mission.Tasks {
		taskSnapshot, ok := byID[mission.Tasks[index].ID]
		if !ok {
			continue
		}
		updateMissionTaskLifecycle(&mission.Tasks[index], taskSnapshot, observedAt)
		mission.Tasks[index].State = taskSnapshot.State
		if taskSnapshot.ResourceClass != "" {
			mission.Tasks[index].ResourceClass = taskSnapshot.ResourceClass
		}
		mission.Tasks[index].QueuePosition = taskSnapshot.QueuePosition
		mission.Tasks[index].Running = taskSnapshot.Running
		mission.Tasks[index].AdmissionBlockedReason = sanitizeRunJournalText(taskSnapshot.AdmissionBlockedReason, 1000)
		budgetCopy := taskSnapshot.Budget
		mission.Tasks[index].BudgetSnapshot = &budgetCopy
	}
	mission.UpdatedAt = observedAt
}

func (s *AppState) beginMissionRunJournal(runID string, req AgentReadOnlyMissionRequest, graph AgentTaskGraph, project, model string, started time.Time) {
	mission := newMissionRecoveryState(req, graph, project, model, started)
	baseline := captureMissionProjectBaseline(project)
	mission.Baseline = &baseline
	state := RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         runID,
		Project:       filepath.Clean(project),
		Model:         sanitizeRunJournalText(model, 300),
		Task:          missionRecoveryTaskLabel(req),
		Phase:         "mission-read-only",
		StartedAt:     started,
		UpdatedAt:     started,
		Mission:       &mission,
		Events: []RunJournalEvent{{
			At:      started,
			Type:    "mission_start",
			Action:  missionRecoveryKindReadOnly,
			Message: sanitizeRunJournalText(req.MissionID, 160),
		}},
	}
	_ = writeRunJournal(state)
}

func (s *AppState) journalMissionSchedulerSnapshot(runID string, snapshot AgentSchedulerSnapshot) {
	s.updateRunJournal(runID, func(state *RunRecoveryState) {
		applyMissionSchedulerSnapshot(state.Mission, snapshot)
	})
}

func (s *AppState) journalMissionSchedulerCheckpoint(runID string, snapshot AgentSchedulerSnapshot, graph *AgentTaskGraph) {
	s.updateRunJournal(runID, func(state *RunRecoveryState) {
		applyMissionSchedulerSnapshot(state.Mission, snapshot)
		applyMissionCompletionEvidence(state.Mission, graph, time.Now())
	})
}

func (s *AppState) finishMissionRunJournal(runID string, result AgentReadOnlyMissionResult, outcome string) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	state, err := loadRunJournal()
	if err != nil || state == nil || state.RunID != runID {
		return
	}
	if state.Mission != nil {
		previousByID := make(map[string]MissionRecoveryTaskState, len(state.Mission.Tasks))
		for _, task := range state.Mission.Tasks {
			previousByID[task.ID] = task
		}
		state.Mission.State = string(result.State)
		state.Mission.Reason = string(result.Reason)
		state.Mission.BudgetExhaustedBy = sanitizeRunJournalText(result.BudgetExhaustedBy, 120)
		state.Mission.Tasks = missionRecoveryTasksFromGraph(result.Graph)
		for index := range state.Mission.Tasks {
			if previous, ok := previousByID[state.Mission.Tasks[index].ID]; ok {
				state.Mission.Tasks[index].ResourceClass = previous.ResourceClass
				state.Mission.Tasks[index].QueuePosition = previous.QueuePosition
				state.Mission.Tasks[index].Running = false
				state.Mission.Tasks[index].AdmissionBlockedReason = previous.AdmissionBlockedReason
				if previous.BudgetSnapshot != nil {
					budgetCopy := *previous.BudgetSnapshot
					state.Mission.Tasks[index].BudgetSnapshot = &budgetCopy
				}
				if previous.Lifecycle != nil {
					lifecycleCopy := *previous.Lifecycle
					state.Mission.Tasks[index].Lifecycle = &lifecycleCopy
				}
				if previous.CompletionEvidence != nil {
					evidenceCopy := *previous.CompletionEvidence
					state.Mission.Tasks[index].CompletionEvidence = &evidenceCopy
				}
			}
			state.Mission.Tasks[index].Usage = result.Run.UsageByTask[state.Mission.Tasks[index].ID]
		}
		applyMissionSchedulerSnapshot(state.Mission, result.Run.Snapshot)
		applyMissionCompletionEvidence(state.Mission, &result.Graph, time.Now())
		accountingCopy := result.Accounting
		state.Mission.Accounting = &accountingCopy
		state.Mission.UpdatedAt = time.Now()
	}
	state.Terminal = true
	state.Phase = "idle"
	state.Outcome = sanitizeRunJournalText(outcome, 200)
	state.Events = append(state.Events, RunJournalEvent{
		At:      time.Now(),
		Type:    "mission_end",
		Action:  missionRecoveryKindReadOnly,
		Message: state.Outcome,
	})
	_ = writeRunJournalUnlocked(*state)
}
