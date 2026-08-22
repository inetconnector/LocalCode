// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func admissionTestMaterialization(t *testing.T, state *RunRecoveryState, current MissionProjectBaseline, taskID string) MissionRecoveryContinuationMaterialization {
	t.Helper()
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	observe := func(_ string, observedAt time.Time) MissionProjectBaseline {
		copyCurrent := current
		copyCurrent.CapturedAt = observedAt
		return copyCurrent
	}
	materialized, err := buildStableMissionRecoveryContinuationWithObserver(state.RunID, taskID, observe)
	if err != nil {
		t.Fatal(err)
	}
	return materialized
}

func admissionTestRetryState(t *testing.T, at time.Time) (*RunRecoveryState, MissionProjectBaseline) {
	t.Helper()
	state, current := prepareMissionRecoveryContinuationTestState(t, at)
	child := &state.Mission.Tasks[1]
	child.State = AgentTaskFailed
	child.Lifecycle = &MissionTaskLifecycle{AttemptCount: 1, RetryCount: 0, StateUpdatedAt: at.Add(-time.Second)}
	child.Budget = AgentBudget{}
	child.BudgetSnapshot = &AgentBudgetSnapshot{
		Limit: AgentBudget{
			ModelCalls:           4,
			ToolCalls:            5,
			EstimatedTokenBudget: 1000,
			TimeSeconds:          60,
		},
		Usage: AgentUsage{
			ModelCalls:      2,
			ToolCalls:       1,
			EstimatedTokens: 250,
			ElapsedMillis:   10000,
		},
	}
	return state, current
}

func TestRecoveryAdmissionRestoresTotalBudgetAndCapsOnlyExecutionCopy(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := admissionTestRetryState(t, at)
	materialized := admissionTestMaterialization(t, state, current, "child")
	graph := materialized.Graph

	if err := prepareRecoveryGraphTaskBudgets(materialized, &graph, Config{}); err != nil {
		t.Fatal(err)
	}
	child := continuationGraphTask(t, graph, "child")
	wantTotal := AgentBudget{ModelCalls: 4, ToolCalls: 5, EstimatedTokenBudget: 1000, TimeSeconds: 60}
	if child.Budget != wantTotal {
		t.Fatalf("recovered total budget=%+v want=%+v", child.Budget, wantTotal)
	}

	executionTask, err := capRecoveryExecutionTaskBudget(child, materialized.HistoricalUsageByTask["child"])
	if err != nil {
		t.Fatal(err)
	}
	wantRemaining := AgentBudget{ModelCalls: 2, ToolCalls: 4, EstimatedTokenBudget: 750, TimeSeconds: 50}
	if executionTask.Budget != wantRemaining {
		t.Fatalf("execution remaining budget=%+v want=%+v", executionTask.Budget, wantRemaining)
	}
	if child.Budget != wantTotal {
		t.Fatalf("detached execution cap mutated durable graph budget: %+v", child.Budget)
	}
}

