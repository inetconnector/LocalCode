// SPDX-License-Identifier: Apache-2.0

package main

import "time"

const (
	missionRecoveryMaxTaskAttempts    = 3
	missionRecoveryMaxMissionAttempts = maxReadOnlyMissionTasks * missionRecoveryMaxTaskAttempts

	missionRecoveryTransitionReuseVerified         = "reuse_verified"
	missionRecoveryTransitionVerifyPostconditions  = "verify_postconditions"
	missionRecoveryTransitionResumeCandidate       = "resume_candidate"
	missionRecoveryTransitionRetryCandidate        = "retry_candidate"
	missionRecoveryTransitionInterruptedReview     = "interrupted_review_required"
	missionRecoveryTransitionPreserveTerminal      = "preserve_terminal"
	missionRecoveryTransitionBlockedReconciliation = "blocked_reconciliation"
	missionRecoveryTransitionBlockedDependency     = "blocked_dependency"
	missionRecoveryTransitionTaskAttemptLimit      = "task_attempt_limit_reached"
	missionRecoveryTransitionMissionAttemptLimit   = "mission_attempt_limit_reached"
	missionRecoveryTransitionInsufficientLifecycle = "insufficient_lifecycle_evidence"
	missionRecoveryTransitionInvalidRecoveryState  = "invalid_recovery_state"
)

type MissionRecoveryTaskTransition struct {
	TaskID             string         `json:"task_id"`
	DurableState       AgentTaskState `json:"durable_state"`
	Action             string         `json:"action"`
	Reason             string         `json:"reason"`
	AttemptCount       int            `json:"attempt_count"`
	RetryCount         int            `json:"retry_count"`
	RequiresNewAttempt bool           `json:"requires_new_attempt,omitempty"`
	Dependencies       []string       `json:"dependencies,omitempty"`
	BlockedBy          []string       `json:"blocked_by,omitempty"`
}

type MissionRecoveryTransitionPlan struct {
	MissionID               string                          `json:"mission_id"`
	ReconciliationState     string                          `json:"reconciliation_state"`
	ObservedAt              time.Time                       `json:"observed_at"`
	Valid                   bool                            `json:"valid"`
	InvalidReason           string                          `json:"invalid_reason,omitempty"`
	MaxTaskAttempts         int                             `json:"max_task_attempts"`
	MaxMissionAttempts      int                             `json:"max_mission_attempts"`
	ObservedMissionAttempts int                             `json:"observed_mission_attempts"`
	ReservedNewAttempts     int                             `json:"reserved_new_attempts"`
	Tasks                   []MissionRecoveryTaskTransition `json:"tasks"`
}

func missionRecoveryTaskAttemptCounts(task MissionRecoveryTaskState) (attempts, retries int, lifecycleKnown bool) {
	if task.Lifecycle == nil {
		return 0, 0, false
	}
	attempts = task.Lifecycle.AttemptCount
	retries = task.Lifecycle.RetryCount
	if attempts < 0 {
		attempts = 0
	}
	if retries < 0 {
		retries = 0
	}
	return attempts, retries, true
}

func missionRecoveryTaskLifecycleValid(task MissionRecoveryTaskState) bool {
	if task.Lifecycle == nil {
		return true
	}
	attempts := task.Lifecycle.AttemptCount
	retries := task.Lifecycle.RetryCount
	if attempts < 0 || attempts > missionRecoveryMaxTaskAttempts || retries < 0 {
		return false
	}
	expectedRetries := 0
	if attempts > 1 {
		expectedRetries = attempts - 1
	}
	return retries == expectedRetries
}

func missionRecoveryTransitionGraphValid(mission *MissionRecoveryState) bool {
	if mission == nil || len(mission.Tasks) == 0 || len(mission.Tasks) > maxReadOnlyMissionTasks {
		return false
	}
	graph := AgentTaskGraph{
		MissionID: mission.MissionID,
		Tasks:     make([]AgentTask, 0, len(mission.Tasks)),
	}
	for _, task := range mission.Tasks {
		if !missionRecoveryTaskLifecycleValid(task) {
			return false
		}
		graph.Tasks = append(graph.Tasks, AgentTask{
			ID:                    task.ID,
			ParentID:              task.ParentID,
			MissionID:             mission.MissionID,
			Role:                  task.Role,
			Objective:             task.Objective,
			Dependencies:          append([]string(nil), task.Dependencies...),
			State:                 task.State,
			RequestedCapabilities: append([]AgentCapability(nil), task.RequestedCapabilities...),
		})
	}
	return validateAgentTaskGraph(graph) == nil
}

