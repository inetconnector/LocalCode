// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const maxReadOnlyMissionTasks = 64

var errAgentMissionBusy = errors.New("agent is already running")

type AgentReadOnlyMissionRequest struct {
	MissionID       string              `json:"mission_id"`
	ParentTaskID    string              `json:"parent_task_id,omitempty"`
	Objective       string              `json:"objective,omitempty"`
	Project         string              `json:"project"`
	Model           string              `json:"model,omitempty"`
	Constraints     []string            `json:"constraints,omitempty"`
	SuccessCriteria []string            `json:"success_criteria,omitempty"`
	Budget          AgentBudget         `json:"budget,omitempty"`
	Tasks           []AgentTaskProposal `json:"tasks"`
}

type AgentReadOnlyMissionResult struct {
	MissionID         string                 `json:"mission_id"`
	Project           string                 `json:"project"`
	Model             string                 `json:"model"`
	State             AgentMissionState      `json:"state"`
	Reason            AgentMissionReason     `json:"reason"`
	BudgetExhaustedBy string                 `json:"budget_exhausted_by,omitempty"`
	Accounting        AgentMissionAccounting `json:"accounting"`
	Graph             AgentTaskGraph         `json:"graph"`
	Run               AgentScheduledRun      `json:"run"`
}

// RunReadOnlyMission is the explicit governance entry above Planner/DAG/Scheduler.
// It does not execute Planner output merely because a Planner proposed it. The
// caller must submit a concrete mission request; LocalCode validates the graph,
// executable roles, requested capability envelope and project boundary before
// granting the fixed read-only runtime capabilities required by each known role.
func (s *AppState) RunReadOnlyMission(ctx context.Context, req AgentReadOnlyMissionRequest) (AgentReadOnlyMissionResult, error) {
	if s == nil {
		return AgentReadOnlyMissionResult{}, errors.New("app state is nil")
	}
	return s.runReadOnlyMissionWithExecutor(ctx, req, s.runNativeReadOnlyAgentTask)
}

