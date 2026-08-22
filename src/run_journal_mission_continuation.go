// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var (
	errMissionRecoveryContinuationUnavailable = errors.New("mission recovery continuation unavailable")
	errMissionRecoveryContinuationCandidate   = errors.New("mission recovery task is not a continuation candidate")
	errMissionRecoveryContinuationBudget      = errors.New("mission recovery continuation budget exhausted")
)

type MissionRecoveryContinuationMaterialization struct {
	RunID                   string                `json:"run_id"`
	MissionID               string                `json:"mission_id"`
	TaskID                  string                `json:"task_id"`
	Action                  string                `json:"action"`
	DurableState            AgentTaskState        `json:"durable_state"`
	ObservedAt              time.Time             `json:"observed_at"`
	JournalSHA256           string                `json:"journal_sha256"`
	SnapshotSHA256          string                `json:"snapshot_sha256"`
	Project                 string                `json:"project"`
	Model                   string                `json:"model"`
	MissionBudget           AgentBudget           `json:"mission_budget"`
	MissionBudgetSnapshot   AgentBudgetSnapshot   `json:"mission_budget_snapshot"`
	HistoricalUsageByTask   map[string]AgentUsage `json:"historical_usage_by_task"`
	HistoricalChildWorkMS   int64                 `json:"historical_child_work_millis"`
	Graph                   AgentTaskGraph        `json:"graph"`
	RequiresNewAttempt      bool                  `json:"requires_new_attempt"`
	ReadOnly                bool                  `json:"read_only"`
	ExecutionAuthorized     bool                  `json:"execution_authorized"`
	SchedulerLeaseGranted   bool                  `json:"scheduler_lease_granted"`
	PersistentStateModified bool                  `json:"persistent_state_modified"`
}

func missionRecoveryUsageValid(u AgentUsage) bool {
	return u.ModelCalls >= 0 && u.ToolCalls >= 0 && u.EstimatedTokens >= 0 && u.ElapsedMillis >= 0
}

func missionRecoveryUsageZero(u AgentUsage) bool {
	return u.ModelCalls == 0 && u.ToolCalls == 0 && u.EstimatedTokens == 0 && u.ElapsedMillis == 0
}

func missionRecoveryUsageEqual(a, b AgentUsage) bool {
	return a.ModelCalls == b.ModelCalls && a.ToolCalls == b.ToolCalls && a.EstimatedTokens == b.EstimatedTokens && a.ElapsedMillis == b.ElapsedMillis
}

func missionRecoveryAcceptedTaskUsage(task MissionRecoveryTaskState) (AgentUsage, error) {
	explicit := task.Usage
	if !missionRecoveryUsageValid(explicit) {
		return AgentUsage{}, fmt.Errorf("task %q has invalid persisted usage", task.ID)
	}
	attempted := task.Lifecycle != nil && task.Lifecycle.AttemptCount > 0
	if task.BudgetSnapshot == nil {
		if attempted {
			return AgentUsage{}, fmt.Errorf("task %q has attempts without budget-snapshot usage evidence", task.ID)
		}
		return explicit, nil
	}
	budgetUsage := task.BudgetSnapshot.Usage
	if !missionRecoveryUsageValid(budgetUsage) {
		return AgentUsage{}, fmt.Errorf("task %q has invalid budget-snapshot usage", task.ID)
	}
	if !missionRecoveryUsageZero(explicit) && !missionRecoveryUsageEqual(explicit, budgetUsage) {
		return AgentUsage{}, fmt.Errorf("task %q has conflicting persisted usage evidence", task.ID)
	}
	if !missionRecoveryUsageZero(explicit) {
		return explicit, nil
	}
	return budgetUsage, nil
}

