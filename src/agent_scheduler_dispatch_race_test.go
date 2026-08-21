// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type scheduledDispatchTestOutcome struct {
	run AgentScheduledRun
	err error
}

func TestScheduledReadOnlyDispatchCancellationWinsCompletionRace(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{
		ID:        "root",
		Role:      "explorer",
		Objective: "Wait until the scheduler cancels this task.",
	}})
	grantSchedulerTestCapabilities(t, &graph, "root")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()

	started := make(chan struct{})
	done := make(chan scheduledDispatchTestOutcome, 1)
	execute := func(ctx context.Context, _ string, _ Config, _ AgentTask) (AgentResult, error) {
		close(started)
		<-ctx.Done()
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: "late child result must not overwrite cancellation",
			Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 10},
		}, ctx.Err()
	}

	go func() {
		run, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutor(t.TempDir(), Config{}, &graph, scheduler, execute)
		done <- scheduledDispatchTestOutcome{run: run, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled child did not start")
	}
	if err := scheduler.CancelTask(&graph, "root"); err != nil {
		t.Fatal(err)
	}

	var outcome scheduledDispatchTestOutcome
	select {
	case outcome = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled dispatch did not stop after cancellation")
	}
	if !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("dispatch error=%v want context.Canceled", outcome.err)
	}
	if len(outcome.run.Results) != 0 || len(outcome.run.UsageByTask) != 0 {
		t.Fatalf("late child result/usage was applied after cancellation: %+v", outcome.run)
	}
	task := agentTaskByID(&graph, "root")
	if task == nil || task.State != AgentTaskCancelled {
		t.Fatalf("cancelled task state=%+v", task)
	}
	if task.Result.Status != "" || task.Result.Summary != "" {
		t.Fatalf("late result overwrote cancelled task: %+v", task.Result)
	}
	snapshot := scheduler.Snapshot(&graph, nil)
	if snapshot.Queued != 0 || snapshot.Running != 0 {
		t.Fatalf("scheduler retained work after cancellation: %+v", snapshot)
	}
	for _, resource := range snapshot.Resources {
		if resource.InUse != 0 {
			t.Fatalf("resource leaked after cancellation: %+v", resource)
		}
	}
}

func TestScheduledReadOnlyDispatchCompletionCancelRaceHasSingleTerminalWinner(t *testing.T) {
	const iterations = 32
	for i := 0; i < iterations; i++ {
		graph := schedulerTestGraph(t, []AgentTaskProposal{{
			ID:        "race",
			Role:      "explorer",
			Objective: "Race completion with cancellation.",
		}})
		grantSchedulerTestCapabilities(t, &graph, "race")
		scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})

		started := make(chan struct{})
		release := make(chan struct{})
		done := make(chan scheduledDispatchTestOutcome, 1)
		execute := func(ctx context.Context, _ string, _ Config, _ AgentTask) (AgentResult, error) {
			close(started)
			select {
			case <-release:
				return AgentResult{
					Status:  AgentResultCompleted,
					Summary: "completed",
					Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 20},
				}, nil
			case <-ctx.Done():
				return AgentResult{Usage: AgentUsage{ModelCalls: 1}}, ctx.Err()
			}
		}

		go func() {
			run, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutor(t.TempDir(), Config{}, &graph, scheduler, execute)
			done <- scheduledDispatchTestOutcome{run: run, err: err}
		}()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: scheduled child did not start", i)
		}

		cancelDone := make(chan error, 1)
		if i%2 == 0 {
			go func() { cancelDone <- scheduler.CancelTask(&graph, "race") }()
			close(release)
		} else {
			close(release)
			go func() { cancelDone <- scheduler.CancelTask(&graph, "race") }()
		}

		var outcome scheduledDispatchTestOutcome
		select {
		case outcome = <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: dispatch did not finish", i)
		}
		if err := <-cancelDone; err != nil {
			t.Fatalf("iteration %d: cancel error: %v", i, err)
		}

		task := agentTaskByID(&graph, "race")
		if task == nil {
			t.Fatalf("iteration %d: task disappeared", i)
		}
		switch task.State {
		case AgentTaskSucceeded:
			if outcome.err != nil {
				t.Fatalf("iteration %d: succeeded task returned error %v", i, outcome.err)
			}
			if len(outcome.run.Results) != 1 || outcome.run.Results[0].Result.Status != AgentResultCompleted {
				t.Fatalf("iteration %d: completion winner missing one result: %+v", i, outcome.run)
			}
			if task.Result.Status != AgentResultCompleted || task.Result.Summary != "completed" {
				t.Fatalf("iteration %d: successful result not preserved: %+v", i, task.Result)
			}
		case AgentTaskCancelled:
			if !errors.Is(outcome.err, context.Canceled) {
				t.Fatalf("iteration %d: cancelled task error=%v", i, outcome.err)
			}
			if len(outcome.run.Results) != 0 || task.Result.Status != "" {
				t.Fatalf("iteration %d: cancellation winner accepted late result: run=%+v task=%+v", i, outcome.run, task)
			}
		default:
			t.Fatalf("iteration %d: race produced invalid terminal state %q", i, task.State)
		}

		snapshot := scheduler.Snapshot(&graph, outcome.run.UsageByTask)
		if snapshot.Queued != 0 || snapshot.Running != 0 {
			t.Fatalf("iteration %d: scheduler retained work: %+v", i, snapshot)
		}
		for _, resource := range snapshot.Resources {
			if resource.InUse != 0 {
				t.Fatalf("iteration %d: resource leaked: %+v", i, resource)
			}
		}
		scheduler.missionCancel()
	}
}

func TestScheduledFinalizePreservesAlreadySuccessfulTaskAgainstCancel(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{ID: "done", Role: "explorer", Objective: "finish"}})
	grantSchedulerTestCapabilities(t, &graph, "done")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	lease, admitted, err := scheduler.AdmitNext(&graph)
	if err != nil || !admitted {
		t.Fatalf("admission admitted=%v err=%v", admitted, err)
	}
	if _, err := scheduler.prepareScheduledAgentTask(&graph, lease, Config{}); err != nil {
		t.Fatal(err)
	}
	result := AgentResult{Status: AgentResultCompleted, Summary: "winner"}
	finalized, err := scheduler.finalizeScheduledAgentTask(&graph, lease, result, AgentTaskSucceeded)
	if err != nil || !finalized.Applied || finalized.State != AgentTaskSucceeded {
		t.Fatalf("finalize=%+v err=%v", finalized, err)
	}
	if err := scheduler.CancelTask(&graph, "done"); err != nil {
		t.Fatal(err)
	}
	task := agentTaskByID(&graph, "done")
	if task == nil || task.State != AgentTaskSucceeded || task.Result.Summary != "winner" {
		t.Fatalf("late cancellation changed completed task: %+v", task)
	}
}
