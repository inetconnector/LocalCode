// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var errMissionRecoveryAdmissionStale = errors.New("mission recovery admission journal changed")

type MissionRecoveryContinuationExecution struct {
	RunID     string            `json:"run_id"`
	MissionID string            `json:"mission_id"`
	TaskID    string            `json:"task_id"`
	Action    string            `json:"action"`
	Graph     AgentTaskGraph    `json:"graph"`
	Run       AgentScheduledRun `json:"run"`
}

func missionRecoveryActiveDuration(elapsedMillis int64) (time.Duration, error) {
	if elapsedMillis < 0 || elapsedMillis > math.MaxInt64/int64(time.Millisecond) {
		return 0, fmt.Errorf("invalid historical active mission time")
	}
	return time.Duration(elapsedMillis) * time.Millisecond, nil
}

func capRecoveryGraphTaskBudgets(graph *AgentTaskGraph, historical map[string]AgentUsage) error {
	if graph == nil {
		return errors.New("recovery continuation graph is nil")
	}
	for index := range graph.Tasks {
		task := &graph.Tasks[index]
		usage := historical[task.ID]
		if !missionRecoveryUsageValid(usage) {
			return fmt.Errorf("task %q has invalid historical usage", task.ID)
		}
		snapshot := agentBudgetSnapshot(task.Budget, usage)
		if task.State == AgentTaskReady && snapshot.Exhausted {
			return fmt.Errorf("%w: task %q %s", errMissionRecoveryContinuationBudget, task.ID, snapshot.ExhaustedBy)
		}
		if task.State != AgentTaskReady {
			continue
		}
		if task.Budget.ModelCalls > 0 {
			task.Budget.ModelCalls = snapshot.Remaining.ModelCalls
		}
		if task.Budget.ToolCalls > 0 {
			task.Budget.ToolCalls = snapshot.Remaining.ToolCalls
		}
		if task.Budget.EstimatedTokenBudget > 0 {
			task.Budget.EstimatedTokenBudget = snapshot.Remaining.EstimatedTokenBudget
		}
		if task.Budget.TimeSeconds > 0 {
			task.Budget.TimeSeconds = snapshot.Remaining.TimeSeconds
		}
	}
	return nil
}

func newRecoveryMissionBudgetTracker(limit AgentBudget, historical AgentUsage, admittedAt time.Time) (*agentMissionBudgetTracker, error) {
	if !missionRecoveryUsageValid(historical) {
		return nil, fmt.Errorf("invalid historical mission usage")
	}
	active, err := missionRecoveryActiveDuration(historical.ElapsedMillis)
	if err != nil {
		return nil, err
	}
	if admittedAt.IsZero() {
		admittedAt = time.Now()
	}
	tracker := newAgentMissionBudgetTracker(limit, admittedAt.Add(-active))
	tracker.usage = AgentUsage{
		ModelCalls:      historical.ModelCalls,
		ToolCalls:       historical.ToolCalls,
		EstimatedTokens: historical.EstimatedTokens,
	}
	return tracker, nil
}