func missionRecoveryHistoricalUsage(mission *MissionRecoveryState) (map[string]AgentUsage, AgentUsage, int64, error) {
	if mission == nil {
		return nil, AgentUsage{}, 0, errMissionRecoveryContinuationUnavailable
	}
	byTask := make(map[string]AgentUsage, len(mission.Tasks))
	total := AgentUsage{}
	var childWorkMillis int64
	for _, task := range mission.Tasks {
		usage, err := missionRecoveryAcceptedTaskUsage(task)
		if err != nil {
			return nil, AgentUsage{}, 0, err
		}
		if missionRecoveryUsageZero(usage) {
			continue
		}
		byTask[task.ID] = usage
		total.ModelCalls += usage.ModelCalls
		total.ToolCalls += usage.ToolCalls
		total.EstimatedTokens += usage.EstimatedTokens
		childWorkMillis += usage.ElapsedMillis
		if !missionRecoveryUsageValid(total) || childWorkMillis < 0 {
			return nil, AgentUsage{}, 0, fmt.Errorf("mission historical usage overflow")
		}
	}
	total.ElapsedMillis = childWorkMillis
	return byTask, total, childWorkMillis, nil
}

func missionRecoveryContinuationRole(task MissionRecoveryTaskState) (AgentRole, error) {
	switch task.Role {
	case AgentRoleExplorer, AgentRolePlanner, AgentRoleReviewer:
		return task.Role, nil
	default:
		return "", fmt.Errorf("task %q has non-executable recovery role %q", task.ID, task.Role)
	}
}

func missionRecoveryTransitionByID(plan MissionRecoveryTransitionPlan, taskID string) (MissionRecoveryTaskTransition, bool) {
	for _, transition := range plan.Tasks {
		if transition.TaskID == taskID {
			return transition, true
		}
	}
	return MissionRecoveryTaskTransition{}, false
}

func missionRecoveryContinuationClosure(plan MissionRecoveryTransitionPlan, taskID string) (map[string]struct{}, error) {
	included := map[string]struct{}{taskID: {}}
	visiting := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("recovery continuation dependency cycle at %q", id)
		}
		transition, ok := missionRecoveryTransitionByID(plan, id)
		if !ok {
			return fmt.Errorf("recovery transition for task %q is missing", id)
		}
		visiting[id] = true
		defer delete(visiting, id)
		for _, dependencyID := range transition.Dependencies {
			dependency, ok := missionRecoveryTransitionByID(plan, dependencyID)
			if !ok || dependency.Action != missionRecoveryTransitionReuseVerified {
				return fmt.Errorf("dependency %q is not currently verified reusable", dependencyID)
			}
			included[dependencyID] = struct{}{}
			if err := visit(dependencyID); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(taskID); err != nil {
		return nil, err
	}
	return included, nil
}

