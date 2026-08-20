// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func validAgentTaskProposals() []AgentTaskProposal {
	return []AgentTaskProposal{
		{ID: "explore", Role: "kernel-explorer", Objective: "Map the kernel interfaces"},
		{ID: "build-memory", Role: "kernel-memory-specialist", Objective: "Implement the allocator", Dependencies: []string{"explore"}, Capabilities: []AgentCapability{AgentCapabilityRepositoryRead}},
		{ID: "review", Role: "reviewer", Objective: "Review the allocator evidence", Dependencies: []string{"build-memory"}},
	}
}

func TestAgentTaskProposalValidationRejectsInvalidIdentifiers(t *testing.T) {
	tests := []struct {
		name      string
		proposals []AgentTaskProposal
		contains  string
	}{
		{"empty id", []AgentTaskProposal{{ID: "", Role: "explorer", Objective: "x"}}, "task id is empty"},
		{"path id", []AgentTaskProposal{{ID: "../escape", Role: "explorer", Objective: "x"}}, "task id"},
		{"empty role", []AgentTaskProposal{{ID: "a", Role: "", Objective: "x"}}, "task role is empty"},
		{"path role", []AgentTaskProposal{{ID: "a", Role: "../../builder", Objective: "x"}}, "task role"},
		{"empty objective", []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "  "}}, "objective is empty"},
		{"invalid dependency", []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "x", Dependencies: []string{"../b"}}}, "dependency id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAgentTaskProposals(tc.proposals)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error=%v want containing %q", err, tc.contains)
			}
		})
	}
}

func TestAgentTaskProposalValidationRejectsDuplicateMissingSelfAndCycle(t *testing.T) {
	tests := []struct {
		name      string
		proposals []AgentTaskProposal
		contains  string
	}{
		{
			"duplicate id",
			[]AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "x"}, {ID: "a", Role: "reviewer", Objective: "y"}},
			"duplicate task id",
		},
		{
			"duplicate dependency",
			[]AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "x"}, {ID: "b", Role: "reviewer", Objective: "y", Dependencies: []string{"a", "a"}}},
			"duplicate dependency",
		},
		{
			"missing dependency",
			[]AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "x", Dependencies: []string{"missing"}}},
			"depends on missing task",
		},
		{
			"self dependency",
			[]AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "x", Dependencies: []string{"a"}}},
			"cannot depend on itself",
		},
		{
			"cycle",
			[]AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "x", Dependencies: []string{"b"}}, {ID: "b", Role: "reviewer", Objective: "y", Dependencies: []string{"a"}}},
			"dependency cycle",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAgentTaskProposals(tc.proposals)
			if err == nil || !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error=%v want containing %q", err, tc.contains)
			}
		})
	}
}

func TestBuildAgentTaskGraphKeepsDynamicRolesAndCapabilityRequestsInert(t *testing.T) {
	graph, err := buildAgentTaskGraph("mission-1", "supervisor", validAgentTaskProposals())
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Tasks) != 3 {
		t.Fatalf("tasks=%d want 3", len(graph.Tasks))
	}
	builder := graph.Tasks[1]
	if builder.Role != AgentRole("kernel-memory-specialist") {
		t.Fatalf("dynamic role=%q", builder.Role)
	}
	if len(builder.RequestedCapabilities) != 1 || builder.RequestedCapabilities[0] != AgentCapabilityRepositoryRead {
		t.Fatalf("requested capabilities=%#v", builder.RequestedCapabilities)
	}
	if len(builder.Capabilities) != 0 {
		t.Fatalf("planner proposal unexpectedly granted executable capabilities: %#v", builder.Capabilities)
	}
	if _, err := normalizeAgentRole(string(builder.Role)); err == nil {
		t.Fatal("dynamic planning role unexpectedly became executable by the read-only child runtime")
	}
	if builder.MissionID != "mission-1" || builder.ParentID != "supervisor" {
		t.Fatalf("unexpected mission/parent: %#v", builder)
	}
}

func TestBuildAgentTaskGraphComputesDeterministicReadiness(t *testing.T) {
	proposals := []AgentTaskProposal{
		{ID: "a", Role: "explorer", Objective: "A"},
		{ID: "b", Role: "planner-helper", Objective: "B"},
		{ID: "c", Role: "reviewer", Objective: "C", Dependencies: []string{"a"}},
		{ID: "d", Role: "reviewer", Objective: "D", Dependencies: []string{"b"}},
	}
	graph, err := buildAgentTaskGraph("mission-1", "", proposals)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := readyAgentTaskIDs(&graph)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(ready, ","), "a,b"; got != want {
		t.Fatalf("ready=%q want %q", got, want)
	}
	if graph.Tasks[2].State != AgentTaskBlocked || !strings.Contains(graph.Tasks[2].StateReason, "waiting for dependencies: a") {
		t.Fatalf("dependent task not blocked with reason: %#v", graph.Tasks[2])
	}
}