func TestRecoveryAdmissionReservationIsCrashReusableAndCountsOnlyOnRunning(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := admissionTestRetryState(t, at)
	materialized := admissionTestMaterialization(t, state, current, "child")
	firstRunID := "continuation-run-one"
	if err := reserveMissionRecoveryContinuation(materialized, firstRunID, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	reserved, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if reserved.RunID != firstRunID || len(reserved.Mission.Tasks) != len(state.Mission.Tasks) {
		t.Fatalf("reservation lost mission identity/tasks: %#v", reserved)
	}
	child := &reserved.Mission.Tasks[1]
	if child.Lifecycle == nil || child.Lifecycle.AttemptCount != 1 || !child.Lifecycle.AttemptReserved {
		t.Fatalf("reservation consumed attempt instead of reserving it: %#v", child.Lifecycle)
	}
	if child.State != AgentTaskReady || child.Running {
		t.Fatalf("reserved child state=%q running=%v", child.State, child.Running)
	}
	if len(reserved.Events) == 0 || !strings.Contains(reserved.Events[len(reserved.Events)-1].Message, "parent_run=control-run") {
		t.Fatalf("reservation did not retain parent run lineage: %#v", reserved.Events)
	}

	// Simulate a process crash after the durable reservation but before any
	// Scheduler existed. The new run must still materialize and the same
	// reservation must be reusable without consuming another attempt.
	observe := func(_ string, observedAt time.Time) MissionProjectBaseline {
		copyCurrent := current
		copyCurrent.CapturedAt = observedAt
		return copyCurrent
	}
	recoveredMaterialized, err := buildStableMissionRecoveryContinuationWithObserver(firstRunID, "child", observe)
	if err != nil {
		t.Fatal(err)
	}
	secondRunID := "continuation-run-two"
	if err := reserveMissionRecoveryContinuation(recoveredMaterialized, secondRunID, at.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	reReserved, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	child = &reReserved.Mission.Tasks[1]
	if reReserved.RunID != secondRunID || child.Lifecycle.AttemptCount != 1 || !child.Lifecycle.AttemptReserved {
		t.Fatalf("crash reuse double-counted reservation: run=%q lifecycle=%#v", reReserved.RunID, child.Lifecycle)
	}

	// The attempt becomes real exactly at the first durable Running checkpoint.
	snapshot := AgentSchedulerSnapshot{
		MissionID: reReserved.Mission.MissionID,
		Tasks: []AgentTaskScheduleSnapshot{{
			TaskID:  "child",
			State:   AgentTaskRunning,
			Running: true,
			Budget: agentBudgetSnapshot(
				AgentBudget{ModelCalls: 4, ToolCalls: 5, EstimatedTokenBudget: 1000, TimeSeconds: 60},
				recoveredMaterialized.HistoricalUsageByTask["child"],
			),
		}},
	}
	(&AppState{}).journalMissionSchedulerCheckpoint(secondRunID, snapshot, nil)
	running, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	child = &running.Mission.Tasks[1]
	if child.Lifecycle.AttemptCount != 2 || child.Lifecycle.RetryCount != 1 || child.Lifecycle.AttemptReserved {
		t.Fatalf("running checkpoint did not consume exactly one attempt: %#v", child.Lifecycle)
	}
}

func TestRecoveryAdmissionRejectsStaleFingerprintWithoutReservation(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := admissionTestRetryState(t, at)
	materialized := admissionTestMaterialization(t, state, current, "child")
	(&AppState{}).updateRunJournal(state.RunID, func(currentState *RunRecoveryState) {
		currentState.Mission.Reason = "concurrent durable change"
	})

	err := reserveMissionRecoveryContinuation(materialized, "must-not-become-run", at.Add(time.Second))
	if !errors.Is(err, errMissionRecoveryAdmissionStale) {
		t.Fatalf("stale reservation error=%v want stale admission", err)
	}
	persisted, loadErr := loadRunJournal()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if persisted.RunID != state.RunID {
		t.Fatalf("stale admission rotated run id to %q", persisted.RunID)
	}
	child := &persisted.Mission.Tasks[1]
	if child.Lifecycle == nil || child.Lifecycle.AttemptReserved || child.Lifecycle.AttemptCount != 1 {
		t.Fatalf("stale admission mutated lifecycle: %#v", child.Lifecycle)
	}
}

func TestRecoveryContinuationFinalizerPreservesUnrelatedTasks(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := admissionTestRetryState(t, at)
	state.RunID = "execution-run"
	state.Mission.Tasks = append(state.Mission.Tasks, MissionRecoveryTaskState{
		ID:        "unrelated",
		Role:      AgentRoleExplorer,
		Objective: "later work",
		State:     AgentTaskPending,
		Model:     "test-model",
	})
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	graph := AgentTaskGraph{MissionID: state.Mission.MissionID, Tasks: []AgentTask{{
		ID:        "child",
		MissionID: state.Mission.MissionID,
		Role:      AgentRoleExplorer,
		Objective: "inspect child",
		State:     AgentTaskSucceeded,
		Model:     "test-model",
		Result:    AgentResult{Status: AgentResultCompleted},
	}}}
	run := AgentScheduledRun{
		MissionID: state.Mission.MissionID,
		UsageByTask: map[string]AgentUsage{
			"child": {ModelCalls: 3, ToolCalls: 2, EstimatedTokens: 300, ElapsedMillis: 12000},
		},
		Snapshot: AgentSchedulerSnapshot{
			MissionID: state.Mission.MissionID,
			Tasks: []AgentTaskScheduleSnapshot{{
				TaskID: "child",
				State:  AgentTaskSucceeded,
				Budget: agentBudgetSnapshot(AgentBudget{ModelCalls: 4, ToolCalls: 5, EstimatedTokenBudget: 1000, TimeSeconds: 60}, AgentUsage{ModelCalls: 3, ToolCalls: 2, EstimatedTokens: 300, ElapsedMillis: 12000}),
			}},
		},
	}
	if err := finishMissionRecoveryContinuation("execution-run", &graph, run, at.Add(time.Second), nil); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Terminal {
		t.Fatal("partial continuation terminalized mission with unrelated pending work")
	}
	if len(persisted.Mission.Tasks) != 3 {
		t.Fatalf("continuation finalizer dropped tasks: %#v", persisted.Mission.Tasks)
	}
	var unrelated MissionRecoveryTaskState
	for _, task := range persisted.Mission.Tasks {
		if task.ID == "unrelated" {
			unrelated = task
		}
	}
	if unrelated.ID == "" || unrelated.State != AgentTaskPending {
		t.Fatalf("unrelated task was not preserved: %#v", unrelated)
	}
}

func TestRecoveryContinuationFinalizerTerminalizesOnlyFullySuccessfulMission(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := admissionTestRetryState(t, at)
	state.RunID = "execution-run"
	state.Mission.Tasks[1].State = AgentTaskSucceeded
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	graph := AgentTaskGraph{MissionID: state.Mission.MissionID, Tasks: []AgentTask{{
		ID:        "child",
		MissionID: state.Mission.MissionID,
		Role:      AgentRoleExplorer,
		Objective: "inspect child",
		State:     AgentTaskSucceeded,
		Model:     "test-model",
		Result:    AgentResult{Status: AgentResultCompleted},
	}}}
	run := AgentScheduledRun{MissionID: state.Mission.MissionID, UsageByTask: map[string]AgentUsage{"child": {}}}
	if err := finishMissionRecoveryContinuation("execution-run", &graph, run, at.Add(time.Second), nil); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.Terminal || persisted.Phase != "idle" || persisted.Mission.State != string(AgentMissionSucceeded) {
		t.Fatalf("fully successful mission not terminalized: %#v", persisted)
	}
}

func TestRunRecoveryContinuationRejectsActiveAppBeforeExecutor(t *testing.T) {
	state := &AppState{Running: true}
	executed := false
	_, err := state.runMissionRecoveryContinuationWithExecutor(nil, "run", "task", func(context.Context, string, Config, AgentTask) (AgentResult, error) {
		executed = true
		return AgentResult{Status: AgentResultCompleted}, nil
	})
	if !errors.Is(err, errMissionRecoveryControlActiveRun) {
		t.Fatalf("active run error=%v", err)
	}
	if executed {
		t.Fatal("executor ran before AppState admission gate")
	}
}
