// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"testing"
)

func TestMissionRecoveryControlBoundaryRejectsAlreadyRunningAgent(t *testing.T) {
	state := &AppState{Running: true}
	called := false
	build := func(string) (MissionRecoveryControlSnapshot, error) {
		called = true
		return MissionRecoveryControlSnapshot{ReadOnly: true}, nil
	}

	_, err := missionRecoveryControlSnapshotForAppState(state, "run", build)
	if !errors.Is(err, errMissionRecoveryControlActiveRun) {
		t.Fatalf("error=%v want active-run rejection", err)
	}
	if called {
		t.Fatal("recovery snapshot builder ran while an agent run was already active")
	}
}

func TestMissionRecoveryControlBoundaryRejectsRunThatStartsDuringObservation(t *testing.T) {
	state := &AppState{}
	build := func(string) (MissionRecoveryControlSnapshot, error) {
		state.mu.Lock()
		state.Running = true
		state.mu.Unlock()
		return MissionRecoveryControlSnapshot{ReadOnly: true}, nil
	}

	_, err := missionRecoveryControlSnapshotForAppState(state, "run", build)
	if !errors.Is(err, errMissionRecoveryControlActiveRun) {
		t.Fatalf("error=%v want active-run rejection", err)
	}
}

func TestMissionRecoveryControlBoundaryReturnsOnlyBuilderSnapshot(t *testing.T) {
	state := &AppState{}
	want := MissionRecoveryControlSnapshot{
		RunID:                 "run",
		MissionID:             "mission",
		SnapshotSHA256:        missionSHA256String("snapshot"),
		ReadOnly:              true,
		ExecutionAuthorized:   false,
		SchedulerLeaseGranted: false,
	}
	build := func(runID string) (MissionRecoveryControlSnapshot, error) {
		if runID != want.RunID {
			t.Fatalf("runID=%q want=%q", runID, want.RunID)
		}
		return want, nil
	}

	got, err := missionRecoveryControlSnapshotForAppState(state, want.RunID, build)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunID != want.RunID || got.MissionID != want.MissionID || got.SnapshotSHA256 != want.SnapshotSHA256 || !got.ReadOnly || got.ExecutionAuthorized || got.SchedulerLeaseGranted {
		t.Fatalf("unexpected recovery control snapshot: %#v", got)
	}
}

func TestMissionRecoveryControlBoundaryPropagatesSnapshotFailure(t *testing.T) {
	state := &AppState{}
	wantErr := errors.New("snapshot failed")
	_, err := missionRecoveryControlSnapshotForAppState(state, "run", func(string) (MissionRecoveryControlSnapshot, error) {
		return MissionRecoveryControlSnapshot{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error=%v want=%v", err, wantErr)
	}
}