func invalidMissionRecoveryTransitionPlan(mission *MissionRecoveryState, observedAt time.Time, reason string) MissionRecoveryTransitionPlan {
	plan := MissionRecoveryTransitionPlan{
		ObservedAt:         observedAt,
		Valid:              false,
		InvalidReason:      reason,
		MaxTaskAttempts:    missionRecoveryMaxTaskAttempts,
		MaxMissionAttempts: missionRecoveryMaxMissionAttempts,
	}
	if observedAt.IsZero() {
		plan.ObservedAt = time.Now()
	}
	if mission == nil {
		return plan
	}
	plan.MissionID = mission.MissionID
	if mission.Reconciliation != nil {
		plan.ReconciliationState = mission.Reconciliation.State
	}
	plan.Tasks = make([]MissionRecoveryTaskTransition, 0, len(mission.Tasks))
	for _, task := range mission.Tasks {
		attempts, retries, _ := missionRecoveryTaskAttemptCounts(task)
		plan.ObservedMissionAttempts += attempts
		plan.Tasks = append(plan.Tasks, MissionRecoveryTaskTransition{
			TaskID:       task.ID,
			DurableState: task.State,
			Action:       missionRecoveryTransitionInvalidRecoveryState,
			Reason:       reason,
			AttemptCount: attempts,
			RetryCount:   retries,
			Dependencies: append([]string(nil), task.Dependencies...),
		})
	}
	return plan
}

func initialMissionRecoveryTaskTransition(task MissionRecoveryTaskState, reconciliationState string) MissionRecoveryTaskTransition {
	attempts, retries, lifecycleKnown := missionRecoveryTaskAttemptCounts(task)
	transition := MissionRecoveryTaskTransition{
		TaskID:       task.ID,
		DurableState: task.State,
		AttemptCount: attempts,
		RetryCount:   retries,
		Dependencies: append([]string(nil), task.Dependencies...),
	}

	if task.Running || task.State == AgentTaskRunning {
		transition.Action = missionRecoveryTransitionInterruptedReview
		transition.Reason = "running_at_interruption_requires_explicit_review"
		return transition
	}

	if task.State == AgentTaskCancelled {
		transition.Action = missionRecoveryTransitionPreserveTerminal
		transition.Reason = "cancelled_state_is_preserved"
		return transition
	}

	if reconciliationState != missionReconcileMatched {
		if task.State == AgentTaskFailed {
			transition.Action = missionRecoveryTransitionPreserveTerminal
			transition.Reason = "failed_state_preserved_until_reconciliation_matches"
			return transition
		}
		transition.Action = missionRecoveryTransitionBlockedReconciliation
		transition.Reason = "current_project_git_reconciliation_not_matched"
		return transition
	}

	switch task.State {
	case AgentTaskSucceeded, AgentTaskCompleted:
		if task.CompletionEvidence != nil && task.CompletionEvidence.VerificationState == missionVerificationVerified {
			transition.Action = missionRecoveryTransitionReuseVerified
			transition.Reason = "durable_success_verified_against_current_match"
			return transition
		}
		transition.Action = missionRecoveryTransitionVerifyPostconditions
		if task.CompletionEvidence != nil && task.CompletionEvidence.VerificationState == missionVerificationFailed {
			transition.Reason = "verification_failed_requires_recheck"
		} else {
			transition.Reason = "durable_success_requires_verification"
		}
		return transition

	case AgentTaskFailed, AgentTaskRetryable:
		if !lifecycleKnown {
			transition.Action = missionRecoveryTransitionInsufficientLifecycle
			transition.Reason = "retry_limit_cannot_be_proven_without_lifecycle"
			return transition
		}
		if attempts >= missionRecoveryMaxTaskAttempts {
			transition.Action = missionRecoveryTransitionTaskAttemptLimit
			transition.Reason = "task_attempt_limit_reached"
			return transition
		}
		transition.Action = missionRecoveryTransitionRetryCandidate
		transition.Reason = "failed_task_with_remaining_attempt_budget"
		transition.RequiresNewAttempt = true
		return transition

	case AgentTaskPending, AgentTaskProposed, AgentTaskReady, AgentTaskBlocked:
		if lifecycleKnown && attempts >= missionRecoveryMaxTaskAttempts {
			transition.Action = missionRecoveryTransitionTaskAttemptLimit
			transition.Reason = "task_attempt_limit_reached"
			return transition
		}
		transition.Action = missionRecoveryTransitionResumeCandidate
		transition.Reason = "nonterminal_task_with_remaining_attempt_budget"
		transition.RequiresNewAttempt = true
		return transition

	default:
		transition.Action = missionRecoveryTransitionInvalidRecoveryState
		transition.Reason = "unsupported_durable_task_state"
		return transition
	}
}

