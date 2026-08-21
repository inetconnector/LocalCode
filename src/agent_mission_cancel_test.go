// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReadOnlyMissionStopAgentTerminalizesUnfinishedGraphAndKeepsExecutionRunIDSeparate(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := AgentReadOnlyMissionRequest{
		MissionID: "mission-cancel-product-boundary",
		Project:   project,
		Model:     "test-model",
		Tasks: []AgentTaskProposal{
			{ID: "done", Role: "explorer", Objective: "Finish before cancellation."},
			{ID: "running", Role: "reviewer", Objective: "Block until cancellation."},
			{ID: "queued", Role: "planner", Objective: "Remain queued behind running."},
			{ID: "blocked", Role: "reviewer", Objective: "Remain dependency blocked.", Dependencies: []string{"running"}},
		},
	}

	runningStarted := make(chan struct{})
	unexpectedExecution := make(chan string, 1)
	type outcome struct {
		mission AgentReadOnlyMissionResult
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(ctx context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
			switch task.ID {
			case "done":
				return AgentResult{
					Status:  AgentResultCompleted,
					Summary: "completed before cancellation",
					Usage:   AgentUsage{ModelCalls: 1, EstimatedTokens: 10},
				}, nil
			case "running":
				close(runningStarted)
				<-ctx.Done()
				return AgentResult{
					Status:  AgentResultCompleted,
					Summary: "late result must be discarded",
					Usage:   AgentUsage{ModelCalls: 1, EstimatedTokens: 20},
				}, ctx.Err()
			default:
				select {
				case unexpectedExecution <- task.ID:
				default:
				}
				return AgentResult{Status: AgentResultCompleted, Summary: "unexpected"}, nil
			}
		})
		done <- outcome{mission: mission, err: err}
	}()

	select {
	case <-runningStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("running mission child did not start")
	}

	state.mu.RLock()
	activeRunID := state.RunID
	state.mu.RUnlock()
	if activeRunID == "" || activeRunID == req.MissionID {
		t.Fatalf("execution RunID=%q must be non-empty and separate from MissionID=%q", activeRunID, req.MissionID)
	}

	if !state.StopAgent() {
		t.Fatal("StopAgent did not see the active mission")
	}

	var got outcome
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("mission did not stop after StopAgent")
	}
	select {
	case taskID := <-unexpectedExecution:
		t.Fatalf("unfinished sibling %q executed after cancellation", taskID)
	default:
	}

	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("mission error=%v want context.Canceled", got.err)
	}
	if got.mission.MissionID != req.MissionID || got.mission.Run.MissionID != req.MissionID {
		t.Fatalf("stable mission identity changed: result=%q run=%q", got.mission.MissionID, got.mission.Run.MissionID)
	}
	if got.mission.State != AgentMissionCancelled || got.mission.Reason != AgentMissionReasonCancelled {
		t.Fatalf("mission outcome state=%q reason=%q", got.mission.State, got.mission.Reason)
	}

	wantStates := map[string]AgentTaskState{
		"done":    AgentTaskSucceeded,
		"running": AgentTaskCancelled,
		"queued":  AgentTaskCancelled,
		"blocked": AgentTaskCancelled,
	}
	for taskID, want := range wantStates {
		task := agentTaskByID(&got.mission.Graph, taskID)
		if task == nil || task.State != want {
			t.Fatalf("task %q state=%v want=%q", taskID, task, want)
		}
	}
	completed := agentTaskByID(&got.mission.Graph, "done")
	if completed == nil || completed.Result.Status != AgentResultCompleted || completed.Result.Summary != "completed before cancellation" {
		t.Fatalf("already-terminal successful task was not preserved: %+v", completed)
	}
	for _, taskID := range []string{"running", "queued", "blocked"} {
		task := agentTaskByID(&got.mission.Graph, taskID)
		if task == nil || task.Result.Status != "" {
			t.Fatalf("cancelled unfinished task %q retained a result: %+v", taskID, task)
		}
	}
	if len(got.mission.Run.Results) != 1 || got.mission.Run.Results[0].TaskID != "done" {
		t.Fatalf("cancelled mission accepted unexpected results: %+v", got.mission.Run.Results)
	}
	if len(got.mission.Run.UsageByTask) != 1 {
		t.Fatalf("cancelled mission usage=%v want only completed task usage", got.mission.Run.UsageByTask)
	}
	if _, ok := got.mission.Run.UsageByTask["done"]; !ok {
		t.Fatalf("completed task usage missing: %v", got.mission.Run.UsageByTask)
	}
}
