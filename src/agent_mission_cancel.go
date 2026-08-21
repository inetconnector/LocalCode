// SPDX-License-Identifier: Apache-2.0

package main

// cancelUnfinishedReadOnlyMissionTasks runs only after scheduled dispatch has
// returned from parent-context cancellation. At that point no child executor is
// still mutating the graph. Successful/failed terminal work is preserved while
// every unfinished task becomes explicitly cancelled so the returned Mission
// graph cannot contain stale ready/blocked work after StopAgent.
func cancelUnfinishedReadOnlyMissionTasks(graph *AgentTaskGraph) {
	if graph == nil {
		return
	}
	for i := range graph.Tasks {
		task := &graph.Tasks[i]
		switch task.State {
		case AgentTaskSucceeded, AgentTaskCompleted, AgentTaskFailed, AgentTaskCancelled:
			continue
		}
		if agentTaskTransitionAllowed(task.State, AgentTaskCancelled) {
			task.State = AgentTaskCancelled
			task.StateReason = ""
		}
	}
}