func (s *AppState) runReadOnlyMissionWithExecutor(ctx context.Context, req AgentReadOnlyMissionRequest, execute scheduledReadOnlyAgentExecutor) (AgentReadOnlyMissionResult, error) {
	result := AgentReadOnlyMissionResult{}
	if s == nil {
		return result, errors.New("app state is nil")
	}
	if execute == nil {
		return result, errors.New("read-only mission executor is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := validateAgentMissionBudget(req.Budget); err != nil {
		return result, err
	}

	s.mu.RLock()
	cfg := s.Config
	selectedModel := strings.TrimSpace(s.Model)
	s.mu.RUnlock()

	missionID := strings.TrimSpace(req.MissionID)
	if err := validateAgentTaskIdentifier(missionID, "mission id"); err != nil {
		return result, err
	}
	if len(req.Tasks) == 0 {
		return result, errors.New("read-only mission has no tasks")
	}
	if len(req.Tasks) > maxReadOnlyMissionTasks {
		return result, fmt.Errorf("read-only mission has %d tasks; maximum is %d", len(req.Tasks), maxReadOnlyMissionTasks)
	}

	project, err := validateReadOnlyMissionProject(cfg, req.Project)
	if err != nil {
		return result, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = selectedModel
	}
	if model == "" {
		model = strings.TrimSpace(cfg.LastModel)
	}
	if model == "" {
		model = strings.TrimSpace(cfg.OllamaDefaultModel)
	}
	if model == "" {
		return result, errors.New("no model selected")
	}

	for _, proposal := range req.Tasks {
		role, roleErr := normalizeReadOnlyMissionRole(proposal.Role)
		if roleErr != nil {
			return result, fmt.Errorf("task %q is not executable in a read-only mission: %w", strings.TrimSpace(proposal.ID), roleErr)
		}
		if capErr := validateReadOnlyMissionRequestedCapabilities(proposal.ID, role, proposal.Capabilities); capErr != nil {
			return result, capErr
		}
	}

	graph, err := buildAgentTaskGraph(missionID, strings.TrimSpace(req.ParentTaskID), req.Tasks)
	if err != nil {
		return result, err
	}
	for i := range graph.Tasks {
		role, roleErr := normalizeReadOnlyMissionRole(string(graph.Tasks[i].Role))
		if roleErr != nil {
			return result, fmt.Errorf("task %q is not executable in a read-only mission: %w", graph.Tasks[i].ID, roleErr)
		}
		graph.Tasks[i].Role = role
		graph.Tasks[i].Capabilities = append([]AgentCapability(nil), capabilitiesForAgentRole(role)...)
		graph.Tasks[i].Model = model
	}

	missionCtx, cancel := context.WithCancel(ctx)
	executionRunID := newID()
	started := time.Now()
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		cancel()
		return result, errAgentMissionBusy
	}
	s.Running = true
	s.Cancel = cancel
	// MissionID is stable product identity. RunID is an execution-scoped token
	// used by the shared active-run controls/journal hooks and must never be a
	// caller-selected identifier that could collide with a stale journal run.
	s.RunID = executionRunID
	s.RunPhase = "mission-read-only"
	s.RunStartedAt = started
	s.LastProgressAt = started
	s.Project = project
	s.Model = model
	s.mu.Unlock()

	s.beginMissionRunJournal(executionRunID, req, graph, project, model, started)

	defer func() {
		cancel()
		s.mu.Lock()
		// ForceStopAgent deliberately changes RunID. Do not let a late mission
		// completion resurrect or otherwise rewrite a force-stopped UI state.
		if s.RunID == executionRunID {
			s.Running = false
			s.Cancel = nil
			s.RunPhase = "idle"
			s.LastProgressAt = time.Now()
		}
		s.mu.Unlock()
	}()

	tracker := newAgentMissionBudgetTracker(req.Budget, started)
	budgetedExecute := func(childCtx context.Context, childProject string, childCfg Config, task AgentTask) (AgentResult, error) {
		constrained, allowed := tracker.prepareTask(task)
		if !allowed {
			return AgentResult{
				Status:  AgentResultBudgetExhausted,
				Summary: "Mission budget exhausted before task: " + tracker.blockedDimension(),
			}, nil
		}
		childResult, childErr := execute(childCtx, childProject, childCfg, constrained)
		tracker.recordObservedUsage(childResult.Usage)
		return childResult, childErr
	}

	scheduler := NewAgentScheduler(missionCtx, AgentResourceLimits{})
	defer scheduler.missionCancel()
	stopMissionStatus := startAgentMissionDesktopStatusMonitor(executionRunID, missionID, project, model, started, scheduler, &graph, tracker)
	checkpoint := func(snapshot AgentSchedulerSnapshot) {
		s.journalMissionSchedulerCheckpoint(executionRunID, snapshot, &graph)
	}
	run, runErr := s.runScheduledReadOnlyAgentGraphWithExecutorAndCheckpoint(project, cfg, &graph, scheduler, budgetedExecute, checkpoint)
	stopMissionStatus()
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		cancelUnfinishedReadOnlyMissionTasks(&graph)
		run.Snapshot = scheduler.Snapshot(&graph, run.UsageByTask)
		s.journalMissionSchedulerCheckpoint(executionRunID, run.Snapshot, &graph)
	}
	finished := time.Now()
	accounting := agentMissionAccounting(req.Budget, run.UsageByTask, started, finished)
	state, reason, budgetExhaustedBy := deriveAgentMissionOutcome(graph, run, runErr, accounting, tracker)

	result = AgentReadOnlyMissionResult{
		MissionID:         missionID,
		Project:           project,
		Model:             model,
		State:             state,
		Reason:            reason,
		BudgetExhaustedBy: budgetExhaustedBy,
		Accounting:        accounting,
		Graph:             graph,
		Run:               run,
	}
	publishFinalAgentMissionDesktopStatus(executionRunID, result, started, finished)
	s.finishMissionRunJournal(executionRunID, result, fmt.Sprintf("%s:%s", state, reason))
	return result, runErr
}

