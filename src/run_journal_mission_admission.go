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
	RunID       string            `json:"run_id"`
	ParentRunID string            `json:"parent_run_id"`
	MissionID   string            `json:"mission_id"`
	TaskID      string            `json:"task_id"`
	Action      string            `json:"action"`
	Graph       AgentTaskGraph    `json:"graph"`
	Run         AgentScheduledRun `json:"run"`
}

func missionRecoveryActiveDuration(elapsedMillis int64) (time.Duration, error) {
	if elapsedMillis < 0 || elapsedMillis > math.MaxInt64/int64(time.Millisecond) {
		return 0, fmt.Errorf("invalid historical active mission time")
	}
	return time.Duration(elapsedMillis) * time.Millisecond, nil
}

func conservativeRecoveryIntBudget(explicit, observed int) int {
	switch {
	case explicit > 0 && observed > 0 && observed < explicit:
		return observed
	case explicit > 0:
		return explicit
	default:
		return observed
	}
}

func conservativeRecoveryInt64Budget(explicit, observed int64) int64 {
	switch {
	case explicit > 0 && observed > 0 && observed < explicit:
		return observed
	case explicit > 0:
		return explicit
	default:
		return observed
	}
}

func recoveryTaskTotalBudget(task MissionRecoveryTaskState, cfg Config) (AgentBudget, error) {
	if err := validateAgentMissionBudget(task.Budget); err != nil {
		return AgentBudget{}, fmt.Errorf("task %q: %w", task.ID, err)
	}
	limit := task.Budget
	attempted := task.Lifecycle != nil && task.Lifecycle.AttemptCount > 0
	if attempted {
		if task.BudgetSnapshot == nil {
			return AgentBudget{}, fmt.Errorf("task %q has attempts without budget-snapshot limit evidence", task.ID)
		}
		observed := task.BudgetSnapshot.Limit
		if err := validateAgentMissionBudget(observed); err != nil {
			return AgentBudget{}, fmt.Errorf("task %q has invalid budget-snapshot limit: %w", task.ID, err)
		}
		usage, err := missionRecoveryAcceptedTaskUsage(task)
		if err != nil {
			return AgentBudget{}, err
		}
		if usage.ModelCalls > 0 && observed.ModelCalls <= 0 {
			return AgentBudget{}, fmt.Errorf("task %q has model usage without persisted model-call limit", task.ID)
		}
		if usage.ToolCalls > 0 && observed.ToolCalls <= 0 {
			return AgentBudget{}, fmt.Errorf("task %q has tool usage without persisted tool-call limit", task.ID)
		}
		if usage.EstimatedTokens > 0 && observed.EstimatedTokenBudget <= 0 {
			return AgentBudget{}, fmt.Errorf("task %q has token usage without persisted token limit", task.ID)
		}
		if usage.ElapsedMillis > 0 && observed.TimeSeconds <= 0 {
			return AgentBudget{}, fmt.Errorf("task %q has elapsed usage without persisted time limit", task.ID)
		}
		limit = AgentBudget{
			ModelCalls:           conservativeRecoveryIntBudget(task.Budget.ModelCalls, observed.ModelCalls),
			ToolCalls:            conservativeRecoveryIntBudget(task.Budget.ToolCalls, observed.ToolCalls),
			EstimatedTokenBudget: conservativeRecoveryInt64Budget(task.Budget.EstimatedTokenBudget, observed.EstimatedTokenBudget),
			TimeSeconds:          conservativeRecoveryIntBudget(task.Budget.TimeSeconds, observed.TimeSeconds),
		}
	}
	return normalizeAgentBudget(limit, task.Role, cfg), nil
}

func prepareRecoveryGraphTaskBudgets(materialized MissionRecoveryContinuationMaterialization, graph *AgentTaskGraph, cfg Config) error {
	if graph == nil {
		return errors.New("recovery continuation graph is nil")
	}
	state, fingerprint, err := loadMissionRecoveryControlState(materialized.RunID)
	if err != nil {
		return err
	}
	if fingerprint != materialized.JournalSHA256 {
		return errMissionRecoveryAdmissionStale
	}
	durableByID := make(map[string]MissionRecoveryTaskState, len(state.Mission.Tasks))
	for _, task := range state.Mission.Tasks {
		durableByID[task.ID] = task
	}
	for index := range graph.Tasks {
		task := &graph.Tasks[index]
		durable, ok := durableByID[task.ID]
		if !ok {
			return fmt.Errorf("recovery task %q disappeared before admission", task.ID)
		}
		usage := materialized.HistoricalUsageByTask[task.ID]
		if !missionRecoveryUsageValid(usage) {
			return fmt.Errorf("task %q has invalid historical usage", task.ID)
		}
		limit, err := recoveryTaskTotalBudget(durable, cfg)
		if err != nil {
			return err
		}
		task.Budget = limit
		snapshot := agentBudgetSnapshot(limit, usage)
		if task.State == AgentTaskReady && snapshot.Exhausted {
			return fmt.Errorf("%w: task %q %s", errMissionRecoveryContinuationBudget, task.ID, snapshot.ExhaustedBy)
		}
	}
	return nil
}

