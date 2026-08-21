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

	limit       AgentBudget
	started     time.Time
	usage       AgentUsage
	blockedBy   string
	constrained map[string]map[string]struct{}
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
	return &agentMissionBudgetTracker{
		limit:       limit,
		started:     started,
		constrained: map[string]map[string]struct{}{},
	}
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
	capped, dimensions := capAgentBudgetToMissionRemaining(task.Budget, t.limit, snapshot.Remaining)
	if len(dimensions) > 0 {
		set := make(map[string]struct{}, len(dimensions))
		for _, dimension := range dimensions {
			set[dimension] = struct{}{}
		}
		t.constrained[task.ID] = set
	}
	task.Budget = capped
	return task, true
}

// recordObservedUsage exists only so the next admission can be constrained by
// work just performed. Final accounting never trusts this speculative tracker:
// it is recomputed from scheduler-accepted UsageByTask so a late cancelled
// child cannot double-count or change the terminal result.
func (t *agentMissionBudgetTracker) recordObservedUsage(usage AgentUsage) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.usage.ModelCalls += usage.ModelCalls
	t.usage.ToolCalls += usage.ToolCalls
	t.usage.EstimatedTokens += usage.EstimatedTokens
}

func (t *agentMissionBudgetTracker) blockedDimension() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.blockedBy
}

func (t *agentMissionBudgetTracker) constrainedTaskDimension(taskID, dimension string) bool {
	if t == nil || dimension == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.constrained[taskID][dimension]
	return ok
}

func (t *agentMissionBudgetTracker) snapshotLocked(now time.Time) AgentBudgetSnapshot {
	usage := t.usage
	usage.ElapsedMillis = now.Sub(t.started).Milliseconds()
	if usage.ElapsedMillis < 0 {
		usage.ElapsedMillis = 0
	}
	return agentBudgetSnapshot(t.limit, usage)
}

func capAgentBudgetToMissionRemaining(child, missionLimit, remaining AgentBudget) (AgentBudget, []string) {
	dimensions := make([]string, 0, 4)
	if missionLimit.ModelCalls > 0 && remaining.ModelCalls < child.ModelCalls {
		child.ModelCalls = remaining.ModelCalls
		dimensions = append(dimensions, "model_calls")
	}
	if missionLimit.ToolCalls > 0 && remaining.ToolCalls < child.ToolCalls {
		child.ToolCalls = remaining.ToolCalls
		dimensions = append(dimensions, "tool_calls")
	}
	if missionLimit.EstimatedTokenBudget > 0 && remaining.EstimatedTokenBudget < child.EstimatedTokenBudget {
		child.EstimatedTokenBudget = remaining.EstimatedTokenBudget
		dimensions = append(dimensions, "estimated_tokens")
	}
	if missionLimit.TimeSeconds > 0 && remaining.TimeSeconds < child.TimeSeconds {
		child.TimeSeconds = remaining.TimeSeconds
		dimensions = append(dimensions, "time")
	}
	return child, dimensions
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

func missionBudgetStoppedAcceptedTask(run AgentScheduledRun, tracker *agentMissionBudgetTracker, exhaustedBy string) bool {
	if tracker == nil || exhaustedBy == "" {
		return false
	}
	for _, scheduled := range run.Results {
		if scheduled.Result.Status != AgentResultBudgetExhausted {
			continue
		}
		if tracker.constrainedTaskDimension(scheduled.TaskID, exhaustedBy) {
			return true
		}
	}
	return false
}

func deriveAgentMissionOutcome(graph AgentTaskGraph, run AgentScheduledRun, runErr error, accounting AgentMissionAccounting, tracker *agentMissionBudgetTracker) (AgentMissionState, AgentMissionReason, string) {
	if blockedBy := tracker.blockedDimension(); blockedBy != "" {
		return AgentMissionBudgetExhausted, AgentMissionReasonBudgetExhausted, blockedBy
	}
	if accounting.Budget.Exhausted && missionBudgetStoppedAcceptedTask(run, tracker, accounting.Budget.ExhaustedBy) {
		return AgentMissionBudgetExhausted, AgentMissionReasonBudgetExhausted, accounting.Budget.ExhaustedBy
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
