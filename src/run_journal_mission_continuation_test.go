// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

func prepareMissionRecoveryContinuationTestState(t *testing.T, at time.Time) (*RunRecoveryState, MissionProjectBaseline) {
	t.Helper()
	state, current := missionRecoveryControlTestState(t, at)
	state.Model = "test-model"
	state.Mission.Model = "test-model"
	state.Mission.Budget = AgentBudget{ModelCalls: 20, ToolCalls: 20, EstimatedTokenBudget: 200000, TimeSeconds: 3600}
	for index := range state.Mission.Tasks {
		state.Mission.Tasks[index].Model = "test-model"
	}
	state.Mission.Tasks[0].BudgetSnapshot = &AgentBudgetSnapshot{
		Usage: AgentUsage{ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 300, ElapsedMillis: 400},
	}
	return state, current
}

func buildMissionRecoveryContinuationTestSnapshot(t *testing.T, state *RunRecoveryState, current MissionProjectBaseline, at time.Time) (string, MissionRecoveryControlSnapshot) {
	t.Helper()
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := buildMissionRecoveryControlSnapshot(state, fingerprint, current, at)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint, snapshot
}

func continuationGraphTask(t *testing.T, graph AgentTaskGraph, taskID string) AgentTask {
	t.Helper()
	for _, task := range graph.Tasks {
		if task.ID == taskID {
			return task
		}
	}
	t.Fatalf("task %q not found in continuation graph", taskID)
	return AgentTask{}
}

func TestMissionRecoveryContinuationMaterializesOnlyVerifiedDependencyClosure(t *testing.T) {
	at := time.Now()
	state, current := prepareMissionRecoveryContinuationTestState(t, at)
	state.Mission.Tasks[0].Capabilities = []AgentCapability{AgentCapabilityPlanning}
	state.Mission.Tasks = append(state.Mission.Tasks, MissionRecoveryTaskState{
		ID:        "other",
		Role:      AgentRoleExplorer,
		Objective: "unrelated pending work",
		State:     AgentTaskPending,
		Model:     "test-model",
	})
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)

	materialized, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child")
	if err != nil {
		t.Fatal(err)
	}
	if materialized.Action != missionRecoveryTransitionResumeCandidate || !materialized.RequiresNewAttempt {
		t.Fatalf("unexpected candidate: %#v", materialized)
	}
	if !materialized.ReadOnly || materialized.ExecutionAuthorized || materialized.SchedulerLeaseGranted || materialized.PersistentStateModified {
		t.Fatalf("materialization gained execution authority: %#v", materialized)
	}
	if len(materialized.Graph.Tasks) != 2 {
		t.Fatalf("continuation graph has %d tasks, want candidate plus one dependency", len(materialized.Graph.Tasks))
	}
	foundation := continuationGraphTask(t, materialized.Graph, "foundation")
	if foundation.State != AgentTaskSucceeded {
		t.Fatalf("foundation state=%q want succeeded", foundation.State)
	}
	if len(foundation.Capabilities) != 2 || foundation.Capabilities[0] != AgentCapabilityRepositoryRead || foundation.Capabilities[1] != AgentCapabilityLSP {
		t.Fatalf("durable capability data was trusted instead of canonical role grants: %#v", foundation.Capabilities)
	}
	child := continuationGraphTask(t, materialized.Graph, "child")
	if child.State != AgentTaskReady {
		t.Fatalf("child state=%q want ready", child.State)
	}
	if _, found := materialized.HistoricalUsageByTask["foundation"]; !found {
		t.Fatal("scheduler-accepted historical usage was not carried forward")
	}
	for _, task := range materialized.Graph.Tasks {
		if task.ID == "other" {
			t.Fatal("unrelated ready work leaked into the bounded continuation graph")
		}
	}
}

