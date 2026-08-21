// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReadOnlyMissionPersistsStructuredTerminalJournal(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Objective = "Inspect authentication token=mission-secret"
	req.Constraints = []string{"Do not mutate files", "password=constraint-secret"}
	req.SuccessCriteria = []string{"Both read-only tasks finish"}

	result, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(context.Context, string, Config, AgentTask) (AgentResult, error) {
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: "Authorization: Bearer child-result-secret",
			Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 128, ElapsedMillis: 5},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != AgentMissionSucceeded {
		t.Fatalf("mission state=%q want=%q", result.State, AgentMissionSucceeded)
	}

	journal, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if journal == nil || journal.Mission == nil {
		t.Fatalf("mission journal missing: %#v", journal)
	}
	if !journal.Terminal || journal.Phase != "idle" {
		t.Fatalf("terminal journal mismatch: %#v", journal)
	}
	mission := journal.Mission
	if mission.MissionID != req.MissionID || mission.Kind != missionRecoveryKindReadOnly || mission.State != string(AgentMissionSucceeded) {
		t.Fatalf("mission identity/state mismatch: %#v", mission)
	}
	if !strings.Contains(mission.Objective, "[REDACTED]") || strings.Contains(mission.Objective, "mission-secret") {
		t.Fatalf("mission objective was not redacted: %q", mission.Objective)
	}
	if len(mission.Constraints) != 2 || !strings.Contains(mission.Constraints[1], "[REDACTED]") || strings.Contains(mission.Constraints[1], "constraint-secret") {
		t.Fatalf("mission constraints were not safely persisted: %#v", mission.Constraints)
	}
	if len(mission.SuccessCriteria) != 1 || mission.SuccessCriteria[0] != "Both read-only tasks finish" {
		t.Fatalf("mission success criteria mismatch: %#v", mission.SuccessCriteria)
	}
	if len(mission.Tasks) != len(req.Tasks) {
		t.Fatalf("persisted tasks=%d want=%d", len(mission.Tasks), len(req.Tasks))
	}
	for _, task := range mission.Tasks {
		if task.State != AgentTaskSucceeded {
			t.Fatalf("task %s durable state=%q want=%q", task.ID, task.State, AgentTaskSucceeded)
		}
		if task.Usage.ModelCalls != 1 {
			t.Fatalf("task %s durable usage=%+v", task.ID, task.Usage)
		}
	}
	if mission.Accounting == nil || mission.Accounting.Usage.ModelCalls != len(req.Tasks) {
		t.Fatalf("mission accounting missing or wrong: %#v", mission.Accounting)
	}
	data, err := os.ReadFile(runJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "child-result-secret") {
		t.Fatalf("raw child result leaked into recovery journal:\n%s", text)
	}
	if recovered := loadRecoverableRun(); recovered != nil {
		t.Fatalf("terminal Mission journal was offered for recovery: %#v", recovered)
	}
}

