// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAgentResourceLimitsNormalizeAndSnapshotAllClasses(t *testing.T) {
	limits := normalizeAgentResourceLimits(AgentResourceLimits{
		MaxQueued:            -1,
		ModelInference:       99,
		ReadCPU:              99,
		Build:                99,
		ExclusiveIntegration: 2,
	})
	defaults := defaultAgentResourceLimits()
	if limits != defaults {
		t.Fatalf("normalized limits=%+v want=%+v", limits, defaults)
	}
	for _, tc := range []struct {
		class AgentResourceClass
		want  int
	}{
		{AgentResourceModelInference, defaults.ModelInference},
		{AgentResourceReadCPU, defaults.ReadCPU},
		{AgentResourceBuild, defaults.Build},
		{AgentResourceIntegration, defaults.ExclusiveIntegration},
	} {
		got, err := limits.limitFor(tc.class)
		if err != nil || got != tc.want {
			t.Fatalf("limitFor(%q)=%d,%v want=%d", tc.class, got, err, tc.want)
		}
	}
	if _, err := limits.limitFor("unknown"); err == nil {
		t.Fatal("unsupported resource class should fail")
	}

	graph := schedulerTestGraph(t, []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "a"}})
	grantSchedulerTestCapabilities(t, &graph, "a")
	scheduler := NewAgentScheduler(context.Background(), limits)
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, map[string]AgentResourceClass{"a": AgentResourceBuild}); err != nil {
		t.Fatal(err)
	}
	snapshot := scheduler.Snapshot(&graph, nil)
	if snapshot.MissionID != graph.MissionID || snapshot.Queued != 1 || snapshot.Running != 0 || len(snapshot.Resources) != 4 {
		t.Fatalf("unexpected scheduler snapshot: %+v", snapshot)
	}
	for _, resource := range snapshot.Resources {
		if resource.InUse != 0 || resource.Available != resource.Limit {
			t.Fatalf("unexpected idle resource snapshot: %+v", resource)
		}
	}
	if snapshot.Tasks[0].ResourceClass != AgentResourceBuild || snapshot.Tasks[0].QueuePosition != 1 {
		t.Fatalf("unexpected queued task snapshot: %+v", snapshot.Tasks[0])
	}
}

func TestAgentSchedulerQueueReadyIsIdempotentAndPrunesNonReady(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "a", Role: "explorer", Objective: "a"},
		{ID: "b", Role: "reviewer", Objective: "b"},
	})
	grantSchedulerTestCapabilities(t, &graph, "a", "b")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	if got := scheduler.Snapshot(&graph, nil).Queued; got != 2 {
		t.Fatalf("duplicate queueing changed size to %d", got)
	}
	if err := transitionAgentTask(&graph, "b", AgentTaskCancelled); err != nil {
		t.Fatal(err)
	}
	if got := scheduler.Snapshot(&graph, nil).Queued; got != 1 {
		t.Fatalf("non-ready queue entry was not pruned, queued=%d", got)
	}
}

func TestAgentSchedulerCancelTaskQueuedAndRunning(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "running", Role: "explorer", Objective: "running"},
		{ID: "queued", Role: "reviewer", Objective: "queued"},
	})
	grantSchedulerTestCapabilities(t, &graph, "running", "queued")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	lease, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok {
		t.Fatalf("admission ok=%v err=%v", ok, err)
	}
	if err := scheduler.CancelTask(&graph, "queued"); err != nil {
		t.Fatal(err)
	}
	if task := agentTaskByID(&graph, "queued"); task.State != AgentTaskCancelled {
		t.Fatalf("queued task state=%q", task.State)
	}
	if err := scheduler.CancelTask(&graph, lease.TaskID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Context.Done():
	case <-time.After(time.Second):
		t.Fatal("active task context not cancelled")
	}
	if task := agentTaskByID(&graph, lease.TaskID); task.State != AgentTaskCancelled {
		t.Fatalf("running task state=%q", task.State)
	}
	if snapshot := scheduler.Snapshot(&graph, nil); snapshot.Queued != 0 || snapshot.Running != 0 || snapshot.Resources[0].InUse != 0 {
		t.Fatalf("scheduler leaked resource after task cancellation: %+v", snapshot)
	}
	if err := scheduler.CancelTask(&graph, "missing"); err == nil {
		t.Fatal("missing task cancellation should fail")
	}
}

