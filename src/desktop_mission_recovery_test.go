// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func desktopRecoveryTestControlSnapshot() MissionRecoveryControlSnapshot {
	return MissionRecoveryControlSnapshot{
		RunID:               "interrupted-run",
		MissionID:           "mission-1",
		ObservedAt:          time.Unix(123, 0),
		JournalSHA256:       strings.Repeat("a", 64),
		SnapshotSHA256:      strings.Repeat("b", 64),
		ReconciliationState: missionReconcileMatched,
		Plan: MissionRecoveryTransitionPlan{
			MissionID: "mission-1",
			Valid:     true,
			Tasks: []MissionRecoveryTaskTransition{
				{TaskID: "foundation", DurableState: AgentTaskSucceeded, Action: missionRecoveryTransitionReuseVerified},
				{TaskID: "child", DurableState: AgentTaskFailed, Action: missionRecoveryTransitionRetryCandidate, RequiresNewAttempt: true},
			},
		},
		ReadOnly: true,
	}
}

func TestDesktopMissionRecoverySnapshotIsBoundedAndCandidateOnlyExecutable(t *testing.T) {
	out := desktopMissionRecoverySnapshotFromControl(desktopRecoveryTestControlSnapshot())
	if !out.Available || !out.Runnable || out.RunID != "interrupted-run" || out.MissionID != "mission-1" {
		t.Fatalf("unexpected snapshot: %#v", out)
	}
	if len(out.Tasks) != 2 || out.Tasks[0].CanContinue || !out.Tasks[1].CanContinue {
		t.Fatalf("unexpected continuation flags: %#v", out.Tasks)
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, forbidden := range []string{"project", "objective", "capabilities", "accounting", "result", "usage"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Fatalf("bounded desktop recovery snapshot leaked %q: %s", forbidden, body)
		}
	}
}

func TestDesktopMissionRecoveryInspectionTransport(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/mission-recovery", nil)
		handleDesktopMissionRecoverySnapshot(rr, req, func(string) (DesktopMissionRecoverySnapshot, error) {
			return DesktopMissionRecoverySnapshot{}, errDesktopMissionRecoveryNotFound
		})
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusNoContent)
		}
	})

	t.Run("bounded-json", func(t *testing.T) {
		want := desktopMissionRecoverySnapshotFromControl(desktopRecoveryTestControlSnapshot())
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/mission-recovery?run_id=interrupted-run", nil)
		handleDesktopMissionRecoverySnapshot(rr, req, func(runID string) (DesktopMissionRecoverySnapshot, error) {
			if runID != "interrupted-run" {
				t.Fatalf("run id=%q", runID)
			}
			return want, nil
		})
		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
		var got DesktopMissionRecoverySnapshot
		if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.RunID != want.RunID || got.JournalSHA256 != want.JournalSHA256 || !got.Tasks[1].CanContinue {
			t.Fatalf("unexpected body: %#v", got)
		}
	})
}

