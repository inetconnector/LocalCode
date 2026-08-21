// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"
)

func diagnosticResource(t *testing.T, diagnostics AgentOrchestrationDiagnostics, class AgentResourceClass) AgentResourceDiagnostics {
	t.Helper()
	for _, resource := range diagnostics.Resources {
		if resource.Class == class {
			return resource
		}
	}
	t.Fatalf("resource diagnostics missing class %q: %#v", class, diagnostics.Resources)
	return AgentResourceDiagnostics{}
}

func readyDiagnosticStatus() Status {
	return Status{
		OllamaOnline:  true,
		SelectedModel: "qwen-test:latest",
		Models:        []ModelInfo{{Name: "qwen-test:latest"}},
	}
}

func TestAgentOrchestrationDiagnosticsDistinguishBackendFailures(t *testing.T) {
	offline := agentOrchestrationDiagnostics(Status{SelectedModel: "qwen-test:latest"}, nil)
	if offline.State != AgentOrchestrationBackendUnavailable || offline.Reason != AgentOrchestrationReasonOllamaOffline {
		t.Fatalf("offline diagnostics = %#v", offline)
	}

	noModel := agentOrchestrationDiagnostics(Status{OllamaOnline: true}, nil)
	if noModel.State != AgentOrchestrationModelUnavailable || noModel.Reason != AgentOrchestrationReasonNoModelSelected {
		t.Fatalf("no-model diagnostics = %#v", noModel)
	}

	missing := agentOrchestrationDiagnostics(Status{OllamaOnline: true, SelectedModel: "missing", Models: []ModelInfo{{Name: "other"}}}, nil)
	if missing.State != AgentOrchestrationModelUnavailable || missing.Reason != AgentOrchestrationReasonSelectedModelMissing {
		t.Fatalf("missing-model diagnostics = %#v", missing)
	}
}

func TestAgentOrchestrationDiagnosticsAtCapacityIsNotSaturationWithoutWaiting(t *testing.T) {
	mission := &AgentMissionDesktopStatus{
		State: "running",
		ResourceLimits: AgentResourceLimits{
			MaxQueued:            8,
			ModelInference:       1,
			ReadCPU:              2,
			Build:                1,
			ExclusiveIntegration: 1,
		},
		Scheduler: AgentSchedulerSnapshot{
			Running: 1,
			Resources: []AgentResourceSnapshot{
				{Class: AgentResourceModelInference, Limit: 1, InUse: 1, Available: 0},
			},
			Tasks: []AgentTaskScheduleSnapshot{{TaskID: "running", State: AgentTaskRunning, ResourceClass: AgentResourceModelInference, Running: true}},
		},
	}
	diagnostics := agentOrchestrationDiagnostics(readyDiagnosticStatus(), mission)
	resource := diagnosticResource(t, diagnostics, AgentResourceModelInference)
	if !resource.AtCapacity || resource.Saturated || resource.Waiting != 0 {
		t.Fatalf("capacity diagnostics = %#v", resource)
	}
	if diagnostics.State != AgentOrchestrationActive || diagnostics.Reason != AgentOrchestrationReasonMissionRunning {
		t.Fatalf("mission at capacity without waiting must stay active, got %#v", diagnostics)
	}
}

func TestAgentOrchestrationDiagnosticsReportWaitingResourceSaturation(t *testing.T) {
	mission := &AgentMissionDesktopStatus{
		State: "running",
		ResourceLimits: AgentResourceLimits{
			MaxQueued:            8,
			ModelInference:       1,
			ReadCPU:              2,
			Build:                1,
			ExclusiveIntegration: 1,
		},
		Scheduler: AgentSchedulerSnapshot{
			Queued:  1,
			Running: 1,
			Resources: []AgentResourceSnapshot{
				{Class: AgentResourceModelInference, Limit: 1, InUse: 1, Available: 0},
			},
			Tasks: []AgentTaskScheduleSnapshot{
				{TaskID: "running", State: AgentTaskRunning, ResourceClass: AgentResourceModelInference, Running: true},
				{TaskID: "waiting", State: AgentTaskReady, ResourceClass: AgentResourceModelInference, QueuePosition: 1},
			},
		},
	}
	diagnostics := agentOrchestrationDiagnostics(readyDiagnosticStatus(), mission)
	resource := diagnosticResource(t, diagnostics, AgentResourceModelInference)
	if !resource.AtCapacity || !resource.Saturated || resource.Waiting != 1 {
		t.Fatalf("saturation diagnostics = %#v", resource)
	}
	if diagnostics.WaitingForModelInference != 1 || diagnostics.LogicalReady != 1 || diagnostics.LogicalRunning != 1 {
		t.Fatalf("logical diagnostics = %#v", diagnostics)
	}
	if diagnostics.State != AgentOrchestrationSaturated || diagnostics.Reason != AgentOrchestrationReasonResourceWaiting {
		t.Fatalf("expected resource saturation, got %#v", diagnostics)
	}
}

func TestAgentOrchestrationDiagnosticsReportQueueLimitSeparately(t *testing.T) {
	mission := &AgentMissionDesktopStatus{
		State: "running",
		ResourceLimits: AgentResourceLimits{
			MaxQueued:            2,
			ModelInference:       2,
			ReadCPU:              2,
			Build:                1,
			ExclusiveIntegration: 1,
		},
		Scheduler: AgentSchedulerSnapshot{Queued: 2},
	}
	diagnostics := agentOrchestrationDiagnostics(readyDiagnosticStatus(), mission)
	if !diagnostics.Queue.AtLimit || diagnostics.Queue.Limit != 2 || diagnostics.Queue.Available != 0 || diagnostics.Queue.FillPercent != 100 {
		t.Fatalf("queue diagnostics = %#v", diagnostics.Queue)
	}
	if diagnostics.State != AgentOrchestrationSaturated || diagnostics.Reason != AgentOrchestrationReasonQueueLimitReached {
		t.Fatalf("expected queue saturation, got %#v", diagnostics)
	}
}

func TestAgentOrchestrationDiagnosticsReadyWhenIdle(t *testing.T) {
	diagnostics := agentOrchestrationDiagnostics(readyDiagnosticStatus(), nil)
	if diagnostics.State != AgentOrchestrationReady || diagnostics.Reason != AgentOrchestrationReasonIdle || diagnostics.MissionActive {
		t.Fatalf("idle diagnostics = %#v", diagnostics)
	}
	resource := diagnosticResource(t, diagnostics, AgentResourceModelInference)
	if resource.Limit != defaultAgentResourceLimits().ModelInference || resource.InUse != 0 || resource.AtCapacity || resource.Saturated {
		t.Fatalf("idle model resource diagnostics = %#v", resource)
	}
}

func TestStatusJSONAlwaysIncludesMachineReadableOrchestrationDiagnostics(t *testing.T) {
	status := readyDiagnosticStatus()
	status.RunID = "diagnostics-no-mission"
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Orchestration AgentOrchestrationDiagnostics `json:"orchestration"`
		Mission       json.RawMessage                `json:"mission"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Orchestration.State != AgentOrchestrationReady || payload.Orchestration.Reason != AgentOrchestrationReasonIdle {
		t.Fatalf("status orchestration diagnostics = %#v", payload.Orchestration)
	}
	if len(payload.Mission) != 0 {
		t.Fatalf("unrelated status unexpectedly included mission payload: %s", payload.Mission)
	}
}
