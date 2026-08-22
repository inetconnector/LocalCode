// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"testing"
	"time"
)

func recoveryTransitionTask(id string, state AgentTaskState, dependencies ...string) MissionRecoveryTaskState {
	return MissionRecoveryTaskState{
		ID:           id,
		Role:         AgentRoleExplorer,
		Objective:    "recover " + id,
		Dependencies: append([]string(nil), dependencies...),
		State:        state,
	}
}

func verifiedRecoveryTransitionEvidence(t *testing.T, at time.Time) *MissionTaskCompletionEvidence {
	t.Helper()
	evidence := missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, at)
	if evidence == nil {
		t.Fatal("completion evidence is nil")
	}
	verificationDigest := missionTaskResultDigest(AgentResult{Status: AgentResultCompleted})
	if err := recordMissionTaskVerificationOutcome(evidence, missionVerificationVerified, verificationDigest, 1, at.Add(time.Second)); err != nil {
		t.Fatalf("record verified completion evidence: %v", err)
	}
	return evidence
}

func matchedRecoveryTransitionMission(tasks ...MissionRecoveryTaskState) *MissionRecoveryState {
	mission := &MissionRecoveryState{
		MissionID: "mission-transition-plan",
		Tasks:     tasks,
		Reconciliation: &MissionRestartReconciliation{
			State:  missionReconcileMatched,
			Reason: "project_and_git_match_baseline",
		},
	}
	checks := missionRecoveryVerifiedPostconditionChecks()
	for index := range mission.Tasks {
		evidence := mission.Tasks[index].CompletionEvidence
		if evidence == nil || evidence.VerificationState != missionVerificationVerified {
			continue
		}
		evidence.LastVerificationCheckCount = len(checks)
		evidence.LastVerificationEvidenceSHA256 = missionTaskPostconditionEvidenceDigest(mission.MissionID, mission.Tasks[index], mission.Reconciliation, checks)
	}
	return mission
}

func TestMissionRecoveryTransitionPlanRequiresVerifiedDependencies(t *testing.T) {
	now := time.Now()
	foundation := recoveryTransitionTask("foundation", AgentTaskSucceeded)
	foundation.CompletionEvidence = verifiedRecoveryTransitionEvidence(t, now.Add(-time.Minute))
	child := recoveryTransitionTask("child", AgentTaskPending, "foundation")
	plan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(foundation, child), now)

	if !plan.Valid || plan.InvalidReason != "" {
		t.Fatalf("valid plan rejected: %#v", plan)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("unexpected task count: %#v", plan.Tasks)
	}
	if plan.Tasks[0].Action != missionRecoveryTransitionReuseVerified {
		t.Fatalf("verified foundation not reusable: %#v", plan.Tasks[0])
	}
	if plan.Tasks[1].Action != missionRecoveryTransitionResumeCandidate || !plan.Tasks[1].RequiresNewAttempt {
		t.Fatalf("verified dependency did not unlock pending child: %#v", plan.Tasks[1])
	}
	if plan.ReservedNewAttempts != 1 || plan.ObservedMissionAttempts != 0 {
		t.Fatalf("unexpected mission attempt accounting: %#v", plan)
	}

	unverified := recoveryTransitionTask("foundation", AgentTaskSucceeded)
	unverified.CompletionEvidence = missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, now.Add(-time.Minute))
	blockedChild := recoveryTransitionTask("child", AgentTaskPending, "foundation")
	blockedPlan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(unverified, blockedChild), now)
	if blockedPlan.Tasks[0].Action != missionRecoveryTransitionVerifyPostconditions {
		t.Fatalf("unverified success did not require verification: %#v", blockedPlan.Tasks[0])
	}
	if blockedPlan.Tasks[1].Action != missionRecoveryTransitionBlockedDependency || len(blockedPlan.Tasks[1].BlockedBy) != 1 || blockedPlan.Tasks[1].BlockedBy[0] != "foundation" {
		t.Fatalf("unverified dependency did not block child: %#v", blockedPlan.Tasks[1])
	}
	if blockedPlan.ReservedNewAttempts != 0 {
		t.Fatalf("blocked dependency reserved an attempt: %#v", blockedPlan)
	}
}