func materializeMissionRecoveryContinuation(state *RunRecoveryState, journalFingerprint string, snapshot MissionRecoveryControlSnapshot, taskID string) (MissionRecoveryContinuationMaterialization, error) {
	var out MissionRecoveryContinuationMaterialization
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || state == nil || state.Terminal || state.Mission == nil || state.Mission.Kind != missionRecoveryKindReadOnly {
		return out, errMissionRecoveryContinuationUnavailable
	}
	mission := state.Mission
	if state.Phase != "mission-read-only" || mission.State != missionRecoveryRunning || state.RunID != snapshot.RunID || mission.MissionID != snapshot.MissionID || snapshot.Plan.MissionID != mission.MissionID {
		return out, errMissionRecoveryContinuationUnavailable
	}
	if !validMissionVerificationDigest(journalFingerprint) || !validMissionVerificationDigest(snapshot.SnapshotSHA256) || !snapshot.ReadOnly || snapshot.ExecutionAuthorized || snapshot.SchedulerLeaseGranted || snapshot.PersistentStateModified {
		return out, errMissionRecoveryContinuationUnavailable
	}
	if !snapshot.Plan.Valid || snapshot.ReconciliationState != missionReconcileMatched || snapshot.Plan.ReconciliationState != missionReconcileMatched {
		return out, errMissionRecoveryContinuationCandidate
	}
	if filepath.Clean(state.Project) != filepath.Clean(mission.Project) || strings.TrimSpace(state.Model) == "" || strings.TrimSpace(state.Model) != strings.TrimSpace(mission.Model) {
		return out, errMissionRecoveryContinuationUnavailable
	}
	if err := validateAgentMissionBudget(mission.Budget); err != nil {
		return out, err
	}
	transition, ok := missionRecoveryTransitionByID(snapshot.Plan, taskID)
	if !ok || !transition.RequiresNewAttempt || (transition.Action != missionRecoveryTransitionResumeCandidate && transition.Action != missionRecoveryTransitionRetryCandidate) {
		return out, errMissionRecoveryContinuationCandidate
	}
	if transition.AttemptCount >= missionRecoveryMaxTaskAttempts || snapshot.Plan.ObservedMissionAttempts >= snapshot.Plan.MaxMissionAttempts {
		return out, errMissionRecoveryContinuationCandidate
	}
	closure, err := missionRecoveryContinuationClosure(snapshot.Plan, taskID)
	if err != nil {
		return out, fmt.Errorf("%w: %v", errMissionRecoveryContinuationCandidate, err)
	}

	graph := AgentTaskGraph{MissionID: mission.MissionID, Tasks: make([]AgentTask, 0, len(closure))}
	for _, durable := range mission.Tasks {
		if _, include := closure[durable.ID]; !include {
			continue
		}
		role, roleErr := missionRecoveryContinuationRole(durable)
		if roleErr != nil {
			return out, roleErr
		}
		if err := validateReadOnlyMissionRequestedCapabilities(durable.ID, role, durable.RequestedCapabilities); err != nil {
			return out, err
		}
		if err := validateAgentMissionBudget(durable.Budget); err != nil {
			return out, fmt.Errorf("task %q: %w", durable.ID, err)
		}
		if strings.TrimSpace(durable.Model) == "" || strings.TrimSpace(durable.Model) != strings.TrimSpace(mission.Model) {
			return out, fmt.Errorf("task %q model does not match recovered mission model", durable.ID)
		}
		taskState := AgentTaskSucceeded
		if durable.ID == taskID {
			taskState = AgentTaskReady
		}
		graph.Tasks = append(graph.Tasks, AgentTask{
			ID:                    durable.ID,
			ParentID:              durable.ParentID,
			MissionID:             mission.MissionID,
			Role:                  role,
			Objective:             durable.Objective,
			Dependencies:          append([]string(nil), durable.Dependencies...),
			State:                 taskState,
			Workspace:             mission.Project,
			RequestedCapabilities: append([]AgentCapability(nil), durable.RequestedCapabilities...),
			Capabilities:          append([]AgentCapability(nil), capabilitiesForAgentRole(role)...),
			Model:                 mission.Model,
			Budget:                durable.Budget,
		})
	}
	if len(graph.Tasks) != len(closure) {
		return out, fmt.Errorf("recovery continuation dependency closure is incomplete")
	}
	if err := validateAgentTaskGraph(graph); err != nil {
		return out, fmt.Errorf("invalid recovery continuation graph: %w", err)
	}
	if err := reconcileAgentTaskGraph(&graph); err != nil {
		return out, fmt.Errorf("reconcile recovery continuation graph: %w", err)
	}
	for _, task := range graph.Tasks {
		if task.ID == taskID {
			if task.State != AgentTaskReady {
				return out, fmt.Errorf("recovery continuation candidate %q is not ready", taskID)
			}
			continue
		}
		if task.State != AgentTaskSucceeded {
			return out, fmt.Errorf("recovery continuation dependency %q is not preserved as verified success", task.ID)
		}
	}

	historicalByTask, historicalUsage, childWorkMillis, err := missionRecoveryHistoricalUsage(mission)
	if err != nil {
		return out, err
	}
	if mission.StartedAt.IsZero() || snapshot.ObservedAt.Before(mission.StartedAt) {
		return out, fmt.Errorf("invalid recovered mission timing evidence")
	}
	budgetUsage := historicalUsage
	budgetUsage.ElapsedMillis = snapshot.ObservedAt.Sub(mission.StartedAt).Milliseconds()
	budgetSnapshot := agentBudgetSnapshot(mission.Budget, budgetUsage)
	if budgetSnapshot.Exhausted {
		return out, fmt.Errorf("%w: %s", errMissionRecoveryContinuationBudget, budgetSnapshot.ExhaustedBy)
	}
	return MissionRecoveryContinuationMaterialization{
		RunID:                   state.RunID,
		MissionID:               mission.MissionID,
		TaskID:                  taskID,
		Action:                  transition.Action,
		DurableState:            transition.DurableState,
		ObservedAt:              snapshot.ObservedAt,
		JournalSHA256:           journalFingerprint,
		SnapshotSHA256:          snapshot.SnapshotSHA256,
		Project:                 mission.Project,
		Model:                   mission.Model,
		MissionBudget:           mission.Budget,
		MissionBudgetSnapshot:   budgetSnapshot,
		HistoricalUsageByTask:   historicalByTask,
		HistoricalChildWorkMS:   childWorkMillis,
		Graph:                   graph,
		RequiresNewAttempt:      true,
		ReadOnly:                true,
		ExecutionAuthorized:     false,
		SchedulerLeaseGranted:   false,
		PersistentStateModified: false,
	}, nil
}

