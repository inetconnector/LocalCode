// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

const missionVerificationUnverified = "unverified"

type MissionTaskCompletionEvidence struct {
	ResultStatus       AgentResultStatus `json:"result_status"`
	ResultSHA256       string            `json:"result_sha256"`
	FindingCount       int               `json:"finding_count"`
	ChangedFileCount   int               `json:"changed_file_count"`
	CommitCount        int               `json:"commit_count"`
	TestCount          int               `json:"test_count"`
	RiskCount          int               `json:"risk_count"`
	SuggestedTaskCount int               `json:"suggested_task_count"`
	VerificationState  string            `json:"verification_state"`
	CompletedAt        time.Time         `json:"completed_at"`
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
	return &MissionTaskCompletionEvidence{
		ResultStatus:       result.Status,
		ResultSHA256:       digest,
		FindingCount:       len(result.Findings),
		ChangedFileCount:   len(result.ChangedFiles),
		CommitCount:        len(result.Commits),
		TestCount:          len(result.Tests),
		RiskCount:          len(result.Risks),
		SuggestedTaskCount: len(result.SuggestedTasks),
		VerificationState:  missionVerificationUnverified,
		CompletedAt:        completedAt,
	}
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