func TestMissionRecoveryTransitionPlanDoesNotReuseMalformedVerifiedEvidence(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		mutate func(*MissionTaskCompletionEvidence)
	}{
		{name: "missing verification attempt", mutate: func(e *MissionTaskCompletionEvidence) { e.VerificationAttemptCount = 0 }},
		{name: "missing verification digest", mutate: func(e *MissionTaskCompletionEvidence) { e.LastVerificationEvidenceSHA256 = "" }},
		{name: "mismatched canonical verification digest", mutate: func(e *MissionTaskCompletionEvidence) {
			e.LastVerificationEvidenceSHA256 = missionSHA256String("wrong-verification")
		}},
		{name: "missing verification checks", mutate: func(e *MissionTaskCompletionEvidence) { e.LastVerificationCheckCount = 0 }},
		{name: "too many verification checks", mutate: func(e *MissionTaskCompletionEvidence) {
			e.LastVerificationCheckCount = maxMissionVerificationChecks + 1
		}},
		{name: "invalid result digest", mutate: func(e *MissionTaskCompletionEvidence) { e.ResultSHA256 = "not-a-sha256" }},
		{name: "non-success result status", mutate: func(e *MissionTaskCompletionEvidence) { e.ResultStatus = AgentResultStatus("failed") }},
		{name: "verification predates completion", mutate: func(e *MissionTaskCompletionEvidence) { e.VerificationUpdatedAt = e.CompletedAt.Add(-time.Second) }},
		{name: "negative structural count", mutate: func(e *MissionTaskCompletionEvidence) { e.FindingCount = -1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			foundation := recoveryTransitionTask("foundation", AgentTaskSucceeded)
			foundation.CompletionEvidence = verifiedRecoveryTransitionEvidence(t, now.Add(-time.Minute))
			child := recoveryTransitionTask("child", AgentTaskPending, "foundation")
			mission := matchedRecoveryTransitionMission(foundation, child)
			test.mutate(mission.Tasks[0].CompletionEvidence)

			plan := planMissionRecoveryTransitions(mission, now)
			if !plan.Valid {
				t.Fatalf("malformed evidence should require re-verification without corrupting the task graph: %#v", plan)
			}
			if plan.Tasks[0].Action != missionRecoveryTransitionVerifyPostconditions || plan.Tasks[0].Reason != "verified_evidence_invalid_requires_recheck" {
				t.Fatalf("malformed verified evidence was trusted: %#v", plan.Tasks[0])
			}
			if plan.Tasks[1].Action != missionRecoveryTransitionBlockedDependency || len(plan.Tasks[1].BlockedBy) != 1 || plan.Tasks[1].BlockedBy[0] != "foundation" {
				t.Fatalf("child was unlocked by malformed verified dependency evidence: %#v", plan.Tasks[1])
			}
			if plan.ReservedNewAttempts != 0 {
				t.Fatalf("malformed verified dependency reserved execution budget: %#v", plan)
			}
		})
	}
}

func TestMissionRecoveryTransitionPlanCurrentDriftOverridesVerifiedHistory(t *testing.T) {
	now := time.Now()
	task := recoveryTransitionTask("verified", AgentTaskSucceeded)
	task.CompletionEvidence = verifiedRecoveryTransitionEvidence(t, now.Add(-time.Minute))
	mission := matchedRecoveryTransitionMission(task)
	mission.Reconciliation.State = missionReconcileGitChanged
	mission.Reconciliation.Reason = "git_worktree_changed"

	plan := planMissionRecoveryTransitions(mission, now)
	if !plan.Valid {
		t.Fatalf("drift is a valid recovery structure, not graph corruption: %#v", plan)
	}
	if plan.Tasks[0].Action != missionRecoveryTransitionBlockedReconciliation || plan.Tasks[0].RequiresNewAttempt {
		t.Fatalf("historical verified state overrode current drift: %#v", plan.Tasks[0])
	}
}

func TestMissionRecoveryTransitionPlanNeverContinuesCrashRunningTask(t *testing.T) {
	task := recoveryTransitionTask("crashed", AgentTaskReady)
	task.Running = true
	plan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(task), time.Now())
	if !plan.Valid {
		t.Fatalf("stale running flag should be classified, not treated as graph corruption: %#v", plan)
	}
	if plan.Tasks[0].Action != missionRecoveryTransitionInterruptedReview || plan.Tasks[0].RequiresNewAttempt {
		t.Fatalf("crash-running task became executable: %#v", plan.Tasks[0])
	}
}

