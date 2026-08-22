// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"testing"
	"time"
)

func TestMissionRecoveryControlDoesNotNormalizeUnknownVerificationState(t *testing.T) {
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
	evidence := state.Mission.Tasks[0].CompletionEvidence
	evidence.VerificationState = MissionVerificationState("corrupt")
	evidence.VerificationAttemptCount = 1
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := buildMissionRecoveryControlSnapshot(state, fingerprint, current, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Verifications) != 1 || !snapshot.Verifications[0].Passed {
		t.Fatalf("expected fresh observation of direct postconditions: %#v", snapshot.Verifications)
	}
	if got := transitionByTaskID(t, snapshot.Plan, "foundation").Action; got != missionRecoveryTransitionVerifyPostconditions {
		t.Fatalf("corrupt verification state became reusable: action=%q", got)
	}
	if state.Mission.Tasks[0].CompletionEvidence.VerificationState != MissionVerificationState("corrupt") {
		t.Fatal("read-only control mutated corrupt durable verification state")
	}
}

func TestMissionRecoveryControlDoesNotNormalizeInvalidVerifiedAttemptCount(t *testing.T) {
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
	evidence := state.Mission.Tasks[0].CompletionEvidence
	evidence.VerificationState = missionVerificationVerified
	evidence.VerificationAttemptCount = 0
	evidence.LastVerificationCheckCount = len(missionRecoveryVerifiedPostconditionChecks())
	evidence.LastVerificationEvidenceSHA256 = missionSHA256String("stale-but-well-formed")
	evidence.VerificationUpdatedAt = at
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := buildMissionRecoveryControlSnapshot(state, fingerprint, current, at)
	if err != nil {
		t.Fatal(err)
	}
	if got := transitionByTaskID(t, snapshot.Plan, "foundation").Action; got != missionRecoveryTransitionVerifyPostconditions {
		t.Fatalf("verified record with missing attempt evidence became reusable: action=%q", got)
	}
}

func TestMissionRecoveryControlFailsAfterBoundedConcurrentChanges(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, current := missionRecoveryControlTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}
	calls := 0
	observe := func(_ string, observedAt time.Time) MissionProjectBaseline {
		calls++
		mutated, _, err := loadMissionRecoveryControlState(state.RunID)
		if err != nil {
			t.Fatal(err)
		}
		mutated.Mission.Reason = missionSHA256String(time.Now().String())
		mutated.Mission.UpdatedAt = observedAt.Add(time.Duration(calls) * time.Nanosecond)
		if err := writeRunJournal(*mutated); err != nil {
			t.Fatal(err)
		}
		copyCurrent := current
		copyCurrent.CapturedAt = observedAt
		return copyCurrent
	}

	_, err := buildStableMissionRecoveryControlSnapshotWithObserver(state.RunID, observe)
	if !errors.Is(err, errMissionRecoveryControlChanged) {
		t.Fatalf("error=%v want bounded concurrent-change failure", err)
	}
	if calls != missionRecoveryControlMaxSnapshotAttempts {
		t.Fatalf("observations=%d want=%d", calls, missionRecoveryControlMaxSnapshotAttempts)
	}
}