func TestInterruptedMissionIsRecoverableButNotNormalChatResume(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	req := AgentReadOnlyMissionRequest{
		MissionID:       "mission-crashed",
		Objective:       "Inspect the project safely",
		Project:         project,
		Model:           "test-model",
		Constraints:     []string{"read only"},
		SuccessCriteria: []string{"report findings"},
		Tasks: []AgentTaskProposal{{
			ID:        "inspect",
			Role:      "explorer",
			Objective: "Inspect files",
		}},
	}
	graph, err := buildAgentTaskGraph(req.MissionID, "", req.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	for index := range graph.Tasks {
		role, roleErr := normalizeAgentRole(string(graph.Tasks[index].Role))
		if roleErr != nil {
			t.Fatal(roleErr)
		}
		graph.Tasks[index].Role = role
		graph.Tasks[index].Capabilities = capabilitiesForAgentRole(role)
		graph.Tasks[index].Model = req.Model
	}

	started := time.Now().Add(-time.Minute)
	state := &AppState{}
	state.beginMissionRunJournal("execution-crashed", req, graph, project, req.Model, started)
	state.journalMissionSchedulerSnapshot("execution-crashed", AgentSchedulerSnapshot{
		MissionID: req.MissionID,
		Queued:    0,
		Running:   1,
		Tasks: []AgentTaskScheduleSnapshot{{
			TaskID:        "inspect",
			State:         AgentTaskRunning,
			ResourceClass: AgentResourceModelInference,
			Running:       true,
			Budget: AgentBudgetSnapshot{
				Limit: AgentBudget{ModelCalls: 4, ToolCalls: 8, EstimatedTokenBudget: 20000, TimeSeconds: 60},
			},
		}},
	})

	recovered := loadRecoverableRun()
	if recovered == nil || recovered.Mission == nil {
		t.Fatalf("interrupted Mission was not recoverable: %#v", recovered)
	}
	if recovered.Mission.State != missionRecoveryRunning || len(recovered.Mission.Tasks) != 1 {
		t.Fatalf("unexpected durable Mission snapshot: %#v", recovered.Mission)
	}
	if recovered.Mission.Tasks[0].State != AgentTaskRunning || recovered.Mission.Tasks[0].ResourceClass != AgentResourceModelInference {
		t.Fatalf("scheduler checkpoint was not persisted: %#v", recovered.Mission.Tasks[0])
	}

	state.Recovery = recovered
	if contextText, originalTask := state.recoveryContextForTask(project, "Weiter"); contextText != "" || originalTask != "" {
		t.Fatalf("Mission leaked into normal chat resume: context=%q task=%q", contextText, originalTask)
	}
	event := recoveryStartupEvent(defaultConfig(), recovered)
	if event.Type != "recovery_available" || !strings.Contains(event.Message, "Mission") || !strings.Contains(event.Detail, req.MissionID) {
		t.Fatalf("Mission startup recovery event mismatch: %#v", event)
	}
	if strings.Contains(strings.ToLower(event.Detail), "automatisch fortgesetzt") && !strings.Contains(strings.ToLower(event.Detail), "nicht automatisch fortgesetzt") {
		t.Fatalf("Mission startup detail implies automatic resume: %q", event.Detail)
	}
}

func TestMissionRecoveryMetadataIsBoundedAndDeepCopied(t *testing.T) {
	values := make([]string, maxMissionRecoveryListItems+8)
	for index := range values {
		values[index] = strings.Repeat("x", maxMissionRecoveryItemLength+100)
	}
	req := AgentReadOnlyMissionRequest{
		MissionID:       "mission-bounded",
		Project:         t.TempDir(),
		Model:           "model",
		Constraints:     values,
		SuccessCriteria: values,
		Tasks: []AgentTaskProposal{{
			ID:        "task",
			Role:      "explorer",
			Objective: "inspect",
		}},
	}
	graph, err := buildAgentTaskGraph(req.MissionID, "", req.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	mission := newMissionRecoveryState(req, graph, req.Project, req.Model, time.Now())
	if len(mission.Constraints) != maxMissionRecoveryListItems || len(mission.SuccessCriteria) != maxMissionRecoveryListItems {
		t.Fatalf("unbounded Mission lists: constraints=%d success=%d", len(mission.Constraints), len(mission.SuccessCriteria))
	}
	if len(mission.Constraints[0]) > maxMissionRecoveryItemLength+len("…") {
		t.Fatalf("Mission list item was not bounded: %d", len(mission.Constraints[0]))
	}
	copyMission := cloneMissionRecoveryState(&mission)
	copyMission.Constraints[0] = "changed"
	copyMission.Tasks[0].Dependencies = append(copyMission.Tasks[0].Dependencies, "other")
	if mission.Constraints[0] == "changed" || len(mission.Tasks[0].Dependencies) != 0 {
		t.Fatalf("Mission recovery clone shares mutable slices: original=%#v copy=%#v", mission, copyMission)
	}
}
