// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"strings"
)

type AgentScheduledTaskResult struct {
	TaskID string      `json:"task_id"`
	Role   AgentRole   `json:"role"`
	Result AgentResult `json:"result"`
}

type AgentScheduledRun struct {
	MissionID   string                     `json:"mission_id"`
	Results     []AgentScheduledTaskResult `json:"results"`
	UsageByTask map[string]AgentUsage      `json:"usage_by_task"`
	Snapshot    AgentSchedulerSnapshot     `json:"snapshot"`
}

type scheduledReadOnlyAgentExecutor func(context.Context, string, Config, AgentTask) (AgentResult, error)
type scheduledReadOnlyAgentCheckpoint func(AgentSchedulerSnapshot)

func scheduledAgentTaskTerminalState(result AgentResult, runErr error) AgentTaskState {
	if runErr != nil {
		return AgentTaskFailed
	}
	switch result.Status {
	case AgentResultCompleted, AgentResultFallback:
		return AgentTaskSucceeded
	case AgentResultBlocked, AgentResultBudgetExhausted:
		return AgentTaskFailed
	default:
		return AgentTaskFailed
	}
}

func cloneAgentUsageByTask(seed map[string]AgentUsage) (map[string]AgentUsage, error) {
	out := make(map[string]AgentUsage, len(seed))
	for taskID, usage := range seed {
		if strings.TrimSpace(taskID) == "" || !missionRecoveryUsageValid(usage) {
			return nil, fmt.Errorf("invalid seeded scheduler usage for task %q", taskID)
		}
		out[taskID] = usage
	}
	return out, nil
}

func addAgentUsage(base, delta AgentUsage) (AgentUsage, error) {
	if !missionRecoveryUsageValid(base) || !missionRecoveryUsageValid(delta) {
		return AgentUsage{}, fmt.Errorf("agent usage must be non-negative")
	}
	out := AgentUsage{
		ModelCalls:      base.ModelCalls + delta.ModelCalls,
		ToolCalls:       base.ToolCalls + delta.ToolCalls,
		EstimatedTokens: base.EstimatedTokens + delta.EstimatedTokens,
		ElapsedMillis:   base.ElapsedMillis + delta.ElapsedMillis,
	}
	if out.ModelCalls < base.ModelCalls || out.ToolCalls < base.ToolCalls || out.EstimatedTokens < base.EstimatedTokens || out.ElapsedMillis < base.ElapsedMillis {
		return AgentUsage{}, fmt.Errorf("agent usage overflow")
	}
	return out, nil
}

// runScheduledReadOnlyAgentGraph connects the Scheduler/Resource Manager to the
// existing isolated Native read-only child runtime. It intentionally does not
// grant capabilities: only tasks already authorized by the trusted parent can
// be admitted by AgentScheduler.
func (s *AppState) runScheduledReadOnlyAgentGraph(project string, cfg Config, graph *AgentTaskGraph, scheduler *AgentScheduler) (AgentScheduledRun, error) {
	if s == nil {
		return AgentScheduledRun{UsageByTask: map[string]AgentUsage{}}, fmt.Errorf("app state is nil")
	}
	return s.runScheduledReadOnlyAgentGraphWithExecutor(project, cfg, graph, scheduler, s.runNativeReadOnlyAgentTask)
}

// runScheduledReadOnlyAgentGraphWithExecutor keeps the graph/scheduler boundary
// deterministic while allowing focused race tests to replace only the isolated
// child runtime. Task preparation and finalization are serialized by the
// scheduler; the executor receives a detached task value and never mutates the
// shared graph directly.
func (s *AppState) runScheduledReadOnlyAgentGraphWithExecutor(project string, cfg Config, graph *AgentTaskGraph, scheduler *AgentScheduler, execute scheduledReadOnlyAgentExecutor) (AgentScheduledRun, error) {
	return s.runScheduledReadOnlyAgentGraphWithExecutorAndCheckpoint(project, cfg, graph, scheduler, execute, nil)
}

