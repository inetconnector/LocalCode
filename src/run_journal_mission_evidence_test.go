// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestMissionCompletionEvidencePersistsWithoutRawResultText(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	started := time.Now().Add(-time.Minute)
	graph := AgentTaskGraph{
		MissionID: "mission-evidence",
		Tasks: []AgentTask{{
			ID:        "inspect",
			Role:      AgentRoleExplorer,
			Objective: "inspect project",
			State:     AgentTaskSucceeded,
			Result: AgentResult{
				Status:       AgentResultCompleted,
				Summary:      "raw-summary-secret-123",
				Findings:     []Finding{{Summary: "raw-finding-secret-456", Path: "private/customer-name.txt", Evidence: "raw-evidence-secret-789"}},
				ChangedFiles: []string{"private/changed-file.txt"},
				Commits:      []string{"deadbeef"},
				Tests:        []TestResult{{Name: "private-test", Status: "pass", Detail: "raw-test-detail"}},
				Risks:        []Risk{{Severity: "low", Summary: "raw-risk-detail"}},
				SuggestedTasks: []AgentTaskProposal{{
					ID:        "follow-up",
					Role:      "explorer",
					Objective: "raw-suggested-task-detail",
				}},
			},
		}},
	}
	state := RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         "run-evidence",
		Project:       project,
		Phase:         "mission-read-only",
		StartedAt:     started,
		Mission: &MissionRecoveryState{
			Kind:      missionRecoveryKindReadOnly,
			MissionID: graph.MissionID,
			Project:   project,
			State:     missionRecoveryRunning,
			Tasks: []MissionRecoveryTaskState{{
				ID:    "inspect",
				Role:  AgentRoleExplorer,
				State: AgentTaskSucceeded,
			}},
		},
	}
	if err := writeRunJournal(state); err != nil {
		t.Fatal(err)
	}

	app := &AppState{}
	app.journalMissionSchedulerCheckpoint(state.RunID, AgentSchedulerSnapshot{}, &graph)

	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Mission == nil || len(loaded.Mission.Tasks) != 1 {
		t.Fatalf("missing mission evidence state: %#v", loaded)
	}
	evidence := loaded.Mission.Tasks[0].CompletionEvidence
	if evidence == nil {
		t.Fatal("completion evidence was not persisted")
	}
	if evidence.ResultStatus != AgentResultCompleted || len(evidence.ResultSHA256) != 64 {
		t.Fatalf("unexpected completion evidence: %#v", evidence)
	}
	if evidence.FindingCount != 1 || evidence.ChangedFileCount != 1 || evidence.CommitCount != 1 || evidence.TestCount != 1 || evidence.RiskCount != 1 || evidence.SuggestedTaskCount != 1 {
		t.Fatalf("unexpected evidence counts: %#v", evidence)
	}
	if evidence.VerificationState != missionVerificationUnverified || evidence.CompletedAt.IsZero() {
		t.Fatalf("unexpected verification state: %#v", evidence)
	}

	raw, err := os.ReadFile(runJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{
		"raw-summary-secret-123",
		"raw-finding-secret-456",
		"private/customer-name.txt",
		"raw-evidence-secret-789",
		"private/changed-file.txt",
		"raw-test-detail",
		"raw-risk-detail",
		"raw-suggested-task-detail",
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("raw result content leaked into run journal: %q", forbidden)
		}
	}
}

