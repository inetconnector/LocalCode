// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newReadOnlyMissionTestState(t *testing.T) (*AppState, string, string) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Mission fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &AppState{
		Config: Config{
			RootProjectDir:     root,
			LastModel:          "test-model",
			OllamaDefaultModel: "test-model",
		},
		Model: "test-model",
	}
	return state, root, project
}

func validReadOnlyMissionRequest(project string) AgentReadOnlyMissionRequest {
	return AgentReadOnlyMissionRequest{
		MissionID: "mission-read-only-1",
		Project:   project,
		Model:     "test-model",
		Tasks: []AgentTaskProposal{
			{
				ID:           "explore",
				Role:         "explorer",
				Objective:    "Inspect the fixture.",
				Capabilities: []AgentCapability{AgentCapabilityRepositoryRead},
			},
			{
				ID:           "review",
				Role:         "reviewer",
				Objective:    "Review the fixture evidence.",
				Dependencies: []string{"explore"},
			},
		},
	}
}

func TestReadOnlyMissionRunsValidatedGraphAndGrantsOnlyRoleCapabilities(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	mission, err := state.RunReadOnlyMission(context.Background(), validReadOnlyMissionRequest(project))
	if err != nil {
		t.Fatal(err)
	}
	if mission.MissionID != "mission-read-only-1" || mission.Project != project || mission.Model != "test-model" {
		t.Fatalf("unexpected mission metadata: %+v", mission)
	}
	if len(mission.Run.Results) != 2 {
		t.Fatalf("results=%d want=2: %+v", len(mission.Run.Results), mission.Run)
	}
	if mission.Run.Results[0].TaskID != "explore" || mission.Run.Results[1].TaskID != "review" {
		t.Fatalf("unexpected deterministic mission order: %+v", mission.Run.Results)
	}
	for _, task := range mission.Graph.Tasks {
		if task.State != AgentTaskSucceeded || task.Result.Status != AgentResultFallback {
			t.Fatalf("task did not finish through deterministic fallback: %+v", task)
		}
		want := capabilitiesForAgentRole(task.Role)
		if len(task.Capabilities) != len(want) {
			t.Fatalf("task %s granted capabilities=%v want=%v", task.ID, task.Capabilities, want)
		}
		for i := range want {
			if task.Capabilities[i] != want[i] {
				t.Fatalf("task %s granted capabilities=%v want=%v", task.ID, task.Capabilities, want)
			}
		}
	}
	explorer := agentTaskByID(&mission.Graph, "explore")
	if explorer == nil || len(explorer.RequestedCapabilities) != 1 || explorer.RequestedCapabilities[0] != AgentCapabilityRepositoryRead {
		t.Fatalf("planner-requested capability data was not preserved separately: %+v", explorer)
	}
	state.mu.RLock()
	running, phase, cancel := state.Running, state.RunPhase, state.Cancel
	state.mu.RUnlock()
	if running || phase != "idle" || cancel != nil {
		t.Fatalf("mission did not release global run state: running=%v phase=%q cancel=%v", running, phase, cancel != nil)
	}
}

