// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

func withMissionRecoveryControlJournal(t *testing.T) {
	t.Helper()
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
}

func missionRecoveryControlTestState(t *testing.T, at time.Time) (*RunRecoveryState, MissionProjectBaseline) {
	t.Helper()
	project := t.TempDir()
	baseline := missionObservedBaseline(project, project, "control-head", nil, at.Add(-time.Minute))
	foundation := MissionRecoveryTaskState{
		ID:        "foundation",
		Role:      AgentRoleExplorer,
		Objective: "inspect foundation",
		State:     AgentTaskSucceeded,
	}
	foundation.CompletionEvidence = missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, at.Add(-time.Second))
	child := MissionRecoveryTaskState{
		ID:           "child",
		Role:         AgentRoleExplorer,
		Objective:    "inspect child",
		Dependencies: []string{"foundation"},
		State:        AgentTaskPending,
	}
	state := &RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         "control-run",
		Project:       project,
		Phase:         "mission-read-only",
		StartedAt:     at.Add(-time.Minute),
		UpdatedAt:     at.Add(-time.Second),
		Mission: &MissionRecoveryState{
			Kind:      missionRecoveryKindReadOnly,
			MissionID: "control-mission",
			Project:   project,
			State:     missionRecoveryRunning,
			Baseline:  &baseline,
			Tasks:     []MissionRecoveryTaskState{foundation, child},
			StartedAt: at.Add(-time.Minute),
			UpdatedAt: at.Add(-time.Second),
		},
	}
	current := missionObservedBaseline(project, project, "control-head", nil, at)
	return state, current
}

func transitionByTaskID(t *testing.T, plan MissionRecoveryTransitionPlan, taskID string) MissionRecoveryTaskTransition {
	t.Helper()
	for _, transition := range plan.Tasks {
		if transition.TaskID == taskID {
			return transition
		}
	}
	t.Fatalf("transition for task %q not found in %#v", taskID, plan.Tasks)
	return MissionRecoveryTaskTransition{}
}

func TestMissionRecoveryControlSnapshotVerifiesTransientlyWithoutMutation(t *testing.T) {
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		t.Fatal(err)
	}
	original := cloneMissionRecoveryState(state.Mission)

	snapshot, err := buildMissionRecoveryControlSnapshot(state, fingerprint, current, at)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.ReadOnly || snapshot.ExecutionAuthorized || snapshot.SchedulerLeaseGranted || snapshot.PersistentStateModified {
		t.Fatalf("control snapshot acquired authority: %#v", snapshot)
	}
	if !validMissionVerificationDigest(snapshot.SnapshotSHA256) {
		t.Fatalf("invalid snapshot digest: %q", snapshot.SnapshotSHA256)
	}
	if snapshot.ReconciliationState != missionReconcileMatched {
		t.Fatalf("reconciliation=%q want=%q", snapshot.ReconciliationState, missionReconcileMatched)
	}
	if len(snapshot.Verifications) != 1 || snapshot.Verifications[0].TaskID != "foundation" || !snapshot.Verifications[0].Passed {
		t.Fatalf("unexpected verification summary: %#v", snapshot.Verifications)
	}
	if got := transitionByTaskID(t, snapshot.Plan, "foundation").Action; got != missionRecoveryTransitionReuseVerified {
		t.Fatalf("foundation action=%q want=%q", got, missionRecoveryTransitionReuseVerified)
	}
	child := transitionByTaskID(t, snapshot.Plan, "child")
	if child.Action != missionRecoveryTransitionResumeCandidate || !child.RequiresNewAttempt {
		t.Fatalf("verified dependency did not unlock child: %#v", child)
	}
	if !reflect.DeepEqual(state.Mission, original) {
		t.Fatalf("read-only snapshot mutated durable mission input\n got: %#v\nwant: %#v", state.Mission, original)
	}
}

