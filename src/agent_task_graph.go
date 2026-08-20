// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	maxAgentTaskIDLength    = 80
	maxAgentRoleLabelLength = 80
)

var agentTaskIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type AgentTaskGraph struct {
	MissionID string      `json:"mission_id"`
	Tasks     []AgentTask `json:"tasks"`
}

func validateAgentTaskIdentifier(value, field string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is empty", field)
	}
	if len(value) > maxAgentTaskIDLength || !agentTaskIdentifierPattern.MatchString(value) {
		return fmt.Errorf("%s %q must be 1-%d characters using only letters, digits, '.', '_' or '-'", field, value, maxAgentTaskIDLength)
	}
	return nil
}

func validateAgentRoleLabel(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("task role is empty")
	}
	if len(value) > maxAgentRoleLabelLength || !agentTaskIdentifierPattern.MatchString(value) {
		return fmt.Errorf("task role %q must be 1-%d characters using only letters, digits, '.', '_' or '-'", value, maxAgentRoleLabelLength)
	}
	return nil
}

func validateAgentTaskProposals(proposals []AgentTaskProposal) error {
	ids := make(map[string]struct{}, len(proposals))
	for i := range proposals {
		proposal := &proposals[i]
		proposal.ID = strings.TrimSpace(proposal.ID)
		proposal.Role = strings.TrimSpace(proposal.Role)
		proposal.Objective = strings.TrimSpace(proposal.Objective)
		if err := validateAgentTaskIdentifier(proposal.ID, "task id"); err != nil {
			return fmt.Errorf("proposal %d: %w", i, err)
		}
		if _, exists := ids[proposal.ID]; exists {
			return fmt.Errorf("duplicate task id %q", proposal.ID)
		}
		ids[proposal.ID] = struct{}{}
		if err := validateAgentRoleLabel(proposal.Role); err != nil {
			return fmt.Errorf("proposal %q: %w", proposal.ID, err)
		}
		if proposal.Objective == "" {
			return fmt.Errorf("proposal %q objective is empty", proposal.ID)
		}

		seenDependencies := make(map[string]struct{}, len(proposal.Dependencies))
		for depIndex, rawDependency := range proposal.Dependencies {
			dependency := strings.TrimSpace(rawDependency)
			proposal.Dependencies[depIndex] = dependency
			if err := validateAgentTaskIdentifier(dependency, "dependency id"); err != nil {
				return fmt.Errorf("proposal %q: %w", proposal.ID, err)
			}
			if dependency == proposal.ID {
				return fmt.Errorf("task %q cannot depend on itself", proposal.ID)
			}
			if _, exists := seenDependencies[dependency]; exists {
				return fmt.Errorf("task %q has duplicate dependency %q", proposal.ID, dependency)
			}
			seenDependencies[dependency] = struct{}{}
		}
	}

	for _, proposal := range proposals {
		for _, dependency := range proposal.Dependencies {
			if _, exists := ids[dependency]; !exists {
				return fmt.Errorf("task %q depends on missing task %q", proposal.ID, dependency)
			}
		}
	}
	return detectAgentTaskProposalCycle(proposals)
}

