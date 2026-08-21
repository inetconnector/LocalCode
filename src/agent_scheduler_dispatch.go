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
	run := AgentScheduledRun{UsageByTask: map[string]AgentUsage{}}
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
	if snapshot := scheduler.Snapshot(graph, run.UsageByTask); snapshot.Running != 0 {
		run.Snapshot = snapshot
		return run, fmt.Errorf("agent scheduler already has %d active task(s)", snapshot.Running)
	}

	for {
		if err := scheduler.QueueReady(graph, nil); err != nil {
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			return run, err
		}
		lease, admitted, err := scheduler.AdmitNext(graph)
		if err != nil {
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			if err == context.Canceled {
				return run, context.Canceled
			}
			return run, err
		}
		if !admitted {
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			return run, nil
		}

		executionTask, err := scheduler.prepareScheduledAgentTask(graph, lease, cfg)
		if err != nil {
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			if lease.Context.Err() != nil {
				return run, context.Canceled
			}
			return run, err
		}

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
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			return run, finalizeErr
		}
		if finalized.Applied {
			run.UsageByTask[executionTask.ID] = result.Usage
			run.Results = append(run.Results, AgentScheduledTaskResult{TaskID: executionTask.ID, Role: executionTask.Role, Result: result})
		}
		if finalized.State == AgentTaskCancelled {
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			return run, context.Canceled
		}
	}
}
