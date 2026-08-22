// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"
)

func TestScheduledRecoveryUsageIsSeededAndAccumulated(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{
		ID: "resume-me", Role: "explorer", Objective: "Resume read-only work.",
	}})
	grantSchedulerTestCapabilities(t, &graph, "resume-me")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	seed := map[string]AgentUsage{
		"resume-me": {ModelCalls: 2, ToolCalls: 3, EstimatedTokens: 400, ElapsedMillis: 5000},
		"sibling":   {ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 100, ElapsedMillis: 1000},
	}
	executed := 0
	run, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded(
		t.TempDir(), Config{}, &graph, scheduler,
		func(context.Context, string, Config, AgentTask) (AgentResult, error) {
			executed++
			return AgentResult{Status: AgentResultCompleted, Usage: AgentUsage{ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 50, ElapsedMillis: 750}}, nil
		}, nil, seed,
	)
	if err != nil {
		t.Fatal(err)
	}
	if executed != 1 {
		t.Fatalf("executed=%d want=1", executed)
	}
	want := AgentUsage{ModelCalls: 3, ToolCalls: 5, EstimatedTokens: 450, ElapsedMillis: 5750}
	if got := run.UsageByTask["resume-me"]; got != want {
		t.Fatalf("cumulative usage=%+v want=%+v", got, want)
	}
	if got := run.UsageByTask["sibling"]; got != seed["sibling"] {
		t.Fatalf("untouched historical sibling usage=%+v want=%+v", got, seed["sibling"])
	}
	seed["resume-me"] = AgentUsage{}
	if got := run.UsageByTask["resume-me"]; got != want {
		t.Fatalf("run usage aliased caller seed: %+v", got)
	}
}

func TestScheduledRecoveryUsageRejectsInvalidSeedBeforeAdmission(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{{ID: "never-run", Role: "explorer", Objective: "Do not run."}})
	grantSchedulerTestCapabilities(t, &graph, "never-run")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{})
	defer scheduler.missionCancel()
	executed := false
	_, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded(
		t.TempDir(), Config{}, &graph, scheduler,
		func(context.Context, string, Config, AgentTask) (AgentResult, error) {
			executed = true
			return AgentResult{Status: AgentResultCompleted}, nil
		}, nil, map[string]AgentUsage{"never-run": {ModelCalls: -1}},
	)
	if err == nil {
		t.Fatal("expected invalid seeded usage error")
	}
	if executed {
		t.Fatal("executor ran before seeded usage validation")
	}
	if snapshot := scheduler.Snapshot(&graph, nil); snapshot.Running != 0 || snapshot.Queued != 0 {
		t.Fatalf("scheduler admitted work before seed validation: %+v", snapshot)
	}
}
