// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
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
	firstAdmittedAt := time.Now()
	if err := reserveMissionRecoveryContinuation(materialized, firstRunID, firstAdmittedAt); err != nil {
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
	secondAdmittedAt := time.Now()
	if err := reserveMissionRecoveryContinuation(recoveredMaterialized, secondRunID, secondAdmittedAt); err != nil {
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

	err := reserveMissionRecoveryContinuation(materialized, "must-not-become-run", time.Now())
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
	if err := finishMissionRecoveryContinuation("execution-run", &graph, run, time.Now(), nil); err != nil {
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
	if err := finishMissionRecoveryContinuation("execution-run", &graph, run, time.Now(), nil); err != nil {
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

func prepareMissionRecoveryAdmissionTestGitProject(t *testing.T, at time.Time) (string, MissionProjectBaseline) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed in test environment")
	}
	project := t.TempDir()
	cfg := normalizeConfig(Config{SchemaVersion: 4, GitEnabled: true, AutoDiscoverTools: true, ToolOverrides: map[string]string{}, EnvironmentVars: map[string]string{}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if out, err := initializeGitRepository(ctx, project, cfg); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	testFile := filepath.Join(project, "file.txt")
	if err := os.WriteFile(testFile, []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "git", "add", "file.txt", ".gitignore")
	cmd.Dir = project
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.CommandContext(ctx, "git", "commit", "-m", "init")
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	baseline := observeMissionProjectBaseline(project, at.Add(-time.Minute), nil)
	return project, baseline
}

func prepareMissionRecoveryAdmissionTestState(t *testing.T, at time.Time) (*RunRecoveryState, MissionProjectBaseline) {
	t.Helper()
	project, baseline := prepareMissionRecoveryAdmissionTestGitProject(t, at)
	foundation := MissionRecoveryTaskState{
		ID:        "foundation",
		Role:      AgentRoleExplorer,
		Objective: "inspect foundation",
		State:     AgentTaskSucceeded,
		Model:     "test-model",
		Budget:    AgentBudget{ModelCalls: 8, ToolCalls: 12, EstimatedTokenBudget: 100000, TimeSeconds: 180},
		Usage:     AgentUsage{ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 300, ElapsedMillis: 400},
		BudgetSnapshot: &AgentBudgetSnapshot{
			Usage: AgentUsage{ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 300, ElapsedMillis: 400},
		},
		CompletionEvidence: missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, at.Add(-time.Second)),
	}
	child := MissionRecoveryTaskState{
		ID:           "child",
		Role:         AgentRoleExplorer,
		Objective:    "inspect child",
		Dependencies: []string{"foundation"},
		State:        AgentTaskPending,
		Model:        "test-model",
		Budget:       AgentBudget{ModelCalls: 8, ToolCalls: 12, EstimatedTokenBudget: 100000, TimeSeconds: 180},
	}
	state := &RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         "admission-run",
		Project:       project,
		Model:         "test-model",
		Phase:         "mission-read-only",
		StartedAt:     at.Add(-time.Minute),
		UpdatedAt:     at.Add(-time.Second),
		Mission: &MissionRecoveryState{
			Kind:      missionRecoveryKindReadOnly,
			MissionID: "admission-mission",
			Project:   project,
			Model:     "test-model",
			State:     missionRecoveryRunning,
			Baseline:  &baseline,
			Budget:    AgentBudget{ModelCalls: 20, ToolCalls: 20, EstimatedTokenBudget: 200000, TimeSeconds: 3600},
			Tasks:     []MissionRecoveryTaskState{foundation, child},
			StartedAt: at.Add(-time.Minute),
			UpdatedAt: at.Add(-time.Second),
		},
	}
	current := observeMissionProjectBaseline(project, at, nil)
	return state, current
}

func TestMissionRecoveryAdmissionCapBudgets(t *testing.T) {
	graph := AgentTaskGraph{
		MissionID: "test-mission",
		Tasks: []AgentTask{
			{
				ID:    "task-ready",
				State: AgentTaskReady,
				Budget: AgentBudget{
					ModelCalls:           10,
					ToolCalls:            20,
					EstimatedTokenBudget: 50000,
					TimeSeconds:          120,
				},
			},
			{
				ID:    "task-succeeded",
				State: AgentTaskSucceeded,
				Budget: AgentBudget{
					ModelCalls: 10,
				},
			},
		},
	}

	historical := map[string]AgentUsage{
		"task-ready": {
			ModelCalls:      3,
			ToolCalls:       5,
			EstimatedTokens: 10000,
			ElapsedMillis:   20000,
		},
		"task-succeeded": {
			ModelCalls: 10,
		},
	}

	ready := graph.Tasks[0]
	executionTask, err := capRecoveryExecutionTaskBudget(ready, historical["task-ready"])
	if err != nil {
		t.Fatalf("unexpected error capping budgets: %v", err)
	}

	if executionTask.Budget.ModelCalls != 7 {
		t.Fatalf("ready task ModelCalls=%d, want 7", executionTask.Budget.ModelCalls)
	}
	if executionTask.Budget.ToolCalls != 15 {
		t.Fatalf("ready task ToolCalls=%d, want 15", executionTask.Budget.ToolCalls)
	}
	if executionTask.Budget.EstimatedTokenBudget != 40000 {
		t.Fatalf("ready task EstimatedTokenBudget=%d, want 40000", executionTask.Budget.EstimatedTokenBudget)
	}
	if executionTask.Budget.TimeSeconds != 100 {
		t.Fatalf("ready task TimeSeconds=%d, want 100", executionTask.Budget.TimeSeconds)
	}

	// Test exhausted ready task budget
	exhaustedTask := AgentTask{
		ID:    "task-exhausted",
		State: AgentTaskReady,
		Budget: AgentBudget{
			ModelCalls: 5,
		},
	}
	exhaustedUsage := AgentUsage{
		ModelCalls: 5,
	}
	if _, err := capRecoveryExecutionTaskBudget(exhaustedTask, exhaustedUsage); !errors.Is(err, errMissionRecoveryContinuationBudget) {
		t.Fatalf("expected budget exhaustion error, got: %v", err)
	}

	// Test invalid historical usage
	invalidUsage := AgentUsage{
		ModelCalls: -1,
	}
	if _, err := capRecoveryExecutionTaskBudget(ready, invalidUsage); err == nil {
		t.Fatal("expected error for negative historical usage")
	}
}

func TestMissionRecoveryAdmissionBudgetTracker(t *testing.T) {
	limit := AgentBudget{
		ModelCalls:           50,
		ToolCalls:            100,
		EstimatedTokenBudget: 500000,
		TimeSeconds:          3600,
	}
	historical := AgentUsage{
		ModelCalls:      10,
		ToolCalls:       20,
		EstimatedTokens: 100000,
		ElapsedMillis:   60000, // 1 minute
	}

	now := time.Now()
	tracker, err := newRecoveryMissionBudgetTracker(limit, historical, now)
	if err != nil {
		t.Fatalf("unexpected error creating tracker: %v", err)
	}

	if tracker.usage.ModelCalls != 10 || tracker.usage.ToolCalls != 20 || tracker.usage.EstimatedTokens != 100000 {
		t.Fatalf("tracker usage not seeded properly: %#v", tracker.usage)
	}

	// started should be rebased backwards by 60 seconds
	wantStarted := now.Add(-60 * time.Second)
	diff := tracker.started.Sub(wantStarted)
	if diff < -time.Millisecond || diff > time.Millisecond {
		t.Fatalf("tracker started=%v, want ~%v", tracker.started, wantStarted)
	}

	// Test invalid usage
	if _, err := newRecoveryMissionBudgetTracker(limit, AgentUsage{ModelCalls: -1}, now); err == nil {
		t.Fatal("expected error for negative usage")
	}

	// Test overflow duration
	if _, err := newRecoveryMissionBudgetTracker(limit, AgentUsage{ElapsedMillis: math.MaxInt64}, now); err == nil {
		t.Fatal("expected error for overflow elapsed millis")
	}
}

func TestMissionRecoveryAdmissionReserveValidationAndErrors(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	state, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	materialized, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child")
	if err != nil {
		t.Fatal(err)
	}

	// Empty executionRunID
	if err := reserveMissionRecoveryContinuation(materialized, "", at); !errors.Is(err, errMissionRecoveryContinuationUnavailable) {
		t.Fatalf("empty executionRunID error=%v", err)
	}

	// Zero admittedAt
	if err := reserveMissionRecoveryContinuation(materialized, "new-run", time.Time{}); !errors.Is(err, errMissionRecoveryContinuationUnavailable) {
		t.Fatalf("zero admittedAt error=%v", err)
	}

	// RequiresNewAttempt is false
	matNoNewAttempt := materialized
	matNoNewAttempt.RequiresNewAttempt = false
	if err := reserveMissionRecoveryContinuation(matNoNewAttempt, "new-run", at); !errors.Is(err, errMissionRecoveryContinuationUnavailable) {
		t.Fatalf("RequiresNewAttempt=false error=%v", err)
	}

	// Invalid JournalSHA256
	matBadDigest := materialized
	matBadDigest.JournalSHA256 = "invalid-sha"
	if err := reserveMissionRecoveryContinuation(matBadDigest, "new-run", at); !errors.Is(err, errMissionRecoveryContinuationUnavailable) {
		t.Fatalf("invalid JournalSHA256 error=%v", err)
	}

	// Tampered journal / fingerprint mismatch
	tamperedState := *state
	tamperedState.Mission.Objective = "tampered objective"
	if err := writeRunJournal(tamperedState); err != nil {
		t.Fatal(err)
	}
	if err := reserveMissionRecoveryContinuation(materialized, "new-run", at); !errors.Is(err, errMissionRecoveryAdmissionStale) {
		t.Fatalf("fingerprint mismatch error=%v want errMissionRecoveryAdmissionStale", err)
	}
}

func TestMissionRecoveryAdmissionReserveAttemptLimits(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := prepareMissionRecoveryAdmissionTestState(t, at)
	state.Mission.Tasks[1].Lifecycle = &MissionTaskLifecycle{AttemptCount: 3, RetryCount: 2}
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	state, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	// Materialize will fail because attempt limit is reached
	if _, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child"); err == nil {
		t.Fatal("expected materialization to reject task with 3 attempts")
	}
}

func TestMissionRecoveryAdmissionReserveAndFinishSuccess(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	state, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	materialized, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child")
	if err != nil {
		t.Fatal(err)
	}

	admittedAt := at.Add(10 * time.Second)
	executionRunID := "exec-run-123"
	if err := reserveMissionRecoveryContinuation(materialized, executionRunID, admittedAt); err != nil {
		t.Fatalf("reservation failed: %v", err)
	}

	// Verify journal after reservation
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != executionRunID {
		t.Fatalf("loaded RunID=%q want %q", loaded.RunID, executionRunID)
	}
	if loaded.Mission.State != missionRecoveryRunning {
		t.Fatalf("mission state=%q want running", loaded.Mission.State)
	}
	childTask := loaded.Mission.Tasks[1]
	if childTask.Lifecycle == nil || !childTask.Lifecycle.AttemptReserved {
		t.Fatalf("candidate Lifecycle.AttemptReserved=%v want true", childTask.Lifecycle.AttemptReserved)
	}
	if childTask.Lifecycle.AttemptCount != 0 {
		t.Fatalf("candidate Lifecycle.AttemptCount=%d want 0", childTask.Lifecycle.AttemptCount)
	}
	if childTask.State != AgentTaskReady {
		t.Fatalf("candidate State=%q want ready", childTask.State)
	}
	if len(loaded.Events) == 0 || loaded.Events[len(loaded.Events)-1].Type != "mission_continuation_reserved" {
		t.Fatalf("expected reservation event in journal, got: %#v", loaded.Events)
	}

	// Cannot reserve again while AttemptReserved is true
	if err := reserveMissionRecoveryContinuation(materialized, "another-run", admittedAt); err == nil {
		t.Fatal("expected error reserving already-reserved attempt")
	}

	// Now test finishMissionRecoveryContinuation
	graph := materialized.Graph
	for i := range graph.Tasks {
		if graph.Tasks[i].ID == "child" {
			graph.Tasks[i].State = AgentTaskSucceeded
			graph.Tasks[i].Result = AgentResult{Status: AgentResultCompleted}
		}
	}
	run := AgentScheduledRun{
		MissionID: graph.MissionID,
		UsageByTask: map[string]AgentUsage{
			"foundation": {ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 300, ElapsedMillis: 400},
			"child":      {ModelCalls: 2, ToolCalls: 1, EstimatedTokens: 500, ElapsedMillis: 600},
		},
		Snapshot: AgentSchedulerSnapshot{
			Queued: 0,
		},
	}
	finishedAt := admittedAt.Add(2 * time.Second)
	if err := finishMissionRecoveryContinuation(executionRunID, &graph, run, finishedAt, nil); err != nil {
		t.Fatalf("finishMissionRecoveryContinuation failed: %v", err)
	}

	// Verify journal after finish
	finishedLoaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	finishedChild := finishedLoaded.Mission.Tasks[1]
	if finishedChild.State != AgentTaskSucceeded {
		t.Fatalf("finished child state=%q want succeeded", finishedChild.State)
	}
	if finishedChild.Lifecycle.AttemptReserved {
		t.Fatal("AttemptReserved flag was not cleared upon finish")
	}
	if finishedChild.Lifecycle.LastFinishedAt.IsZero() {
		t.Fatal("LastFinishedAt was not set")
	}
	if finishedChild.Usage.ModelCalls != 2 {
		t.Fatalf("child usage ModelCalls=%d want 2", finishedChild.Usage.ModelCalls)
	}
	if finishedLoaded.Mission.Accounting == nil || finishedLoaded.Mission.Accounting.Usage.ModelCalls != 3 {
		t.Fatalf("mission accounting total ModelCalls=%v want 3", finishedLoaded.Mission.Accounting)
	}
	lastEvent := finishedLoaded.Events[len(finishedLoaded.Events)-1]
	if lastEvent.Type != "mission_end" {
		t.Fatalf("last event=%q want mission_end", lastEvent.Type)
	}
}

func TestMissionRecoveryAdmissionAppStateEndToEnd(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{
		Config: Config{
			LastModel: "test-model",
		},
	}

	// Active run rejection
	appState.Running = true
	if _, err := appState.RunMissionRecoveryContinuation(context.Background(), state.RunID, "child"); !errors.Is(err, errMissionRecoveryControlActiveRun) {
		t.Fatalf("active run error=%v want errMissionRecoveryControlActiveRun", err)
	}
	appState.Running = false

	// Mock scheduledReadOnlyAgentExecutor
	executedTaskID := ""
	mockExecutor := func(ctx context.Context, project string, cfg Config, task AgentTask) (AgentResult, error) {
		executedTaskID = task.ID
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: "Child task successfully executed in test",
			Usage: AgentUsage{
				ModelCalls:      2,
				ToolCalls:       1,
				EstimatedTokens: 400,
				ElapsedMillis:   500,
			},
		}, nil
	}

	exec, err := appState.runMissionRecoveryContinuationWithExecutor(context.Background(), state.RunID, "child", mockExecutor)
	if err != nil {
		t.Fatalf("runMissionRecoveryContinuationWithExecutor failed: %v", err)
	}

	if executedTaskID != "child" {
		t.Fatalf("executed task ID=%q want child", executedTaskID)
	}
	if exec.TaskID != "child" || exec.MissionID != "admission-mission" {
		t.Fatalf("unexpected execution result: %#v", exec)
	}

	// Verify AppState state returned to idle
	if appState.Running {
		t.Fatal("AppState.Running is still true after execution")
	}
	if appState.RunPhase != "idle" {
		t.Fatalf("AppState.RunPhase=%q want idle", appState.RunPhase)
	}

	// Verify final journal
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	childTask := loaded.Mission.Tasks[1]
	if childTask.State != AgentTaskSucceeded {
		t.Fatalf("child task state=%q want succeeded", childTask.State)
	}
	if childTask.Lifecycle == nil || childTask.Lifecycle.AttemptReserved {
		t.Fatal("child lifecycle AttemptReserved is not false")
	}
	if childTask.Lifecycle.AttemptCount != 1 {
		t.Fatalf("child AttemptCount=%d want 1", childTask.Lifecycle.AttemptCount)
	}
}

