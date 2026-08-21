// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func deleteAgentMissionDesktopStatus(runID string) {
	agentMissionDesktopStatuses.Lock()
	delete(agentMissionDesktopStatuses.byRun, runID)
	agentMissionDesktopStatuses.Unlock()
}

func TestStatusJSONIncludesMissionOnlyForMatchingExecutionRun(t *testing.T) {
	runID := "status-run-match"
	deleteAgentMissionDesktopStatus(runID)
	defer deleteAgentMissionDesktopStatus(runID)
	publishAgentMissionDesktopStatus(AgentMissionDesktopStatus{
		MissionID:      "mission-status-json",
		ExecutionRunID: runID,
		State:          "running",
		UpdatedAt:      time.Now(),
	})

	payload, err := json.Marshal(Status{RunID: runID, RunPhase: "mission-read-only"})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		RunID   string                     `json:"run_id"`
		Mission *AgentMissionDesktopStatus `json:"mission"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.RunID != runID || got.Mission == nil || got.Mission.MissionID != "mission-status-json" || got.Mission.State != "running" {
		t.Fatalf("unexpected status payload: %s", payload)
	}

	payload, err = json.Marshal(Status{RunID: "different-run"})
	if err != nil {
		t.Fatal(err)
	}
	var withoutMission map[string]json.RawMessage
	if err := json.Unmarshal(payload, &withoutMission); err != nil {
		t.Fatal(err)
	}
	if _, exists := withoutMission["mission"]; exists {
		t.Fatalf("unrelated run leaked mission status: %s", payload)
	}
}

func TestReadOnlyMissionPublishesLiveAndTerminalDesktopStatus(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.MissionID = "mission-desktop-live"
	req.Tasks = []AgentTaskProposal{
		{ID: "running", Role: "explorer", Objective: "Wait for cancellation."},
		{ID: "queued", Role: "reviewer", Objective: "Remain queued until cancellation."},
	}

	started := make(chan struct{})
	type outcome struct {
		mission AgentReadOnlyMissionResult
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(ctx context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
			if task.ID == "running" {
				close(started)
				<-ctx.Done()
				return AgentResult{Usage: AgentUsage{ModelCalls: 1}}, ctx.Err()
			}
			return AgentResult{Status: AgentResultCompleted}, nil
		})
		done <- outcome{mission: mission, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("mission child did not start")
	}

	state.mu.RLock()
	runID := state.RunID
	phase := state.RunPhase
	state.mu.RUnlock()
	if runID == "" || runID == req.MissionID || phase != "mission-read-only" {
		t.Fatalf("unexpected mission run state: runID=%q missionID=%q phase=%q", runID, req.MissionID, phase)
	}
	defer deleteAgentMissionDesktopStatus(runID)

	deadline := time.Now().Add(2 * time.Second)
	var live AgentMissionDesktopStatus
	for time.Now().Before(deadline) {
		candidate, ok := agentMissionDesktopStatusForRun(runID)
		if ok && candidate.State == "running" && candidate.Scheduler.Running == 1 && candidate.Scheduler.Queued == 1 {
			live = candidate
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if live.MissionID != req.MissionID {
		t.Fatalf("live mission status not published: %+v", live)
	}
	if len(live.Scheduler.Tasks) != 2 {
		t.Fatalf("live scheduler tasks=%d want=2: %+v", len(live.Scheduler.Tasks), live.Scheduler)
	}
	for _, task := range live.Scheduler.Tasks {
		if task.ResourceClass != AgentResourceModelInference {
			t.Fatalf("task %s resource class=%q want model-inference", task.TaskID, task.ResourceClass)
		}
	}

	if !state.StopAgent() {
		t.Fatal("StopAgent did not see active mission")
	}
	var got outcome
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("mission did not stop")
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("mission error=%v want context.Canceled", got.err)
	}

	terminal, ok := agentMissionDesktopStatusForRun(runID)
	if !ok {
		t.Fatal("terminal mission desktop status missing")
	}
	if terminal.State != string(AgentMissionCancelled) || terminal.Reason != AgentMissionReasonCancelled {
		t.Fatalf("unexpected terminal status: %+v", terminal)
	}
	if terminal.Scheduler.Running != 0 || terminal.Scheduler.Queued != 0 {
		t.Fatalf("terminal scheduler did not drain: %+v", terminal.Scheduler)
	}
	for _, task := range terminal.Scheduler.Tasks {
		if task.State != AgentTaskCancelled {
			t.Fatalf("terminal task %s state=%q want cancelled", task.TaskID, task.State)
		}
		if task.ResourceClass != AgentResourceModelInference {
			t.Fatalf("terminal task %s resource class=%q want model-inference", task.TaskID, task.ResourceClass)
		}
	}
}