func transitionCanSatisfyDependency(transition MissionRecoveryTaskTransition) bool {
	return transition.Action == missionRecoveryTransitionReuseVerified
}

func transitionNeedsDependencyGate(transition MissionRecoveryTaskTransition) bool {
	switch transition.Action {
	case missionRecoveryTransitionReuseVerified, missionRecoveryTransitionResumeCandidate, missionRecoveryTransitionRetryCandidate:
		return true
	default:
		return false
	}
}

func planMissionRecoveryTransitions(mission *MissionRecoveryState, observedAt time.Time) MissionRecoveryTransitionPlan {
	if !missionRecoveryTransitionGraphValid(mission) {
		return invalidMissionRecoveryTransitionPlan(mission, observedAt, "invalid_recovery_task_graph")
	}
	plan := MissionRecoveryTransitionPlan{
		MissionID:          mission.MissionID,
		ObservedAt:         observedAt,
		Valid:              true,
		MaxTaskAttempts:    missionRecoveryMaxTaskAttempts,
		MaxMissionAttempts: missionRecoveryMaxMissionAttempts,
	}
	if observedAt.IsZero() {
		plan.ObservedAt = time.Now()
	}
	if mission.Reconciliation != nil {
		plan.ReconciliationState = mission.Reconciliation.State
	}

	plan.Tasks = make([]MissionRecoveryTaskTransition, 0, len(mission.Tasks))
	indexByID := make(map[string]int, len(mission.Tasks))
	for _, task := range mission.Tasks {
		attempts, _, _ := missionRecoveryTaskAttemptCounts(task)
		plan.ObservedMissionAttempts += attempts
		indexByID[task.ID] = len(plan.Tasks)
		plan.Tasks = append(plan.Tasks, initialMissionRecoveryTaskTransition(task, plan.ReconciliationState))
	}

	// Dependency safety is stricter than mere task readiness: any task that
	// could later be reused or executed requires every dependency to be a
	// currently matched, verified durable success.
	for index := range plan.Tasks {
		if !transitionNeedsDependencyGate(plan.Tasks[index]) {
			continue
		}
		blocked := make([]string, 0)
		for _, dependencyID := range plan.Tasks[index].Dependencies {
			dependencyIndex, ok := indexByID[dependencyID]
			if !ok || dependencyIndex == index || !transitionCanSatisfyDependency(plan.Tasks[dependencyIndex]) {
				blocked = append(blocked, dependencyID)
			}
		}
		if len(blocked) == 0 {
			continue
		}
		plan.Tasks[index].Action = missionRecoveryTransitionBlockedDependency
		plan.Tasks[index].Reason = "dependency_not_currently_verified_reusable"
		plan.Tasks[index].RequiresNewAttempt = false
		plan.Tasks[index].BlockedBy = blocked
	}

	// Reserve the fixed mission-wide attempt budget deterministically in the
	// durable task order. This is planning evidence only; no reservation is an
	// execution lease or Scheduler admission.
	remaining := missionRecoveryMaxMissionAttempts - plan.ObservedMissionAttempts
	if remaining < 0 {
		remaining = 0
	}
	for index := range plan.Tasks {
		if !plan.Tasks[index].RequiresNewAttempt {
			continue
		}
		if remaining == 0 {
			plan.Tasks[index].Action = missionRecoveryTransitionMissionAttemptLimit
			plan.Tasks[index].Reason = "mission_attempt_limit_reached"
			plan.Tasks[index].RequiresNewAttempt = false
			continue
		}
		remaining--
		plan.ReservedNewAttempts++
	}
	return plan
}