func reserveMissionRecoveryContinuation(materialized MissionRecoveryContinuationMaterialization, executionRunID string, admittedAt time.Time) error {
	executionRunID = strings.TrimSpace(executionRunID)
	if executionRunID == "" || admittedAt.IsZero() || !materialized.RequiresNewAttempt || !validMissionVerificationDigest(materialized.JournalSHA256) {
		return errMissionRecoveryContinuationUnavailable
	}
	active, err := missionRecoveryActiveDuration(materialized.MissionBudgetSnapshot.Usage.ElapsedMillis)
	if err != nil {
		return err
	}

	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	path := runJournalPath()
	expected, err := readFileVersion(path)
	if err != nil {
		return err
	}
	state, err := loadRunJournal()
	if err != nil {
		return err
	}
	if err := verifyFileVersion(path, expected); err != nil {
		return fmt.Errorf("%w: %v", errMissionRecoveryAdmissionStale, err)
	}
	if state == nil || state.Terminal || state.Mission == nil || state.RunID != materialized.RunID || state.Mission.MissionID != materialized.MissionID || state.Phase != "mission-read-only" {
		return errMissionRecoveryContinuationUnavailable
	}
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		return err
	}
	if fingerprint != materialized.JournalSHA256 {
		return errMissionRecoveryAdmissionStale
	}

	candidateIndex := -1
	for index := range state.Mission.Tasks {
		if state.Mission.Tasks[index].ID == materialized.TaskID {
			candidateIndex = index
			break
		}
	}
	if candidateIndex < 0 {
		return errMissionRecoveryContinuationCandidate
	}
	candidate := &state.Mission.Tasks[candidateIndex]
	if candidate.Lifecycle == nil {
		candidate.Lifecycle = &MissionTaskLifecycle{}
	}
	if candidate.Lifecycle.AttemptReserved || candidate.Lifecycle.AttemptCount < 0 || candidate.Lifecycle.AttemptCount >= missionRecoveryMaxTaskAttempts {
		return errMissionRecoveryContinuationCandidate
	}
	candidate.Lifecycle.AttemptCount++
	candidate.Lifecycle.RetryCount = candidate.Lifecycle.AttemptCount - 1
	if candidate.Lifecycle.RetryCount < 0 {
		candidate.Lifecycle.RetryCount = 0
	}
	candidate.Lifecycle.AttemptReserved = true
	candidate.Lifecycle.AttemptReservedAt = admittedAt
	candidate.Lifecycle.StateUpdatedAt = admittedAt
	candidate.State = AgentTaskReady
	candidate.Running = false
	candidate.QueuePosition = 0
	candidate.AdmissionBlockedReason = ""
	candidate.CompletionEvidence = nil

	// Rebase the active-time anchor so crash/offline downtime remains excluded
	// while all previously consumed active wall time remains charged.
	state.Mission.StartedAt = admittedAt.Add(-active)
	state.Mission.UpdatedAt = admittedAt
	state.Mission.State = missionRecoveryRunning
	state.Mission.Reason = ""
	state.Mission.BudgetExhaustedBy = ""
	state.RunID = executionRunID
	state.StartedAt = admittedAt
	state.UpdatedAt = admittedAt
	state.Terminal = false
	state.Outcome = ""
	state.Events = append(state.Events, RunJournalEvent{At: admittedAt, Type: "mission_continuation_reserved", Action: materialized.Action, Message: sanitizeRunJournalText(materialized.TaskID, 160)})
	if len(state.Events) > 64 {
		state.Events = append([]RunJournalEvent(nil), state.Events[len(state.Events)-64:]...)
	}
	state.SchemaVersion = runJournalSchemaVersion
	data, err := json.MarshalIndent(*state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := atomicWriteFileIfVersion(path, data, 0o600, expected); err != nil {
		return fmt.Errorf("%w: %v", errMissionRecoveryAdmissionStale, err)
	}
	return nil
}