func TestAgentTaskTransitionSuccessReleasesDependent(t *testing.T) {
	graph, err := buildAgentTaskGraph("mission-1", "", []AgentTaskProposal{
		{ID: "build", Role: "builder", Objective: "Build"},
		{ID: "review", Role: "reviewer", Objective: "Review", Dependencies: []string{"build"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "build", AgentTaskRunning); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[0].State != AgentTaskRunning || graph.Tasks[1].State != AgentTaskBlocked {
		t.Fatalf("unexpected running states: %#v", graph.Tasks)
	}
	if err := transitionAgentTask(&graph, "build", AgentTaskSucceeded); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[0].State != AgentTaskSucceeded || graph.Tasks[1].State != AgentTaskReady {
		t.Fatalf("success did not release dependency: %#v", graph.Tasks)
	}
}

func TestLegacyCompletedStateSatisfiesDependency(t *testing.T) {
	graph, err := buildAgentTaskGraph("mission-1", "", []AgentTaskProposal{
		{ID: "legacy", Role: "explorer", Objective: "Legacy"},
		{ID: "next", Role: "reviewer", Objective: "Next", Dependencies: []string{"legacy"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "legacy", AgentTaskRunning); err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "legacy", AgentTaskCompleted); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[1].State != AgentTaskReady {
		t.Fatalf("legacy completed state did not satisfy dependency: %#v", graph.Tasks)
	}
}

func TestAgentTaskFailureBlocksDependentAndRetryReopens(t *testing.T) {
	graph, err := buildAgentTaskGraph("mission-1", "", []AgentTaskProposal{
		{ID: "build", Role: "builder", Objective: "Build"},
		{ID: "test", Role: "tester", Objective: "Test", Dependencies: []string{"build"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "build", AgentTaskRunning); err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "build", AgentTaskFailed); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[1].State != AgentTaskBlocked || !strings.Contains(graph.Tasks[1].StateReason, "build is failed") {
		t.Fatalf("failure not propagated: %#v", graph.Tasks)
	}
	if err := transitionAgentTask(&graph, "build", AgentTaskRetryable); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[0].State != AgentTaskReady {
		t.Fatalf("retryable root did not reconcile to ready: %#v", graph.Tasks[0])
	}
	if err := transitionAgentTask(&graph, "build", AgentTaskRunning); err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "build", AgentTaskSucceeded); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[1].State != AgentTaskReady {
		t.Fatalf("dependent not released after retry success: %#v", graph.Tasks)
	}
}

func TestAgentTaskCancelledDependencyBlocksDependent(t *testing.T) {
	graph, err := buildAgentTaskGraph("mission-1", "", []AgentTaskProposal{
		{ID: "a", Role: "explorer", Objective: "A"},
		{ID: "b", Role: "reviewer", Objective: "B", Dependencies: []string{"a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "a", AgentTaskCancelled); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[1].State != AgentTaskBlocked || !strings.Contains(graph.Tasks[1].StateReason, "a is cancelled") {
		t.Fatalf("cancel not propagated: %#v", graph.Tasks)
	}
}

func TestAgentTaskTransitionRejectsUnsafeOrTerminalMoves(t *testing.T) {
	graph, err := buildAgentTaskGraph("mission-1", "", []AgentTaskProposal{
		{ID: "a", Role: "explorer", Objective: "A"},
		{ID: "b", Role: "reviewer", Objective: "B", Dependencies: []string{"a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "b", AgentTaskRunning); err == nil || !strings.Contains(err.Error(), "cannot transition") {
		t.Fatalf("blocked->running unexpectedly allowed: %v", err)
	}
	if err := transitionAgentTask(&graph, "missing", AgentTaskRunning); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing task error=%v", err)
	}
	if err := transitionAgentTask(&graph, "a", AgentTaskRunning); err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "a", AgentTaskSucceeded); err != nil {
		t.Fatal(err)
	}
	if err := transitionAgentTask(&graph, "a", AgentTaskRunning); err == nil {
		t.Fatal("terminal succeeded task restarted unexpectedly")
	}
}

func TestAgentTaskGraphValidationRejectsMissionStateAndParentCorruption(t *testing.T) {
	graph, err := buildAgentTaskGraph("mission-1", "parent", []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	corrupt := graph
	corrupt.Tasks = append([]AgentTask(nil), graph.Tasks...)
	corrupt.Tasks[0].MissionID = "mission-other"
	if err := validateAgentTaskGraph(corrupt); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mission mismatch error=%v", err)
	}
	corrupt = graph
	corrupt.Tasks = append([]AgentTask(nil), graph.Tasks...)
	corrupt.Tasks[0].State = AgentTaskState("invented")
	if err := validateAgentTaskGraph(corrupt); err == nil || !strings.Contains(err.Error(), "unsupported state") {
		t.Fatalf("state error=%v", err)
	}
	corrupt = graph
	corrupt.Tasks = append([]AgentTask(nil), graph.Tasks...)
	corrupt.Tasks[0].ParentID = "a"
	if err := validateAgentTaskGraph(corrupt); err == nil || !strings.Contains(err.Error(), "cannot also be its parent") {
		t.Fatalf("parent error=%v", err)
	}
}

func TestBuildAgentTaskGraphRejectsInvalidMissionAndParent(t *testing.T) {
	proposals := []AgentTaskProposal{{ID: "a", Role: "explorer", Objective: "A"}}
	if _, err := buildAgentTaskGraph("../mission", "", proposals); err == nil {
		t.Fatal("invalid mission id accepted")
	}
	if _, err := buildAgentTaskGraph("mission", "../parent", proposals); err == nil {
		t.Fatal("invalid parent id accepted")
	}
	if _, err := buildAgentTaskGraph("mission", "a", proposals); err == nil || !strings.Contains(err.Error(), "cannot also be its parent") {
		t.Fatalf("task-as-parent error=%v", err)
	}
}
