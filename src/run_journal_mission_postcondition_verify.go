// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	missionPostconditionCheckReconciliation = "project_git_reconciliation_matched"
	missionPostconditionCheckNotRunning     = "task_not_running"
	missionPostconditionCheckDurableSuccess = "task_durable_success"
	missionPostconditionCheckEvidence       = "completion_evidence_present"
	missionPostconditionCheckResultStatus   = "completion_result_status_success"
	missionPostconditionCheckResultDigest   = "completion_result_digest_valid"
)

type MissionTaskPostconditionCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}

type MissionTaskPostconditionVerification struct {
	TaskID          string                          `json:"task_id"`
	Passed          bool                            `json:"passed"`
	AlreadyVerified bool                            `json:"already_verified,omitempty"`
	EvidenceSHA256  string                          `json:"evidence_sha256"`
	CheckCount      int                             `json:"check_count"`
	ObservedAt      time.Time                       `json:"observed_at"`
	Checks          []MissionTaskPostconditionCheck `json:"checks"`
}

type missionTaskPostconditionDigestInput struct {
	MissionID                  string                          `json:"mission_id"`
	TaskID                     string                          `json:"task_id"`
	CompletionResultSHA256     string                          `json:"completion_result_sha256,omitempty"`
	ReconciliationState        string                          `json:"reconciliation_state"`
	ReconciliationReason       string                          `json:"reconciliation_reason"`
	CurrentProjectIdentityHash string                          `json:"current_project_identity_sha256,omitempty"`
	CurrentGitState            string                          `json:"current_git_state"`
	CurrentGitRootHash         string                          `json:"current_git_root_sha256,omitempty"`
	CurrentGitHead             string                          `json:"current_git_head,omitempty"`
	CurrentGitStatusHash       string                          `json:"current_git_status_sha256,omitempty"`
	Checks                     []MissionTaskPostconditionCheck `json:"checks"`
}

