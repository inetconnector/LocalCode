// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"
)

func TestMissionReplanningSuccessAndGraphLinkage(t *testing.T) {
	replanner := NewMissionReplanner(nil)
	cfg := Config{}

	initialGraph := &AgentTaskGraph{
		MissionID: "mission-replan-test",
		Tasks: []AgentTask{
			{
				ID:        "task-1-explorer",
				Role:      AgentRoleExplorer,
				Objective: "Explore architecture",
				State:     AgentTaskSucceeded,
			},
			{
				ID:           "task-2-builder",
				Role:         AgentRoleBuilder,
				Objective:    "Implement feature",
				Dependencies: []string{"task-1-explorer"},
				State:        AgentTaskRunning,
			},
			{
				ID:           "task-3-final-integrator",
				Role:         AgentRoleIntegrator,
				Objective:    "Integrate into main",
				Dependencies: []string{"task-2-builder"},
				State:        AgentTaskProposed,
			},
		},
	}

	repair := &RepairProposal{
		Summary:      "Fix syntax error in handler",
		FailingTests: []string{"TestHandler"},
	}

	newGraph, record, err := replanner.ReplanFailedTask(
		context.Background(),
		initialGraph,
		"task-2-builder",
		"Syntax error on line 42",
		repair,
		cfg,
	)
	if err != nil {
		t.Fatalf("ReplanFailedTask failed: %v", err)
	}

	if record == nil || record.CycleNumber != 1 {
		t.Fatalf("expected cycle number 1, got %+v", record)
	}

	// Verify task count: 3 original + 3 repair = 6 tasks
	if len(newGraph.Tasks) != 6 {
		t.Fatalf("expected 6 tasks in new graph, got %d", len(newGraph.Tasks))
	}

	// Verify that original failed task is marked failed
	if newGraph.Tasks[1].State != AgentTaskFailed {
		t.Fatalf("original failed task state=%q, want failed", newGraph.Tasks[1].State)
	}

	// Verify downstream task dependency was rerouted from task-2-builder to the reviewer
	var downstreamTask *AgentTask
	for i := range newGraph.Tasks {
		if newGraph.Tasks[i].ID == "task-3-final-integrator" {
			downstreamTask = &newGraph.Tasks[i]
			break
		}
	}
	if downstreamTask == nil {
		t.Fatal("task-3-final-integrator missing")
	}
	if len(downstreamTask.Dependencies) != 1 || downstreamTask.Dependencies[0] != record.GeneratedTasks[2] {
		t.Fatalf("downstream task dependency not rerouted to reviewer: %+v", downstreamTask.Dependencies)
	}
}

func TestMissionReplanningMaxAttemptsExceeded(t *testing.T) {
	replanner := NewMissionReplanner(nil)
	cfg := Config{}

	initialGraph := &AgentTaskGraph{
		MissionID: "mission-max-attempts",
		Tasks: []AgentTask{
			{
				ID:        "task-faulty",
				Role:      AgentRoleBuilder,
				Objective: "Faulty task",
				State:     AgentTaskRunning,
			},
		},
	}

	// 1st cycle
	g1, _, err := replanner.ReplanFailedTask(context.Background(), initialGraph, "task-faulty", "Error 1", &RepairProposal{Summary: "Fix 1"}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// 2nd cycle (different error)
	g2, _, err := replanner.ReplanFailedTask(context.Background(), g1, "task-faulty", "Error 2", &RepairProposal{Summary: "Fix 2"}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// 3rd cycle (different error)
	g3, _, err := replanner.ReplanFailedTask(context.Background(), g2, "task-faulty", "Error 3", &RepairProposal{Summary: "Fix 3"}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// 4th cycle must fail with max attempts error
	_, _, err = replanner.ReplanFailedTask(context.Background(), g3, "task-faulty", "Error 4", &RepairProposal{Summary: "Fix 4"}, cfg)
	if !errors.Is(err, errReplanningMaxAttemptsExceeded) {
		t.Fatalf("expected errReplanningMaxAttemptsExceeded, got: %v", err)
	}
}

func TestMissionReplanningStagnationDetection(t *testing.T) {
	replanner := NewMissionReplanner(nil)
	cfg := Config{}

	initialGraph := &AgentTaskGraph{
		MissionID: "mission-stagnation",
		Tasks: []AgentTask{
			{
				ID:        "task-stuck",
				Role:      AgentRoleBuilder,
				Objective: "Stuck task",
				State:     AgentTaskRunning,
			},
		},
	}

	repair := &RepairProposal{
		Summary:      "Same bug",
		FailingTests: []string{"TestBug"},
	}

	// 1st identical error
	g1, _, err := replanner.ReplanFailedTask(context.Background(), initialGraph, "task-stuck", "Same bug message", repair, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// 2nd identical error
	g2, _, err := replanner.ReplanFailedTask(context.Background(), g1, "task-stuck", "Same bug message", repair, cfg)
	if err != nil {
		t.Fatal(err)
	}

	// 3rd identical error must trigger stagnation halt
	_, _, err = replanner.ReplanFailedTask(context.Background(), g2, "task-stuck", "Same bug message", repair, cfg)
	if !errors.Is(err, errReplanningStagnationDetected) {
		t.Fatalf("expected errReplanningStagnationDetected on 3rd identical failure, got: %v", err)
	}
}