func TestReadOnlyMissionRejectsUnknownRoleBeforeExecution(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Tasks = []AgentTaskProposal{{ID: "build", Role: "builder", Objective: "Mutate the repository."}}
	calls := 0
	_, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(context.Context, string, Config, AgentTask) (AgentResult, error) {
		calls++
		return AgentResult{Status: AgentResultCompleted}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "not executable in a read-only mission") {
		t.Fatalf("unknown mutation role error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("rejected mutation role executed %d child calls", calls)
	}
}

func TestReadOnlyMissionRejectsCapabilityEscalationBeforeExecution(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Tasks[0].Capabilities = append(req.Tasks[0].Capabilities, AgentCapability("filesystem-write"))
	calls := 0
	_, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(context.Context, string, Config, AgentTask) (AgentResult, error) {
		calls++
		return AgentResult{Status: AgentResultCompleted}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "outside the read-only explorer role envelope") {
		t.Fatalf("capability escalation error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("rejected capability escalation executed %d child calls", calls)
	}
}

func TestReadOnlyMissionRejectsInvalidGraphAndProjectBoundary(t *testing.T) {
	state, root, project := newReadOnlyMissionTestState(t)

	cycle := validReadOnlyMissionRequest(project)
	cycle.Tasks = []AgentTaskProposal{
		{ID: "a", Role: "explorer", Objective: "a", Dependencies: []string{"b"}},
		{ID: "b", Role: "reviewer", Objective: "b", Dependencies: []string{"a"}},
	}
	if _, err := state.RunReadOnlyMission(context.Background(), cycle); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error=%v", err)
	}

	outside := validReadOnlyMissionRequest(filepath.Join(t.TempDir(), "outside"))
	if _, err := state.RunReadOnlyMission(context.Background(), outside); err == nil {
		t.Fatal("mission outside configured project root was accepted")
	}

	nested := filepath.Join(project, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	nestedReq := validReadOnlyMissionRequest(nested)
	if _, err := state.RunReadOnlyMission(context.Background(), nestedReq); err == nil || !strings.Contains(err.Error(), "direct project folders") {
		t.Fatalf("nested project error=%v", err)
	}

	state.Config.HiddenProjects = []string{filepath.Join(root, "demo")}
	if _, err := state.RunReadOnlyMission(context.Background(), validReadOnlyMissionRequest(project)); err == nil || !strings.Contains(err.Error(), "hidden or archived") {
		t.Fatalf("hidden project error=%v", err)
	}
}

func TestReadOnlyMissionRespectsGlobalBusyState(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	state.Running = true
	calls := 0
	_, err := state.runReadOnlyMissionWithExecutor(context.Background(), validReadOnlyMissionRequest(project), func(context.Context, string, Config, AgentTask) (AgentResult, error) {
		calls++
		return AgentResult{Status: AgentResultCompleted}, nil
	})
	if !errors.Is(err, errAgentMissionBusy) {
		t.Fatalf("busy mission error=%v", err)
	}
	if calls != 0 {
		t.Fatalf("busy mission executed %d child calls", calls)
	}
}

func TestReadOnlyMissionStopAgentCancelsScheduledChildAndReleasesRunState(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Tasks = req.Tasks[:1]

	started := make(chan struct{})
	type outcome struct {
		mission AgentReadOnlyMissionResult
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		mission, err := state.runReadOnlyMissionWithExecutor(context.Background(), req, func(ctx context.Context, _ string, _ Config, _ AgentTask) (AgentResult, error) {
			close(started)
			<-ctx.Done()
			return AgentResult{Usage: AgentUsage{ModelCalls: 1}}, ctx.Err()
		})
		done <- outcome{mission: mission, err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("mission child did not start")
	}
	if !state.StopAgent() {
		t.Fatal("StopAgent did not see the active read-only mission")
	}

	var got outcome
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("read-only mission did not stop after cancellation")
	}
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("mission error=%v want context.Canceled", got.err)
	}
	if len(got.mission.Run.Results) != 0 || len(got.mission.Run.UsageByTask) != 0 {
		t.Fatalf("cancelled mission accepted late child output: %+v", got.mission.Run)
	}
	task := agentTaskByID(&got.mission.Graph, "explore")
	if task == nil || task.State != AgentTaskCancelled || task.Result.Status != "" {
		t.Fatalf("cancelled mission task=%+v", task)
	}
	state.mu.RLock()
	running, phase, cancel := state.Running, state.RunPhase, state.Cancel
	state.mu.RUnlock()
	if running || phase != "idle" || cancel != nil {
		t.Fatalf("cancelled mission leaked global run state: running=%v phase=%q cancel=%v", running, phase, cancel != nil)
	}
}

func TestReadOnlyMissionRejectsOversizedTaskSetAndPreCancelledContext(t *testing.T) {
	state, _, project := newReadOnlyMissionTestState(t)
	req := validReadOnlyMissionRequest(project)
	req.Tasks = make([]AgentTaskProposal, maxReadOnlyMissionTasks+1)
	for i := range req.Tasks {
		req.Tasks[i] = AgentTaskProposal{ID: "task-" + strings.Repeat("x", i%3+1) + "-" + string(rune('a'+i%26)), Role: "explorer", Objective: "bounded"}
	}
	if _, err := state.RunReadOnlyMission(context.Background(), req); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("oversized mission error=%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := state.RunReadOnlyMission(ctx, validReadOnlyMissionRequest(project)); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-cancelled mission error=%v", err)
	}
}