func buildStableMissionRecoveryContinuationWithObserver(runID, taskID string, observe func(string, time.Time) MissionProjectBaseline) (MissionRecoveryContinuationMaterialization, error) {
	if observe == nil {
		observe = observeMissionRecoveryControlProject
	}
	for attempt := 0; attempt < missionRecoveryControlMaxSnapshotAttempts; attempt++ {
		state, fingerprint, err := loadMissionRecoveryControlState(runID)
		if err != nil {
			return MissionRecoveryContinuationMaterialization{}, err
		}
		observedAt := time.Now()
		current := observe(state.Mission.Project, observedAt)
		snapshot, err := buildMissionRecoveryControlSnapshot(state, fingerprint, current, observedAt)
		if err != nil {
			return MissionRecoveryContinuationMaterialization{}, err
		}
		materialized, err := materializeMissionRecoveryContinuation(state, fingerprint, snapshot, taskID)
		if err != nil {
			return MissionRecoveryContinuationMaterialization{}, err
		}
		_, currentFingerprint, err := loadMissionRecoveryControlState(runID)
		if err != nil {
			return MissionRecoveryContinuationMaterialization{}, err
		}
		if currentFingerprint == fingerprint {
			return materialized, nil
		}
	}
	return MissionRecoveryContinuationMaterialization{}, errMissionRecoveryControlChanged
}

type missionRecoveryContinuationBuilder func(string, string) (MissionRecoveryContinuationMaterialization, error)

func missionRecoveryContinuationForAppState(state *AppState, runID, taskID string, build missionRecoveryContinuationBuilder) (MissionRecoveryContinuationMaterialization, error) {
	if state == nil {
		return MissionRecoveryContinuationMaterialization{}, errMissionRecoveryContinuationUnavailable
	}
	if build == nil {
		build = func(runID, taskID string) (MissionRecoveryContinuationMaterialization, error) {
			return buildStableMissionRecoveryContinuationWithObserver(runID, taskID, nil)
		}
	}
	state.mu.RLock()
	running := state.Running
	state.mu.RUnlock()
	if running {
		return MissionRecoveryContinuationMaterialization{}, errMissionRecoveryControlActiveRun
	}
	materialized, err := build(runID, taskID)
	if err != nil {
		return MissionRecoveryContinuationMaterialization{}, err
	}
	state.mu.RLock()
	running = state.Running
	state.mu.RUnlock()
	if running {
		return MissionRecoveryContinuationMaterialization{}, errMissionRecoveryControlActiveRun
	}
	return materialized, nil
}

// MissionRecoveryContinuationMaterialization reconstructs a bounded in-memory
// graph for one currently eligible resume/retry candidate plus only its verified
// reusable dependency closure. It performs no child/model execution, does not
// reserve a Scheduler lease, writes no journal state and is not an execution
// authorization. A later dispatch path must recompute this materialization at
// its own atomic admission boundary rather than trust a previously returned one.
func (state *AppState) MissionRecoveryContinuationMaterialization(runID, taskID string) (MissionRecoveryContinuationMaterialization, error) {
	return missionRecoveryContinuationForAppState(state, runID, taskID, nil)
}