func TestMissionRecoveryTransitionPlanRetryLimitsUseDurableLifecycle(t *testing.T) {
	oneAttempt := recoveryTransitionTask("retry", AgentTaskFailed)
	oneAttempt.Lifecycle = &MissionTaskLifecycle{AttemptCount: 1, RetryCount: 0}
	plan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(oneAttempt), time.Now())
	if plan.Tasks[0].Action != missionRecoveryTransitionRetryCandidate || !plan.Tasks[0].RequiresNewAttempt {
		t.Fatalf("failed task with remaining budget not retry candidate: %#v", plan.Tasks[0])
	}
	if plan.ObservedMissionAttempts != 1 || plan.ReservedNewAttempts != 1 {
		t.Fatalf("retry plan attempt accounting incorrect: %#v", plan)
	}

	limit := recoveryTransitionTask("retry", AgentTaskFailed)
	limit.Lifecycle = &MissionTaskLifecycle{AttemptCount: missionRecoveryMaxTaskAttempts, RetryCount: missionRecoveryMaxTaskAttempts - 1}
	limitPlan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(limit), time.Now())
	if limitPlan.Tasks[0].Action != missionRecoveryTransitionTaskAttemptLimit || limitPlan.Tasks[0].RequiresNewAttempt {
		t.Fatalf("task attempt limit not enforced: %#v", limitPlan.Tasks[0])
	}

	legacyFailed := recoveryTransitionTask("retry", AgentTaskFailed)
	legacyPlan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(legacyFailed), time.Now())
	if legacyPlan.Tasks[0].Action != missionRecoveryTransitionInsufficientLifecycle || legacyPlan.Tasks[0].RequiresNewAttempt {
		t.Fatalf("failed legacy task without lifecycle became retryable: %#v", legacyPlan.Tasks[0])
	}
}

func TestMissionRecoveryTransitionPlanRejectsMalformedRecoveryGraph(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		tasks []MissionRecoveryTaskState
	}{
		{
			name: "duplicate task id",
			tasks: []MissionRecoveryTaskState{
				recoveryTransitionTask("dup", AgentTaskPending),
				recoveryTransitionTask("dup", AgentTaskPending),
			},
		},
		{
			name: "missing dependency",
			tasks: []MissionRecoveryTaskState{
				recoveryTransitionTask("child", AgentTaskPending, "missing"),
			},
		},
		{
			name: "dependency cycle",
			tasks: []MissionRecoveryTaskState{
				recoveryTransitionTask("a", AgentTaskPending, "b"),
				recoveryTransitionTask("b", AgentTaskPending, "a"),
			},
		},
		{
			name: "inconsistent lifecycle",
			tasks: func() []MissionRecoveryTaskState {
				task := recoveryTransitionTask("bad-life", AgentTaskFailed)
				task.Lifecycle = &MissionTaskLifecycle{AttemptCount: 2, RetryCount: 0}
				return []MissionRecoveryTaskState{task}
			}(),
		},
		{
			name: "attempt count above fixed limit",
			tasks: func() []MissionRecoveryTaskState {
				task := recoveryTransitionTask("bad-limit", AgentTaskFailed)
				task.Lifecycle = &MissionTaskLifecycle{AttemptCount: missionRecoveryMaxTaskAttempts + 1, RetryCount: missionRecoveryMaxTaskAttempts}
				return []MissionRecoveryTaskState{task}
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(test.tasks...), now)
			if plan.Valid || plan.InvalidReason != "invalid_recovery_task_graph" {
				t.Fatalf("malformed recovery graph accepted: %#v", plan)
			}
			for _, transition := range plan.Tasks {
				if transition.Action != missionRecoveryTransitionInvalidRecoveryState || transition.RequiresNewAttempt {
					t.Fatalf("malformed recovery task received executable transition: %#v", transition)
				}
			}
		})
	}
}

func TestMissionRecoveryTransitionPlanFixedMissionAttemptBound(t *testing.T) {
	now := time.Now()
	tasks := make([]MissionRecoveryTaskState, 0, maxReadOnlyMissionTasks)
	for index := 0; index < maxReadOnlyMissionTasks; index++ {
		task := recoveryTransitionTask(fmt.Sprintf("task-%02d", index), AgentTaskSucceeded)
		task.Lifecycle = &MissionTaskLifecycle{AttemptCount: missionRecoveryMaxTaskAttempts, RetryCount: missionRecoveryMaxTaskAttempts - 1}
		task.CompletionEvidence = verifiedRecoveryTransitionEvidence(t, now.Add(-time.Minute))
		tasks = append(tasks, task)
	}
	plan := planMissionRecoveryTransitions(matchedRecoveryTransitionMission(tasks...), now)
	if !plan.Valid {
		t.Fatalf("maximum valid recovery plan rejected: %#v", plan)
	}
	if plan.ObservedMissionAttempts != missionRecoveryMaxMissionAttempts || plan.ReservedNewAttempts != 0 {
		t.Fatalf("mission attempt bound not represented exactly: %#v", plan)
	}
	for _, transition := range plan.Tasks {
		if transition.Action != missionRecoveryTransitionReuseVerified || transition.RequiresNewAttempt {
			t.Fatalf("verified max-attempt task should be reusable without a new attempt: %#v", transition)
		}
	}
}
