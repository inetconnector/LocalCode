// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type AgentMissionState string

const (
	AgentMissionSucceeded       AgentMissionState = "succeeded"
	AgentMissionFailed          AgentMissionState = "failed"
	AgentMissionCancelled       AgentMissionState = "cancelled"
	AgentMissionBudgetExhausted AgentMissionState = "budget_exhausted"
)

type AgentMissionReason string

const (
	AgentMissionReasonCompleted           AgentMissionReason = "completed"
	AgentMissionReasonCancelled           AgentMissionReason = "cancelled"
	AgentMissionReasonRuntimeError        AgentMissionReason = "runtime_error"
	AgentMissionReasonTaskFailed          AgentMissionReason = "task_failed"
	AgentMissionReasonTaskBudgetExhausted AgentMissionReason = "task_budget_exhausted"
	AgentMissionReasonBudgetExhausted     AgentMissionReason = "mission_budget_exhausted"
	AgentMissionReasonIncomplete          AgentMissionReason = "incomplete"
)

type AgentMissionAccounting struct {
	Usage           AgentUsage          `json:"usage"`
	ChildWorkMillis int64               `json:"child_work_millis"`
	Budget          AgentBudgetSnapshot `json:"budget"`
}

type agentMissionBudgetTracker struct {
	mu sync.Mutex

	limit     AgentBudget
	started   time.Time
	usage     AgentUsage
	blockedBy string
}

func validateAgentMissionBudget(budget AgentBudget) error {
	if budget.ModelCalls < 0 {
		return fmt.Errorf("mission model-call budget cannot be negative")
	}
	if budget.ToolCalls < 0 {
		return fmt.Errorf("mission tool-call budget cannot be negative")
	}
	if budget.EstimatedTokenBudget < 0 {
		return fmt.Errorf("mission estimated-token budget cannot be negative")
	}
	if budget.TimeSeconds < 0 {
		return fmt.Errorf("mission time budget cannot be negative")
	}
	return nil
}

func newAgentMissionBudgetTracker(limit AgentBudget, started time.Time) *agentMissionBudgetTracker {
	if started.IsZero() {
		started = time.Now()
	}
	return &agentMissionBudgetTracker{limit: limit, started: started}
}

// prepareTask applies the remaining mission budget only as a further
// restriction to the already-normalized detached child task. It can never
// increase a child budget or grant a capability.
func (t *agentMissionBudgetTracker) prepareTask(task AgentTask) (AgentTask, bool) {
	if t == nil {
		return task, true
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := t.snapshotLocked(time.Now())
	if snapshot.Exhausted {
		t.blockedBy = snapshot.ExhaustedBy
		return task, false
	}
	task.Budget = capAgentBudgetToMissionRemaining(task.Budget, t.limit, snapshot.Remaining)
	return task, true
}

func (t *agentMissionBudgetTracker) recordAppliedUsage(usage AgentUsage, childResult AgentResultStatus) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.usage.ModelCalls += usage.ModelCalls
	t.usage.ToolCalls += usage.ToolCalls
	t.usage.EstimatedTokens += usage.EstimatedTokens

	// A child that only hits its own normal budget remains a task-level failure.
	// Mark mission exhaustion only when the aggregate mission limit is now
	// exhausted and the child itself stopped on a budget boundary.
	if childResult == AgentResultBudgetExhausted {
		snapshot := t.snapshotLocked(time.Now())
		if snapshot.Exhausted {
			t.blockedBy = snapshot.ExhaustedBy
		}
	}
}

func (t *agentMissionBudgetTracker) blockedDimension() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.blockedBy
}

func (t *agentMissionBudgetTracker) snapshotLocked(now time.Time) AgentBudgetSnapshot {
	usage := t.usage
	usage.ElapsedMillis = now.Sub(t.started).Milliseconds()
	if usage.ElapsedMillis < 0 {
		usage.ElapsedMillis = 0
	}
	return agentBudgetSnapshot(t.limit, usage)
}

func capAgentBudgetToMissionRemaining(child, missionLimit, remaining AgentBudget) AgentBudget {
	if missionLimit.ModelCalls > 0 && remaining.ModelCalls < child.ModelCalls {
		child.ModelCalls = remaining.ModelCalls
	}
	if missionLimit.ToolCalls > 0 && remaining.ToolCalls < child.ToolCalls {
		child.ToolCalls = remaining.ToolCalls
	}
	if missionLimit.EstimatedTokenBudget > 0 && remaining.EstimatedTokenBudget < child.EstimatedTokenBudget {
		child.EstimatedTokenBudget = remaining.EstimatedTokenBudget
	}
	if missionLimit.TimeSeconds > 0 && remaining.TimeSeconds < child.TimeSeconds {
		child.TimeSeconds = remaining.TimeSeconds
	}
	return child
}

func agentMissionAccounting(limit AgentBudget, usageByTask map[string]AgentUsage, started, finished time.Time) AgentMissionAccounting {
	usage := AgentUsage{}
	var childWorkMillis int64
	for _, childUsage := range usageByTask {
		usage.ModelCalls += childUsage.ModelCalls
		usage.ToolCalls += childUsage.ToolCalls
		usage.EstimatedTokens += childUsage.EstimatedTokens
		childWorkMillis += childUsage.ElapsedMillis
	}
	usage.ElapsedMillis = finished.Sub(started).Milliseconds()
	if usage.ElapsedMillis < 0 {
		usage.ElapsedMillis = 0
	}
	return AgentMissionAccounting{
		Usage:           usage,
		ChildWorkMillis: childWorkMillis,
		Budget:          agentBudgetSnapshot(limit, usage),
	}
}

func deriveAgentMissionOutcome(graph AgentTaskGraph, runErr error, blockedBy string) (AgentMissionState, AgentMissionReason, string) {
	if blockedBy != "" {
		return AgentMissionBudgetExhausted, AgentMissionReasonBudgetExhausted, blockedBy
	}
	if runErr != nil {
		if runErr == context.Canceled || runErr == context.DeadlineExceeded {
			return AgentMissionCancelled, AgentMissionReasonCancelled, ""
		}
		return AgentMissionFailed, AgentMissionReasonRuntimeError, ""
	}

	allSucceeded := len(graph.Tasks) > 0
	for _, task := range graph.Tasks {
		if isSuccessfulAgentTaskState(task.State) {
			continue
		}
		allSucceeded = false
		if task.State == AgentTaskCancelled {
			return AgentMissionCancelled, AgentMissionReasonCancelled, ""
		}
		if task.Result.Status == AgentResultBudgetExhausted {
			return AgentMissionFailed, AgentMissionReasonTaskBudgetExhausted, ""
		}
		if task.State == AgentTaskFailed || task.State == AgentTaskBlocked {
			return AgentMissionFailed, AgentMissionReasonTaskFailed, ""
		}
	}
	if allSucceeded {
		return AgentMissionSucceeded, AgentMissionReasonCompleted, ""
	}
	return AgentMissionFailed, AgentMissionReasonIncomplete, ""
}
