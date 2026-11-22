// SPDX-License-Identifier: Apache-2.0
// Pure in-process projection tests. No HTTP endpoint, transport or mission start.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func minimalMissionTestStatus(runID string) AgentMissionDesktopStatus {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	return AgentMissionDesktopStatus{
		MissionID:      "MISSION-TELEMETRY-TEST",
		ExecutionRunID: runID,
		Project:        "C:\\Private\\test-do-not-export",
		Model:          "fake-token-PRIVATE-CANARY",
		State:          "running",
		Scheduler: AgentSchedulerSnapshot{
			MissionID: "MISSION-TELEMETRY-TEST",
			Queued:    1,
			Running:   2,
		},
		StartedAt: now.Add(-time.Second),
		UpdatedAt: now,
	}
}

func TestMissionMinimalNoSecretsOrExtraFields(t *testing.T) {
	raw := minimalMissionTestStatus("run-private-test-1")
	encoded, err := agentMissionBuildMinimalJSON(raw)
	if err != nil {
		t.Fatalf("valid local status refused: %v", err)
	}
	if bytes.Contains(encoded, []byte("Private")) ||
		bytes.Contains(encoded, []byte("fake-token")) {
		t.Fatal("private source data leaked into projected JSON")
	}
	if len(encoded) > agentMissionMinimalTelemetryLimit {
		t.Fatal("minimal telemetry exceeded explicit bound")
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	fields := []string{"schema_version", "mission_id", "execution_run_id", "state", "started_at", "updated_at", "scheduler"}
	if len(document) != len(fields) {
		t.Fatal("unexpected top-level field count")
	}
	for _, field := range fields {
		if _, ok := document[field]; !ok {
			t.Fatalf("missing allowed field: %s", field)
		}
	}
	nested, ok := document["scheduler"].(map[string]any)
	if !ok || len(nested) != 3 {
		t.Fatal("unexpected scheduler shape")
	}
	for _, field := range []string{"mission_id", "queued", "running"} {
		if _, ok := nested[field]; !ok {
			t.Fatalf("missing scheduler field: %s", field)
		}
	}
}

func TestMissionMinimalReadbackBoundToRealRun(t *testing.T) {
	runID := "run-minimal-telemetry-fixture-20261122"
	publishAgentMissionDesktopStatus(minimalMissionTestStatus(runID))
	actual, err := agentMissionMinimalJSONForRun(runID)
	if err != nil || len(actual) == 0 {
		t.Fatalf("existing local status rejected: %v", err)
	}
	for _, candidate := range []string{"", "../escape", "run-foreign-no-such-status", "run-foreign\\nsecret"} {
		body, err := agentMissionMinimalJSONForRun(candidate)
		if len(body) != 0 || !errors.Is(err, errAgentMissionMinimalInvalid) {
			t.Fatalf("invalid/unknown run must be blocked")
		}
	}
}

func TestMissionMinimalInvalidSourceNeverSerialized(t *testing.T) {
	raw := minimalMissionTestStatus("run-invalid-1")
	raw.Scheduler.MissionID = "foreign-mission"
	data, err := agentMissionBuildMinimalJSON(raw)
	if len(data) != 0 || !errors.Is(err, errAgentMissionMinimalInvalid) {
		t.Fatal("foreign scheduler mission not rejected")
	}
	raw = minimalMissionTestStatus("run-invalid-2")
	raw.State = "release_approved"
	data, err = agentMissionBuildMinimalJSON(raw)
	if len(data) != 0 || !errors.Is(err, errAgentMissionMinimalInvalid) {
		t.Fatal("unapproved state not rejected")
	}
}
