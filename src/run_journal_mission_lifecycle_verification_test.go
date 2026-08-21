// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"
	"time"
)

func TestMissionTaskLifecycleCountsOnlyNewRunningTransitions(t *testing.T) {
	mission := &MissionRecoveryState{Tasks: []MissionRecoveryTaskState{{ID: "inspect", State: AgentTaskReady}}}
	running := AgentSchedulerSnapshot{Tasks: []AgentTaskScheduleSnapshot{{TaskID: "inspect", State: AgentTaskRunning, Running: true}}}
	failed := AgentSchedulerSnapshot{Tasks: []AgentTaskScheduleSnapshot{{TaskID: "inspect", State: AgentTaskFailed}}}
	ready := AgentSchedulerSnapshot{Tasks: []AgentTaskScheduleSnapshot{{TaskID: "inspect", State: AgentTaskReady}}}
	succeeded := AgentSchedulerSnapshot{Tasks: []AgentTaskScheduleSnapshot{{TaskID: "inspect", State: AgentTaskSucceeded}}}

	applyMissionSchedulerSnapshot(mission, running)
	lifecycle := mission.Tasks[0].Lifecycle
	if lifecycle == nil || lifecycle.AttemptCount != 1 || lifecycle.RetryCount != 0 || lifecycle.LastStartedAt.IsZero() {
		t.Fatalf("unexpected first attempt lifecycle: %#v", lifecycle)
	}
	firstStartedAt := lifecycle.LastStartedAt

	applyMissionSchedulerSnapshot(mission, running)
	if lifecycle.AttemptCount != 1 || lifecycle.RetryCount != 0 || !lifecycle.LastStartedAt.Equal(firstStartedAt) {
		t.Fatalf("duplicate running snapshot counted as another attempt: %#v", lifecycle)
	}

	applyMissionSchedulerSnapshot(mission, failed)
	if lifecycle.LastFinishedAt.IsZero() {
		t.Fatalf("failed attempt did not record finish timestamp: %#v", lifecycle)
	}
	firstFinishedAt := lifecycle.LastFinishedAt

	applyMissionSchedulerSnapshot(mission, ready)
	applyMissionSchedulerSnapshot(mission, running)
	if lifecycle.AttemptCount != 2 || lifecycle.RetryCount != 1 {
		t.Fatalf("retry transition not counted correctly: %#v", lifecycle)
	}
	if !lifecycle.LastStartedAt.After(firstStartedAt) && !lifecycle.LastStartedAt.Equal(firstStartedAt) {
		t.Fatalf("retry start timestamp moved backwards: %#v", lifecycle)
	}

	applyMissionSchedulerSnapshot(mission, succeeded)
	if lifecycle.LastFinishedAt.Before(firstFinishedAt) {
		t.Fatalf("final finish timestamp moved backwards: %#v", lifecycle)
	}
	if mission.Tasks[0].State != AgentTaskSucceeded || mission.Tasks[0].Running {
		t.Fatalf("scheduler state not applied: %#v", mission.Tasks[0])
	}
}

func TestMissionTaskLifecycleDeepCopyIsIndependent(t *testing.T) {
	mission := &MissionRecoveryState{Tasks: []MissionRecoveryTaskState{{
		ID: "inspect",
		Lifecycle: &MissionTaskLifecycle{
			AttemptCount:   2,
			RetryCount:     1,
			StateUpdatedAt: time.Now(),
		},
	}}}
	clone := cloneMissionRecoveryState(mission)
	if clone == nil || clone.Tasks[0].Lifecycle == nil {
		t.Fatalf("missing cloned lifecycle: %#v", clone)
	}
	clone.Tasks[0].Lifecycle.AttemptCount = 99
	if mission.Tasks[0].Lifecycle.AttemptCount != 2 {
		t.Fatalf("clone mutated original lifecycle: %#v", mission.Tasks[0].Lifecycle)
	}
}

func TestMissionVerificationOutcomeRequiresBoundedDigestEvidence(t *testing.T) {
	completedAt := time.Now().Add(-time.Minute)
	evidence := missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted, Summary: "transient child output"}, completedAt)
	if evidence == nil {
		t.Fatal("completion evidence missing")
	}
	if evidence.VerificationState != missionVerificationUnverified || evidence.VerificationAttemptCount != 0 || !evidence.VerificationUpdatedAt.Equal(completedAt) {
		t.Fatalf("unexpected initial verification state: %#v", evidence)
	}

	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationVerified, "not-a-sha256", 1, time.Now()); err == nil {
		t.Fatal("invalid verification digest was accepted")
	}
	if evidence.VerificationState != missionVerificationUnverified || evidence.VerificationAttemptCount != 0 {
		t.Fatalf("rejected verification mutated state: %#v", evidence)
	}

	failedAt := time.Now()
	failedDigest := missionSHA256String("bounded failed verification evidence")
	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationFailed, failedDigest, 2, failedAt); err != nil {
		t.Fatal(err)
	}
	if evidence.VerificationState != missionVerificationFailed || evidence.VerificationAttemptCount != 1 || evidence.LastVerificationEvidenceSHA256 != failedDigest || evidence.LastVerificationCheckCount != 2 || !evidence.VerificationUpdatedAt.Equal(failedAt) {
		t.Fatalf("unexpected failed verification record: %#v", evidence)
	}

	verifiedAt := failedAt.Add(time.Second)
	verifiedDigest := missionSHA256String("bounded successful verification evidence")
	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationVerified, verifiedDigest, 3, verifiedAt); err != nil {
		t.Fatal(err)
	}
	if evidence.VerificationState != missionVerificationVerified || evidence.VerificationAttemptCount != 2 || evidence.LastVerificationEvidenceSHA256 != verifiedDigest || evidence.LastVerificationCheckCount != 3 || !evidence.VerificationUpdatedAt.Equal(verifiedAt) {
		t.Fatalf("unexpected verified record: %#v", evidence)
	}

	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationFailed, missionSHA256String("late stale verification"), 1, verifiedAt.Add(time.Second)); err == nil {
		t.Fatal("terminal verified evidence was allowed to regress")
	}
	if evidence.VerificationState != missionVerificationVerified || evidence.VerificationAttemptCount != 2 {
		t.Fatalf("terminal verification state changed after rejected regression: %#v", evidence)
	}
}

func TestMissionVerificationOutcomeRejectsUnsupportedStateAndCheckCount(t *testing.T) {
	evidence := missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, time.Now())
	digest := missionSHA256String("verification evidence")
	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationUnverified, digest, 1, time.Now()); err == nil {
		t.Fatal("unverified was accepted as a verification outcome")
	}
	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationFailed, digest, 0, time.Now()); err == nil {
		t.Fatal("zero verification checks were accepted")
	}
	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationFailed, digest, maxMissionVerificationChecks+1, time.Now()); err == nil {
		t.Fatal("excessive verification checks were accepted")
	}
}