func capRecoveryExecutionTaskBudget(task AgentTask, historical AgentUsage) (AgentTask, error) {
	if !missionRecoveryUsageValid(historical) {
		return task, fmt.Errorf("task %q has invalid historical usage", task.ID)
	}
	snapshot := agentBudgetSnapshot(task.Budget, historical)
	if snapshot.Exhausted {
		return task, fmt.Errorf("%w: task %q %s", errMissionRecoveryContinuationBudget, task.ID, snapshot.ExhaustedBy)
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
	return task, nil
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

func writeMissionRecoveryJournalAtVersion(state RunRecoveryState, expected fileVersion) error {
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
	return atomicWriteFileIfVersion(runJournalPath(), data, 0o600, expected)
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
	missionAttempts := 0
	for index := range state.Mission.Tasks {
		if lifecycle := state.Mission.Tasks[index].Lifecycle; lifecycle != nil {
			if lifecycle.AttemptCount < 0 {
				return errMissionRecoveryContinuationCandidate
			}
			missionAttempts += lifecycle.AttemptCount
		}
		if state.Mission.Tasks[index].ID == materialized.TaskID {
			candidateIndex = index
		}
	}
	if candidateIndex < 0 {
		return errMissionRecoveryContinuationCandidate
	}
	candidate := &state.Mission.Tasks[candidateIndex]
	if candidate.Lifecycle == nil {
		candidate.Lifecycle = &MissionTaskLifecycle{}
	}
	if candidate.Lifecycle.AttemptCount < 0 || candidate.Lifecycle.AttemptCount >= missionRecoveryMaxTaskAttempts || missionAttempts >= missionRecoveryMaxMissionAttempts {
		return errMissionRecoveryContinuationCandidate
	}
	if candidate.Lifecycle.AttemptReserved != !candidate.Lifecycle.AttemptReservedAt.IsZero() {
		return errMissionRecoveryContinuationCandidate
	}
	// Persist admission intent before any Scheduler exists, but do not consume
	// retry budget yet. AttemptCount advances only at the first durable Running
	// checkpoint. A crash in this gap therefore leaves a reusable reservation.
	if !candidate.Lifecycle.AttemptReserved {
		candidate.Lifecycle.AttemptReserved = true
		candidate.Lifecycle.AttemptReservedAt = admittedAt
	}
	candidate.Lifecycle.StateUpdatedAt = admittedAt
	candidate.State = AgentTaskReady
	candidate.Running = false
	candidate.QueuePosition = 0
	candidate.AdmissionBlockedReason = ""
	candidate.CompletionEvidence = nil

	parentRunID := state.RunID
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
	state.Events = append(state.Events, RunJournalEvent{
		At:     admittedAt,
		Type:   "mission_continuation_reserved",
		Action: materialized.Action,
		Message: sanitizeRunJournalText(
			"task="+materialized.TaskID+";parent_run="+parentRunID,
			400,
		),
	})
	if err := writeMissionRecoveryJournalAtVersion(*state, expected); err != nil {
		return fmt.Errorf("%w: %v", errMissionRecoveryAdmissionStale, err)
	}
	return nil
}

func cancelUnfinishedMissionRecoveryTasks(mission *MissionRecoveryState) {
	if mission == nil {
		return
	}
	for index := range mission.Tasks {
		task := &mission.Tasks[index]
		switch task.State {
		case AgentTaskSucceeded, AgentTaskCompleted, AgentTaskFailed, AgentTaskCancelled:
			continue
		}
		task.State = AgentTaskCancelled
		task.Running = false
		task.QueuePosition = 0
		task.AdmissionBlockedReason = ""
		if task.Lifecycle != nil {
			task.Lifecycle.AttemptReserved = false
			task.Lifecycle.AttemptReservedAt = time.Time{}
		}
	}
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
	allSucceeded := len(state.Mission.Tasks) > 0
	for index := range state.Mission.Tasks {
		durable := &state.Mission.Tasks[index]
		if task, ok := graphByID[durable.ID]; ok {
			durable.State = task.State
			durable.Running = false
			if durable.Lifecycle != nil {
				durable.Lifecycle.AttemptReserved = false
				durable.Lifecycle.AttemptReservedAt = time.Time{}
				if durable.Lifecycle.LastFinishedAt.IsZero() {
					durable.Lifecycle.LastFinishedAt = finishedAt
				}
				durable.Lifecycle.StateUpdatedAt = finishedAt
			}
		}
		usage, exists := run.UsageByTask[durable.ID]
		if exists {
			if !missionRecoveryUsageValid(usage) {
				return fmt.Errorf("task %q has invalid continued usage", durable.ID)
			}
			durable.Usage = usage
		} else {
			var usageErr error
			usage, usageErr = missionRecoveryAcceptedTaskUsage(*durable)
			if usageErr != nil {
				return usageErr
			}
		}
		usageByTask[durable.ID] = usage
		if !isSuccessfulAgentTaskState(durable.State) {
			allSucceeded = false
		}
	}
	accounting := agentMissionAccounting(state.Mission.Budget, usageByTask, state.Mission.StartedAt, finishedAt)
	state.Mission.Accounting = &accounting
	state.Mission.UpdatedAt = finishedAt
	state.UpdatedAt = finishedAt

	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		cancelUnfinishedMissionRecoveryTasks(state.Mission)
		state.Mission.State = string(AgentMissionCancelled)
		state.Mission.Reason = string(AgentMissionReasonCancelled)
		state.Mission.BudgetExhaustedBy = ""
		state.Phase = "idle"
		state.Terminal = true
		state.Outcome = string(AgentMissionCancelled) + ":" + string(AgentMissionReasonCancelled)
		state.Events = append(state.Events, RunJournalEvent{At: finishedAt, Type: "mission_end", Action: missionRecoveryKindReadOnly, Message: state.Outcome})
		if err := writeMissionRecoveryJournalAtVersion(*state, expected); err != nil {
			return fmt.Errorf("%w: %v", errMissionRecoveryAdmissionStale, err)
		}
		return nil
	}

	if runErr == nil && allSucceeded {
		state.Mission.State = string(AgentMissionSucceeded)
		state.Mission.Reason = string(AgentMissionReasonCompleted)
		state.Mission.BudgetExhaustedBy = ""
		state.Phase = "idle"
		state.Terminal = true
		state.Outcome = string(AgentMissionSucceeded) + ":" + string(AgentMissionReasonCompleted)
		state.Events = append(state.Events, RunJournalEvent{At: finishedAt, Type: "mission_end", Action: missionRecoveryKindReadOnly, Message: state.Outcome})
		if err := writeMissionRecoveryJournalAtVersion(*state, expected); err != nil {
			return fmt.Errorf("%w: %v", errMissionRecoveryAdmissionStale, err)
		}
		return nil
	}

	state.Mission.State = missionRecoveryRunning
	state.Mission.Reason = ""
	state.Mission.BudgetExhaustedBy = ""
	if accounting.Budget.Exhausted {
		state.Mission.BudgetExhaustedBy = accounting.Budget.ExhaustedBy
	}
	state.Phase = "mission-read-only"
	state.Terminal = false
	state.Outcome = ""
	message := "checkpoint"
	if runErr != nil {
		message = sanitizeRunJournalText(runErr.Error(), 200)
	}
	state.Events = append(state.Events, RunJournalEvent{At: finishedAt, Type: "mission_continuation_checkpoint", Action: missionRecoveryKindReadOnly, Message: message})
	if err := writeMissionRecoveryJournalAtVersion(*state, expected); err != nil {
		return fmt.Errorf("%w: %v", errMissionRecoveryAdmissionStale, err)
	}
	return nil
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
	cfg := s.Config
	materialized, err := buildStableMissionRecoveryContinuationWithObserver(runID, taskID, nil)
	if err != nil {
		s.mu.Unlock()
		return out, err
	}
	graph := materialized.Graph
	if err := prepareRecoveryGraphTaskBudgets(materialized, &graph, cfg); err != nil {
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
		remainingTask, remainingErr := capRecoveryExecutionTaskBudget(task, materialized.HistoricalUsageByTask[task.ID])
		if remainingErr != nil {
			return AgentResult{Status: AgentResultBudgetExhausted, Summary: remainingErr.Error()}, remainingErr
		}
		constrained, allowed := tracker.prepareTask(remainingTask)
		if !allowed {
			return AgentResult{Status: AgentResultBudgetExhausted, Summary: "Mission budget exhausted before task: " + tracker.blockedDimension()}, nil
		}
		result, childErr := execute(childCtx, childProject, childCfg, constrained)
		tracker.recordObservedUsage(result.Usage)
		return result, childErr
	}

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
	out = MissionRecoveryContinuationExecution{
		RunID:       executionRunID,
		ParentRunID: materialized.RunID,
		MissionID:   materialized.MissionID,
		TaskID:      materialized.TaskID,
		Action:      materialized.Action,
		Graph:       graph,
		Run:         run,
	}
	return out, runErr
}

// RunMissionRecoveryContinuation is the first execution-capable recovery
// boundary. It recomputes #67 materialization while holding the AppState run
// gate, durably reserves the next attempt against the exact journal fingerprint,
// and only then creates a Scheduler that may admit read-only child work.
func (s *AppState) RunMissionRecoveryContinuation(ctx context.Context, runID, taskID string) (MissionRecoveryContinuationExecution, error) {
	return s.runMissionRecoveryContinuationWithExecutor(ctx, runID, taskID, s.runNativeReadOnlyAgentTask)
}
