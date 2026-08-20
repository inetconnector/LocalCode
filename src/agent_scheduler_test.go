// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func schedulerTestGraph(t *testing.T, proposals []AgentTaskProposal) AgentTaskGraph {
	t.Helper()
	graph, err := buildAgentTaskGraph("mission-scheduler-test", "", proposals)
	if err != nil {
		t.Fatal(err)
	}
	return graph
}

func grantSchedulerTestCapabilities(t *testing.T, graph *AgentTaskGraph, ids ...string) {
	t.Helper()
	for _, id := range ids {
		task := agentTaskByID(graph, id)
		if task == nil {
			t.Fatalf("task %q missing", id)
		}
		role, err := normalizeAgentRole(string(task.Role))
		if err != nil {
			t.Fatal(err)
		}
		task.Capabilities = capabilitiesForAgentRole(role)
		task.Budget = AgentBudget{ModelCalls: 4, ToolCalls: 8, EstimatedTokenBudget: 20000, TimeSeconds: 60}
	}
}

func TestAgentSchedulerOneModelSlotAndReadinessRelease(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "explore-a", Role: "explorer", Objective: "inspect A"},
		{ID: "review-b", Role: "reviewer", Objective: "review B"},
	})
	grantSchedulerTestCapabilities(t, &graph, "explore-a", "review-b")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()

	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	first, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok {
		t.Fatalf("first admission ok=%v err=%v", ok, err)
	}
	if first.TaskID != "explore-a" || first.ResourceClass != AgentResourceModelInference {
		t.Fatalf("unexpected first lease: %+v", first)
	}
	if task := agentTaskByID(&graph, "explore-a"); task == nil || task.State != AgentTaskRunning {
		t.Fatalf("first task not running: %+v", task)
	}
	if _, ok, err := scheduler.AdmitNext(&graph); err != nil || ok {
		t.Fatalf("second model task should wait for one-slot resource: ok=%v err=%v", ok, err)
	}

	if err := scheduler.Release(&graph, first, AgentTaskSucceeded); err != nil {
		t.Fatal(err)
	}
	second, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok || second.TaskID != "review-b" {
		t.Fatalf("second admission=%+v ok=%v err=%v", second, ok, err)
	}
}

func TestAgentSchedulerDifferentResourceCanBypassSaturatedClassWithoutStarvation(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "model-a", Role: "explorer", Objective: "model A"},
		{ID: "model-b", Role: "planner", Objective: "model B"},
		{ID: "read-c", Role: "explorer", Objective: "cheap read"},
	})
	grantSchedulerTestCapabilities(t, &graph, "model-a", "model-b", "read-c")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{ModelInference: 1, ReadCPU: 2})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, map[string]AgentResourceClass{"read-c": AgentResourceReadCPU}); err != nil {
		t.Fatal(err)
	}

	modelA, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok || modelA.TaskID != "model-a" {
		t.Fatalf("model A admission=%+v ok=%v err=%v", modelA, ok, err)
	}
	readC, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok || readC.TaskID != "read-c" || readC.ResourceClass != AgentResourceReadCPU {
		t.Fatalf("read admission=%+v ok=%v err=%v", readC, ok, err)
	}
	if err := scheduler.Release(&graph, modelA, AgentTaskSucceeded); err != nil {
		t.Fatal(err)
	}
	modelB, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok || modelB.TaskID != "model-b" {
		t.Fatalf("older waiting model task starved: lease=%+v ok=%v err=%v", modelB, ok, err)
	}
}

func TestAgentSchedulerQueuesOnlyReadyTasksAndReleasesDependencies(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "root", Role: "explorer", Objective: "root"},
		{ID: "dependent", Role: "reviewer", Objective: "dependent", Dependencies: []string{"root"}},
	})
	grantSchedulerTestCapabilities(t, &graph, "root", "dependent")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()

	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	if snapshot := scheduler.Snapshot(&graph, nil); snapshot.Queued != 1 {
		t.Fatalf("queued=%d want=1", snapshot.Queued)
	}
	rootLease, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok || rootLease.TaskID != "root" {
		t.Fatalf("root admission=%+v ok=%v err=%v", rootLease, ok, err)
	}
	if err := scheduler.Release(&graph, rootLease, AgentTaskSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	dependent, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok || dependent.TaskID != "dependent" {
		t.Fatalf("dependent admission=%+v ok=%v err=%v", dependent, ok, err)
	}
}

