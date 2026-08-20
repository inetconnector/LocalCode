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
//
// The runner is deterministic at the graph boundary. It queues all logically
// ready tasks, lets the scheduler choose an admissible lease, executes exactly
// that task through runNativeReadOnlyAgentTask, stores its structured result,
// releases the lease into a terminal graph state, then reconciles dependencies
// before the next admission. The default scheduler therefore keeps local model
// inference at one active task while still representing multiple ready tasks in
// its bounded queue.
func (s *AppState) runScheduledReadOnlyAgentGraph(project string, cfg Config, graph *AgentTaskGraph, scheduler *AgentScheduler) (AgentScheduledRun, error) {
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

		task := agentTaskByID(graph, lease.TaskID)
		if task == nil {
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			return run, fmt.Errorf("scheduled task %q disappeared after admission", lease.TaskID)
		}
		task.Budget = normalizeAgentBudget(task.Budget, task.Role, cfg)
		executionTask := *task
		result, runErr := s.runNativeReadOnlyAgentTask(lease.Context, project, cfg, executionTask)
		if runErr != nil {
			result = AgentResult{
				Status:  AgentResultBlocked,
				Summary: "Scheduled read-only child execution failed: " + strings.TrimSpace(runErr.Error()),
			}
		}
		task.Result = result
		run.UsageByTask[task.ID] = result.Usage
		run.Results = append(run.Results, AgentScheduledTaskResult{TaskID: task.ID, Role: task.Role, Result: result})

		if lease.Context.Err() != nil {
			if task.State == AgentTaskRunning {
				if releaseErr := scheduler.Release(graph, lease, AgentTaskCancelled); releaseErr != nil {
					run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
					return run, releaseErr
				}
			}
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			return run, context.Canceled
		}

		next := scheduledAgentTaskTerminalState(result, runErr)
		if err := scheduler.Release(graph, lease, next); err != nil {
			run.Snapshot = scheduler.Snapshot(graph, run.UsageByTask)
			return run, err
		}
	}
}