func validateReadOnlyMissionProject(cfg Config, requested string) (string, error) {
	root := strings.TrimSpace(cfg.RootProjectDir)
	if root == "" {
		return "", errors.New("project root is not configured")
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", errors.New("read-only mission project is required")
	}
	project, err := directProjectFolder(root, requested)
	if err != nil {
		return "", err
	}
	if projectListContains(cfg.HiddenProjects, project) {
		return "", errors.New("read-only mission project is hidden or archived")
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return "", errors.New("read-only mission project directory not found")
	}
	return project, nil
}

func validateReadOnlyMissionRequestedCapabilities(taskID string, role AgentRole, requested []AgentCapability) error {
	allowed := make(map[AgentCapability]struct{})
	for _, capability := range capabilitiesForAgentRole(role) {
		allowed[capability] = struct{}{}
	}
	seen := make(map[AgentCapability]struct{}, len(requested))
	for _, capability := range requested {
		if _, duplicate := seen[capability]; duplicate {
			return fmt.Errorf("task %q requests duplicate capability %q", strings.TrimSpace(taskID), capability)
		}
		seen[capability] = struct{}{}
		if _, ok := allowed[capability]; !ok {
			return fmt.Errorf("task %q requests capability %q outside the read-only %s role envelope", strings.TrimSpace(taskID), capability, role)
		}
	}
	return nil
}

func normalizeReadOnlyMissionRole(raw string) (AgentRole, error) {
	role, err := normalizeAgentRole(raw)
	if err != nil {
		return "", err
	}
	switch role {
	case AgentRoleExplorer, AgentRolePlanner, AgentRoleReviewer:
		return role, nil
	default:
		return "", fmt.Errorf("role %q is not executable in a read-only mission", raw)
	}
}

type AgentMutationMissionRequest struct {
	MissionID       string              `json:"mission_id"`
	ParentTaskID    string              `json:"parent_task_id,omitempty"`
	Objective       string              `json:"objective,omitempty"`
	Project         string              `json:"project"`
	Model           string              `json:"model,omitempty"`
	TargetBranch    string              `json:"target_branch,omitempty"`
	Authorized      bool                `json:"authorized"`
	Limits          AgentResourceLimits `json:"limits,omitempty"`
	Constraints     []string            `json:"constraints,omitempty"`
	SuccessCriteria []string            `json:"success_criteria,omitempty"`
	Budget          AgentBudget         `json:"budget,omitempty"`
	Tasks           []AgentTaskProposal `json:"tasks"`
}

type AgentMutationMissionResult struct {
	MissionID         string                        `json:"mission_id"`
	Project           string                        `json:"project"`
	Model             string                        `json:"model"`
	State             AgentMissionState             `json:"state"`
	Reason            AgentMissionReason            `json:"reason"`
	BudgetExhaustedBy string                        `json:"budget_exhausted_by,omitempty"`
	Accounting        AgentMissionAccounting        `json:"accounting"`
	Graph             AgentTaskGraph                `json:"graph"`
	Run               AgentScheduledRun             `json:"run"`
	Integrations      map[string]*IntegrationResult `json:"integrations,omitempty"`
	ReplanRecords     []*ReplanRecord               `json:"replan_records,omitempty"`
	ReplanHaltReason  string                        `json:"replan_halt_reason,omitempty"`
}

type governedMutationTaskExecutor func(ctx context.Context, workspaceRoot string, cfg Config, task AgentTask) (AgentResult, error)