func TestMissionRecoveryControlSnapshotRechecksMalformedVerifiedEvidence(t *testing.T) {
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
	evidence := state.Mission.Tasks[0].CompletionEvidence
	evidence.VerificationState = missionVerificationVerified
	evidence.VerificationAttemptCount = 1
	evidence.LastVerificationCheckCount = len(missionRecoveryVerifiedPostconditionChecks())
	evidence.LastVerificationEvidenceSHA256 = missionSHA256String("wrong-verification-evidence")
	evidence.VerificationUpdatedAt = at
	originalDigest := evidence.LastVerificationEvidenceSHA256
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := buildMissionRecoveryControlSnapshot(state, fingerprint, current, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Verifications) != 1 || !snapshot.Verifications[0].Passed {
		t.Fatalf("malformed verified evidence was not freshly checked: %#v", snapshot.Verifications)
	}
	if got := transitionByTaskID(t, snapshot.Plan, "foundation").Action; got != missionRecoveryTransitionReuseVerified {
		t.Fatalf("fresh transient verification did not produce reusable snapshot: %#v", snapshot.Plan.Tasks)
	}
	if state.Mission.Tasks[0].CompletionEvidence.LastVerificationEvidenceSHA256 != originalDigest {
		t.Fatal("control snapshot repaired durable verification evidence instead of remaining read-only")
	}
}

func TestMissionRecoveryControlSnapshotCurrentDriftBlocksContinuation(t *testing.T) {
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
	current.GitStatusSHA256 = missionSHA256Bytes([]byte(" M changed.go\x00"))
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := buildMissionRecoveryControlSnapshot(state, fingerprint, current, at)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ReconciliationState != missionReconcileGitChanged {
		t.Fatalf("reconciliation=%q want=%q", snapshot.ReconciliationState, missionReconcileGitChanged)
	}
	if len(snapshot.Verifications) != 0 {
		t.Fatalf("drifted recovery attempted postcondition verification: %#v", snapshot.Verifications)
	}
	for _, transition := range snapshot.Plan.Tasks {
		if transition.Action == missionRecoveryTransitionResumeCandidate || transition.Action == missionRecoveryTransitionRetryCandidate || transition.Action == missionRecoveryTransitionReuseVerified {
			t.Fatalf("drift produced continuation candidate: %#v", transition)
		}
	}
}

func TestMissionRecoveryControlStableSnapshotDoesNotWriteJournal(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
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

	snapshot, err := buildStableMissionRecoveryControlSnapshotWithObserver(state.RunID, observe)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(runJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only mission recovery control changed active-run.json")
	}
	if calls != 1 {
		t.Fatalf("stable snapshot observations=%d want=1", calls)
	}
	if snapshot.ExecutionAuthorized || snapshot.PersistentStateModified {
		t.Fatalf("stable snapshot gained authority: %#v", snapshot)
	}
}

func TestMissionRecoveryControlRetriesWhenJournalChangesDuringObservation(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	calls := 0
	observe := func(_ string, observedAt time.Time) MissionProjectBaseline {
		calls++
		if calls == 1 {
			mutated, _, err := loadMissionRecoveryControlState(state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			mutated.Mission.Reason = "concurrent-observation-change"
			mutated.Mission.UpdatedAt = observedAt.Add(time.Nanosecond)
			if err := writeRunJournal(*mutated); err != nil {
				t.Fatal(err)
			}
		}
		copyCurrent := current
		copyCurrent.CapturedAt = observedAt
		return copyCurrent
	}

	snapshot, err := buildStableMissionRecoveryControlSnapshotWithObserver(state.RunID, observe)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("control observations=%d want=2 after one concurrent change", calls)
	}
	if snapshot.Plan.MissionID != state.Mission.MissionID || !snapshot.ReadOnly {
		t.Fatalf("unexpected stable snapshot after retry: %#v", snapshot)
	}
}

func TestMissionRecoveryControlRejectsWrongOrTerminalRun(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := missionRecoveryControlTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadMissionRecoveryControlState("different-run"); !errors.Is(err, errMissionRecoveryControlUnavailable) {
		t.Fatalf("wrong run error=%v want unavailable", err)
	}

	state.Terminal = true
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadMissionRecoveryControlState(state.RunID); !errors.Is(err, errMissionRecoveryControlUnavailable) {
		t.Fatalf("terminal run error=%v want unavailable", err)
	}
}
