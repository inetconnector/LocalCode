// SPDX-License-Identifier: Apache-2.0

package main

import "time"

type MissionTaskLifecycle struct {
	AttemptCount   int       `json:"attempt_count"`
	RetryCount     int       `json:"retry_count"`
	StateUpdatedAt time.Time `json:"state_updated_at"`
	LastStartedAt  time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt time.Time `json:"last_finished_at,omitempty"`
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