// runScheduledReadOnlyAgentGraphWithExecutorAndCheckpoint publishes scheduler
// snapshots only after scheduler-owned state transitions. The callback is
// observation-only: it cannot affect admission, capabilities or task state.
func (s *AppState) runScheduledReadOnlyAgentGraphWithExecutorAndCheckpoint(project string, cfg Config, graph *AgentTaskGraph, scheduler *AgentScheduler, execute scheduledReadOnlyAgentExecutor, checkpoint scheduledReadOnlyAgentCheckpoint) (AgentScheduledRun, error) {
	return s.runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded(project, cfg, graph, scheduler, execute, checkpoint, nil)
}

// runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded is the recovery
// accounting boundary. Historical durable usage is copied into the run before
// admission and every newly observed task usage is accumulated onto that seed;
// a continuation therefore cannot reset a task budget by starting a new
// scheduler invocation.
func (s *AppState) runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded(project string, cfg Config, graph *AgentTaskGraph, scheduler *AgentScheduler, execute scheduledReadOnlyAgentExecutor, checkpoint scheduledReadOnlyAgentCheckpoint, seed map[string]AgentUsage) (AgentScheduledRun, error) {
	usageByTask, err := cloneAgentUsageByTask(seed)
	if err != nil {
		return AgentScheduledRun{UsageByTask: map[string]AgentUsage{}}, err
	}
	run := AgentScheduledRun{UsageByTask: usageByTask}
	if s == nil {
		return run, fmt.Errorf("app state is nil")
	}
	if graph == nil {
		return run, fmt.Errorf("task graph is nil")
	}
	if scheduler == nil {
		return run, fmt.Errorf("agent scheduler is nil")
	}
	if execute == nil {
		return run, fmt.Errorf("scheduled read-only executor is nil")
	}
	if err := validateAgentTaskGraph(*graph); err != nil {
		return run, err
	}
	run.MissionID = graph.MissionID
	publishCheckpoint := func() AgentSchedulerSnapshot {
		snapshot := scheduler.Snapshot(graph, run.UsageByTask)
		run.Snapshot = snapshot
		if checkpoint != nil {
			checkpoint(snapshot)
		}
		return snapshot
	}
	if snapshot := publishCheckpoint(); snapshot.Running != 0 {
		return run, fmt.Errorf("agent scheduler already has %d active task(s)", snapshot.Running)
	}

	for {
		if err := scheduler.QueueReady(graph, nil); err != nil {
			publishCheckpoint()
			return run, err
		}
		publishCheckpoint()
		lease, admitted, err := scheduler.AdmitNext(graph)
		if err != nil {
			publishCheckpoint()
			if err == context.Canceled {
				return run, context.Canceled
			}
			return run, err
		}
		if !admitted {
			publishCheckpoint()
			return run, nil
		}

		executionTask, err := scheduler.prepareScheduledAgentTask(graph, lease, cfg)
		if err != nil {
			publishCheckpoint()
			if lease.Context.Err() != nil {
				return run, context.Canceled
			}
			return run, err
		}
		publishCheckpoint()

		result, runErr := execute(lease.Context, project, cfg, executionTask)
		if runErr != nil {
			result = AgentResult{
				Status:  AgentResultBlocked,
				Summary: "Scheduled read-only child execution failed: " + strings.TrimSpace(runErr.Error()),
				Usage:   result.Usage,
			}
		}
		next := scheduledAgentTaskTerminalState(result, runErr)
		if lease.Context.Err() != nil {
			next = AgentTaskCancelled
		}

		finalized, finalizeErr := scheduler.finalizeScheduledAgentTask(graph, lease, result, next)
		if finalizeErr != nil {
			publishCheckpoint()
			return run, finalizeErr
		}
		if finalized.Applied {
			cumulative, usageErr := addAgentUsage(run.UsageByTask[executionTask.ID], result.Usage)
			if usageErr != nil {
				publishCheckpoint()
				return run, fmt.Errorf("task %q usage: %w", executionTask.ID, usageErr)
			}
			run.UsageByTask[executionTask.ID] = cumulative
			run.Results = append(run.Results, AgentScheduledTaskResult{TaskID: executionTask.ID, Role: executionTask.Role, Result: result})
		}
		publishCheckpoint()
		if finalized.State == AgentTaskCancelled {
			return run, context.Canceled
		}
	}
}