func detectAgentTaskProposalCycle(proposals []AgentTaskProposal) error {
	byID := make(map[string]AgentTaskProposal, len(proposals))
	for _, proposal := range proposals {
		byID[proposal.ID] = proposal
	}
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	states := make(map[string]int, len(proposals))
	var visit func(string) error
	visit = func(id string) error {
		switch states[id] {
		case visiting:
			return fmt.Errorf("dependency cycle detected at task %q", id)
		case visited:
			return nil
		}
		states[id] = visiting
		for _, dependency := range byID[id].Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		states[id] = visited
		return nil
	}
	for _, proposal := range proposals {
		if states[proposal.ID] == unvisited {
			if err := visit(proposal.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildAgentTaskGraph(missionID, parentID string, proposals []AgentTaskProposal) (AgentTaskGraph, error) {
	missionID = strings.TrimSpace(missionID)
	parentID = strings.TrimSpace(parentID)
	if err := validateAgentTaskIdentifier(missionID, "mission id"); err != nil {
		return AgentTaskGraph{}, err
	}
	if parentID != "" {
		if err := validateAgentTaskIdentifier(parentID, "parent task id"); err != nil {
			return AgentTaskGraph{}, err
		}
	}
	proposals = cloneAgentTaskProposals(proposals)
	if err := validateAgentTaskProposals(proposals); err != nil {
		return AgentTaskGraph{}, err
	}

	graph := AgentTaskGraph{MissionID: missionID, Tasks: make([]AgentTask, 0, len(proposals))}
	for _, proposal := range proposals {
		if proposal.ID == parentID {
			return AgentTaskGraph{}, fmt.Errorf("task %q cannot also be its parent task", proposal.ID)
		}
		graph.Tasks = append(graph.Tasks, AgentTask{
			ID:                    proposal.ID,
			ParentID:              parentID,
			MissionID:             missionID,
			Role:                  AgentRole(proposal.Role),
			Objective:             proposal.Objective,
			Dependencies:          append([]string(nil), proposal.Dependencies...),
			State:                 AgentTaskProposed,
			RequestedCapabilities: append([]AgentCapability(nil), proposal.Capabilities...),
			Capabilities:          nil,
		})
	}
	if err := reconcileAgentTaskGraph(&graph); err != nil {
		return AgentTaskGraph{}, err
	}
	return graph, nil
}

func cloneAgentTaskProposals(proposals []AgentTaskProposal) []AgentTaskProposal {
	cloned := make([]AgentTaskProposal, len(proposals))
	for i, proposal := range proposals {
		cloned[i] = proposal
		cloned[i].Dependencies = append([]string(nil), proposal.Dependencies...)
		cloned[i].Capabilities = append([]AgentCapability(nil), proposal.Capabilities...)
	}
	return cloned
}

func validateAgentTaskGraph(graph AgentTaskGraph) error {
	if err := validateAgentTaskIdentifier(graph.MissionID, "mission id"); err != nil {
		return err
	}
	proposals := make([]AgentTaskProposal, 0, len(graph.Tasks))
	for _, task := range graph.Tasks {
		if strings.TrimSpace(task.MissionID) != graph.MissionID {
			return fmt.Errorf("task %q mission id %q does not match graph mission %q", task.ID, task.MissionID, graph.MissionID)
		}
		if task.ParentID != "" {
			if err := validateAgentTaskIdentifier(task.ParentID, "parent task id"); err != nil {
				return fmt.Errorf("task %q: %w", task.ID, err)
			}
			if task.ParentID == task.ID {
				return fmt.Errorf("task %q cannot also be its parent task", task.ID)
			}
		}
		if !isKnownAgentTaskState(task.State) {
			return fmt.Errorf("task %q has unsupported state %q", task.ID, task.State)
		}
		proposals = append(proposals, AgentTaskProposal{
			ID:           task.ID,
			Role:         string(task.Role),
			Objective:    task.Objective,
			Dependencies: append([]string(nil), task.Dependencies...),
			Capabilities: append([]AgentCapability(nil), task.RequestedCapabilities...),
		})
	}
	return validateAgentTaskProposals(proposals)
}

func isKnownAgentTaskState(state AgentTaskState) bool {
	switch state {
	case AgentTaskPending, AgentTaskRunning, AgentTaskCompleted, AgentTaskBlocked, AgentTaskFailed,
		AgentTaskProposed, AgentTaskReady, AgentTaskSucceeded, AgentTaskCancelled, AgentTaskRetryable:
		return true
	default:
		return false
	}
}

func isSuccessfulAgentTaskState(state AgentTaskState) bool {
	return state == AgentTaskCompleted || state == AgentTaskSucceeded
}

func isFailedAgentTaskState(state AgentTaskState) bool {
	return state == AgentTaskFailed || state == AgentTaskCancelled
}

func reconcileAgentTaskGraph(graph *AgentTaskGraph) error {
	if graph == nil {
		return fmt.Errorf("task graph is nil")
	}
	if err := validateAgentTaskGraph(*graph); err != nil {
		return err
	}
	byID := make(map[string]*AgentTask, len(graph.Tasks))
	for i := range graph.Tasks {
		byID[graph.Tasks[i].ID] = &graph.Tasks[i]
	}

	// Repeated deterministic passes let upstream blocked/ready state propagate
	// without introducing an execution scheduler in this phase.
	for pass := 0; pass <= len(graph.Tasks); pass++ {
		changed := false
		for i := range graph.Tasks {
			task := &graph.Tasks[i]
			if task.State == AgentTaskRunning || isSuccessfulAgentTaskState(task.State) || task.State == AgentTaskFailed || task.State == AgentTaskCancelled {
				continue
			}
			allSucceeded := true
			failureReason := ""
			waiting := make([]string, 0, len(task.Dependencies))
			for _, dependencyID := range task.Dependencies {
				dependency := byID[dependencyID]
				if isFailedAgentTaskState(dependency.State) {
					failureReason = fmt.Sprintf("dependency %s is %s", dependencyID, dependency.State)
					allSucceeded = false
					break
				}
				if !isSuccessfulAgentTaskState(dependency.State) {
					allSucceeded = false
					waiting = append(waiting, dependencyID)
				}
			}

			nextState := AgentTaskReady
			reason := ""
			if failureReason != "" {
				nextState = AgentTaskBlocked
				reason = failureReason
			} else if !allSucceeded {
				nextState = AgentTaskBlocked
				reason = "waiting for dependencies: " + strings.Join(waiting, ", ")
			}
			if task.State != nextState || task.StateReason != reason {
				task.State = nextState
				task.StateReason = reason
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return nil
}

func readyAgentTaskIDs(graph *AgentTaskGraph) ([]string, error) {
	if err := reconcileAgentTaskGraph(graph); err != nil {
		return nil, err
	}
	ready := make([]string, 0)
	for _, task := range graph.Tasks {
		if task.State == AgentTaskReady {
			ready = append(ready, task.ID)
		}
	}
	return ready, nil
}

func transitionAgentTask(graph *AgentTaskGraph, id string, next AgentTaskState) error {
	if graph == nil {
		return fmt.Errorf("task graph is nil")
	}
	if !isKnownAgentTaskState(next) {
		return fmt.Errorf("unsupported next task state %q", next)
	}
	if err := validateAgentTaskGraph(*graph); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	var task *AgentTask
	for i := range graph.Tasks {
		if graph.Tasks[i].ID == id {
			task = &graph.Tasks[i]
			break
		}
	}
	if task == nil {
		return fmt.Errorf("task %q not found", id)
	}
	if !agentTaskTransitionAllowed(task.State, next) {
		return fmt.Errorf("task %q cannot transition from %q to %q", id, task.State, next)
	}
	task.State = next
	task.StateReason = ""
	return reconcileAgentTaskGraph(graph)
}

func agentTaskTransitionAllowed(current, next AgentTaskState) bool {
	if current == next {
		return true
	}
	switch current {
	case AgentTaskProposed, AgentTaskPending, AgentTaskBlocked:
		return next == AgentTaskCancelled
	case AgentTaskReady:
		return next == AgentTaskRunning || next == AgentTaskCancelled
	case AgentTaskRunning:
		return next == AgentTaskSucceeded || next == AgentTaskCompleted || next == AgentTaskFailed || next == AgentTaskCancelled || next == AgentTaskRetryable
	case AgentTaskFailed:
		return next == AgentTaskRetryable
	case AgentTaskRetryable:
		return next == AgentTaskReady || next == AgentTaskCancelled
	case AgentTaskSucceeded, AgentTaskCompleted, AgentTaskCancelled:
		return false
	default:
		return false
	}
}
func buildPlannerTaskGraph(parent AgentTask, result AgentResult) (AgentTaskGraph, error) {
	if parent.Role != AgentRolePlanner {
		return AgentTaskGraph{}, fmt.Errorf("task %q is not a planner", parent.ID)
	}
	missionID := strings.TrimSpace(parent.MissionID)
	if missionID == "" {
		missionID = strings.TrimSpace(parent.ID)
	}
	return buildAgentTaskGraph(missionID, strings.TrimSpace(parent.ID), result.SuggestedTasks)
}
