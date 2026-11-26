// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func createManagedTestGitRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "swarm-repo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", project).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	_ = exec.Command("git", "-C", project, "config", "user.name", "LocalCode Swarm Test").Run()
	_ = exec.Command("git", "-C", project, "config", "user.email", "swarm@localcode.local").Run()
	_ = exec.Command("git", "-C", project, "config", "commit.gpgsign", "false").Run()

	readme := filepath.Join(project, "README.md")
	if err := os.WriteFile(readme, []byte("# Swarm Repo\nbase line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", project, "add", "README.md").CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", project, "commit", "-m", "Initial commit").CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
	return root, project
}

func recordAtomicPeak32(peak *atomic.Int32, candidate int32) {
	for {
		current := peak.Load()
		if candidate <= current || peak.CompareAndSwap(current, candidate) {
			return
		}
	}
}

func TestAgentSchedulerResourceClassDifferentiation(t *testing.T) {
	cases := []struct {
		name string
		task AgentTask
		want AgentResourceClass
	}{
		{
			name: "integrator uses exclusive integration",
			task: AgentTask{Role: AgentRoleIntegrator, Objective: "merge builder branch"},
			want: AgentResourceIntegration,
		},
		{
			name: "test_agent uses build resource",
			task: AgentTask{Role: AgentRoleTestAgent, Objective: "run unit tests"},
			want: AgentResourceBuild,
		},
		{
			name: "deterministic preflight explorer uses read-cpu",
			task: AgentTask{Role: AgentRoleExplorer, Objective: deterministicSubagentTaskPrefix + "scan symbols"},
			want: AgentResourceReadCPU,
		},
		{
			name: "model=deterministic reviewer uses read-cpu",
			task: AgentTask{Role: AgentRoleReviewer, Objective: "static check", Model: "deterministic"},
			want: AgentResourceReadCPU,
		},
		{
			name: "model explorer uses model-inference",
			task: AgentTask{Role: AgentRoleExplorer, Objective: "inspect architecture", Model: "qwen2.5"},
			want: AgentResourceModelInference,
		},
		{
			name: "builder uses model-inference",
			task: AgentTask{Role: AgentRoleBuilder, Objective: "implement feature", Model: "qwen2.5"},
			want: AgentResourceModelInference,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resourceClassForAgentTask(tc.task); got != tc.want {
				t.Fatalf("resourceClassForAgentTask(%+v)=%q want %q", tc.task, got, tc.want)
			}
		})
	}

	// Verify mixed resource classes run concurrently even when ModelInference=1,
	// while ExclusiveIntegration=1 keeps two integrator tasks strictly serialized.
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "model-explore", Role: "explorer", Objective: "model exploration"},
		{ID: "cpu-scan", Role: "explorer", Objective: deterministicSubagentTaskPrefix + "static repo scan"},
		{ID: "build-test", Role: "test_agent", Objective: "run build verification"},
		{ID: "int-1", Role: "integrator", Objective: "merge branch 1"},
		{ID: "int-2", Role: "integrator", Objective: "merge branch 2"},
	})
	grantSchedulerTestCapabilities(t, &graph, "model-explore", "cpu-scan", "build-test", "int-1", "int-2")
	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{
		ModelInference:       1,
		ReadCPU:              2,
		Build:                1,
		ExclusiveIntegration: 1,
	})
	defer scheduler.missionCancel()

	var nonIntArrived atomic.Int32
	nonIntBarrier := make(chan struct{})
	var intInFlight atomic.Int32
	var intPeak atomic.Int32
	var totalInFlight atomic.Int32
	var totalPeak atomic.Int32

	execute := func(ctx context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		cur := totalInFlight.Add(1)
		recordAtomicPeak32(&totalPeak, cur)
		defer totalInFlight.Add(-1)

		if task.Role == AgentRoleIntegrator {
			ic := intInFlight.Add(1)
			recordAtomicPeak32(&intPeak, ic)
			defer intInFlight.Add(-1)
			time.Sleep(25 * time.Millisecond)
		} else {
			if nonIntArrived.Add(1) == 3 {
				close(nonIntBarrier)
			}
			select {
			case <-nonIntBarrier:
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return AgentResult{}, ctx.Err()
			}
		}
		return AgentResult{Status: AgentResultCompleted, Summary: task.ID + " ok"}, nil
	}

	run, err := (&AppState{}).runScheduledAgentGraphConcurrent(t.TempDir(), Config{}, &graph, scheduler, execute, nil, nil)
	if err != nil {
		t.Fatalf("concurrent mixed-class run failed: %v", err)
	}
	if len(run.Results) != 5 {
		t.Fatalf("results=%d want 5", len(run.Results))
	}
	if totalPeak.Load() < 3 {
		t.Fatalf("expected mixed resource classes to overlap concurrently, got peak=%d", totalPeak.Load())
	}
	if intPeak.Load() != 1 {
		t.Fatalf("integrator tasks must stay strictly serialized (ExclusiveIntegration=1), got peak=%d", intPeak.Load())
	}
}

