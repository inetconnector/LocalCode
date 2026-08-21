// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"
)

const agentMissionDesktopStatusLimit = 32

type AgentMissionDesktopStatus struct {
	MissionID         string                 `json:"mission_id"`
	ExecutionRunID    string                 `json:"execution_run_id"`
	Project           string                 `json:"project"`
	Model             string                 `json:"model"`
	State             string                 `json:"state"`
	Reason            AgentMissionReason     `json:"reason,omitempty"`
	BudgetExhaustedBy string                 `json:"budget_exhausted_by,omitempty"`
	Budget            AgentBudgetSnapshot    `json:"budget"`
	Scheduler         AgentSchedulerSnapshot `json:"scheduler"`
	ResourceLimits    AgentResourceLimits    `json:"resource_limits"`
	StartedAt         time.Time              `json:"started_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type AgentOrchestrationDiagnosticState string

type AgentOrchestrationDiagnosticReason string

const (
	AgentOrchestrationReady              AgentOrchestrationDiagnosticState = "ready"
	AgentOrchestrationActive             AgentOrchestrationDiagnosticState = "active"
	AgentOrchestrationSaturated          AgentOrchestrationDiagnosticState = "saturated"
	AgentOrchestrationBackendUnavailable AgentOrchestrationDiagnosticState = "backend_unavailable"
	AgentOrchestrationModelUnavailable   AgentOrchestrationDiagnosticState = "model_unavailable"

	AgentOrchestrationReasonIdle                 AgentOrchestrationDiagnosticReason = "idle"
	AgentOrchestrationReasonMissionRunning       AgentOrchestrationDiagnosticReason = "mission_running"
	AgentOrchestrationReasonOllamaOffline        AgentOrchestrationDiagnosticReason = "ollama_offline"
	AgentOrchestrationReasonNoModelSelected      AgentOrchestrationDiagnosticReason = "no_model_selected"
	AgentOrchestrationReasonSelectedModelMissing AgentOrchestrationDiagnosticReason = "selected_model_missing"
	AgentOrchestrationReasonQueueLimitReached    AgentOrchestrationDiagnosticReason = "queue_limit_reached"
	AgentOrchestrationReasonResourceWaiting      AgentOrchestrationDiagnosticReason = "resource_waiting"
)

type AgentModelBackendDiagnostics struct {
	Online                 bool   `json:"online"`
	SelectedModel          string `json:"selected_model,omitempty"`
	SelectedModelAvailable bool   `json:"selected_model_available"`
	InstalledModelCount    int    `json:"installed_model_count"`
	Error                  string `json:"error,omitempty"`
}

type AgentQueueDiagnostics struct {
	Queued      int `json:"queued"`
	Limit       int `json:"limit"`
	Available   int `json:"available"`
	FillPercent int `json:"fill_percent"`
	AtLimit     bool `json:"at_limit"`
}

type AgentResourceDiagnostics struct {
	Class      AgentResourceClass `json:"class"`
	Limit      int                `json:"limit"`
	InUse      int                `json:"in_use"`
	Available  int                `json:"available"`
	Waiting    int                `json:"waiting"`
	AtCapacity bool               `json:"at_capacity"`
	Saturated  bool               `json:"saturated"`
}

type AgentOrchestrationDiagnostics struct {
	State                    AgentOrchestrationDiagnosticState  `json:"state"`
	Reason                   AgentOrchestrationDiagnosticReason `json:"reason"`
	MissionActive            bool                               `json:"mission_active"`
	LogicalReady             int                                `json:"logical_ready"`
	LogicalRunning           int                                `json:"logical_running"`
	LogicalBlocked           int                                `json:"logical_blocked"`
	WaitingForModelInference int                                `json:"waiting_for_model_inference"`
	Backend                  AgentModelBackendDiagnostics       `json:"backend"`
	Queue                    AgentQueueDiagnostics              `json:"queue"`
	Resources                []AgentResourceDiagnostics         `json:"resources"`
}

var agentMissionDesktopStatuses = struct {
	sync.RWMutex
	byRun map[string]AgentMissionDesktopStatus
}{byRun: map[string]AgentMissionDesktopStatus{}}

func publishAgentMissionDesktopStatus(status AgentMissionDesktopStatus) {
	if status.ExecutionRunID == "" {
		return
	}
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = time.Now()
	}
	agentMissionDesktopStatuses.Lock()
	defer agentMissionDesktopStatuses.Unlock()
	if len(agentMissionDesktopStatuses.byRun) >= agentMissionDesktopStatusLimit {
		var oldestRun string
		var oldest time.Time
		for runID, candidate := range agentMissionDesktopStatuses.byRun {
			if oldestRun == "" || candidate.UpdatedAt.Before(oldest) {
				oldestRun = runID
				oldest = candidate.UpdatedAt
			}
		}
		if oldestRun != "" && oldestRun != status.ExecutionRunID {
			delete(agentMissionDesktopStatuses.byRun, oldestRun)
		}
	}
	agentMissionDesktopStatuses.byRun[status.ExecutionRunID] = status
}

func agentMissionDesktopStatusForRun(runID string) (AgentMissionDesktopStatus, bool) {
	if runID == "" {
		return AgentMissionDesktopStatus{}, false
	}
	agentMissionDesktopStatuses.RLock()
	defer agentMissionDesktopStatuses.RUnlock()
	status, ok := agentMissionDesktopStatuses.byRun[runID]
	return status, ok
}

func agentMissionTrackerBudgetSnapshot(tracker *agentMissionBudgetTracker, now time.Time) AgentBudgetSnapshot {
	if tracker == nil {
		return AgentBudgetSnapshot{}
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	return tracker.snapshotLocked(now)
}

func normalizeMissionDesktopSchedulerSnapshot(snapshot AgentSchedulerSnapshot) AgentSchedulerSnapshot {
	for i := range snapshot.Tasks {
		if snapshot.Tasks[i].ResourceClass == "" {
			snapshot.Tasks[i].ResourceClass = AgentResourceModelInference
		}
	}
	return snapshot
}

func agentSchedulerLimitsSnapshot(scheduler *AgentScheduler) AgentResourceLimits {
	if scheduler == nil {
		return normalizeAgentResourceLimits(AgentResourceLimits{})
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return scheduler.limits
}

func publishRunningAgentMissionDesktopStatus(executionRunID, missionID, project, model string, started time.Time, scheduler *AgentScheduler, graph *AgentTaskGraph, tracker *agentMissionBudgetTracker) {
	now := time.Now()
	snapshot := AgentSchedulerSnapshot{}
	if scheduler != nil {
		snapshot = normalizeMissionDesktopSchedulerSnapshot(scheduler.Snapshot(graph, nil))
	}
	publishAgentMissionDesktopStatus(AgentMissionDesktopStatus{
		MissionID:      missionID,
		ExecutionRunID: executionRunID,
		Project:        project,
		Model:          model,
		State:          "running",
		Budget:         agentMissionTrackerBudgetSnapshot(tracker, now),
		Scheduler:      snapshot,
		ResourceLimits: agentSchedulerLimitsSnapshot(scheduler),
		StartedAt:      started,
		UpdatedAt:      now,
	})
}

func publishFinalAgentMissionDesktopStatus(executionRunID string, result AgentReadOnlyMissionResult, started, finished time.Time) {
	limits := normalizeAgentResourceLimits(AgentResourceLimits{})
	if prior, ok := agentMissionDesktopStatusForRun(executionRunID); ok {
		limits = prior.ResourceLimits
	}
	publishAgentMissionDesktopStatus(AgentMissionDesktopStatus{
		MissionID:         result.MissionID,
		ExecutionRunID:    executionRunID,
		Project:           result.Project,
		Model:             result.Model,
		State:             string(result.State),
		Reason:            result.Reason,
		BudgetExhaustedBy: result.BudgetExhaustedBy,
		Budget:            result.Accounting.Budget,
		Scheduler:         normalizeMissionDesktopSchedulerSnapshot(result.Run.Snapshot),
		ResourceLimits:    limits,
		StartedAt:         started,
		UpdatedAt:         finished,
	})
}

func startAgentMissionDesktopStatusMonitor(executionRunID, missionID, project, model string, started time.Time, scheduler *AgentScheduler, graph *AgentTaskGraph, tracker *agentMissionBudgetTracker) func() {
	stop := make(chan struct{})
	done := make(chan struct{})
	publishRunningAgentMissionDesktopStatus(executionRunID, missionID, project, model, started, scheduler, graph, tracker)
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				publishRunningAgentMissionDesktopStatus(executionRunID, missionID, project, model, started, scheduler, graph, tracker)
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func agentSelectedModelAvailable(models []ModelInfo, selected string) bool {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return false
	}
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(model.Name), selected) {
			return true
		}
	}
	return false
}

func agentResourceLimitsForDiagnostics(mission *AgentMissionDesktopStatus) AgentResourceLimits {
	if mission == nil {
		return normalizeAgentResourceLimits(AgentResourceLimits{})
	}
	limits := mission.ResourceLimits
	if limits.MaxQueued <= 0 {
		return normalizeAgentResourceLimits(AgentResourceLimits{})
	}
	return normalizeAgentResourceLimits(limits)
}

func agentResourceSnapshotsForDiagnostics(mission *AgentMissionDesktopStatus, limits AgentResourceLimits) []AgentResourceSnapshot {
	if mission != nil && len(mission.Scheduler.Resources) > 0 {
		return mission.Scheduler.Resources
	}
	classes := []AgentResourceClass{AgentResourceModelInference, AgentResourceReadCPU, AgentResourceBuild, AgentResourceIntegration}
	resources := make([]AgentResourceSnapshot, 0, len(classes))
	for _, class := range classes {
		limit, _ := limits.limitFor(class)
		resources = append(resources, AgentResourceSnapshot{Class: class, Limit: limit, Available: limit})
	}
	return resources
}

func agentOrchestrationDiagnostics(status Status, mission *AgentMissionDesktopStatus) AgentOrchestrationDiagnostics {
	limits := agentResourceLimitsForDiagnostics(mission)
	diagnostics := AgentOrchestrationDiagnostics{
		MissionActive: mission != nil && mission.State == "running",
		Backend: AgentModelBackendDiagnostics{
			Online:                 status.OllamaOnline,
			SelectedModel:          strings.TrimSpace(status.SelectedModel),
			SelectedModelAvailable: agentSelectedModelAvailable(status.Models, status.SelectedModel),
			InstalledModelCount:    len(status.Models),
			Error:                  strings.TrimSpace(status.OllamaError),
		},
	}

	queued := 0
	waitingByClass := map[AgentResourceClass]int{}
	if mission != nil {
		queued = mission.Scheduler.Queued
		for _, task := range mission.Scheduler.Tasks {
			switch task.State {
			case AgentTaskReady:
				diagnostics.LogicalReady++
			case AgentTaskRunning:
				diagnostics.LogicalRunning++
			case AgentTaskBlocked:
				diagnostics.LogicalBlocked++
			}
			if task.QueuePosition > 0 {
				class := task.ResourceClass
				if class == "" {
					class = AgentResourceModelInference
				}
				waitingByClass[class]++
			}
		}
	}
	queueLimit := limits.MaxQueued
	queueAvailable := queueLimit - queued
	if queueAvailable < 0 {
		queueAvailable = 0
	}
	fillPercent := 0
	if queueLimit > 0 {
		fillPercent = queued * 100 / queueLimit
		if fillPercent > 100 {
			fillPercent = 100
		}
	}
	diagnostics.Queue = AgentQueueDiagnostics{
		Queued:      queued,
		Limit:       queueLimit,
		Available:   queueAvailable,
		FillPercent: fillPercent,
		AtLimit:     queueLimit > 0 && queued >= queueLimit,
	}

	for _, resource := range agentResourceSnapshotsForDiagnostics(mission, limits) {
		limit := resource.Limit
		if limit <= 0 {
			limit, _ = limits.limitFor(resource.Class)
		}
		inUse := resource.InUse
		available := limit - inUse
		if available < 0 {
			available = 0
		}
		waiting := waitingByClass[resource.Class]
		atCapacity := limit > 0 && inUse >= limit
		diagnostic := AgentResourceDiagnostics{
			Class:      resource.Class,
			Limit:      limit,
			InUse:      inUse,
			Available:  available,
			Waiting:    waiting,
			AtCapacity: atCapacity,
			Saturated:  atCapacity && waiting > 0,
		}
		if resource.Class == AgentResourceModelInference {
			diagnostics.WaitingForModelInference = waiting
		}
		diagnostics.Resources = append(diagnostics.Resources, diagnostic)
	}

	switch {
	case !diagnostics.Backend.Online:
		diagnostics.State = AgentOrchestrationBackendUnavailable
		diagnostics.Reason = AgentOrchestrationReasonOllamaOffline
	case diagnostics.Backend.SelectedModel == "":
		diagnostics.State = AgentOrchestrationModelUnavailable
		diagnostics.Reason = AgentOrchestrationReasonNoModelSelected
	case !diagnostics.Backend.SelectedModelAvailable:
		diagnostics.State = AgentOrchestrationModelUnavailable
		diagnostics.Reason = AgentOrchestrationReasonSelectedModelMissing
	case diagnostics.Queue.AtLimit:
		diagnostics.State = AgentOrchestrationSaturated
		diagnostics.Reason = AgentOrchestrationReasonQueueLimitReached
	default:
		for _, resource := range diagnostics.Resources {
			if resource.Saturated {
				diagnostics.State = AgentOrchestrationSaturated
				diagnostics.Reason = AgentOrchestrationReasonResourceWaiting
				return diagnostics
			}
		}
		if diagnostics.MissionActive {
			diagnostics.State = AgentOrchestrationActive
			diagnostics.Reason = AgentOrchestrationReasonMissionRunning
		} else {
			diagnostics.State = AgentOrchestrationReady
			diagnostics.Reason = AgentOrchestrationReasonIdle
		}
	}
	return diagnostics
}

// MarshalJSON keeps /api/status as the single Desktop status source while
// attaching read-only Mission telemetry when its execution-scoped RunID is
// known. Orchestration diagnostics are derived observation only; neither the
// Mission registry nor diagnostics alter scheduler policy or recovery state.
func (s Status) MarshalJSON() ([]byte, error) {
	type statusAlias Status
	payload := struct {
		statusAlias
		Mission       *AgentMissionDesktopStatus     `json:"mission,omitempty"`
		Orchestration AgentOrchestrationDiagnostics `json:"orchestration"`
	}{statusAlias: statusAlias(s)}
	if mission, ok := agentMissionDesktopStatusForRun(s.RunID); ok {
		payload.Mission = &mission
	}
	payload.Orchestration = agentOrchestrationDiagnostics(s, payload.Mission)
	return json.Marshal(payload)
}
