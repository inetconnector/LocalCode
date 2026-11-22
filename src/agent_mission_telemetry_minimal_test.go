// SPDX-License-Identifier: Apache-2.0
//
// These tests use the REAL LocalCode types and fake in-memory status data.
// No ports, secrets, commands, tasks, or authentication operations are used.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

const agentMinimalFakeCanary = "fake-secret-for-telemetry-test-9a27f3"

func agentMinimalFakeStatus() AgentMissionDesktopStatus {
	updated := time.Date(2026, 11, 22, 7, 0, 0, 0, time.UTC)
	return AgentMissionDesktopStatus{
		MissionID:         "test-mission-9a27f3",
		ExecutionRunID:    "test-run-9a27f3",
		State:             "running",
		Project:           agentMinimalFakeCanary,
		Model:             agentMinimalFakeCanary,
		Reason:            AgentMissionReason(agentMinimalFakeCanary),
		BudgetExhaustedBy: agentMinimalFakeCanary,
		Budget:            AgentBudgetSnapshot{ExhaustedBy: agentMinimalFakeCanary},
		ResourceLimits:    AgentResourceLimits{MaxQueued: 128},
		Scheduler: AgentSchedulerSnapshot{
			MissionID: "test-mission-9a27f3",
			Queued:    1,
			Running:   2,
			Tasks: []AgentTaskScheduleSnapshot{
				{TaskID: agentMinimalFakeCanary, AdmissionBlockedReason: agentMinimalFakeCanary},
			},
			Resources: []AgentResourceSnapshot{{Class: AgentResourceClass(agentMinimalFakeCanary)}},
		},
		StartedAt: updated.Add(-time.Minute),
		UpdatedAt: updated,
	}
}

func TestAgentMinimalDesktopTelemetryAllowlist(t *testing.T) {
	raw := agentMinimalFakeStatus()
	data, err := agentMissionBuildMinimalJSON(raw)
	if err != nil {
		t.Fatal("valid in-memory status was rejected")
	}
	if len(data) > agentMissionMinimalTelemetryLimit {
		t.Fatal("status exceeds fixed size bound")
	}
	if bytes.Contains(data, []byte(agentMinimalFakeCanary)) {
		t.Fatal("private canary leaked to the minimized output")
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal("invalid minimized JSON")
	}
	fields := []string{"schema_version", "mission_id", "execution_run_id", "state", "started_at", "updated_at", "scheduler"}
	if len(result) != len(fields) {
		t.Fatal("unexpected top-level fields")
	}
	for _, key := range fields {
		if _, ok := result[key]; !ok {
			t.Fatal("missing required field in minimized status")
		}
	}
	if result["schema_version"] != agentMissionMinimalTelemetrySchema {
		t.Fatal("unexpected minimal schema version")
	}
	sched, ok := result["scheduler"].(map[string]any)
	if !ok || len(sched) != 3 {
		t.Fatal("unapproved scheduler detail was included")
	}
	for _, key := range []string{"mission_id", "queued", "running"} {
		if _, ok := sched[key]; !ok {
			t.Fatal("missing fixed scheduler field")
		}
	}
	for _, forbidden := range []string{"project", "model", "reason", "budget", "resource_limits", "tasks", "stdout", "api_key", "token"} {
		if bytes.Contains(bytes.ToLower(data), []byte(forbidden)) {
			t.Fatal("unapproved metadata or field leaked")
		}
	}
}

func TestAgentMinimalDesktopTelemetryOnlySafeSourceFields(t *testing.T) {
	raw := agentMinimalFakeStatus()
	raw.Project = agentMinimalFakeCanary
	raw.Model = agentMinimalFakeCanary
	raw.Reason = AgentMissionReason(agentMinimalFakeCanary)
	raw.BudgetExhaustedBy = agentMinimalFakeCanary
	raw.Budget.ExhaustedBy = agentMinimalFakeCanary
	raw.Scheduler.Tasks = []AgentTaskScheduleSnapshot{{TaskID: agentMinimalFakeCanary}}
	raw.Scheduler.Resources = []AgentResourceSnapshot{{Class: AgentResourceClass(agentMinimalFakeCanary)}}
	data, err := agentMissionBuildMinimalJSON(raw)
	if err != nil || bytes.Contains(data, []byte(agentMinimalFakeCanary)) {
		t.Fatal("private source status must not be serialized")
	}
}

func TestAgentMinimalDesktopTelemetryInvalidInputIsRejectedWithoutEcho(t *testing.T) {
	mutations := []func(*AgentMissionDesktopStatus){
		func(x *AgentMissionDesktopStatus) { x.MissionID = "" },
		func(x *AgentMissionDesktopStatus) { x.MissionID = "../outside" },
		func(x *AgentMissionDesktopStatus) { x.MissionID = strings.Repeat("m", 129) },
		func(x *AgentMissionDesktopStatus) { x.ExecutionRunID = "bad\nrun" },
		func(x *AgentMissionDesktopStatus) { x.ExecutionRunID = "" },
		func(x *AgentMissionDesktopStatus) { x.State = "release_approved" },
		func(x *AgentMissionDesktopStatus) { x.Scheduler.MissionID = "foreign-mission" },
		func(x *AgentMissionDesktopStatus) { x.Scheduler.Queued = -1 },
		func(x *AgentMissionDesktopStatus) { x.Scheduler.Running = 65 },
		func(x *AgentMissionDesktopStatus) { x.StartedAt = time.Time{} },
		func(x *AgentMissionDesktopStatus) { x.UpdatedAt = x.StartedAt.Add(-time.Second) },
	}
	for i, mutation := range mutations {
		raw := agentMinimalFakeStatus()
		mutation(&raw)
		data, err := agentMissionBuildMinimalJSON(raw)
		if len(data) != 0 || !errors.Is(err, errAgentMissionMinimalInvalid) {
			t.Fatalf("invalid case number %d was accepted", i)
		}
		if strings.Contains(err.Error(), agentMinimalFakeCanary) {
			t.Fatal("private input echoed into error")
		}
	}
}

func TestAgentMinimalDesktopSourceStoreReadbackDoesNotLeak(t *testing.T) {
	raw := agentMinimalFakeStatus()
	const testRun = "unique-minimal-internal-source-test-9a27f3"
	raw.ExecutionRunID = testRun
	agentMissionDesktopStatuses.Lock()
	old, prior := agentMissionDesktopStatuses.byRun[testRun]
	agentMissionDesktopStatuses.byRun[testRun] = raw
	agentMissionDesktopStatuses.Unlock()
	defer func() {
		agentMissionDesktopStatuses.Lock()
		if prior {
			agentMissionDesktopStatuses.byRun[testRun] = old
		} else {
			delete(agentMissionDesktopStatuses.byRun, testRun)
		}
		agentMissionDesktopStatuses.Unlock()
	}()

	data, err := agentMissionMinimalJSONForRun(testRun)
	if err != nil || len(data) == 0 || bytes.Contains(data, []byte(agentMinimalFakeCanary)) {
		t.Fatal("private process-local snapshot was not safely minimized")
	}
	if result, err := agentMissionMinimalJSONForRun("invalid\nrun"); len(result) != 0 || !errors.Is(err, errAgentMissionMinimalInvalid) {
		t.Fatal("invalid run identifier was not blocked")
	}
	if result, err := agentMissionMinimalJSONForRun("absent-minimal-run"); len(result) != 0 || !errors.Is(err, errAgentMissionMinimalInvalid) {
		t.Fatal("unknown source run was not blocked")
	}
}