func TestSwarmParallelSchedulerPeakInFlight(t *testing.T) {
	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "worker-1", Role: "explorer", Objective: "parallel task 1"},
		{ID: "worker-2", Role: "explorer", Objective: "parallel task 2"},
		{ID: "worker-3", Role: "reviewer", Objective: "parallel task 3"},
		{ID: "worker-4", Role: "planner", Objective: "parallel task 4"},
		{ID: "join", Role: "reviewer", Objective: "join all 4", Dependencies: []string{"worker-1", "worker-2", "worker-3", "worker-4"}},
	})
	grantSchedulerTestCapabilities(t, &graph, "worker-1", "worker-2", "worker-3", "worker-4", "join")

	scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{
		ModelInference: 4,
		MaxQueued:      32,
	})
	defer scheduler.missionCancel()

	var inFlight atomic.Int32
	var peak atomic.Int32
	var arrived atomic.Int32
	barrier := make(chan struct{})
	var barrierOnce sync.Once

	execute := func(ctx context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		current := inFlight.Add(1)
		recordAtomicPeak32(&peak, current)
		defer inFlight.Add(-1)

		if strings.HasPrefix(task.ID, "worker-") {
			if arrived.Add(1) >= 4 {
				barrierOnce.Do(func() { close(barrier) })
			}
			select {
			case <-barrier:
			case <-time.After(2 * time.Second):
				return AgentResult{Status: AgentResultBlocked, Summary: "parallel barrier timeout"}, errors.New("parallel barrier timeout")
			case <-ctx.Done():
				return AgentResult{}, ctx.Err()
			}
		}
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: task.ID + " completed",
			Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 25},
		}, nil
	}

	run, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutor(t.TempDir(), Config{}, &graph, scheduler, execute)
	if err != nil {
		t.Fatalf("parallel dispatch failed: %v", err)
	}
	if got := peak.Load(); got < 3 {
		t.Fatalf("peak_executor_inflight=%d want >= 3", got)
	}
	if len(run.Results) != 5 {
		t.Fatalf("results=%d want 5", len(run.Results))
	}
	if run.Results[len(run.Results)-1].TaskID != "join" {
		t.Fatalf("join task did not execute last: %+v", run.Results)
	}
	for _, id := range []string{"worker-1", "worker-2", "worker-3", "worker-4", "join"} {
		task := agentTaskByID(&graph, id)
		if task == nil || task.State != AgentTaskSucceeded {
			t.Fatalf("task %q state=%+v want succeeded", id, task)
		}
	}
	if run.Snapshot.Running != 0 || run.Snapshot.Queued != 0 {
		t.Fatalf("scheduler did not drain: %+v", run.Snapshot)
	}
	for _, res := range run.Snapshot.Resources {
		if res.InUse != 0 {
			t.Fatalf("resource leaked: %+v", res)
		}
	}
}