func TestDesktopMissionRecoveryContinueTransportRejectsMalformedAndStale(t *testing.T) {
	valid := DesktopMissionRecoveryContinueRequest{
		RunID:          "interrupted-run",
		MissionID:      "mission-1",
		TaskID:         "child",
		Action:         missionRecoveryTransitionRetryCandidate,
		JournalSHA256:  strings.Repeat("a", 64),
		SnapshotSHA256: strings.Repeat("b", 64),
	}

	t.Run("unknown-field", func(t *testing.T) {
		data, _ := json.Marshal(map[string]any{
			"run_id": valid.RunID, "mission_id": valid.MissionID, "task_id": valid.TaskID, "action": valid.Action,
			"journal_sha256": valid.JournalSHA256, "snapshot_sha256": valid.SnapshotSHA256, "authority": true,
		})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/mission-recovery/continue", bytes.NewReader(data))
		handleDesktopMissionRecoveryContinue(rr, req, func(context.Context, string, string, MissionRecoveryContinuationPreconditions) (MissionRecoveryContinuationAdmission, error) {
			t.Fatal("starter ran for malformed request")
			return MissionRecoveryContinuationAdmission{}, nil
		})
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("stale", func(t *testing.T) {
		data, _ := json.Marshal(valid)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/mission-recovery/continue", bytes.NewReader(data))
		handleDesktopMissionRecoveryContinue(rr, req, func(_ context.Context, runID, taskID string, expected MissionRecoveryContinuationPreconditions) (MissionRecoveryContinuationAdmission, error) {
			if runID != valid.RunID || taskID != valid.TaskID || expected.JournalSHA256 != valid.JournalSHA256 {
				t.Fatalf("unexpected preconditions: run=%q task=%q expected=%#v", runID, taskID, expected)
			}
			return MissionRecoveryContinuationAdmission{}, errMissionRecoveryAdmissionPrecondition
		})
		if rr.Code != http.StatusConflict {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	})
}

func TestDesktopMissionRecoveryContinueTransportReturnsAcceptedAdmission(t *testing.T) {
	reqBody := DesktopMissionRecoveryContinueRequest{
		RunID:          "interrupted-run",
		MissionID:      "mission-1",
		TaskID:         "child",
		Action:         missionRecoveryTransitionRetryCandidate,
		JournalSHA256:  strings.Repeat("a", 64),
		SnapshotSHA256: strings.Repeat("b", 64),
	}
	data, _ := json.Marshal(reqBody)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/mission-recovery/continue", bytes.NewReader(data))
	handleDesktopMissionRecoveryContinue(rr, req, func(_ context.Context, runID, taskID string, expected MissionRecoveryContinuationPreconditions) (MissionRecoveryContinuationAdmission, error) {
		if runID != reqBody.RunID || taskID != reqBody.TaskID || expected.Action != reqBody.Action || expected.SnapshotSHA256 != reqBody.SnapshotSHA256 {
			t.Fatalf("unexpected admission input: run=%q task=%q expected=%#v", runID, taskID, expected)
		}
		return MissionRecoveryContinuationAdmission{RunID: "new-run", ParentRunID: runID, MissionID: expected.MissionID, TaskID: taskID, Action: expected.Action, AcceptedAt: time.Now()}, nil
	})
	if rr.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var admitted MissionRecoveryContinuationAdmission
	if err := json.NewDecoder(rr.Body).Decode(&admitted); err != nil {
		t.Fatal(err)
	}
	if admitted.RunID != "new-run" || admitted.ParentRunID != reqBody.RunID {
		t.Fatalf("unexpected admission: %#v", admitted)
	}
}

func TestMissionRecoveryAsyncAdmissionRevalidatesAndOwnsCancellation(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := admissionTestRetryState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	observe := func(_ string, observedAt time.Time) MissionProjectBaseline {
		copyCurrent := current
		copyCurrent.CapturedAt = observedAt
		return copyCurrent
	}
	control, err := buildStableMissionRecoveryControlSnapshotWithObserver(state.RunID, observe)
	if err != nil {
		t.Fatal(err)
	}

	app := &AppState{Config: Config{}}
	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	executor := func(ctx context.Context, _ string, _ Config, _ AgentTask) (AgentResult, error) {
		started <- struct{}{}
		<-ctx.Done()
		finished <- struct{}{}
		return AgentResult{Status: AgentResultCancelled}, ctx.Err()
	}
	expected := MissionRecoveryContinuationPreconditions{
		MissionID:      control.MissionID,
		Action:         missionRecoveryTransitionRetryCandidate,
		JournalSHA256:  control.JournalSHA256,
		SnapshotSHA256: control.SnapshotSHA256,
	}
	admitted, err := app.startMissionRecoveryContinuationWithExecutorAndObserver(context.Background(), state.RunID, "child", expected, executor, observe)
	if err != nil {
		t.Fatal(err)
	}
	app.mu.RLock()
	running := app.Running
	runID := app.RunID
	phase := app.RunPhase
	app.mu.RUnlock()
	if !running || runID != admitted.RunID || phase != "mission-read-only-continuation" || admitted.ParentRunID != state.RunID {
		t.Fatalf("admission not visible before return: running=%v run=%q phase=%q admitted=%#v", running, runID, phase, admitted)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("accepted recovery executor did not start")
	}
	if !app.StopAgent() {
		t.Fatal("StopAgent did not own accepted recovery cancellation")
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("recovery executor was not cancelled")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		app.mu.RLock()
		stillRunning := app.Running
		app.mu.RUnlock()
		if !stillRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("AppState did not leave running state after cancellation")
		}
		time.Sleep(10 * time.Millisecond)
	}
	persisted, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if persisted == nil || !persisted.Terminal || persisted.Mission == nil || persisted.Mission.State != string(AgentMissionCancelled) {
		t.Fatalf("accepted recovery cancellation not terminalized: %#v", persisted)
	}
}

func TestMissionRecoveryAsyncAdmissionRejectsStaleBeforeExecutor(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := admissionTestRetryState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	observe := func(_ string, observedAt time.Time) MissionProjectBaseline {
		copyCurrent := current
		copyCurrent.CapturedAt = observedAt
		return copyCurrent
	}
	control, err := buildStableMissionRecoveryControlSnapshotWithObserver(state.RunID, observe)
	if err != nil {
		t.Fatal(err)
	}
	app := &AppState{}
	executed := false
	_, err = app.startMissionRecoveryContinuationWithExecutorAndObserver(context.Background(), state.RunID, "child", MissionRecoveryContinuationPreconditions{
		MissionID:      control.MissionID,
		Action:         missionRecoveryTransitionRetryCandidate,
		JournalSHA256:  strings.Repeat("f", 64),
		SnapshotSHA256: control.SnapshotSHA256,
	}, func(context.Context, string, Config, AgentTask) (AgentResult, error) {
		executed = true
		return AgentResult{}, nil
	}, observe)
	if !errors.Is(err, errMissionRecoveryAdmissionPrecondition) {
		t.Fatalf("error=%v want stale precondition", err)
	}
	if executed {
		t.Fatal("executor ran after stale precondition")
	}
	app.mu.RLock()
	running := app.Running
	app.mu.RUnlock()
	if running {
		t.Fatal("stale precondition reserved AppState running state")
	}
	persisted, loadErr := loadRunJournal()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if persisted.RunID != state.RunID {
		t.Fatalf("stale precondition rotated durable run to %q", persisted.RunID)
	}
}