func resolveMutationMissionResourceLimits(requested AgentResourceLimits) AgentResourceLimits {
	defaults := AgentResourceLimits{
		MaxQueued:            256,
		ModelInference:       4,
		ReadCPU:              4,
		Build:                2,
		ExclusiveIntegration: 1,
	}
	if requested.MaxQueued <= 0 {
		requested.MaxQueued = defaults.MaxQueued
	}
	if requested.ModelInference <= 0 {
		requested.ModelInference = defaults.ModelInference
	}
	if requested.ReadCPU <= 0 {
		requested.ReadCPU = defaults.ReadCPU
	}
	if requested.Build <= 0 {
		requested.Build = defaults.Build
	}
	requested.ExclusiveIntegration = 1
	return normalizeAgentResourceLimits(requested)
}

// RunGovernedMutationMission is the explicitly authorized, governed mutation
// swarm entry point. Unlike RunReadOnlyMission (which stays strictly read-only
// and fail-closed), it wires together:
//   - AgentFactory role profiles & capability sanitization,
//   - Concurrent DAG scheduling via runScheduledAgentGraphConcurrent,
//   - Isolated Git worktrees (CreateAgentWorktree / CollectAgentWorktreeResult /
//     CommitAgentWorktreeChanges / deferred CleanupAgentWorktree) for builders,
//   - Serialized non-fast-forward merges via IntegrateBuilderWorktree, and
//   - Bounded automatic replanning with stagnation guard via MissionReplanner.
func (s *AppState) RunGovernedMutationMission(ctx context.Context, req AgentMutationMissionRequest) (AgentMutationMissionResult, error) {
	if s == nil {
		return AgentMutationMissionResult{}, errors.New("app state is nil")
	}
	defaultExecute := func(childCtx context.Context, workspaceRoot string, childCfg Config, task AgentTask) (AgentResult, error) {
		switch task.Role {
		case AgentRoleBuilder:
			return s.runNativeBuilderAgentTask(childCtx, workspaceRoot, childCfg, task)
		default:
			return s.runNativeReadOnlyAgentTask(childCtx, workspaceRoot, childCfg, task)
		}
	}
	return s.runGovernedMutationMissionWithExecutor(ctx, req, defaultExecute)
}