func TestSwarmParallelRaceCancellationZeroLeaks(t *testing.T) {
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()

	graph := schedulerTestGraph(t, []AgentTaskProposal{
		{ID: "fast-winner", Role: "explorer", Objective: "finishes before cancel"},
		{ID: "slow-1", Role: "explorer", Objective: "blocked until cancel"},
		{ID: "slow-2", Role: "reviewer", Objective: "blocked until cancel"},
		{ID: "slow-3", Role: "planner", Objective: "blocked until cancel"},
		{ID: "downstream", Role: "reviewer", Objective: "depends on slow tasks", Dependencies: []string{"slow-1", "slow-2"}},
	})
	grantSchedulerTestCapabilities(t, &graph, "fast-winner", "slow-1", "slow-2", "slow-3", "downstream")

	scheduler := NewAgentScheduler(parentCtx, AgentResourceLimits{ModelInference: 4})
	defer scheduler.missionCancel()

	var activeWorkers atomic.Int32
	var slowStarted atomic.Int32
	allSlowStarted := make(chan struct{})
	var allSlowOnce sync.Once
	fastDone := make(chan struct{})

	execute := func(ctx context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
		activeWorkers.Add(1)
		defer activeWorkers.Add(-1)

		if task.ID == "fast-winner" {
			return AgentResult{
				Status:  AgentResultCompleted,
				Summary: "completed before cancellation",
				Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 15},
			}, nil
		}

		if slowStarted.Add(1) >= 3 {
			allSlowOnce.Do(func() { close(allSlowStarted) })
		}
		<-ctx.Done()
		return AgentResult{
			Status:  AgentResultCompleted,
			Summary: "late partial output must be discarded",
			Usage:   AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 99},
		}, ctx.Err()
	}

	checkpoint := func(snap AgentSchedulerSnapshot) {
		for _, ts := range snap.Tasks {
			if ts.TaskID == "fast-winner" && ts.State == AgentTaskSucceeded {
				select {
				case <-fastDone:
				default:
					close(fastDone)
				}
			}
		}
	}

	done := make(chan scheduledDispatchTestOutcome, 1)
	go func() {
		run, err := (&AppState{}).runScheduledAgentGraphConcurrent(t.TempDir(), Config{}, &graph, scheduler, execute, checkpoint, nil)
		done <- scheduledDispatchTestOutcome{run: run, err: err}
	}()

	select {
	case <-allSlowStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("parallel workers did not start concurrently")
	}
	select {
	case <-fastDone:
	case <-time.After(3 * time.Second):
		t.Fatal("fast-winner did not finalize before cancellation")
	}

	cancelParent()

	var outcome scheduledDispatchTestOutcome
	select {
	case outcome = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("parallel dispatch did not return after cancellation")
	}

	if !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", outcome.err)
	}
	if active := activeWorkers.Load(); active != 0 {
		t.Fatalf("goroutine leak detected: %d worker goroutine(s) still active after return", active)
	}

	// Completion-first invariant: fast-winner stays succeeded with its result.
	winner := agentTaskByID(&graph, "fast-winner")
	if winner == nil || winner.State != AgentTaskSucceeded || winner.Result.Summary != "completed before cancellation" {
		t.Fatalf("completion-first task corrupted: %+v", winner)
	}

	// Cancellation-first invariant: slow tasks are cancelled and late results discarded.
	for _, id := range []string{"slow-1", "slow-2", "slow-3"} {
		task := agentTaskByID(&graph, id)
		if task == nil || task.State != AgentTaskCancelled {
			t.Fatalf("cancelled task %q state=%+v want cancelled", id, task)
		}
		if task.Result.Status != "" || task.Result.Summary != "" {
			t.Fatalf("late result leaked into cancelled task %q: %+v", id, task.Result)
		}
	}

	if len(outcome.run.Results) != 1 || outcome.run.Results[0].TaskID != "fast-winner" {
		t.Fatalf("unexpected run results after mid-flight cancel: %+v", outcome.run.Results)
	}

	snap := scheduler.Snapshot(&graph, outcome.run.UsageByTask)
	if snap.Running != 0 || snap.Queued != 0 {
		t.Fatalf("scheduler retained work after cancel: %+v", snap)
	}
	for _, res := range snap.Resources {
		if res.InUse != 0 {
			t.Fatalf("resource leaked after cancel: %+v", res)
		}
	}
}

