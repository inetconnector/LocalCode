// SPDX-License-Identifier: Apache-2.0
//
// Private, in-process LocalCode telemetry projection for an operator-bound
// mission. This file adds NO HTTP, IPC, network connection, logging, release
// permission or operator authentication. Do NOT call from a public endpoint.
// A transport is forbidden until mutually authenticated encryption, isolated
// process identity and source/scope validation are independently verified.
package main

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

const agentMissionMinimalTelemetrySchema = "nomamica.localcode.desktop-minimal.v1"
const agentMissionMinimalTelemetryLimit = 1024

var (
	agentMissionMinimalID         = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
	errAgentMissionMinimalInvalid = errors.New("MINIMAL_TELEMETRY_INVALID")
)

// Fixed, tiny schema. No embedded maps, raw JSON, paths, model details,
// credentials, prompts, budgets, logs, environment or task descriptors.
type agentMissionMinimalScheduler struct {
	MissionID string `json:"mission_id"`
	Queued    int    `json:"queued"`
	Running   int    `json:"running"`
}

type agentMissionMinimalTelemetry struct {
	SchemaVersion  string                       `json:"schema_version"`
	MissionID      string                       `json:"mission_id"`
	ExecutionRunID string                       `json:"execution_run_id"`
	State          string                       `json:"state"`
	StartedAt      time.Time                    `json:"started_at"`
	UpdatedAt      time.Time                    `json:"updated_at"`
	Scheduler      agentMissionMinimalScheduler `json:"scheduler"`
}

func agentMissionMinimalStateAllowed(state string) bool {
	switch state {
	case "running", "succeeded", "failed", "cancelled", "budget_exhausted":
		return true
	default:
		return false
	}
}

// agentMissionBuildMinimalTelemetry reads only the seven allowlisted values.
// This is a source-side projection: never marshal AgentMissionDesktopStatus.
// The raw desktop status must not leave this function's process boundary.
func agentMissionBuildMinimalTelemetry(
	status AgentMissionDesktopStatus,
) (agentMissionMinimalTelemetry, error) {
	if !agentMissionMinimalID.MatchString(status.MissionID) ||
		!agentMissionMinimalID.MatchString(status.ExecutionRunID) ||
		status.Scheduler.MissionID != status.MissionID ||
		!agentMissionMinimalStateAllowed(status.State) ||
		status.Scheduler.Queued < 0 || status.Scheduler.Queued > 64 ||
		status.Scheduler.Running < 0 || status.Scheduler.Running > 64 ||
		status.StartedAt.IsZero() || status.UpdatedAt.IsZero() ||
		status.UpdatedAt.Before(status.StartedAt) {
		return agentMissionMinimalTelemetry{}, errAgentMissionMinimalInvalid
	}
	return agentMissionMinimalTelemetry{
		SchemaVersion:  agentMissionMinimalTelemetrySchema,
		MissionID:      status.MissionID,
		ExecutionRunID: status.ExecutionRunID,
		State:          status.State,
		StartedAt:      status.StartedAt.UTC(),
		UpdatedAt:      status.UpdatedAt.UTC(),
		Scheduler: agentMissionMinimalScheduler{
			MissionID: status.Scheduler.MissionID,
			Queued:    status.Scheduler.Queued,
			Running:   status.Scheduler.Running,
		},
	}, nil
}

// No transport, network, filesystem or logger. Errors have constant text.
// The only JSON produced is the allowlisted, size-limited status structure.
func agentMissionBuildMinimalJSON(status AgentMissionDesktopStatus) ([]byte, error) {
	minimal, err := agentMissionBuildMinimalTelemetry(status)
	if err != nil {
		return nil, errAgentMissionMinimalInvalid
	}
	encoded, err := json.Marshal(minimal)
	if err != nil || len(encoded) > agentMissionMinimalTelemetryLimit {
		return nil, errAgentMissionMinimalInvalid
	}
	return encoded, nil
}

// agentMissionMinimalJSONForBoundRun reads LocalCode's existing in-process
// status store, verifies the exact operator-approved run/mission/project
// labels, and serializes ONLY the strict public field allowlist. The private
// project value is compared in memory but is never copied into the JSON.
// The supplied labels must come from an independently trusted operator.
// This function does not authenticate a caller or authorize any transport.
func agentMissionMinimalJSONForBoundRun(executionRunID, missionID, project string) ([]byte, error) {
	if !agentMissionMinimalID.MatchString(executionRunID) ||
		!agentMissionMinimalID.MatchString(missionID) ||
		project == "" {
		return nil, errAgentMissionMinimalInvalid
	}
	status, ok := agentMissionDesktopStatusForRun(executionRunID)
	if !ok || status.ExecutionRunID != executionRunID ||
		status.MissionID != missionID || status.Project != project {
		return nil, errAgentMissionMinimalInvalid
	}
	return agentMissionBuildMinimalJSON(status)
}
