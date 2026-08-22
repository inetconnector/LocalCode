// SPDX-License-Identifier: Apache-2.0

package main

import "time"

type MissionTaskLifecycle struct {
	AttemptCount      int       `json:"attempt_count"`
	RetryCount        int       `json:"retry_count"`
	AttemptReserved   bool      `json:"attempt_reserved,omitempty"`
	AttemptReservedAt time.Time `json:"attempt_reserved_at,omitempty"`
	StateUpdatedAt    time.Time `json:"state_updated_at"`
	LastStartedAt     time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt    time.Time `json:"last_finished_at,omitempty"`
}

func updateMissionTaskLifecycle(task *MissionRecoveryTaskState, snapshot AgentTaskScheduleSnapshot, observedAt time.Time) {
	if task == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	if task.Lifecycle == nil {
		task.Lifecycle = &MissionTaskLifecycle{StateUpdatedAt: observedAt}
	}
	lifecycle := task.Lifecycle
	stateChanged := task.State != snapshot.State || task.Running != snapshot.Running
	if stateChanged {
		lifecycle.StateUpdatedAt = observedAt
	}
	if snapshot.Running && !task.Running {
		// A continuation reservation is durable admission intent, not an
		// execution attempt. Count the attempt only once the Scheduler has
		// actually transitioned the task to Running. If the process crashes
		// after reservation but before this checkpoint, the reserved attempt can
		// be reused without consuming retry budget.
		lifecycle.AttemptReserved = false
		lifecycle.AttemptReservedAt = time.Time{}
		lifecycle.AttemptCount++
		if lifecycle.AttemptCount > 1 {
			lifecycle.RetryCount = lifecycle.AttemptCount - 1
		} else {
			lifecycle.RetryCount = 0
		}
		lifecycle.LastStartedAt = observedAt
	}
	if !snapshot.Running && task.Running {
		lifecycle.LastFinishedAt = observedAt
	}
}