func TestAgentSchedulerDoesNotTurnRequestedCapabilitiesOrDynamicRolesIntoAuthority(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "planned", Role: "explorer", Objective: "planned", Capabilities: []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP}},
		{ID: "dynamic", Role: "kernel-memory-specialist", Objective: "dynamic role"},
	})
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := scheduler.AdmitNext(&graph); err != nil || ok {
		t.Fatalf("planner-requested capabilities must not self-grant: ok=%v err=%v", ok, err)
	}
	planned := agentTaskByID(&graph, "planned")
	if len(planned.Capabilities) != 0 || len(planned.RequestedCapabilities) != 2 {
		t.Fatalf("capability separation changed: granted=%v requested=%v", planned.Capabilities, planned.RequestedCapabilities)
	}
	snapshot := scheduler.Snapshot(&graph, nil)
	if len(snapshot.Tasks) != 2 {
		t.Fatalf("task snapshots=%d", len(snapshot.Tasks))
	}
	if !strings.Contains(snapshot.Tasks[0].AdmissionBlockedReason, "missing granted capability") {
		t.Fatalf("planned admission reason=%q", snapshot.Tasks[0].AdmissionBlockedReason)
	}
	if !strings.Contains(snapshot.Tasks[1].AdmissionBlockedReason, "planning data") {
		t.Fatalf("dynamic admission reason=%q", snapshot.Tasks[1].AdmissionBlockedReason)
	}
}

func TestAgentSchedulerMissionCancellationCancelsQueuedAndRunningTasks(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "running", Role: "explorer", Objective: "running"},
		{ID: "queued", Role: "reviewer", Objective: "queued"},
	})
	grantSchedulerTestCapabilities(t, &graph, "running", "queued")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	if err := scheduler.QueueReady(&graph, nil); err != nil {
		t.Fatal(err)
	}
	lease, ok, err := scheduler.AdmitNext(&graph)
	if err != nil || !ok {
		t.Fatalf("admission ok=%v err=%v", ok, err)
	}
	if err := scheduler.CancelMission(&graph); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lease.Context.Done():
	case <-time.After(time.Second):
		t.Fatal("running lease context was not cancelled")
	}
	for _, id := range []string{"running", "queued"} {
		if task := agentTaskByID(&graph, id); task == nil || task.State != AgentTaskCancelled {
			t.Fatalf("task %s state=%v", id, task)
		}
	}
	snapshot := scheduler.Snapshot(&graph, nil)
	if !snapshot.Cancelled || snapshot.Queued != 0 || snapshot.Running != 0 {
		t.Fatalf("unexpected cancelled snapshot: %+v", snapshot)
	}
	if snapshot.Resources[0].InUse != 0 {
		t.Fatalf("model resource leaked: %+v", snapshot.Resources[0])
	}
}

func TestAgentBudgetSnapshotAndHardStop(t *testing.T) {
	budget := AgentBudget{ModelCalls: 2, ToolCalls: 4, EstimatedTokenBudget: 1000, TimeSeconds: 10}
	usage := AgentUsage{ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 250, ElapsedMillis: 2500}
	snapshot := agentBudgetSnapshot(budget, usage)
	if snapshot.Exhausted || snapshot.Remaining.ModelCalls != 1 || snapshot.Remaining.ToolCalls != 2 || snapshot.Remaining.EstimatedTokenBudget != 750 || snapshot.Remaining.TimeSeconds != 8 {
		t.Fatalf("unexpected budget snapshot: %+v", snapshot)
	}
	usage.ModelCalls = 2
	result, stopped := agentBudgetHardStop(budget, usage)
	if !stopped || result.Status != AgentResultBudgetExhausted || !strings.Contains(result.Summary, "model_calls") {
		t.Fatalf("hard stop=%v result=%+v", stopped, result)
	}
}

func TestAgentSchedulerQueueBoundIsAtomic(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "a", Role: "explorer", Objective: "a"},
		{ID: "b", Role: "reviewer", Objective: "b"},
	})
	grantSchedulerTestCapabilities(t, &graph, "a", "b")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{MaxQueued: 1})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, nil); err == nil {
		t.Fatal("expected queue bound error")
	}
	if snapshot := scheduler.Snapshot(&graph, nil); snapshot.Queued != 0 {
		t.Fatalf("queue mutation should be atomic, queued=%d", snapshot.Queued)
	}
}

func TestAgentSchedulerRejectsUnsupportedResourceClass(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "a"}})
	grantSchedulerTestCapabilities(t, &graph, "a")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	if err := scheduler.QueueReady(&graph, map[string]AgentResourceClass{"a": "gpu-magic"}); err == nil {
		t.Fatal("expected unsupported resource class error")
	}
}
