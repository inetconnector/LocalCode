// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"testing"
)

func TestAgentSchedulerSaturationPreservesClassFIFOAndAllowsCrossClassBypass(t *testing.T) {
	proposals := []AgentTaskProposal{
		{ID: "model-0", Role: "explorer", Objective: "model 0"},
		{ID: "model-1", Role: "planner", Objective: "model 1"},
		{ID: "model-2", Role: "reviewer", Objective: "model 2"},
		{ID: "read-0", Role: "explorer", Objective: "read 0"},
		{ID: "model-3", Role: "explorer", Objective: "model 3"},
		{ID: "read-1", Role: "reviewer", Objective: "read 1"},
		{ID: "model-4", Role: "planner", Objective: "model 4"},
		{ID: "read-2", Role: "explorer", Objective: "read 2"},
		{ID: "model-5", Role: "reviewer", Objective: "model 5"},
	}
	graph := schedulerTestGraph(t, proposals)
	ids := make([]string, 0, len(proposals))
	for _, proposal := range proposals {
		ids = append(ids, proposal.ID)
	}
	grantSchedulerTestCapabilities(t, &graph, ids...)

	overrides := map[string]AgentResourceClass{
		"read-0": AgentResourceReadCPU,
		"read-1": AgentResourceReadCPU,
		"read-2": AgentResourceReadCPU,
	}
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{
		MaxQueued:      32,
		ModelInference: 2,
		ReadCPU:        2,
	})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, overrides); err != nil {
		t.Fatal(err)
	}

	admit := func(want string) AgentResourceLease {
		t.Helper()
		lease, ok, err := scheduler.AdmitNext(&graph)
		if err != nil || !ok {
			t.Fatalf("admit %q: ok=%v err=%v", want, ok, err)
		}
		if lease.TaskID != want {
			t.Fatalf("admitted %q want %q", lease.TaskID, want)
		}
		return lease
	}
	release := func(lease AgentResourceLease) {
		t.Helper()
		if err := scheduler.Release(&graph, lease, AgentTaskSucceeded); err != nil {
			t.Fatalf("release %q: %v", lease.TaskID, err)
		}
	}

	model0 := admit("model-0")
	model1 := admit("model-1")
	read0 := admit("read-0")
	read1 := admit("read-1")

	if _, ok, err := scheduler.AdmitNext(&graph); err != nil || ok {
		t.Fatalf("all relevant resource classes are saturated; admission ok=%v err=%v", ok, err)
	}
	snapshot := scheduler.Snapshot(&graph, nil)
	if snapshot.Running != 4 || snapshot.Queued != 5 {
		t.Fatalf("saturated snapshot running=%d queued=%d", snapshot.Running, snapshot.Queued)
	}
	resourceState := map[AgentResourceClass]AgentResourceSnapshot{}
	for _, resource := range snapshot.Resources {
		resourceState[resource.Class] = resource
	}
	if got := resourceState[AgentResourceModelInference]; got.InUse != 2 || got.Available != 0 {
		t.Fatalf("model resource not saturated: %+v", got)
	}
	if got := resourceState[AgentResourceReadCPU]; got.InUse != 2 || got.Available != 0 {
		t.Fatalf("read resource not saturated: %+v", got)
	}

	// Freeing one model slot must admit the oldest waiting model task, not a
	// newer task from the same class.
	release(model0)
	model2 := admit("model-2")

	// Model inference is saturated again. Freeing one read slot must allow the
	// waiting read task to bypass older model tasks that cannot currently run.
	release(read0)
	read2 := admit("read-2")

	// Continue draining model work. FIFO within the saturated class must hold.
	release(model1)
	model3 := admit("model-3")
	release(model2)
	model4 := admit("model-4")
	release(model3)
	model5 := admit("model-5")

	if _, ok, err := scheduler.AdmitNext(&graph); err != nil || ok {
		t.Fatalf("all queued work should now be admitted: ok=%v err=%v", ok, err)
	}

	for _, lease := range []AgentResourceLease{read1, read2, model4, model5} {
		release(lease)
	}
	final := scheduler.Snapshot(&graph, nil)
	if final.Queued != 0 || final.Running != 0 {
		t.Fatalf("scheduler did not drain after saturation test: %+v", final)
	}
	for _, resource := range final.Resources {
		if resource.InUse != 0 {
			t.Fatalf("resource leaked after drain: %+v", resource)
		}
	}
	for _, id := range ids {
		task := agentTaskByID(&graph, id)
		if task == nil || task.State != AgentTaskSucceeded {
			t.Fatalf("task %q state=%+v want succeeded", id, task)
		}
	}
}

func TestScheduledReadOnlyGraphDrainsLargeFanOutFanInWithoutStarvation(t *testing.T) {
	const branchCount = 12
	proposals := make([]AgentTaskProposal, 0, branchCount+2)
	proposals = append(proposals, AgentTaskProposal{ID: "root", Role: "explorer", Objective: "root"})
	branchIDs := make([]string, 0, branchCount)
	for i := 0; i < branchCount; i++ {
		id := fmt.Sprintf("branch-%02d", i)
		role := "explorer"
		if i%3 == 1 {
			role = "planner"
		} else if i%3 == 2 {
			role = "reviewer"
		}
		branchIDs = append(branchIDs, id)
		proposals = append(proposals, AgentTaskProposal{
			ID:           id,
			Role:         role,
			Objective:    "independent fan-out work",
			Dependencies: []string{"root"},
		})
	}
	proposals = append(proposals, AgentTaskProposal{
		ID:           "join",
		Role:         "reviewer",
		Objective:    "join every fan-out branch",
		Dependencies: append([]string(nil), branchIDs...),
	})

	graph := schedulerTestGraph(t, proposals)
	allIDs := make([]string, 0, len(proposals))
	for _, proposal := range proposals {
		allIDs = append(allIDs, proposal.ID)
	}
	grantSchedulerTestCapabilities(t, &graph, allIDs...)
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{ModelInference: 1, MaxQueued: 64})
	defer scheduler.missionCancel()

	executed := make([]string, 0, len(proposals))
	execute := func(_ context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		executed = append(executed, task.ID)
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: task.ID + " completed",
			Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 10},
		}, nil
	}

	run, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutor(t.TempDir(), Config{}, &graph, scheduler, execute)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := make([]string, 0, len(proposals))
	wantOrder = append(wantOrder, "root")
	wantOrder = append(wantOrder, branchIDs...)
	wantOrder = append(wantOrder, "join")
	if len(executed) != len(wantOrder) || len(run.Results) != len(wantOrder) || len(run.UsageByTask) != len(wantOrder) {
		t.Fatalf("large DAG did not fully drain: executed=%d results=%d usage=%d want=%d", len(executed), len(run.Results), len(run.UsageByTask), len(wantOrder))
	}
	for i, want := range wantOrder {
		if executed[i] != want || run.Results[i].TaskID != want {
			t.Fatalf("dispatch[%d] executed=%q result=%q want=%q", i, executed[i], run.Results[i].TaskID, want)
		}
		task := agentTaskByID(&graph, want)
		if task == nil || task.State != AgentTaskSucceeded || task.Result.Status != AgentResultCompleted {
			t.Fatalf("task %q did not finish successfully: %+v", want, task)
		}
	}
	if run.Snapshot.Queued != 0 || run.Snapshot.Running != 0 {
		t.Fatalf("large DAG scheduler did not drain: %+v", run.Snapshot)
	}
	for _, resource := range run.Snapshot.Resources {
		if resource.InUse != 0 {
			t.Fatalf("resource leaked after large DAG: %+v", resource)
		}
	}
}
