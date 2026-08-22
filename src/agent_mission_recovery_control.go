// SPDX-License-Identifier: Apache-2.0

package main

import "errors"

var errMissionRecoveryControlActiveRun = errors.New("mission recovery control unavailable while an agent run is active")

type missionRecoveryControlSnapshotBuilder func(string) (MissionRecoveryControlSnapshot, error)

func missionRecoveryControlSnapshotForAppState(state *AppState, runID string, build missionRecoveryControlSnapshotBuilder) (MissionRecoveryControlSnapshot, error) {
	if state == nil {
		return MissionRecoveryControlSnapshot{}, errMissionRecoveryControlUnavailable
	}
	if build == nil {
		build = buildStableMissionRecoveryControlSnapshot
	}

	state.mu.RLock()
	running := state.Running
	state.mu.RUnlock()
	if running {
		return MissionRecoveryControlSnapshot{}, errMissionRecoveryControlActiveRun
	}

	snapshot, err := build(runID)
	if err != nil {
		return MissionRecoveryControlSnapshot{}, err
	}

	state.mu.RLock()
	running = state.Running
	state.mu.RUnlock()
	if running {
		return MissionRecoveryControlSnapshot{}, errMissionRecoveryControlActiveRun
	}
	return snapshot, nil
}

// MissionRecoveryControlSnapshot is the trusted AppState boundary for observing
// interrupted read-only Mission recovery state. It deliberately has no dispatch,
// Scheduler-admission, capability-grant, resume, retry, replay, or journal-write
// side effects. Any later continuation path must call this boundary immediately
// before its own separately governed dispatch decision and then revalidate again.
func (state *AppState) MissionRecoveryControlSnapshot(runID string) (MissionRecoveryControlSnapshot, error) {
	return missionRecoveryControlSnapshotForAppState(state, runID, nil)
}