func (s *AppState) runGovernedMutationMissionWithExecutor(ctx context.Context, req AgentMutationMissionRequest, execute governedMutationTaskExecutor) (AgentMutationMissionResult, error) {
	result := AgentMutationMissionResult{
		Integrations: map[string]*IntegrationResult{},
	}
	if s == nil {
		return result, errors.New("app state is nil")
	}
	if !req.Authorized {
		return result, errors.New("governed mutation mission requires explicit authorization")
	}
	if execute == nil {
		return result, errors.New("mutation mission executor is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := validateAgentMissionBudget(req.Budget); err != nil {
		return result, err
	}

	s.mu.RLock()
	cfg := s.Config
	selectedModel := strings.TrimSpace(s.Model)
	s.mu.RUnlock()

	missionID := strings.TrimSpace(req.MissionID)
	if err := validateAgentTaskIdentifier(missionID, "mission id"); err != nil {
		return result, err
	}
	if len(req.Tasks) == 0 {
		return result, errors.New("mutation mission has no tasks")
	}
	if len(req.Tasks) > maxMissionTasksLimit {
		return result, fmt.Errorf("mutation mission has %d tasks; maximum is %d", len(req.Tasks), maxMissionTasksLimit)
	}

	project, err := validateReadOnlyMissionProject(cfg, req.Project)
	if err != nil {
		return result, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = selectedModel
	}
	if model == "" {
		model = strings.TrimSpace(cfg.LastModel)
	}
	if model == "" {
		model = strings.TrimSpace(cfg.OllamaDefaultModel)
	}
	if model == "" {
		return result, errors.New("no model selected")
	}

	graph, err := buildAgentTaskGraph(missionID, strings.TrimSpace(req.ParentTaskID), req.Tasks)
	if err != nil {
		return result, err
	}

	factory := NewAgentFactory()
	replanner := NewMissionReplanner(factory)
	for i := range graph.Tasks {
		governedRole, quarantined, reason := factory.MapDynamicRoleToGovernedRole(string(graph.Tasks[i].Role))
		graph.Tasks[i].Role = governedRole
		if quarantined {
			graph.Tasks[i].Capabilities = factory.GovernedCapabilitiesForTask(AgentRoleExplorer, nil)
			graph.Tasks[i].StateReason = reason
		} else {
			graph.Tasks[i].Capabilities = factory.GovernedCapabilitiesForTask(governedRole, graph.Tasks[i].RequestedCapabilities)
		}
		graph.Tasks[i].Model = model
		graph.Tasks[i].Workspace = project
		graph.Tasks[i].Budget = defaultAgentBudget(governedRole, cfg)
	}

	missionCtx, cancel := context.WithCancel(ctx)
	executionRunID := newID()
	started := time.Now()
	s.mu.Lock()
	if s.Running {
		s.mu.Unlock()
		cancel()
		return result, errAgentMissionBusy
	}
	s.Running = true
	s.Cancel = cancel
	s.RunID = executionRunID
	s.RunPhase = "mission-mutation"
	s.RunStartedAt = started
	s.LastProgressAt = started
	s.Project = project
	s.Model = model
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		if s.RunID == executionRunID {
			s.Running = false
			s.Cancel = nil
			s.RunPhase = "idle"
			s.LastProgressAt = time.Now()
		}
		s.mu.Unlock()
	}()

	var (
		wtMu                sync.Mutex
		worktreesByTask     = map[string]*AgentWorktreeWorkspace{}
		allWorktrees        []*AgentWorktreeWorkspace
		integratedWorktrees = map[string]bool{}
		integrations        = map[string]*IntegrationResult{}

		topoMu   sync.RWMutex
		depsByID = map[string][]string{}
	)

	snapshotTopology := func(g *AgentTaskGraph) {
		topoMu.Lock()
		defer topoMu.Unlock()
		depsByID = make(map[string][]string, len(g.Tasks))
		for _, t := range g.Tasks {
			depsByID[t.ID] = append([]string(nil), t.Dependencies...)
		}
	}

	// Collect ancestor builder worktrees in deterministic dependency order.
	findAncestorWorktrees := func(taskID string, onlyUnintegrated bool) []*AgentWorktreeWorkspace {
		topoMu.RLock()
		depsSnapshot := make(map[string][]string, len(depsByID))
		for k, v := range depsByID {
			depsSnapshot[k] = v
		}
		topoMu.RUnlock()

		visited := map[string]bool{}
		var orderedBuilderIDs []string
		var walk func(id string)
		walk = func(id string) {
			for _, dep := range depsSnapshot[id] {
				if visited[dep] {
					continue
				}
				visited[dep] = true
				walk(dep)
				orderedBuilderIDs = append(orderedBuilderIDs, dep)
			}
		}
		walk(taskID)

		wtMu.Lock()
		defer wtMu.Unlock()
		var out []*AgentWorktreeWorkspace
		for _, ancID := range orderedBuilderIDs {
			wt, ok := worktreesByTask[ancID]
			if !ok || wt == nil || !wt.Active {
				continue
			}
			if onlyUnintegrated && integratedWorktrees[wt.ID] {
				continue
			}
			out = append(out, wt)
		}
		return out
	}

	// Always clean up every worktree created during this mission, even on
	// failure or cancellation.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		wtMu.Lock()
		toClean := append([]*AgentWorktreeWorkspace(nil), allWorktrees...)
		wtMu.Unlock()
		for _, wt := range toClean {
			if wt != nil && wt.Active {
				_ = CleanupAgentWorktree(cleanupCtx, wt, true, cfg)
			}
		}
	}()

	tracker := newAgentMissionBudgetTracker(req.Budget, started)
	governedExecute := func(childCtx context.Context, _ string, childCfg Config, task AgentTask) (AgentResult, error) {
		constrained, allowed := tracker.prepareTask(task)
		if !allowed {
			return AgentResult{
				Status:  AgentResultBudgetExhausted,
				Summary: "Mission budget exhausted before task: " + tracker.blockedDimension(),
			}, nil
		}

		var (
			childResult AgentResult
			childErr    error
		)

		switch constrained.Role {
		case AgentRoleBuilder:
			wt, wtErr := CreateAgentWorktree(childCtx, project, missionID, constrained.ID, "", childCfg)
			if wtErr != nil {
				childResult = AgentResult{
					Status:  AgentResultBlocked,
					Summary: "Failed to create isolated builder worktree: " + wtErr.Error(),
				}
				childErr = wtErr
				break
			}
			wtMu.Lock()
			allWorktrees = append(allWorktrees, wt)
			worktreesByTask[constrained.ID] = wt
			wtMu.Unlock()

			builderTask := constrained
			builderTask.Workspace = wt.WorktreePath
			builderTask.Worktree = wt.WorktreePath
			childResult, childErr = execute(childCtx, wt.WorktreePath, childCfg, builderTask)

			if childErr != nil || scheduledAgentTaskTerminalState(childResult, childErr) != AgentTaskSucceeded {
				wtMu.Lock()
				delete(worktreesByTask, constrained.ID)
				wtMu.Unlock()
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
				_ = CleanupAgentWorktree(cleanupCtx, wt, true, childCfg)
				cleanupCancel()
				break
			}

			collected, collectErr := CollectAgentWorktreeResult(childCtx, wt, childCfg)
			if collectErr != nil {
				childResult = AgentResult{
					Status:  AgentResultBlocked,
					Summary: "Failed to collect builder worktree result: " + collectErr.Error(),
					Usage:   childResult.Usage,
				}
				childErr = collectErr
				break
			}
			commitMsg := fmt.Sprintf("Builder %s: %s", constrained.ID, truncateText(strings.TrimSpace(constrained.Objective), 72))
			commitSHA, committedFiles, commitErr := CommitAgentWorktreeChanges(childCtx, wt, commitMsg, childCfg)
			if commitErr != nil {
				childResult = AgentResult{
					Status:  AgentResultBlocked,
					Summary: "Failed to commit builder worktree changes: " + commitErr.Error(),
					Usage:   childResult.Usage,
				}
				childErr = commitErr
				break
			}
			childResult.ChangedFiles = appendUniqueFilePaths(childResult.ChangedFiles, collected.ChangedFiles...)
			childResult.ChangedFiles = appendUniqueFilePaths(childResult.ChangedFiles, committedFiles...)
			if commitSHA != "" {
				childResult.Commits = appendUniqueFilePaths(childResult.Commits, commitSHA)
			}

		case AgentRoleIntegrator:
			targets := findAncestorWorktrees(constrained.ID, true)
			if len(targets) == 0 {
				childResult = AgentResult{
					Status:  AgentResultCompleted,
					Summary: "No pending builder worktrees to integrate.",
				}
				break
			}
			var allChanged []string
			var allCommits []string
			for _, wt := range targets {
				intRes, intErr := IntegrateBuilderWorktree(childCtx, wt, req.TargetBranch, childCfg)
				wtMu.Lock()
				if intRes != nil {
					integrations[wt.TaskID] = intRes
					integrations[constrained.ID] = intRes
				}
				wtMu.Unlock()
				if intErr != nil {
					detail := intErr.Error()
					if intRes != nil && strings.TrimSpace(intRes.Detail) != "" {
						detail = intRes.Detail
					}
					childResult = AgentResult{
						Status:  AgentResultBlocked,
						Summary: fmt.Sprintf("Integration failed for builder task %s: %s", wt.TaskID, detail),
					}
					childErr = intErr
					break
				}
				wtMu.Lock()
				integratedWorktrees[wt.ID] = true
				wtMu.Unlock()
				if intRes != nil {
					allChanged = appendUniqueFilePaths(allChanged, intRes.ChangedFiles...)
					if intRes.MergedCommit != "" {
						allCommits = appendUniqueFilePaths(allCommits, intRes.MergedCommit)
					}
				}
			}
			if childErr == nil {
				childResult = AgentResult{
					Status:       AgentResultCompleted,
					Summary:      fmt.Sprintf("Integrated %d builder worktree(s) sequentially via --no-ff.", len(targets)),
					ChangedFiles: allChanged,
					Commits:      allCommits,
				}
			}

		case AgentRoleTestAgent:
			testRoot := project
			if anc := findAncestorWorktrees(constrained.ID, false); len(anc) > 0 {
				testRoot = anc[len(anc)-1].WorktreePath
			}
			testTask := constrained
			testTask.Workspace = testRoot
			childResult, childErr = execute(childCtx, testRoot, childCfg, testTask)
			decision, proposal := EvaluateTestResults(req.SuccessCriteria, childResult.Tests, childErr)
			if decision == DecisionRepair && proposal != nil {
				childResult.Status = AgentResultBlocked
				if strings.TrimSpace(childResult.Summary) == "" {
					childResult.Summary = proposal.Summary
				}
			}

		default:
			childResult, childErr = execute(childCtx, project, childCfg, constrained)
		}

		tracker.recordObservedUsage(childResult.Usage)
		return childResult, childErr
	}

	limits := resolveMutationMissionResourceLimits(req.Limits)
	scheduler := NewAgentScheduler(missionCtx, limits)
	defer scheduler.missionCancel()

	cumulativeRun := AgentScheduledRun{
		MissionID:   missionID,
		UsageByTask: map[string]AgentUsage{},
	}
	replannedFailedTasks := map[string]bool{}
	var replanRecords []*ReplanRecord
	replanHaltReason := ""
	var runErr error

	for {
		snapshotTopology(&graph)
		passRun, passErr := s.runScheduledAgentGraphConcurrent(project, cfg, &graph, scheduler, governedExecute, nil, cumulativeRun.UsageByTask)
		cumulativeRun.Results = append(cumulativeRun.Results, passRun.Results...)
		cumulativeRun.UsageByTask = passRun.UsageByTask
		cumulativeRun.Snapshot = passRun.Snapshot
		runErr = passErr

		if passErr != nil || tracker.blockedDimension() != "" {
			break
		}

		// Find any newly failed builder/test task eligible for bounded replanning.
		var failedTask *AgentTask
		for i := range graph.Tasks {
			candidate := &graph.Tasks[i]
			if candidate.State != AgentTaskFailed || replannedFailedTasks[candidate.ID] {
				continue
			}
			if candidate.Role == AgentRoleBuilder || candidate.Role == AgentRoleTestAgent {
				failedTask = candidate
				break
			}
		}
		if failedTask == nil {
			break
		}

		replannedFailedTasks[failedTask.ID] = true
		reason := strings.TrimSpace(failedTask.Result.Summary)
		if reason == "" {
			reason = strings.TrimSpace(failedTask.StateReason)
		}
		if reason == "" {
			reason = fmt.Sprintf("task %s failed", failedTask.ID)
		}
		_, proposal := EvaluateTestResults(req.SuccessCriteria, failedTask.Result.Tests, nil)
		if proposal == nil {
			proposal = &RepairProposal{
				Summary:       reason,
				AffectedPaths: append([]string(nil), failedTask.Result.ChangedFiles...),
			}
		}

		newGraph, record, replanErr := replanner.ReplanFailedTask(missionCtx, &graph, failedTask.ID, reason, proposal, cfg)
		if replanErr != nil {
			replanHaltReason = replanErr.Error()
			break
		}
		replanRecords = append(replanRecords, record)
		for i := range newGraph.Tasks {
			if strings.TrimSpace(newGraph.Tasks[i].MissionID) == "" {
				newGraph.Tasks[i].MissionID = missionID
			}
			if strings.TrimSpace(newGraph.Tasks[i].Model) == "" {
				newGraph.Tasks[i].Model = model
			}
			if strings.TrimSpace(newGraph.Tasks[i].Workspace) == "" {
				newGraph.Tasks[i].Workspace = project
			}
		}
		if err := reconcileAgentTaskGraph(newGraph); err != nil {
			replanHaltReason = err.Error()
			break
		}
		graph = *newGraph
	}

	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		cancelUnfinishedReadOnlyMissionTasks(&graph)
		cumulativeRun.Snapshot = scheduler.Snapshot(&graph, cumulativeRun.UsageByTask)
	}

	finished := time.Now()
	accounting := agentMissionAccounting(req.Budget, cumulativeRun.UsageByTask, started, finished)
	state, reason, budgetExhaustedBy := deriveMutationMissionOutcome(graph, cumulativeRun, runErr, accounting, tracker, replanRecords, replanHaltReason)

	wtMu.Lock()
	integrationsCopy := make(map[string]*IntegrationResult, len(integrations))
	for k, v := range integrations {
		integrationsCopy[k] = v
	}
	wtMu.Unlock()

	result = AgentMutationMissionResult{
		MissionID:         missionID,
		Project:           project,
		Model:             model,
		State:             state,
		Reason:            reason,
		BudgetExhaustedBy: budgetExhaustedBy,
		Accounting:        accounting,
		Graph:             graph,
		Run:               cumulativeRun,
		Integrations:      integrationsCopy,
		ReplanRecords:     replanRecords,
		ReplanHaltReason:  replanHaltReason,
	}
	return result, runErr
}

