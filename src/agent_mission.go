// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
		role, roleErr := normalizeAgentRole(proposal.Role)
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
		role, roleErr := normalizeAgentRole(string(graph.Tasks[i].Role))
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
		s.journalMissionSchedulerSnapshot(executionRunID, snapshot)
	}
	run, runErr := s.runScheduledReadOnlyAgentGraphWithExecutorAndCheckpoint(project, cfg, &graph, scheduler, budgetedExecute, checkpoint)
	stopMissionStatus()
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		cancelUnfinishedReadOnlyMissionTasks(&graph)
		run.Snapshot = scheduler.Snapshot(&graph, run.UsageByTask)
		s.journalMissionSchedulerSnapshot(executionRunID, run.Snapshot)
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