func TestMissionRecoveryAdmissionAppStateHandlesCancellation(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{
		Config: Config{
			LastModel: "test-model",
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	mockExecutor := func(ctx context.Context, project string, cfg Config, task AgentTask) (AgentResult, error) {
		cancel() // cancel during execution
		return AgentResult{Status: AgentResultCompleted}, ctx.Err()
	}

	_, err := appState.runMissionRecoveryContinuationWithExecutor(ctx, state.RunID, "child", mockExecutor)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled error, got: %v", err)
	}

	// AppState returned to idle
	if appState.Running {
		t.Fatal("AppState.Running is true after cancellation")
	}

	// Journal was updated and candidate marked cancelled
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	childTask := loaded.Mission.Tasks[1]
	if childTask.State != AgentTaskCancelled {
		t.Fatalf("child task state=%q want cancelled", childTask.State)
	}
}

func TestMissionRecoveryAdmissionRepeatedRestarts(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	// Add a grandchild task dependent on child
	state.Mission.Tasks = append(state.Mission.Tasks, MissionRecoveryTaskState{
		ID:           "grandchild",
		Role:         AgentRoleExplorer,
		Objective:    "inspect grandchild",
		Dependencies: []string{"child"},
		State:        AgentTaskPending,
		Model:        "test-model",
		Budget:       AgentBudget{ModelCalls: 8, ToolCalls: 12, EstimatedTokenBudget: 100000, TimeSeconds: 180},
	})
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{
		Config: Config{
			LastModel: "test-model",
		},
	}

	// 1st Continuation: execute "child"
	mockExecutor1 := func(ctx context.Context, project string, cfg Config, task AgentTask) (AgentResult, error) {
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: "Child finished",
			Usage: AgentUsage{
				ModelCalls:      1,
				ToolCalls:       1,
				EstimatedTokens: 200,
				ElapsedMillis:   300,
			},
		}, nil
	}

	exec1, err := appState.runMissionRecoveryContinuationWithExecutor(context.Background(), state.RunID, "child", mockExecutor1)
	if err != nil {
		t.Fatalf("1st continuation failed: %v", err)
	}
	if exec1.TaskID != "child" {
		t.Fatalf("exec1 TaskID=%q want child", exec1.TaskID)
	}

	// 2nd Continuation: execute "grandchild"
	mockExecutor2 := func(ctx context.Context, project string, cfg Config, task AgentTask) (AgentResult, error) {
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: "Grandchild finished",
			Usage: AgentUsage{
				ModelCalls:      2,
				ToolCalls:       2,
				EstimatedTokens: 400,
				ElapsedMillis:   500,
			},
		}, nil
	}

	exec2, err := appState.runMissionRecoveryContinuationWithExecutor(context.Background(), exec1.RunID, "grandchild", mockExecutor2)
	if err != nil {
		t.Fatalf("2nd continuation failed: %v", err)
	}
	if exec2.TaskID != "grandchild" {
		t.Fatalf("exec2 TaskID=%q want grandchild", exec2.TaskID)
	}

	// Verify all tasks in journal are succeeded
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Mission.Tasks) != 3 {
		t.Fatalf("task count=%d want 3", len(loaded.Mission.Tasks))
	}
	for _, task := range loaded.Mission.Tasks {
		if task.State != AgentTaskSucceeded {
			t.Fatalf("task %q state=%q want succeeded", task.ID, task.State)
		}
		if task.Lifecycle != nil && task.Lifecycle.AttemptReserved {
			t.Fatalf("task %q still has AttemptReserved=true", task.ID)
		}
	}
	// Verify total accumulated usage
	if loaded.Mission.Accounting == nil || loaded.Mission.Accounting.Usage.ModelCalls != 4 {
		t.Fatalf("total ModelCalls=%v want 4 (1 foundation + 1 child + 2 grandchild)", loaded.Mission.Accounting)
	}
}

func TestMissionRecoveryAdmissionDriftTOCTOU(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	state, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, snapshot := buildMissionRecoveryContinuationTestSnapshot(t, state, current, at)
	materialized, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, "child")
	if err != nil {
		t.Fatal(err)
	}

	// Tamper journal by changing a task state directly on disk
	tamperedState := *state
	tamperedState.Mission.Tasks[0].State = AgentTaskFailed
	if err := writeRunJournal(tamperedState); err != nil {
		t.Fatal(err)
	}

	// Admission must fail closed with stale journal error
	err = reserveMissionRecoveryContinuation(materialized, "new-exec-run", at.Add(5*time.Second))
	if !errors.Is(err, errMissionRecoveryAdmissionStale) {
		t.Fatalf("expected errMissionRecoveryAdmissionStale on TOCTOU drift, got: %v", err)
	}
}
