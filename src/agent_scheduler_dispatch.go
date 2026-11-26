// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
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

type scheduledTaskCompletion struct {
	lease         AgentResourceLease
	executionTask AgentTask
	result        AgentResult
	runErr        error
}

// runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded is the recovery
// accounting boundary. Historical durable usage is copied into the run before
// admission and every newly observed task usage is accumulated onto that seed;
// a continuation therefore cannot reset a task budget by starting a new
// scheduler invocation.
//
// When scheduler resource limits permit multiple concurrent slots (e.g.
// Limits.ModelInference > 1 or independent resource classes), ready tasks run
// concurrently in isolated worker goroutines. When effective concurrency is 1,
// execution stays strictly deterministic FIFO.
func (s *AppState) runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded(project string, cfg Config, graph *AgentTaskGraph, scheduler *AgentScheduler, execute scheduledReadOnlyAgentExecutor, checkpoint scheduledReadOnlyAgentCheckpoint, seed map[string]AgentUsage) (AgentScheduledRun, error) {
	return s.runScheduledAgentGraphConcurrent(project, cfg, graph, scheduler, execute, checkpoint, seed)
}

// runScheduledAgentGraphConcurrent executes a validated AgentTaskGraph through
// AgentScheduler using an asynchronous goroutine dispatcher.
//
// Invariants:
//   - Only the single coordinator goroutine calls QueueReady, AdmitNext,
//     prepareScheduledAgentTask, finalizeScheduledAgentTask, checkpoint, and
//     mutates run.Results / run.UsageByTask.
//   - Worker goroutines operate strictly on detached AgentTask copies and
//     report results back via a buffered channel.
//   - On cancellation or terminal error, all in-flight worker goroutines are
//     drained and joined via sync.WaitGroup before returning (0 leaked
//     goroutines), preserving cancellation-first and completion-first semantics.
func (s *AppState) runScheduledAgentGraphConcurrent(project string, cfg Config, graph *AgentTaskGraph, scheduler *AgentScheduler, execute scheduledReadOnlyAgentExecutor, checkpoint scheduledReadOnlyAgentCheckpoint, seed map[string]AgentUsage) (AgentScheduledRun, error) {
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

	bufSize := len(graph.Tasks)
	if bufSize < 16 {
		bufSize = 16
	}
	completions := make(chan scheduledTaskCompletion, bufSize)
	var wg sync.WaitGroup
	inFlight := 0

	finalizeCompletion := func(comp scheduledTaskCompletion) (agentScheduledFinalizeOutcome, error) {
		result := comp.result
		if comp.runErr != nil {
			prefix := "Scheduled read-only child execution failed: "
			if comp.executionTask.Role == AgentRoleBuilder || comp.executionTask.Role == AgentRoleIntegrator || comp.executionTask.Role == AgentRoleTestAgent {
				prefix = "Scheduled child execution failed: "
			}
			result = AgentResult{
				Status:  AgentResultBlocked,
				Summary: prefix + strings.TrimSpace(comp.runErr.Error()),
				Usage:   comp.result.Usage,
			}
		}
		next := scheduledAgentTaskTerminalState(result, comp.runErr)
		if comp.lease.Context.Err() != nil {
			next = AgentTaskCancelled
		}

		finalized, finalizeErr := scheduler.finalizeScheduledAgentTask(graph, comp.lease, result, next)
		if finalizeErr != nil {
			publishCheckpoint()
			return finalized, finalizeErr
		}
		if finalized.Applied {
			cumulative, usageErr := addAgentUsage(run.UsageByTask[comp.executionTask.ID], result.Usage)
			if usageErr != nil {
				publishCheckpoint()
				return finalized, fmt.Errorf("task %q usage: %w", comp.executionTask.ID, usageErr)
			}
			run.UsageByTask[comp.executionTask.ID] = cumulative
			run.Results = append(run.Results, AgentScheduledTaskResult{
				TaskID: comp.executionTask.ID,
				Role:   comp.executionTask.Role,
				Result: result,
			})
		}
		publishCheckpoint()
		return finalized, nil
	}

	drainInFlight := func(cancelRemaining bool) {
		if cancelRemaining && inFlight > 0 {
			_ = scheduler.CancelMission(graph)
		}
		for inFlight > 0 {
			comp := <-completions
			inFlight--
			_, _ = finalizeCompletion(comp)
		}
		wg.Wait()
		publishCheckpoint()
	}

	for {
		if err := scheduler.QueueReady(graph, nil); err != nil {
			drainInFlight(errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded))
			return run, err
		}
		publishCheckpoint()

		for {
			lease, admitted, err := scheduler.AdmitNext(graph)
			if err != nil {
				if err == context.Canceled || errors.Is(err, context.Canceled) {
					drainInFlight(true)
					return run, context.Canceled
				}
				drainInFlight(true)
				return run, err
			}
			if !admitted {
				break
			}

			executionTask, err := scheduler.prepareScheduledAgentTask(graph, lease, cfg)
			if err != nil {
				if lease.Context.Err() != nil {
					drainInFlight(true)
					return run, context.Canceled
				}
				_ = scheduler.CancelTask(graph, lease.TaskID)
				drainInFlight(true)
				return run, err
			}
			publishCheckpoint()

			inFlight++
			wg.Add(1)
			go func(taskLease AgentResourceLease, taskCopy AgentTask) {
				defer wg.Done()
				result, runErr := execute(taskLease.Context, project, cfg, taskCopy)
				completions <- scheduledTaskCompletion{
					lease:         taskLease,
					executionTask: taskCopy,
					result:        result,
					runErr:        runErr,
				}
			}(lease, executionTask)
		}

		if inFlight == 0 {
			publishCheckpoint()
			wg.Wait()
			return run, nil
		}

		var comp scheduledTaskCompletion
		select {
		case comp = <-completions:
			inFlight--
		case <-scheduler.missionCtx.Done():
			drainInFlight(true)
			return run, context.Canceled
		}

		finalized, finalizeErr := finalizeCompletion(comp)
		if finalizeErr != nil {
			drainInFlight(true)
			return run, finalizeErr
		}
		if finalized.State == AgentTaskCancelled {
			drainInFlight(inFlight > 0)
			return run, context.Canceled
		}
	}
}
