// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
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
	StartedAt         time.Time              `json:"started_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
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
		StartedAt:      started,
		UpdatedAt:      now,
	})
}

func publishFinalAgentMissionDesktopStatus(executionRunID string, result AgentReadOnlyMissionResult, started, finished time.Time) {
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

// MarshalJSON keeps /api/status as the single Desktop status source while
// attaching read-only Mission telemetry when its execution-scoped RunID is
// known. The registry is ephemeral observation only; run_journal.go remains
// the sole durable recovery authority.
func (s Status) MarshalJSON() ([]byte, error) {
	type statusAlias Status
	payload := struct {
		statusAlias
		Mission *AgentMissionDesktopStatus `json:"mission,omitempty"`
	}{statusAlias: statusAlias(s)}
	if mission, ok := agentMissionDesktopStatusForRun(s.RunID); ok {
		payload.Mission = &mission
	}
	return json.Marshal(payload)
}