func TestAgentSchedulerRejectsStaleLeaseAndReleasesFailedTask(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "a"}})
	grantSchedulerTestCapabilities(t, &graph, "a")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	lease, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok {
		t.Fatalf("admission ok=%v err=%v", ok, err)
	}
	stale := lease
	stale.token++
	if err := scheduler.Release(&graph, stale, AgentTaskSucceeded); err == nil {
		t.Fatal("stale lease should not release resource")
	}
	if err := scheduler.Release(&graph, lease, AgentTaskFailed); err != nil {
		t.Fatal(err)
	}
	if task := agentTaskByID(&graph, "a"); task.State != AgentTaskFailed {
		t.Fatalf("released task state=%q", task.State)
	}
	if snapshot := scheduler.Snapshot(&graph, nil); snapshot.Running != 0 || snapshot.Resources[0].InUse != 0 {
		t.Fatalf("resource not released after failure: %+v", snapshot)
	}
	if err := scheduler.Release(&graph, lease, AgentTaskSucceeded); err == nil {
		t.Fatal("already released lease should fail")
	}
}

func TestAgentSchedulerParentCancellationStopsAdmission(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	scheduler := NewAgentScheduler(parent, AgentResourceLimits{})
	graph := schedulerTestGraph(t, []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "a"}})
	grantSchedulerTestCapabilities(t, &graph, "a")
	cancel()
	if err := scheduler.QueueReady(&graph, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("QueueReady error=%v want context.Canceled", err)
	}
	if _, ok, err := scheduler.AdmitNext(&graph); !errors.Is(err, context.Canceled) || ok {
		t.Fatalf("AdmitNext ok=%v err=%v want cancellation", ok, err)
	}
	if snapshot := scheduler.Snapshot(&graph, nil); !snapshot.Cancelled {
		t.Fatalf("snapshot should expose parent cancellation: %+v", snapshot)
	}
}

func TestAgentBudgetSnapshotAllExhaustionDimensions(t *testing.T) {
	budget := AgentBudget{ModelCalls: 2, ToolCalls: 3, EstimatedTokenBudget: 100, TimeSeconds: 5}
	cases := []struct {
		name string
		use  AgentUsage
		by   string
	}{
		{"model", AgentUsage{ModelCalls: 2}, "model_calls"},
		{"tool", AgentUsage{ToolCalls: 3}, "tool_calls"},
		{"tokens", AgentUsage{EstimatedTokens: 100}, "estimated_tokens"},
		{"time", AgentUsage{ElapsedMillis: 5000}, "time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := agentBudgetSnapshot(budget, tc.use)
			if !snapshot.Exhausted || snapshot.ExhaustedBy != tc.by {
				t.Fatalf("snapshot=%+v", snapshot)
			}
			result, stopped := agentBudgetHardStop(budget, tc.use)
			if !stopped || result.Status != AgentResultBudgetExhausted || result.Usage != tc.use {
				t.Fatalf("stopped=%v result=%+v", stopped, result)
			}
		})
	}

	over := agentBudgetSnapshot(budget, AgentUsage{ModelCalls: 10, ToolCalls: 10, EstimatedTokens: 1000, ElapsedMillis: 10000})
	if over.Remaining.ModelCalls != 0 || over.Remaining.ToolCalls != 0 || over.Remaining.EstimatedTokenBudget != 0 || over.Remaining.TimeSeconds != 0 {
		t.Fatalf("remaining budget must clamp at zero: %+v", over.Remaining)
	}
	if _, stopped := agentBudgetHardStop(budget, AgentUsage{}); stopped {
		t.Fatal("unused budget should not hard-stop")
	}
}

func TestAgentSchedulerSnapshotTracksActiveResourceAndUsage(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{ID: "a", Role: "planner", Objective: "a"}})
	grantSchedulerTestCapabilities(t, &graph, "a")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	lease, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok {
		t.Fatalf("admission ok=%v err=%v", ok, err)
	}
	usage := map[string]AgentUsage{"a": {ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 100, ElapsedMillis: 1000}}
	snapshot := scheduler.Snapshot(&graph, usage)
	if snapshot.Running != 1 || snapshot.Queued != 0 || snapshot.Resources[0].InUse != 1 || snapshot.Resources[0].Available != 0 {
		t.Fatalf("unexpected active scheduler snapshot: %+v", snapshot)
	}
	if !snapshot.Tasks[0].Running || snapshot.Tasks[0].ResourceClass != AgentResourceModelInference || snapshot.Tasks[0].Budget.Usage != usage["a"] {
		t.Fatalf("unexpected active task snapshot: %+v", snapshot.Tasks[0])
	}
	if err := scheduler.Release(&graph, lease, AgentTaskSucceeded); err != nil {
		t.Fatal(err)
	}
}

func TestAgentTaskAdmissionReasonRequiresRoleCapabilities(t *testing.T) {
	task := AgentTask{Role: AgentRoleReviewer}
	if reason := agentTaskAdmissionReason(task); reason == "" {
		t.Fatal("reviewer without grants should be blocked")
	}
	task.Capabilities = capabilitiesForAgentRole(AgentRoleReviewer)
	if reason := agentTaskAdmissionReason(task); reason != "" {
		t.Fatalf("fully granted reviewer reason=%q", reason)
	}
	if got := resourceClassForAgentTask(task); got != AgentResourceModelInference {
		t.Fatalf("default resource=%q", got)
	}
}