func TestMissionCompletionEvidenceIsImmutableAfterFirstAcceptedCompletion(t *testing.T) {
	mission := &MissionRecoveryState{Tasks: []MissionRecoveryTaskState{{ID: "inspect"}}}
	firstAt := time.Now().Add(-time.Minute)
	graph := AgentTaskGraph{Tasks: []AgentTask{{
		ID:    "inspect",
		State: AgentTaskSucceeded,
		Result: AgentResult{
			Status:  AgentResultCompleted,
			Summary: "first result",
		},
	}}}

	applyMissionCompletionEvidence(mission, &graph, firstAt)
	if mission.Tasks[0].CompletionEvidence == nil {
		t.Fatal("first evidence missing")
	}
	first := *mission.Tasks[0].CompletionEvidence
	graph.Tasks[0].Result.Summary = "late stale replacement"
	graph.Tasks[0].Result.Findings = []Finding{{Summary: "late finding"}}
	applyMissionCompletionEvidence(mission, &graph, time.Now())

	got := mission.Tasks[0].CompletionEvidence
	if got.ResultSHA256 != first.ResultSHA256 || !got.CompletedAt.Equal(first.CompletedAt) || got.FindingCount != first.FindingCount {
		t.Fatalf("completion evidence was overwritten: first=%#v got=%#v", first, got)
	}
}

func TestMissionCompletionEvidenceRequiresAcceptedSuccess(t *testing.T) {
	mission := &MissionRecoveryState{Tasks: []MissionRecoveryTaskState{{ID: "running"}, {ID: "failed"}, {ID: "blocked-result"}}}
	graph := AgentTaskGraph{Tasks: []AgentTask{
		{ID: "running", State: AgentTaskRunning, Result: AgentResult{Status: AgentResultCompleted}},
		{ID: "failed", State: AgentTaskFailed, Result: AgentResult{Status: AgentResultCompleted}},
		{ID: "blocked-result", State: AgentTaskSucceeded, Result: AgentResult{Status: AgentResultBlocked}},
	}}
	applyMissionCompletionEvidence(mission, &graph, time.Now())
	for _, task := range mission.Tasks {
		if task.CompletionEvidence != nil {
			t.Fatalf("unexpected completion evidence for %s: %#v", task.ID, task.CompletionEvidence)
		}
	}
}

func TestMissionCompletionEvidenceSurvivesFinalGraphRebuild(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	completedAt := time.Now().Add(-time.Minute)
	firstEvidence := missionTaskCompletionEvidence(AgentResult{Status: AgentResultCompleted, Summary: "original result"}, completedAt)
	state := RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         "run-final-evidence",
		Project:       project,
		Phase:         "mission-read-only",
		StartedAt:     completedAt.Add(-time.Minute),
		Mission: &MissionRecoveryState{
			Kind:      missionRecoveryKindReadOnly,
			MissionID: "mission-final-evidence",
			Project:   project,
			State:     missionRecoveryRunning,
			Tasks: []MissionRecoveryTaskState{{
				ID:                 "inspect",
				State:              AgentTaskSucceeded,
				CompletionEvidence: firstEvidence,
			}},
		},
	}
	if err := writeRunJournal(state); err != nil {
		t.Fatal(err)
	}

	result := AgentReadOnlyMissionResult{
		MissionID: "mission-final-evidence",
		State:     AgentMissionSucceeded,
		Graph: AgentTaskGraph{MissionID: "mission-final-evidence", Tasks: []AgentTask{{
			ID:    "inspect",
			State: AgentTaskSucceeded,
			Result: AgentResult{
				Status:  AgentResultCompleted,
				Summary: "different final result that must not replace first evidence",
			},
		}},
		},
		Run: AgentScheduledRun{UsageByTask: map[string]AgentUsage{}},
	}
	app := &AppState{}
	app.finishMissionRunJournal(state.RunID, result, "succeeded")

	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Mission == nil || len(loaded.Mission.Tasks) != 1 || loaded.Mission.Tasks[0].CompletionEvidence == nil {
		t.Fatalf("completion evidence lost during final rebuild: %#v", loaded)
	}
	got := loaded.Mission.Tasks[0].CompletionEvidence
	if got.ResultSHA256 != firstEvidence.ResultSHA256 || !got.CompletedAt.Equal(firstEvidence.CompletedAt) {
		t.Fatalf("completion evidence changed during final rebuild: first=%#v got=%#v", firstEvidence, got)
	}
}