func TestMissionRecoveryContinuationMaterializesRetryCandidate(t *testing.T) {
	at := time.Now()
	state, current := prepareMissionRecoveryContinuationTestState(t, at)
	state.Mission.Tasks[1].State = AgentTaskFailed
	state.Mission.Tasks[1].Lifecycle = &MissionTaskLifecycle{AttemptCount: 1, RetryCount: 0}
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)

	materialized, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child")
	if err != nil {
		t.Fatal(err)
	}
	if materialized.Action != missionRecoveryTransitionRetryCandidate || materialized.DurableState != AgentTaskFailed {
		t.Fatalf("unexpected retry materialization: %#v", materialized)
	}
	if got := continuationGraphTask(t, materialized.Graph, "child").State; got != AgentTaskReady {
		t.Fatalf("retry candidate state=%q want ready", got)
	}
}

func TestMissionRecoveryContinuationRejectsNonCandidateAndCapabilityEscalation(t *testing.T) {
	at := time.Now()
	state, current := prepareMissionRecoveryContinuationTestState(t, at)
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	if _, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "foundation"); !errors.Is(err, errMissionRecoveryContinuationCandidate) {
		t.Fatalf("verified dependency error=%v want candidate rejection", err)
	}

	state.Mission.Tasks[1].RequestedCapabilities = []AgentCapability{AgentCapabilityPlanning}
	fingerprint, snapshot = buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	if _, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child"); err == nil {
		t.Fatal("capability escalation in durable recovery metadata was accepted")
	}
}

func TestMissionRecoveryContinuationRejectsConflictingUsageAndExhaustedBudget(t *testing.T) {
	at := time.Now()
	state, current := prepareMissionRecoveryContinuationTestState(t, at)
	state.Mission.Tasks[0].Usage = AgentUsage{ModelCalls: 2}
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	if _, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child"); err == nil {
		t.Fatal("conflicting historical usage evidence was accepted")
	}

	state, current = prepareMissionRecoveryContinuationTestState(t, at)
	state.Mission.Budget.ModelCalls = 1
	fingerprint, snapshot = buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	if _, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child"); !errors.Is(err, errMissionRecoveryContinuationBudget) {
		t.Fatalf("budget error=%v want exhausted budget", err)
	}
}

func TestStableMissionRecoveryContinuationDoesNotWriteJournal(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := prepareMissionRecoveryContinuationTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(runJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	observe := func(_ string, observedAt time.Time) MissionProjectBaseline {
		calls++
		copyCurrent := current
		copyCurrent.CapturedAt = observedAt
		return copyCurrent
	}
	materialized, err := buildStableMissionRecoveryContinuationWithObserver(state.RunID, "child", observe)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(runJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("continuation materialization modified active-run.json")
	}
	if calls != 1 || materialized.TaskID != "child" || materialized.ExecutionAuthorized {
		t.Fatalf("unexpected stable materialization: calls=%d materialized=%#v", calls, materialized)
	}
}

func TestMissionRecoveryContinuationAppStateRejectsActiveRunRaces(t *testing.T) {
	state := &AppState{Running: true}
	called := false
	builder := func(string, string) (MissionRecoveryContinuationMaterialization, error) {
		called = true
		return MissionRecoveryContinuationMaterialization{}, nil
	}
	if _, err := missionRecoveryContinuationForAppState(state, "run", "task", builder); !errors.Is(err, errMissionRecoveryControlActiveRun) {
		t.Fatalf("already-running error=%v", err)
	}
	if called {
		t.Fatal("continuation builder ran while agent was already active")
	}

	state = &AppState{}
	builder = func(string, string) (MissionRecoveryContinuationMaterialization, error) {
		state.mu.Lock()
		state.Running = true
		state.mu.Unlock()
		return MissionRecoveryContinuationMaterialization{ReadOnly: true}, nil
	}
	if _, err := missionRecoveryContinuationForAppState(state, "run", "task", builder); !errors.Is(err, errMissionRecoveryControlActiveRun) {
		t.Fatalf("mid-observation running error=%v", err)
	}
}
