// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strings"
	"testing"
)

func TestMissionAccountingAggregatesAcceptedUsageOnceAndSeparatesWallTime(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	calls := 0
	mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(_ context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		calls++
		usage := AgentUsage{ModelCalls: 1, ToolCalls: 2, EstimatedTokens: 100, ElapsedMillis: 1_000_000}
		if task.ID == "review" {
			usage = AgentUsage{ModelCalls: 2, ToolCalls: 3, EstimatedTokens: 200, ElapsedMillis: 2_000_000}
		}
		return AgentResult{Status: AgentResultCompleted, Summary: "done", Usage: usage}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("executor calls=%d want=2", calls)
	}
	if mission.State != AgentMissionSucceeded || mission.Reason != AgentMissionReasonCompleted {
		t.Fatalf("unexpected mission outcome: state=%q reason=%q", mission.State, mission.Reason)
	}
	usage := mission.Accounting.Usage
	if usage.ModelCalls != 3 || usage.ToolCalls != 5 || usage.EstimatedTokens != 300 {
		t.Fatalf("aggregate usage=%+v", usage)
	}
	if mission.Accounting.ChildWorkMillis != 3_000_000 {
		t.Fatalf("child work millis=%d want=3000000", mission.Accounting.ChildWorkMillis)
	}
	if usage.ElapsedMillis >= mission.Accounting.ChildWorkMillis {
		t.Fatalf("wall time was incorrectly summed from child elapsed times: usage=%+v child_work=%d", usage, mission.Accounting.ChildWorkMillis)
	}
}

func TestMissionBudgetStopsBeforeNextTaskWithoutExecutingIt(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Budget = AgentBudget{ModelCalls: 1}
	calls := 0
	mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(_ context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		calls++
		if task.ID != "explore" {
			t.Fatalf("budget allowed unexpected task %q to reach executor", task.ID)
		}
		if task.Budget.ModelCalls != 1 {
			t.Fatalf("mission limit did not tighten child budget: %+v", task.Budget)
		}
		return AgentResult{Status: AgentResultCompleted, Summary: "done", Usage: AgentUsage{ModelCalls: 1}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("executor calls=%d want=1", calls)
	}
	if mission.State != AgentMissionBudgetExhausted || mission.Reason != AgentMissionReasonBudgetExhausted || mission.BudgetExhaustedBy != "model_calls" {
		t.Fatalf("unexpected mission budget outcome: state=%q reason=%q by=%q", mission.State, mission.Reason, mission.BudgetExhaustedBy)
	}
	if !mission.Accounting.Budget.Exhausted || mission.Accounting.Budget.ExhaustedBy != "model_calls" {
		t.Fatalf("mission budget snapshot=%+v", mission.Accounting.Budget)
	}
	if mission.Accounting.Usage.ModelCalls != 1 {
		t.Fatalf("mission usage=%+v", mission.Accounting.Usage)
	}
	review := agentTaskByID(&mission.Graph, "review")
	if review == nil || review.State != AgentTaskFailed || review.Result.Status != AgentResultBudgetExhausted {
		t.Fatalf("budget-blocked task=%+v", review)
	}
}

func TestMissionCompletingExactlyAtLimitStillSucceeds(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Tasks = req.Tasks[:1]
	req.Budget = AgentBudget{ModelCalls: 1}
	mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(_ context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		if task.Budget.ModelCalls != 1 {
			t.Fatalf("child model budget=%d want=1", task.Budget.ModelCalls)
		}
		return AgentResult{Status: AgentResultCompleted, Summary: "finished on final allowed call", Usage: AgentUsage{ModelCalls: 1}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if mission.State != AgentMissionSucceeded || mission.Reason != AgentMissionReasonCompleted || mission.BudgetExhaustedBy != "" {
		t.Fatalf("exact-limit completion was misclassified: state=%q reason=%q by=%q", mission.State, mission.Reason, mission.BudgetExhaustedBy)
	}
	if !mission.Accounting.Budget.Exhausted || mission.Accounting.Budget.ExhaustedBy != "model_calls" {
		t.Fatalf("budget snapshot should still report no remaining model calls: %+v", mission.Accounting.Budget)
	}
}

func TestMissionBudgetClassifiesConstrainedChildBudgetStopAsMissionExhaustion(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Tasks = req.Tasks[:1]
	req.Budget = AgentBudget{ModelCalls: 2}
	mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(_ context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		if task.Budget.ModelCalls != 2 {
			t.Fatalf("mission budget did not constrain child: %+v", task.Budget)
		}
		return AgentResult{Status: AgentResultBudgetExhausted, Summary: "model limit", Usage: AgentUsage{ModelCalls: 2}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if mission.State != AgentMissionBudgetExhausted || mission.Reason != AgentMissionReasonBudgetExhausted || mission.BudgetExhaustedBy != "model_calls" {
		t.Fatalf("constrained child stop was not classified as mission exhaustion: %+v", mission)
	}
}

func TestChildBudgetExhaustionWithoutMissionLimitRemainsTaskFailure(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Tasks = req.Tasks[:1]
	mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(_ context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		return AgentResult{Status: AgentResultBudgetExhausted, Summary: "child limit", Usage: AgentUsage{ModelCalls: task.Budget.ModelCalls}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if mission.State != AgentMissionFailed || mission.Reason != AgentMissionReasonTaskBudgetExhausted || mission.BudgetExhaustedBy != "" {
		t.Fatalf("child-only exhaustion was misclassified: state=%q reason=%q by=%q", mission.State, mission.Reason, mission.BudgetExhaustedBy)
	}
	if mission.Accounting.Budget.Exhausted {
		t.Fatalf("unlimited mission budget must not be exhausted: %+v", mission.Accounting.Budget)
	}
}

func TestMissionBudgetRejectsNegativeLimitsBeforeExecution(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	cases := []AgentBudget{
		{ModelCalls: -1},
		{ToolCalls: -1},
		{EstimatedTokenBudget: -1},
		{TimeSeconds: -1},
	}
	for _, budget := range cases {
		req := validReadOnlyMissionRequest(project)
		req.Budget = budget
		calls := 0
		_, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(context.Context, string, Config, AgentTask) (AgentResult, error) {
			calls++
			return AgentResult{Status: AgentResultCompleted}, nil
		})
		if err == nil || !strings.Contains(err.Error(), "cannot be negative") {
			t.Fatalf("budget=%+v error=%v", budget, err)
		}
		if calls != 0 {
			t.Fatalf("invalid mission budget executed %d child calls", calls)
		}
	}
}
