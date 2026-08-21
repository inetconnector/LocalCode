// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScheduledReadOnlyGraphDispatchesAndReconcilesDependencies(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Scheduler fixture\n\nalpha beta gamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "explore-root", Role: "explorer", Objective: "Inspect README.md and identify the project fixture."},
		{ID: "review-after", Role: "reviewer", Objective: "Review the fixture evidence.", Dependencies: []string{"explore-root"}},
	})
	grantSchedulerTestCapabilities(t, &graph, "explore-root", "review-after")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()

	state := &AppState{}
	run, err := state.runScheduledReadOnlyAgentGraph(project, Config{}, &graph, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Results) != 2 {
		t.Fatalf("results=%d want=2: %+v", len(run.Results), run.Results)
	}
	if run.Results[0].TaskID != "explore-root" || run.Results[1].TaskID != "review-after" {
		t.Fatalf("unexpected deterministic dispatch order: %+v", run.Results)
	}
	for _, id := range []string{"explore-root", "review-after"} {
		task := agentTaskByID(&graph, id)
		if task == nil || task.State != AgentTaskSucceeded {
			t.Fatalf("task %s did not succeed: %+v", id, task)
		}
		if task.Result.Status != AgentResultFallback {
			t.Fatalf("task %s result status=%q want fallback with nil Ollama", id, task.Result.Status)
		}
		if _, ok := run.UsageByTask[id]; !ok {
			t.Fatalf("task %s usage was not collected", id)
		}
	}
	if run.Snapshot.Queued != 0 || run.Snapshot.Running != 0 {
		t.Fatalf("scheduler did not drain: %+v", run.Snapshot)
	}
}

func TestScheduledReadOnlyGraphHandlesFanOutAndFanIn(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# DAG fixture\n\nfan out then fan in\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "root", Role: "explorer", Objective: "Inspect the root fixture."},
		{ID: "left", Role: "explorer", Objective: "Inspect the left branch.", Dependencies: []string{"root"}},
		{ID: "right", Role: "reviewer", Objective: "Inspect the right branch.", Dependencies: []string{"root"}},
		{ID: "join", Role: "reviewer", Objective: "Review both branch results.", Dependencies: []string{"left", "right"}},
	})
	grantSchedulerTestCapabilities(t, &graph, "root", "left", "right", "join")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()

	run, err := (&AppState{}).runScheduledReadOnlyAgentGraph(project, Config{}, &graph, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{"root", "left", "right", "join"}
	if len(run.Results) != len(wantOrder) {
		t.Fatalf("results=%d want=%d: %+v", len(run.Results), len(wantOrder), run.Results)
	}
	for i, want := range wantOrder {
		if got := run.Results[i].TaskID; got != want {
			t.Fatalf("result[%d]=%q want=%q: %+v", i, got, want, run.Results)
		}
		task := agentTaskByID(&graph, want)
		if task == nil || task.State != AgentTaskSucceeded {
			t.Fatalf("task %s state=%+v want succeeded", want, task)
		}
	}
	if run.Snapshot.Queued != 0 || run.Snapshot.Running != 0 {
		t.Fatalf("fan-out/fan-in scheduler did not drain: %+v", run.Snapshot)
	}
}

func TestScheduledReadOnlyGraphHonorsCancelledMissionBeforeDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	graph := schedulerTestGraph(t, []AgentTaskProposal{{
		ID:        "never-run",
		Role:      "explorer",
		Objective: "This task must not execute after mission cancellation.",
	}})
	grantSchedulerTestCapabilities(t, &graph, "never-run")
	scheduler := NewAgentScheduler(ctx, AgentResourceLimits{})
	defer scheduler.missionCancel()

	run, err := (&AppState{}).runScheduledReadOnlyAgentGraph(t.TempDir(), Config{}, &graph, scheduler)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context canceled", err)
	}
	if len(run.Results) != 0 {
		t.Fatalf("cancelled mission executed child work: %+v", run.Results)
	}
	if task := agentTaskByID(&graph, "never-run"); task == nil || task.State != AgentTaskReady {
		t.Fatalf("pre-dispatch cancellation mutated graph task unexpectedly: %+v", task)
	}
	if run.Snapshot.Running != 0 || run.Snapshot.Queued != 0 {
		t.Fatalf("cancelled scheduler retained resources: %+v", run.Snapshot)
	}
}

func TestScheduledReadOnlyGraphNeverSelfGrantsPlannerRequestedCapabilities(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{
		ID:           "planned",
		Role:         "planner",
		Objective:    "Plan read-only work.",
		Capabilities: []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP, AgentCapabilityPlanning},
	}})
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()

	run, err := (&AppState{}).runScheduledReadOnlyAgentGraph(t.TempDir(), Config{}, &graph, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Results) != 0 {
		t.Fatalf("unauthorized task executed: %+v", run.Results)
	}
	task := agentTaskByID(&graph, "planned")
	if task == nil || len(task.Capabilities) != 0 || len(task.RequestedCapabilities) != 3 {
		t.Fatalf("requested capabilities leaked into granted authority: %+v", task)
	}
	if run.Snapshot.Queued != 1 || len(run.Snapshot.Tasks) != 1 {
		t.Fatalf("unexpected blocked scheduler snapshot: %+v", run.Snapshot)
	}
	if !strings.Contains(run.Snapshot.Tasks[0].AdmissionBlockedReason, "missing granted capability") {
		t.Fatalf("blocked reason=%q", run.Snapshot.Tasks[0].AdmissionBlockedReason)
	}
}

func TestScheduledAgentTaskTerminalState(t *testing.T) {
	cases := []struct {
		name   string
		result AgentResult
		err    error
		want   AgentTaskState
	}{
		{name: "completed", result: AgentResult{Status: AgentResultCompleted}, want: AgentTaskSucceeded},
		{name: "fallback", result: AgentResult{Status: AgentResultFallback}, want: AgentTaskSucceeded},
		{name: "blocked", result: AgentResult{Status: AgentResultBlocked}, want: AgentTaskFailed},
		{name: "budget", result: AgentResult{Status: AgentResultBudgetExhausted}, want: AgentTaskFailed},
		{name: "unknown", result: AgentResult{}, want: AgentTaskFailed},
		{name: "runtime error", result: AgentResult{Status: AgentResultCompleted}, err: context.DeadlineExceeded, want: AgentTaskFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scheduledAgentTaskTerminalState(tc.result, tc.err); got != tc.want {
				t.Fatalf("state=%q want=%q", got, tc.want)
			}
		})
	}
}
