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

type MissionVerificationState string

const (
	missionVerificationUnverified MissionVerificationState = "unverified"
	missionVerificationVerified   MissionVerificationState = "verified"
	missionVerificationFailed     MissionVerificationState = "failed"
	maxMissionVerificationChecks                           = 32
)

type MissionTaskCompletionEvidence struct {
	ResultStatus                   AgentResultStatus        `json:"result_status"`
	ResultSHA256                   string                   `json:"result_sha256"`
	FindingCount                   int                      `json:"finding_count"`
	ChangedFileCount               int                      `json:"changed_file_count"`
	CommitCount                    int                      `json:"commit_count"`
	TestCount                      int                      `json:"test_count"`
	RiskCount                      int                      `json:"risk_count"`
	SuggestedTaskCount             int                      `json:"suggested_task_count"`
	VerificationState              MissionVerificationState `json:"verification_state"`
	VerificationAttemptCount       int                      `json:"verification_attempt_count"`
	LastVerificationEvidenceSHA256 string                   `json:"last_verification_evidence_sha256,omitempty"`
	LastVerificationCheckCount     int                      `json:"last_verification_check_count,omitempty"`
	VerificationUpdatedAt          time.Time                `json:"verification_updated_at"`
	CompletedAt                    time.Time                `json:"completed_at"`
}

func missionTaskResultDigest(result AgentResult) string {
	data, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func missionTaskCompletionEvidence(result AgentResult, completedAt time.Time) *MissionTaskCompletionEvidence {
	if result.Status != AgentResultCompleted && result.Status != AgentResultFallback {
		return nil
	}
	digest := missionTaskResultDigest(result)
	if digest == "" {
		return nil
	}
	if completedAt.IsZero() {
		completedAt = time.Now()
	}
	return &MissionTaskCompletionEvidence{
		ResultStatus:          result.Status,
		ResultSHA256:          digest,
		FindingCount:          len(result.Findings),
		ChangedFileCount:      len(result.ChangedFiles),
		CommitCount:           len(result.Commits),
		TestCount:             len(result.Tests),
		RiskCount:             len(result.Risks),
		SuggestedTaskCount:    len(result.SuggestedTasks),
		VerificationState:     missionVerificationUnverified,
		VerificationUpdatedAt: completedAt,
		CompletedAt:           completedAt,
	}
}

func validMissionVerificationDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func recordMissionTaskVerificationOutcome(evidence *MissionTaskCompletionEvidence, next MissionVerificationState, evidenceSHA256 string, checkCount int, observedAt time.Time) error {
	if evidence == nil {
		return fmt.Errorf("mission completion evidence is nil")
	}
	if next != missionVerificationVerified && next != missionVerificationFailed {
		return fmt.Errorf("unsupported mission verification outcome %q", next)
	}
	if evidence.VerificationState == missionVerificationVerified {
		return fmt.Errorf("verified mission completion evidence is terminal")
	}
	if evidence.VerificationState != missionVerificationUnverified && evidence.VerificationState != missionVerificationFailed {
		return fmt.Errorf("unsupported current mission verification state %q", evidence.VerificationState)
	}
	if !validMissionVerificationDigest(evidenceSHA256) {
		return fmt.Errorf("mission verification evidence digest must be canonical sha256")
	}
	if checkCount <= 0 || checkCount > maxMissionVerificationChecks {
		return fmt.Errorf("mission verification check count must be between 1 and %d", maxMissionVerificationChecks)
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	evidence.VerificationState = next
	evidence.VerificationAttemptCount++
	evidence.LastVerificationEvidenceSHA256 = evidenceSHA256
	evidence.LastVerificationCheckCount = checkCount
	evidence.VerificationUpdatedAt = observedAt
	return nil
}

func applyMissionCompletionEvidence(mission *MissionRecoveryState, graph *AgentTaskGraph, completedAt time.Time) {
	if mission == nil || graph == nil {
		return
	}
	byID := make(map[string]AgentTask, len(graph.Tasks))
	for _, task := range graph.Tasks {
		byID[task.ID] = task
	}
	for index := range mission.Tasks {
		if mission.Tasks[index].CompletionEvidence != nil {
			continue
		}
		graphTask, ok := byID[mission.Tasks[index].ID]
		if !ok || (graphTask.State != AgentTaskSucceeded && graphTask.State != AgentTaskCompleted) {
			continue
		}
		mission.Tasks[index].CompletionEvidence = missionTaskCompletionEvidence(graphTask.Result, completedAt)
	}
}
