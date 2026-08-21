// SPDX-License-Identifier: Apache-2.0

package main

import "fmt"

type agentScheduledFinalizeOutcome struct {
	State   AgentTaskState
	Applied bool
}

// prepareScheduledAgentTask snapshots one scheduler-owned running task while the
// scheduler lock is held. The child runtime receives the detached copy, never a
// pointer into the shared task graph. This keeps concurrent cancellation from
// racing with child setup or budget normalization.
func (s *AgentScheduler) prepareScheduledAgentTask(graph *AgentTaskGraph, lease AgentResourceLease, cfg Config) (AgentTask, error) {
	if s == nil {
		return AgentTask{}, fmt.Errorf("agent scheduler is nil")
	}
	if graph == nil {
		return AgentTask{}, fmt.Errorf("task graph is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	active, exists := s.active[lease.TaskID]
	if !exists || active.lease.token != lease.token || active.lease.ResourceClass != lease.ResourceClass {
		return AgentTask{}, fmt.Errorf("task %q does not hold this scheduler lease", lease.TaskID)
	}
	task := agentTaskByID(graph, lease.TaskID)
	if task == nil {
		return AgentTask{}, fmt.Errorf("scheduled task %q disappeared after admission", lease.TaskID)
	}
	if task.State != AgentTaskRunning {
		return AgentTask{}, fmt.Errorf("scheduled task %q is %q instead of running", lease.TaskID, task.State)
	}

	task.Budget = normalizeAgentBudget(task.Budget, task.Role, cfg)
	executionTask := *task
	executionTask.Dependencies = append([]string(nil), task.Dependencies...)
	executionTask.RequestedCapabilities = append([]AgentCapability(nil), task.RequestedCapabilities...)
	executionTask.Capabilities = append([]AgentCapability(nil), task.Capabilities...)
	return executionTask, nil
}

// finalizeScheduledAgentTask makes child completion and scheduler cancellation
// deterministic at one lock boundary. If cancellation wins first, its terminal
// graph state is preserved and the late child result is not written. If child
// completion wins first, the result/state transition and resource release are
// committed before a later cancellation can inspect the task. A cancellation
// finalized from the lease context also discards the child result rather than
// presenting partial/late work as a normal completed result.
func (s *AgentScheduler) finalizeScheduledAgentTask(graph *AgentTaskGraph, lease AgentResourceLease, result AgentResult, next AgentTaskState) (agentScheduledFinalizeOutcome, error) {
	outcome := agentScheduledFinalizeOutcome{}
	if s == nil {
		return outcome, fmt.Errorf("agent scheduler is nil")
	}
	if graph == nil {
		return outcome, fmt.Errorf("task graph is nil")
	}
	if next != AgentTaskSucceeded && next != AgentTaskFailed && next != AgentTaskCancelled {
		return outcome, fmt.Errorf("unsupported scheduled terminal state %q", next)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	task := agentTaskByID(graph, lease.TaskID)
	if task == nil {
		return outcome, fmt.Errorf("scheduled task %q disappeared before finalization", lease.TaskID)
	}
	outcome.State = task.State

	active, exists := s.active[lease.TaskID]
	if !exists {
		if task.State == AgentTaskCancelled || task.State == AgentTaskFailed || isSuccessfulAgentTaskState(task.State) {
			return outcome, nil
		}
		return outcome, fmt.Errorf("task %q no longer holds a scheduler lease while in state %q", lease.TaskID, task.State)
	}
	if active.lease.token != lease.token || active.lease.ResourceClass != lease.ResourceClass {
		return outcome, fmt.Errorf("task %q does not hold this scheduler lease", lease.TaskID)
	}
	if task.State != AgentTaskRunning {
		s.releaseActiveLeaseLocked(lease.TaskID, active)
		outcome.State = task.State
		if task.State == AgentTaskCancelled || task.State == AgentTaskFailed || isSuccessfulAgentTaskState(task.State) {
			return outcome, nil
		}
		return outcome, fmt.Errorf("scheduled task %q left running state before finalization: %q", lease.TaskID, task.State)
	}

	if err := transitionAgentTask(graph, lease.TaskID, next); err != nil {
		return outcome, err
	}
	task = agentTaskByID(graph, lease.TaskID)
	if task == nil {
		return outcome, fmt.Errorf("scheduled task %q disappeared during finalization", lease.TaskID)
	}
	if next != AgentTaskCancelled {
		task.Result = result
		outcome.Applied = true
	}
	s.releaseActiveLeaseLocked(lease.TaskID, active)
	outcome.State = next
	return outcome, nil
}
