// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMissionPostconditionEvaluationRequiresMatchedDurableSuccess(t *testing.T) {
	now := time.Now()
	evidence := missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted, Summary: "transient"}, now.Add(-time.Minute))
	mission := &MissionRecoveryState{
		MissionID: "mission-verify",
		Tasks: []MissionRecoveryTaskState{{
			ID:                 "inspect",
			State:              AgentTaskSucceeded,
			CompletionEvidence: evidence,
		}},
	}
	reconciliation := &MissionRestartReconciliation{
		State:  missionReconcileMatched,
		Reason: "project_and_git_match_baseline",
		Current: MissionProjectBaseline{
			ProjectIdentitySHA256: missionSHA256String("project"),
			GitState:              missionGitStateObserved,
			GitRootSHA256:         missionSHA256String("root"),
			GitHead:               "abc123",
			GitStatusSHA256:       missionSHA256Bytes(nil),
		},
	}

	result, err := evaluateMissionTaskPostconditions(mission, "inspect", reconciliation, now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.AlreadyVerified || result.CheckCount != 6 || !validMissionVerificationDigest(result.EvidenceSHA256) {
		t.Fatalf("unexpected verification result: %#v", result)
	}
	for _, check := range result.Checks {
		if !check.Passed {
			t.Fatalf("matched durable success failed check: %#v", check)
		}
	}

	mission.Tasks[0].CompletionEvidence.VerificationState = missionVerificationVerified
	already, err := evaluateMissionTaskPostconditions(mission, "inspect", reconciliation, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !already.Passed || !already.AlreadyVerified {
		t.Fatalf("verified matching task was not idempotent: %#v", already)
	}

	drift := *reconciliation
	drift.State = missionReconcileGitChanged
	drift.Reason = "git_worktree_changed"
	blocked, err := evaluateMissionTaskPostconditions(mission, "inspect", &drift, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if blocked.Passed || blocked.AlreadyVerified {
		t.Fatalf("verified task overrode current drift: %#v", blocked)
	}
}

func TestMissionPostconditionEvaluationRejectsRunningMissingOrInvalidEvidence(t *testing.T) {
	now := time.Now()
	reconciliation := &MissionRestartReconciliation{State: missionReconcileMatched, Reason: "project_and_git_match_baseline"}
	cases := []MissionRecoveryTaskState{
		{ID: "running", State: AgentTaskRunning, Running: true, CompletionEvidence: missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, now)},
		{ID: "missing", State: AgentTaskSucceeded},
		{ID: "invalid", State: AgentTaskSucceeded, CompletionEvidence: &MissionTaskCompletionEvidence{ResultStatus: AgentResultCompleted, ResultSHA256: "invalid", VerificationState: missionVerificationUnverified}},
	}
	for _, task := range cases {
		mission := &MissionRecoveryState{MissionID: "mission-invalid", Tasks: []MissionRecoveryTaskState{task}}
		result, err := evaluateMissionTaskPostconditions(mission, task.ID, reconciliation, now)
		if err != nil {
			t.Fatal(err)
		}
		if result.Passed {
			t.Fatalf("unsafe task passed verification: task=%#v result=%#v", task, result)
		}
	}
}

func TestMissionReconciliationUsesVerifiedStateOnlyWhenProjectMatches(t *testing.T) {
	evidence := missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted}, time.Now())
	evidence.VerificationState = missionVerificationVerified
	task := MissionRecoveryTaskState{
		ID:                 "inspect",
		State:              AgentTaskSucceeded,
		Lifecycle:          &MissionTaskLifecycle{AttemptCount: 2, RetryCount: 1},
		CompletionEvidence: evidence,
	}
	matched := missionTaskReconciliation(task, missionReconcileMatched)
	if matched.Disposition != missionTaskDispositionTerminal || matched.Reason != "durable_success_verified_against_matching_project" {
		t.Fatalf("verified matching task not terminal: %#v", matched)
	}
	if matched.AttemptCount != 2 || matched.RetryCount != 1 || matched.VerificationState != missionVerificationVerified {
		t.Fatalf("reconciliation omitted lifecycle/verification data: %#v", matched)
	}

	blocked := missionTaskReconciliation(task, missionReconcileGitChanged)
	if blocked.Disposition != missionTaskDispositionBlocked {
		t.Fatalf("verified task overrode drift: %#v", blocked)
	}

	evidence.VerificationState = missionVerificationFailed
	recheck := missionTaskReconciliation(task, missionReconcileMatched)
	if recheck.Disposition != missionTaskDispositionVerifyPostconditions || recheck.Reason != "previous_verification_failed_requires_recheck" {
		t.Fatalf("failed verification did not require recheck: %#v", recheck)
	}
}

func initMissionVerificationGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	project := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", project}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	run("init")
	run("config", "user.email", "localcode-tests@example.invalid")
	run("config", "user.name", "LocalCode Tests")
	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "tracked.txt")
	run("commit", "-m", "initial")
	return project
}

func TestVerifyRecoverableMissionTaskPersistsVerificationAndLaterDrift(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := initMissionVerificationGitRepo(t)
	baseline := captureMissionProjectBaseline(project)
	started := time.Now().Add(-time.Minute)
	updated := started.Add(10 * time.Second)
	evidence := missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted, Summary: "never persist this raw result"}, updated)
	state := RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         "run-postcondition-verify",
		Project:       project,
		Phase:         "mission-read-only",
		StartedAt:     started,
		UpdatedAt:     updated,
		Mission: &MissionRecoveryState{
			Kind:      missionRecoveryKindReadOnly,
			MissionID: "mission-postcondition-verify",
			Project:   project,
			State:     missionRecoveryRunning,
			Baseline:  &baseline,
			UpdatedAt: updated,
			Tasks: []MissionRecoveryTaskState{{
				ID:                 "inspect",
				State:              AgentTaskSucceeded,
				Lifecycle:          &MissionTaskLifecycle{AttemptCount: 1, StateUpdatedAt: updated, LastStartedAt: started, LastFinishedAt: updated},
				CompletionEvidence: evidence,
			}},
		},
	}
	if err := writeRunJournal(state); err != nil {
		t.Fatal(err)
	}

	result, err := verifyRecoverableMissionTaskPostconditions(state.RunID, "inspect")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed || result.AlreadyVerified || result.CheckCount != 6 {
		t.Fatalf("unexpected first verification: %#v", result)
	}
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Mission == nil || loaded.Mission.Reconciliation == nil {
		t.Fatalf("verification did not persist recovery state: %#v", loaded)
	}
	verified := loaded.Mission.Tasks[0].CompletionEvidence
	if verified == nil || verified.VerificationState != missionVerificationVerified || verified.VerificationAttemptCount != 1 || verified.LastVerificationEvidenceSHA256 != result.EvidenceSHA256 {
		t.Fatalf("verification record not persisted: %#v", verified)
	}
	if loaded.Mission.Reconciliation.State != missionReconcileMatched || loaded.Mission.Reconciliation.Tasks[0].Disposition != missionTaskDispositionTerminal {
		t.Fatalf("verified task reconciliation not refreshed: %#v", loaded.Mission.Reconciliation)
	}

	if err := os.WriteFile(filepath.Join(project, "tracked.txt"), []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	drift, err := verifyRecoverableMissionTaskPostconditions(state.RunID, "inspect")
	if err != nil {
		t.Fatal(err)
	}
	if drift.Passed || drift.AlreadyVerified {
		t.Fatalf("current git drift was ignored: %#v", drift)
	}
	loaded, err = loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mission.Reconciliation.State != missionReconcileGitChanged || loaded.Mission.Reconciliation.Tasks[0].Disposition != missionTaskDispositionBlocked {
		t.Fatalf("drift reconciliation was not persisted: %#v", loaded.Mission.Reconciliation)
	}
	if loaded.Mission.Tasks[0].CompletionEvidence.VerificationState != missionVerificationVerified || loaded.Mission.Tasks[0].CompletionEvidence.VerificationAttemptCount != 1 {
		t.Fatalf("drift regressed terminal verification evidence: %#v", loaded.Mission.Tasks[0].CompletionEvidence)
	}
}
