// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestScheduledReadOnlyDispatchParentCancellationDiscardsChildResult(t *testing.T) {
	project := t.TempDir()
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	graph := schedulerTestGraph(t, []AgentTaskProposal{{
		ID:        "parent-cancel",
		Role:      "explorer",
		Objective: "Wait for parent mission cancellation.",
	}})
	grantSchedulerTestCapabilities(t, &graph, "parent-cancel")
	scheduler := NewAgentScheduler(parent, AgentResourceLimits{})
	defer scheduler.missionCancel()

	started := make(chan struct{})
	done := make(chan scheduledDispatchTestOutcome, 1)
	execute := func(ctx context.Context, _ string, _ Config, _ AgentTask) (AgentResult, error) {
		close(started)
		<-ctx.Done()
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: "partial work after parent cancellation",
			Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 30},
		}, ctx.Err()
	}

	go func() {
		run, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutor(project, Config{}, &graph, scheduler, execute)
		done <- scheduledDispatchTestOutcome{run: run, err: err}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled child did not start")
	}
	cancelParent()

	var outcome scheduledDispatchTestOutcome
	select {
	case outcome = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch did not stop after parent cancellation")
	}
	if !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("dispatch error=%v want context.Canceled", outcome.err)
	}
	if len(outcome.run.Results) != 0 || len(outcome.run.UsageByTask) != 0 {
		t.Fatalf("parent cancellation exposed partial child result/usage: %+v", outcome.run)
	}
	task := agentTaskByID(&graph, "parent-cancel")
	if task == nil || task.State != AgentTaskCancelled {
		t.Fatalf("task after parent cancellation=%+v", task)
	}
	if task.Result.Status != "" || task.Result.Summary != "" {
		t.Fatalf("parent cancellation persisted partial result: %+v", task.Result)
	}
	snapshot := scheduler.Snapshot(&graph, nil)
	if snapshot.Queued != 0 || snapshot.Running != 0 {
		t.Fatalf("scheduler retained work after parent cancellation: %+v", snapshot)
	}
	for _, resource := range snapshot.Resources {
		if resource.InUse != 0 {
			t.Fatalf("resource leaked after parent cancellation: %+v", resource)
		}
	}
}