func deriveMutationMissionOutcome(
	graph AgentTaskGraph,
	run AgentScheduledRun,
	runErr error,
	accounting AgentMissionAccounting,
	tracker *agentMissionBudgetTracker,
	replanRecords []*ReplanRecord,
	replanHaltReason string,
) (AgentMissionState, AgentMissionReason, string) {
	if blockedBy := tracker.blockedDimension(); blockedBy != "" {
		return AgentMissionBudgetExhausted, AgentMissionReasonBudgetExhausted, blockedBy
	}
	if accounting.Budget.Exhausted && missionBudgetStoppedAcceptedTask(run, tracker, accounting.Budget.ExhaustedBy) {
		return AgentMissionBudgetExhausted, AgentMissionReasonBudgetExhausted, accounting.Budget.ExhaustedBy
	}
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			return AgentMissionCancelled, AgentMissionReasonCancelled, ""
		}
		return AgentMissionFailed, AgentMissionReasonRuntimeError, ""
	}
	if strings.TrimSpace(replanHaltReason) != "" {
		return AgentMissionFailed, AgentMissionReasonTaskFailed, ""
	}

	superseded := make(map[string]bool, len(replanRecords)*3)
	for _, rec := range replanRecords {
		if rec == nil {
			continue
		}
		superseded[rec.OriginalTaskID] = true
	}

	allSucceeded := len(graph.Tasks) > 0
	for _, task := range graph.Tasks {
		if isSuccessfulAgentTaskState(task.State) {
			continue
		}
		// Historical failed task that was superseded by a repair cycle (or
		// unexecuted tail of an earlier superseded repair cycle).
		if superseded[task.ID] && task.State == AgentTaskFailed {
			continue
		}
		if task.State == AgentTaskCancelled && strings.HasPrefix(task.StateReason, "Superseded by repair cycle ") {
			continue
		}
		allSucceeded = false
		if task.State == AgentTaskCancelled {
			return AgentMissionCancelled, AgentMissionReasonCancelled, ""
		}
		if task.Result.Status == AgentResultBudgetExhausted {
			return AgentMissionFailed, AgentMissionReasonTaskBudgetExhausted, ""
		}
		if task.State == AgentTaskFailed || task.State == AgentTaskBlocked {
			return AgentMissionFailed, AgentMissionReasonTaskFailed, ""
		}
	}
	if allSucceeded {
		return AgentMissionSucceeded, AgentMissionReasonCompleted, ""
	}
	return AgentMissionFailed, AgentMissionReasonIncomplete, ""
}