func TestSubagentBuilderActionSchemaAndWorktreeContainment(t *testing.T) {
	// 1. Disallowed actions must always fail validation.
	for _, bad := range []nativeBuilderAction{
		{Action: "run_command", Message: "shell"},
		{Action: "git", Message: "git"},
		{Action: "delete_file", Path: "README.md", Message: "delete"},
		{Action: "web_search", Query: "x", Message: "web"},
		{Action: "mcp_call_tool", Name: "x", Message: "mcp"},
		{Action: "subagent_analyze", Message: "spawn"},
		{Action: "write_file", Message: "missing path"},
		{Action: "replace_text", Path: "a.go", Message: "missing old_text"},
	} {
		if err := validateNativeBuilderAction(bad); err == nil {
			t.Fatalf("expected invalid builder action to be rejected: %+v", bad)
		}
	}

	// 2. Allowed actions pass validation.
	for _, okAction := range []nativeBuilderAction{
		{Action: "list_files", Message: "list"},
		{Action: "read_file", Path: "README.md", Message: "read"},
		{Action: "search_text", Query: "Swarm", Message: "search"},
		{Action: "write_file", Path: "new.go", Content: "package main\n", Message: "write"},
		{Action: "replace_text", Path: "README.md", OldText: "base", NewText: "next", Message: "replace"},
		{Action: "finish", Message: "done", Result: &AgentResult{Summary: "built"}},
	} {
		if err := validateNativeBuilderAction(okAction); err != nil {
			t.Fatalf("valid builder action rejected %+v: %v", okAction, err)
		}
	}

	// 3. Running builder directly against main repo (outside managed worktrees root) MUST fail closed.
	_, project := createManagedTestGitRepo(t)
	cfg := defaultConfig()
	state := &AppState{Config: cfg}
	task := AgentTask{
		ID:           "builder-direct-escape",
		Role:         AgentRoleBuilder,
		Objective:    "try editing main repo directly",
		Capabilities: []AgentCapability{AgentCapabilityRepositoryRead, AgentCapabilityLSP, AgentCapabilityBuilderWorktree},
		Model:        "fake-builder-model",
	}
	if _, err := state.runNativeBuilderAgentTask(context.Background(), project, cfg, task); !errors.Is(err, errWorkspaceEscapeAttempted) {
		t.Fatalf("expected errWorkspaceEscapeAttempted for main repo path, got: %v", err)
	}

	// 4. Running builder inside a real managed Git worktree via mock Ollama edits only the worktree.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	wt, err := CreateAgentWorktree(ctx, project, "mission-builder-schema", "task-builder-1", "", cfg)
	if err != nil {
		t.Fatalf("CreateAgentWorktree failed: %v", err)
	}
	defer func() { _ = CleanupAgentWorktree(context.Background(), wt, true, cfg) }()

	// Path traversal and .git modification inside worktree must be blocked.
	if _, err := state.executeNativeBuilderAction(ctx, wt.WorktreePath, cfg, nativeBuilderAction{
		Action: "write_file", Path: "../escape.txt", Content: "bad", Message: "escape",
	}); !errors.Is(err, errWorkspaceEscapeAttempted) {
		t.Fatalf("expected path escape error, got: %v", err)
	}
	if _, err := state.executeNativeBuilderAction(ctx, wt.WorktreePath, cfg, nativeBuilderAction{
		Action: "write_file", Path: ".git/config", Content: "bad", Message: "git tamper",
	}); !errors.Is(err, errWorkspaceEscapeAttempted) {
		t.Fatalf("expected .git tamper error, got: %v", err)
	}

	var step atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := step.Add(1)
		var content string
		switch n {
		case 1:
			content = `{"action":"write_file","message":"Create module","path":"module.go","content":"package main\n\nfunc Module() string { return \"v1\" }\n"}`
		case 2:
			content = `{"action":"replace_text","message":"Upgrade module","path":"module.go","old_text":"\"v1\"","new_text":"\"v2\""}`
		default:
			content = `{"action":"finish","message":"Done","result":{"summary":"Created and updated module.go in worktree","changed_files":["module.go"]}}`
		}
		_ = json.NewEncoder(w).Encode(OllamaChatResponse{Message: OllamaMessage{Role: "assistant", Content: content}, Done: true})
	}))
	defer server.Close()

	state.Ollama = &OllamaClient{BaseURL: server.URL, HTTP: server.Client(), ContextLength: 8192}
	res, err := state.runNativeBuilderAgentTask(ctx, wt.WorktreePath, cfg, task)
	if err != nil {
		t.Fatalf("runNativeBuilderAgentTask failed: %v", err)
	}
	if res.Status != AgentResultCompleted {
		t.Fatalf("builder status=%q want completed; res=%+v", res.Status, res)
	}
	wtBytes, err := os.ReadFile(filepath.Join(wt.WorktreePath, "module.go"))
	if err != nil || !strings.Contains(string(wtBytes), `"v2"`) {
		t.Fatalf("worktree module.go missing expected edit: err=%v content=%q", err, string(wtBytes))
	}
	if _, err := os.Stat(filepath.Join(project, "module.go")); !os.IsNotExist(err) {
		t.Fatalf("main repo must remain untouched by builder worktree edits, stat err=%v", err)
	}
}

