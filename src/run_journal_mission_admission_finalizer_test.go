// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"
	"time"
)

func TestRecoveryContinuationFinalizerAccountsUntouchedHistoricalUsage(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := admissionTestRetryState(t, at)
	state.RunID = "execution-accounting-run"
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	graph := AgentTaskGraph{MissionID: state.Mission.MissionID, Tasks: []AgentTask{{
		ID:        "child",
		MissionID: state.Mission.MissionID,
		Role:      AgentRoleExplorer,
		Objective: "inspect child",
		State:     AgentTaskFailed,
		Model:     "test-model",
		Result:    AgentResult{Status: AgentResultBlocked},
	}}}
	cumulativeChild := AgentUsage{ModelCalls: 3, ToolCalls: 2, EstimatedTokens: 300, ElapsedMillis: 12000}
	run := AgentScheduledRun{
		MissionID:   state.Mission.MissionID,
		UsageByTask: map[string]AgentUsage{"child": cumulativeChild},
		Snapshot: AgentSchedulerSnapshot{
			MissionID: state.Mission.MissionID,
			Tasks: []AgentTaskScheduleSnapshot{{
				TaskID: "child",
				State:  AgentTaskFailed,
				Budget: agentBudgetSnapshot(AgentBudget{ModelCalls: 4, ToolCalls: 5, EstimatedTokenBudget: 1000, TimeSeconds: 60}, cumulativeChild),
			}},
		},
	}
	if err := finishMissionRecoveryContinuation("execution-accounting-run", &graph, run, at.Add(time.Second), nil); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Mission.Accounting == nil {
		t.Fatal("continuation finalizer did not persist Mission accounting")
	}
	// The untouched foundation task carries accepted historical usage only in
	// BudgetSnapshot.Usage in the fixture. It must still contribute to global
	// Mission accounting together with the cumulative child usage.
	want := AgentUsage{ModelCalls: 4, ToolCalls: 4, EstimatedTokens: 600}
	got := persisted.Mission.Accounting.Usage
	if got.ModelCalls != want.ModelCalls || got.ToolCalls != want.ToolCalls || got.EstimatedTokens != want.EstimatedTokens {
		t.Fatalf("mission accounting dropped untouched historical usage: got=%+v want(non-time)=%+v", got, want)
	}
	if persisted.Mission.Accounting.ChildWorkMillis != 12400 {
		t.Fatalf("child work=%d want=12400", persisted.Mission.Accounting.ChildWorkMillis)
	}
}

func TestRecoveryContinuationCancellationTerminalizesWholeMission(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := admissionTestRetryState(t, at)
	state.RunID = "execution-cancel-run"
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
		State:     AgentTaskCancelled,
		Model:     "test-model",
	}}}
	run := AgentScheduledRun{MissionID: state.Mission.MissionID, UsageByTask: map[string]AgentUsage{}}
	if err := finishMissionRecoveryContinuation("execution-cancel-run", &graph, run, at.Add(time.Second), context.Canceled); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.Terminal || persisted.Phase != "idle" {
		t.Fatalf("cancelled continuation remained recoverable: %#v", persisted)
	}
	if persisted.Mission.State != string(AgentMissionCancelled) || persisted.Mission.Reason != string(AgentMissionReasonCancelled) {
		t.Fatalf("cancelled mission state/reason=%q/%q", persisted.Mission.State, persisted.Mission.Reason)
	}
	for _, task := range persisted.Mission.Tasks {
		if task.ID == "unrelated" && task.State != AgentTaskCancelled {
			t.Fatalf("unrelated unfinished task survived Mission cancellation: %#v", task)
		}
	}
}