func missionTaskPostconditionEvidenceDigest(missionID string, task MissionRecoveryTaskState, reconciliation *MissionRestartReconciliation, checks []MissionTaskPostconditionCheck) string {
	input := missionTaskPostconditionDigestInput{
		MissionID: strings.TrimSpace(missionID),
		TaskID:    task.ID,
		Checks:    append([]MissionTaskPostconditionCheck(nil), checks...),
	}
	if task.CompletionEvidence != nil {
		input.CompletionResultSHA256 = task.CompletionEvidence.ResultSHA256
	}
	if reconciliation != nil {
		input.ReconciliationState = reconciliation.State
		input.ReconciliationReason = reconciliation.Reason
		input.CurrentProjectIdentityHash = reconciliation.Current.ProjectIdentitySHA256
		input.CurrentGitState = reconciliation.Current.GitState
		input.CurrentGitRootHash = reconciliation.Current.GitRootSHA256
		input.CurrentGitHead = reconciliation.Current.GitHead
		input.CurrentGitStatusHash = reconciliation.Current.GitStatusSHA256
	}
	data, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func evaluateMissionTaskPostconditions(mission *MissionRecoveryState, taskID string, reconciliation *MissionRestartReconciliation, observedAt time.Time) (MissionTaskPostconditionVerification, error) {
	result := MissionTaskPostconditionVerification{TaskID: strings.TrimSpace(taskID), ObservedAt: observedAt}
	if mission == nil {
		return result, fmt.Errorf("mission recovery state is nil")
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
		result.ObservedAt = observedAt
	}
	var task *MissionRecoveryTaskState
	for index := range mission.Tasks {
		if mission.Tasks[index].ID == result.TaskID {
			task = &mission.Tasks[index]
			break
		}
	}
	if task == nil {
		return result, fmt.Errorf("mission recovery task %q not found", result.TaskID)
	}

	reconciled := reconciliation != nil && reconciliation.State == missionReconcileMatched
	notRunning := !task.Running && task.State != AgentTaskRunning
	durableSuccess := task.State == AgentTaskSucceeded || task.State == AgentTaskCompleted
	hasEvidence := task.CompletionEvidence != nil
	resultStatusSuccess := false
	resultDigestValid := false
	if task.CompletionEvidence != nil {
		resultStatusSuccess = task.CompletionEvidence.ResultStatus == AgentResultCompleted || task.CompletionEvidence.ResultStatus == AgentResultFallback
		resultDigestValid = validMissionVerificationDigest(task.CompletionEvidence.ResultSHA256)
	}
	result.Checks = []MissionTaskPostconditionCheck{
		{Name: missionPostconditionCheckReconciliation, Passed: reconciled},
		{Name: missionPostconditionCheckNotRunning, Passed: notRunning},
		{Name: missionPostconditionCheckDurableSuccess, Passed: durableSuccess},
		{Name: missionPostconditionCheckEvidence, Passed: hasEvidence},
		{Name: missionPostconditionCheckResultStatus, Passed: resultStatusSuccess},
		{Name: missionPostconditionCheckResultDigest, Passed: resultDigestValid},
	}
	result.Passed = true
	for _, check := range result.Checks {
		if !check.Passed {
			result.Passed = false
			break
		}
	}
	result.CheckCount = len(result.Checks)
	result.EvidenceSHA256 = missionTaskPostconditionEvidenceDigest(mission.MissionID, *task, reconciliation, result.Checks)
	if !validMissionVerificationDigest(result.EvidenceSHA256) {
		return result, fmt.Errorf("failed to build mission postcondition evidence digest")
	}
	if task.CompletionEvidence != nil && task.CompletionEvidence.VerificationState == missionVerificationVerified && result.Passed {
		result.AlreadyVerified = true
	}
	return result, nil
}

func verifyRecoverableMissionTaskPostconditions(runID, taskID string) (MissionTaskPostconditionVerification, error) {
	var result MissionTaskPostconditionVerification
	runID = strings.TrimSpace(runID)
	taskID = strings.TrimSpace(taskID)
	if runID == "" || taskID == "" {
		return result, fmt.Errorf("run id and task id are required")
	}

	runJournalFileMu.Lock()
	state, err := loadRunJournal()
	if err != nil || state == nil {
		runJournalFileMu.Unlock()
		if err != nil {
			return result, err
		}
		return result, fmt.Errorf("run journal is unavailable")
	}
	if state.RunID != runID || state.Mission == nil || state.Terminal {
		runJournalFileMu.Unlock()
		return result, fmt.Errorf("recoverable mission run %q is not available", runID)
	}
	snapshot := *state
	snapshot.Mission = cloneMissionRecoveryState(state.Mission)
	snapshotMissionUpdatedAt := state.Mission.UpdatedAt
	runJournalFileMu.Unlock()

	reconciled := reconcileRecoverableMission(&snapshot)
	if reconciled == nil || reconciled.Mission == nil || reconciled.Mission.Reconciliation == nil {
		return result, fmt.Errorf("mission reconciliation is unavailable")
	}
	result, err = evaluateMissionTaskPostconditions(reconciled.Mission, taskID, reconciled.Mission.Reconciliation, time.Now())
	if err != nil {
		return result, err
	}

	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	current, err := loadRunJournal()
	if err != nil || current == nil {
		if err != nil {
			return result, err
		}
		return result, fmt.Errorf("run journal is unavailable")
	}
	if current.RunID != runID || current.Mission == nil || current.Terminal {
		return result, fmt.Errorf("recoverable mission run changed during verification")
	}
	if !current.Mission.UpdatedAt.Equal(snapshotMissionUpdatedAt) {
		return result, fmt.Errorf("mission recovery state changed during verification")
	}

	var currentTask *MissionRecoveryTaskState
	for index := range current.Mission.Tasks {
		if current.Mission.Tasks[index].ID == taskID {
			currentTask = &current.Mission.Tasks[index]
			break
		}
	}
	if currentTask == nil || currentTask.CompletionEvidence == nil {
		return result, fmt.Errorf("mission completion evidence is unavailable")
	}
	persistReconciliation := func(recompute bool) error {
		if recompute {
			current.Mission.Reconciliation = reconcileMissionRecoveryWithCurrent(current.Mission, reconciled.Mission.Reconciliation.Current, reconciled.Mission.Reconciliation.ObservedAt)
		} else {
			reconciliationCopy := *reconciled.Mission.Reconciliation
			reconciliationCopy.Tasks = append([]MissionRecoveryTaskReconciliation(nil), reconciled.Mission.Reconciliation.Tasks...)
			current.Mission.Reconciliation = &reconciliationCopy
		}
		current.Mission.UpdatedAt = time.Now()
		return writeRunJournalUnlocked(*current)
	}
	if currentTask.CompletionEvidence.VerificationState == missionVerificationVerified {
		if err := persistReconciliation(false); err != nil {
			return result, err
		}
		return result, nil
	}

	next := missionVerificationFailed
	if result.Passed {
		next = missionVerificationVerified
	}
	if err := recordMissionTaskVerificationOutcome(currentTask.CompletionEvidence, next, result.EvidenceSHA256, result.CheckCount, result.ObservedAt); err != nil {
		return result, err
	}
	if err := persistReconciliation(true); err != nil {
		return result, err
	}
	return result, nil
}