func TestSwarmGovernedMutationMissionMultiWorktreeAndReplanningE2E(t *testing.T) {
	t.Run("rejects unauthorized request and quarantines unknown roles", func(t *testing.T) {
		root, project := createManagedTestGitRepo(t)
		state := &AppState{
			Config: Config{RootProjectDir: root, LastModel: "test-model"},
			Model:  "test-model",
		}
		req := AgentMutationMissionRequest{
			MissionID:  "mission-unauth",
			Project:    project,
			Model:      "test-model",
			Authorized: false,
			Tasks:      []AgentTaskProposal{{ID: "b1", Role: "builder", Objective: "should not run"}},
		}
		if _, err := state.RunGovernedMutationMission(context.Background(), req); err == nil || !strings.Contains(err.Error(), "authorization") {
			t.Fatalf("expected authorization failure, got: %v", err)
		}

		// Unknown dynamic role must be quarantined to read-only explorer by AgentFactory.
		req.Authorized = true
		req.MissionID = "mission-quarantine"
		req.Tasks = []AgentTaskProposal{{
			ID:           "rogue",
			Role:         "shell_root_admin",
			Objective:    "try to escalate",
			Capabilities: []AgentCapability{AgentCapabilityBuilderWorktree, AgentCapabilityIntegration},
		}}
		var observedRole AgentRole
		var observedCaps []AgentCapability
		res, err := state.runGovernedMutationMissionWithExecutor(context.Background(), req, func(_ context.Context, _ string, _ Config, task AgentTask) (AgentResult, error) {
			observedRole = task.Role
			observedCaps = append([]AgentCapability(nil), task.Capabilities...)
			return AgentResult{Status: AgentResultCompleted, Summary: "quarantined read-only execution"}, nil
		})
		if err != nil {
			t.Fatalf("quarantined mission failed: %v", err)
		}
		if res.State != AgentMissionSucceeded {
			t.Fatalf("state=%q want succeeded", res.State)
		}
		if observedRole != AgentRoleExplorer {
			t.Fatalf("rogue role was not quarantined to explorer: got %q", observedRole)
		}
		for _, cap := range observedCaps {
			if cap == AgentCapabilityBuilderWorktree || cap == AgentCapabilityIntegration {
				t.Fatalf("quarantined task retained mutating capability %q", cap)
			}
		}
	})

	t.Run("two parallel builders in isolated worktrees merged sequentially via --no-ff and cleaned up", func(t *testing.T) {
		root, project := createManagedTestGitRepo(t)
		state := &AppState{
			Config: Config{RootProjectDir: root, LastModel: "test-model"},
			Model:  "test-model",
		}

		var builderInFlight atomic.Int32
		var builderPeak atomic.Int32
		var builderArrived atomic.Int32
		builderBarrier := make(chan struct{})
		var barrierOnce sync.Once

		var mu sync.Mutex
		worktreePaths := map[string]string{}

		req := AgentMutationMissionRequest{
			MissionID:  "mission-parallel-builders",
			Project:    project,
			Model:      "test-model",
			Authorized: true,
			Limits:     AgentResourceLimits{ModelInference: 4, ReadCPU: 4, Build: 2, ExclusiveIntegration: 1},
			Tasks: []AgentTaskProposal{
				{ID: "builder-alpha", Role: "builder", Objective: "Create alpha.go"},
				{ID: "builder-beta", Role: "builder", Objective: "Create beta.go"},
				{ID: "integrate-alpha", Role: "integrator", Objective: "Merge alpha", Dependencies: []string{"builder-alpha"}},
				{ID: "integrate-beta", Role: "integrator", Objective: "Merge beta", Dependencies: []string{"builder-beta"}},
			},
		}

		execFn := func(ctx context.Context, workspaceRoot string, _ Config, task AgentTask) (AgentResult, error) {
			if task.Role != AgentRoleBuilder {
				return AgentResult{Status: AgentResultCompleted, Summary: task.ID + " ok"}, nil
			}
			// Verify strict worktree isolation: NEVER main project dir!
			if filepath.Clean(workspaceRoot) == filepath.Clean(project) {
				return AgentResult{}, errors.New("builder received main repo instead of isolated worktree")
			}
			if err := validateWorktreePathContainment(workspaceRoot); err != nil {
				return AgentResult{}, err
			}

			cur := builderInFlight.Add(1)
			recordAtomicPeak32(&builderPeak, cur)
			defer builderInFlight.Add(-1)

			if builderArrived.Add(1) >= 2 {
				barrierOnce.Do(func() { close(builderBarrier) })
			}
			select {
			case <-builderBarrier:
			case <-time.After(5 * time.Second):
				return AgentResult{}, errors.New("parallel builder barrier timeout")
			case <-ctx.Done():
				return AgentResult{}, ctx.Err()
			}

			mu.Lock()
			worktreePaths[task.ID] = workspaceRoot
			mu.Unlock()

			fileName := "alpha.go"
			content := "package main\n\nfunc Alpha() string { return \"alpha\" }\n"
			if task.ID == "builder-beta" {
				fileName = "beta.go"
				content = "package main\n\nfunc Beta() string { return \"beta\" }\n"
			}
			if err := os.WriteFile(filepath.Join(workspaceRoot, fileName), []byte(content), 0o644); err != nil {
				return AgentResult{}, err
			}
			return AgentResult{
				Status:       AgentResultCompleted,
				Summary:      "Wrote " + fileName,
				ChangedFiles: []string{fileName},
				Usage:        AgentUsage{ModelCalls: 1, ToolCalls: 1, EstimatedTokens: 50},
			}, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		mission, err := state.runGovernedMutationMissionWithExecutor(ctx, req, execFn)
		if err != nil {
			t.Fatalf("mutation mission failed: %v", err)
		}
		if mission.State != AgentMissionSucceeded {
			t.Fatalf("mission state=%q reason=%q want succeeded; tasks=%+v", mission.State, mission.Reason, mission.Graph.Tasks)
		}
		if builderPeak.Load() < 2 {
			t.Fatalf("expected 2 parallel builders simultaneously, got peak=%d", builderPeak.Load())
		}
		if worktreePaths["builder-alpha"] == "" || worktreePaths["builder-beta"] == "" || worktreePaths["builder-alpha"] == worktreePaths["builder-beta"] {
			t.Fatalf("builders did not use distinct worktrees: %+v", worktreePaths)
		}

		// Both files must be merged into main repo.
		alphaBytes, err := os.ReadFile(filepath.Join(project, "alpha.go"))
		if err != nil || !strings.Contains(string(alphaBytes), `return "alpha"`) {
			t.Fatalf("alpha.go missing or corrupted in main repo: err=%v content=%q", err, string(alphaBytes))
		}
		betaBytes, err := os.ReadFile(filepath.Join(project, "beta.go"))
		if err != nil || !strings.Contains(string(betaBytes), `return "beta"`) {
			t.Fatalf("beta.go missing or corrupted in main repo: err=%v content=%q", err, string(betaBytes))
		}

		// Verify --no-ff merge commits exist in git history.
		mergeLogOut, err := exec.Command("git", "-C", project, "log", "--merges", "--oneline").CombinedOutput()
		if err != nil {
			t.Fatalf("git log --merges failed: %v\n%s", err, mergeLogOut)
		}
		mergeLines := strings.Split(strings.TrimSpace(string(mergeLogOut)), "\n")
		if len(mergeLines) < 2 {
			t.Fatalf("expected at least 2 --no-ff merge commits, got:\n%s", string(mergeLogOut))
		}

		// Verify all worktrees were cleanly removed by defer CleanupAgentWorktree.
		for id, wtPath := range worktreePaths {
			if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
				t.Fatalf("worktree for %s was not cleaned up at %s (stat err=%v)", id, wtPath, err)
			}
		}
		wtListOut, err := exec.Command("git", "-C", project, "worktree", "list", "--porcelain").CombinedOutput()
		if err != nil {
			t.Fatalf("git worktree list failed: %v\n%s", err, wtListOut)
		}
		if strings.Count(string(wtListOut), "worktree ") != 1 {
			t.Fatalf("expected only main repo in git worktree list after cleanup, got:\n%s", string(wtListOut))
		}
	})

	t.Run("intentional builder failure triggers MissionReplanner repair and successful integration", func(t *testing.T) {
		root, project := createManagedTestGitRepo(t)
		state := &AppState{
			Config: Config{RootProjectDir: root, LastModel: "test-model"},
			Model:  "test-model",
		}

		var repairWorktrees []string
		var wtMu sync.Mutex

		req := AgentMutationMissionRequest{
			MissionID:  "mission-replan-recover",
			Project:    project,
			Model:      "test-model",
			Authorized: true,
			Tasks: []AgentTaskProposal{
				{ID: "builder-flaky", Role: "builder", Objective: "Implement handler"},
				{ID: "integrator-main", Role: "integrator", Objective: "Merge handler", Dependencies: []string{"builder-flaky"}},
			},
		}

		execFn := func(_ context.Context, workspaceRoot string, _ Config, task AgentTask) (AgentResult, error) {
			switch task.Role {
			case AgentRoleBuilder:
				wtMu.Lock()
				repairWorktrees = append(repairWorktrees, workspaceRoot)
				wtMu.Unlock()
				if task.ID == "builder-flaky" {
					return AgentResult{
						Status:  AgentResultBlocked,
						Summary: "initial build failed: syntax error in handler.go",
					}, nil
				}
				// Replanned repair-builder succeeds in a fresh worktree.
				if err := os.WriteFile(filepath.Join(workspaceRoot, "handler.go"), []byte("package main\n\nfunc Handle() bool { return true }\n"), 0o644); err != nil {
					return AgentResult{}, err
				}
				return AgentResult{
					Status:       AgentResultCompleted,
					Summary:      "Repaired handler.go cleanly",
					ChangedFiles: []string{"handler.go"},
				}, nil
			case AgentRoleTestAgent:
				return AgentResult{
					Status:  AgentResultCompleted,
					Summary: "All tests pass",
					Tests:   []TestResult{{Name: "TestHandle", Status: "PASS"}},
				}, nil
			default:
				return AgentResult{Status: AgentResultCompleted, Summary: "Review approved"}, nil
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		mission, err := state.runGovernedMutationMissionWithExecutor(ctx, req, execFn)
		if err != nil {
			t.Fatalf("replan recovery mission failed: %v", err)
		}
		if mission.State != AgentMissionSucceeded {
			t.Fatalf("mission state=%q reason=%q halt=%q want succeeded", mission.State, mission.Reason, mission.ReplanHaltReason)
		}
		if len(mission.ReplanRecords) != 1 {
			t.Fatalf("replan records=%d want 1", len(mission.ReplanRecords))
		}
		if mission.ReplanRecords[0].OriginalTaskID != "builder-flaky" || mission.ReplanRecords[0].CycleNumber != 1 {
			t.Fatalf("unexpected replan record: %+v", mission.ReplanRecords[0])
		}
		data, err := os.ReadFile(filepath.Join(project, "handler.go"))
		if err != nil || !strings.Contains(string(data), "func Handle() bool") {
			t.Fatalf("repaired handler.go not integrated into main repo: err=%v content=%q", err, string(data))
		}
		for _, wtPath := range repairWorktrees {
			if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
				t.Fatalf("worktree %s was not cleaned up", wtPath)
			}
		}
	})

	t.Run("repeated identical failures trigger MissionReplanner stagnation halt", func(t *testing.T) {
		root, project := createManagedTestGitRepo(t)
		state := &AppState{
			Config: Config{RootProjectDir: root, LastModel: "test-model"},
			Model:  "test-model",
		}

		var builderAttempts atomic.Int32
		var createdWorktrees []string
		var wtMu sync.Mutex

		req := AgentMutationMissionRequest{
			MissionID:  "mission-replan-stagnation",
			Project:    project,
			Model:      "test-model",
			Authorized: true,
			Tasks: []AgentTaskProposal{
				{ID: "builder-stuck", Role: "builder", Objective: "Fix impossible bug"},
				{ID: "integrator-main", Role: "integrator", Objective: "Merge fix", Dependencies: []string{"builder-stuck"}},
			},
		}

		execFn := func(_ context.Context, workspaceRoot string, _ Config, task AgentTask) (AgentResult, error) {
			if task.Role == AgentRoleBuilder {
				builderAttempts.Add(1)
				wtMu.Lock()
				createdWorktrees = append(createdWorktrees, workspaceRoot)
				wtMu.Unlock()
				return AgentResult{
					Status:  AgentResultBlocked,
					Summary: "compile error: undefined symbol PersistentBug",
				}, nil
			}
			return AgentResult{Status: AgentResultCompleted, Summary: "ok"}, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		mission, err := state.runGovernedMutationMissionWithExecutor(ctx, req, execFn)
		if err != nil {
			t.Fatalf("unexpected runtime error: %v", err)
		}
		if mission.State != AgentMissionFailed {
			t.Fatalf("mission state=%q want failed on stagnation", mission.State)
		}
		if !strings.Contains(strings.ToLower(mission.ReplanHaltReason), "stagnation") {
			t.Fatalf("expected stagnation halt reason, got %q", mission.ReplanHaltReason)
		}
		if len(mission.ReplanRecords) != 2 {
			t.Fatalf("expected 2 replan cycles before 3rd identical failure triggers stagnation, got %d", len(mission.ReplanRecords))
		}
		if got := builderAttempts.Load(); got != 3 {
			t.Fatalf("builderAttempts=%d want 3 (original + 2 repair attempts before stagnation halt)", got)
		}
		for _, wtPath := range createdWorktrees {
			if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
				t.Fatalf("stagnated worktree %s was not cleaned up", wtPath)
			}
		}
	})
}