func finishMissionRecoveryContinuation(executionRunID string, graph *AgentTaskGraph, run AgentScheduledRun, finishedAt time.Time, runErr error) error {
	if strings.TrimSpace(executionRunID) == "" || graph == nil {
		return errMissionRecoveryContinuationUnavailable
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	state, err := loadRunJournal()
	if err != nil {
		return err
	}
	if state == nil || state.Terminal || state.Mission == nil || state.RunID != executionRunID || state.Mission.MissionID != graph.MissionID {
		return errMissionRecoveryContinuationUnavailable
	}

	applyMissionSchedulerSnapshot(state.Mission, run.Snapshot)
	applyMissionCompletionEvidence(state.Mission, graph, finishedAt)
	graphByID := make(map[string]AgentTask, len(graph.Tasks))
	for _, task := range graph.Tasks {
		graphByID[task.ID] = task
	}
	usageByTask := make(map[string]AgentUsage, len(state.Mission.Tasks))
	for index := range state.Mission.Tasks {
		durable := &state.Mission.Tasks[index]
		if task, ok := graphByID[durable.ID]; ok {
			durable.State = task.State
			durable.Running = false
			if usage, exists := run.UsageByTask[durable.ID]; exists {
				durable.Usage = usage
			}
			if durable.Lifecycle != nil && durable.Lifecycle.AttemptReserved {
				durable.Lifecycle.AttemptReserved = false
				durable.Lifecycle.AttemptReservedAt = time.Time{}
				durable.Lifecycle.LastFinishedAt = finishedAt
			}
		}
		usageByTask[durable.ID] = durable.Usage
	}
	accounting := agentMissionAccounting(state.Mission.Budget, usageByTask, state.Mission.StartedAt, finishedAt)
	state.Mission.Accounting = &accounting
	state.Mission.State = missionRecoveryRunning
	state.Mission.Reason = ""
	state.Mission.BudgetExhaustedBy = ""
	if accounting.Budget.Exhausted {
		state.Mission.BudgetExhaustedBy = accounting.Budget.ExhaustedBy
	}
	state.Mission.UpdatedAt = finishedAt
	state.Phase = "mission-read-only"
	state.UpdatedAt = finishedAt
	state.Terminal = false
	state.Outcome = ""
	message := "checkpoint"
	if runErr != nil {
		message = sanitizeRunJournalText(runErr.Error(), 200)
	}
	state.Events = append(state.Events, RunJournalEvent{At: finishedAt, Type: "mission_continuation_checkpoint", Action: missionRecoveryKindReadOnly, Message: message})
	return writeRunJournalUnlocked(*state)
}

func (s *AppState) runMissionRecoveryContinuationWithExecutor(ctx context.Context, runID, taskID string, execute scheduledReadOnlyAgentExecutor) (MissionRecoveryContinuationExecution, error) {
	var out MissionRecoveryContinuationExecution
	if s == nil {
		return out, errMissionRecoveryContinuationUnavailable
	}
	if execute == nil {
		return out, errors.New("read-only mission executor is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}

	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		return out, errMissionRecoveryControlActiveRun
	}
	materialized, err := buildStableMissionRecoveryContinuationWithObserver(runID, taskID, nil)
	if err != nil {
		s.mu.Unlock()
		return out, err
	}
	graph := materialized.Graph
	if err := capRecoveryGraphTaskBudgets(&graph, materialized.HistoricalUsageByTask); err != nil {
		s.mu.Unlock()
		return out, err
	}
	admittedAt := time.Now()
	executionRunID := newID()
	if err := reserveMissionRecoveryContinuation(materialized, executionRunID, admittedAt); err != nil {
		s.mu.Unlock()
		return out, err
	}
	missionCtx, cancel := context.WithCancel(ctx)
	s.Running = true
	s.Cancel = cancel
	s.RunID = executionRunID
	s.RunPhase = "mission-read-only-continuation"
	s.RunStartedAt = admittedAt
	s.LastProgressAt = admittedAt
	s.Project = materialized.Project
	s.Model = materialized.Model
	s.Recovery = nil
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		if s.RunID == executionRunID {
			s.Running = false
			s.Cancel = nil
			s.RunPhase = "idle"
			s.LastProgressAt = time.Now()
			s.Recovery = loadRecoverableRun()
		}
		s.mu.Unlock()
	}()

	tracker, err := newRecoveryMissionBudgetTracker(materialized.MissionBudget, materialized.MissionBudgetSnapshot.Usage, admittedAt)
	if err != nil {
		_ = finishMissionRecoveryContinuation(executionRunID, &graph, AgentScheduledRun{MissionID: graph.MissionID, UsageByTask: materialized.HistoricalUsageByTask}, time.Now(), err)
		return out, err
	}
	budgetedExecute := func(childCtx context.Context, childProject string, childCfg Config, task AgentTask) (AgentResult, error) {
		constrained, allowed := tracker.prepareTask(task)
		if !allowed {
			return AgentResult{Status: AgentResultBudgetExhausted, Summary: "Mission budget exhausted before task: " + tracker.blockedDimension()}, nil
		}
		result, childErr := execute(childCtx, childProject, childCfg, constrained)
		tracker.recordObservedUsage(result.Usage)
		return result, childErr
	}

	s.mu.RLock()
	cfg := s.Config
	s.mu.RUnlock()
	scheduler := NewAgentScheduler(missionCtx, AgentResourceLimits{})
	defer scheduler.missionCancel()
	checkpoint := func(snapshot AgentSchedulerSnapshot) {
		s.journalMissionSchedulerCheckpoint(executionRunID, snapshot, &graph)
	}
	run, runErr := s.runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded(materialized.Project, cfg, &graph, scheduler, budgetedExecute, checkpoint, materialized.HistoricalUsageByTask)
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		cancelUnfinishedReadOnlyMissionTasks(&graph)
		run.Snapshot = scheduler.Snapshot(&graph, run.UsageByTask)
		s.journalMissionSchedulerCheckpoint(executionRunID, run.Snapshot, &graph)
	}
	finishedAt := time.Now()
	if finishErr := finishMissionRecoveryContinuation(executionRunID, &graph, run, finishedAt, runErr); finishErr != nil && runErr == nil {
		runErr = finishErr
	}
	out = MissionRecoveryContinuationExecution{RunID: executionRunID, MissionID: materialized.MissionID, TaskID: materialized.TaskID, Action: materialized.Action, Graph: graph, Run: run}
	return out, runErr
}

// RunMissionRecoveryContinuation is the first execution-capable recovery
// boundary. It recomputes #67 materialization while holding the AppState run
// gate, durably reserves the next attempt against the exact journal fingerprint,
// and only then creates a Scheduler that may admit read-only child work.
func (s *AppState) RunMissionRecoveryContinuation(ctx context.Context, runID, taskID string) (MissionRecoveryContinuationExecution, error) {
	return s.runMissionRecoveryContinuationWithExecutor(ctx, runID, taskID, s.runNativeReadOnlyAgentTask)
}
