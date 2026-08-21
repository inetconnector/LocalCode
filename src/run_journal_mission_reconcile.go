// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	missionGitStateObserved      = "observed"
	missionGitStateNotRepository = "not_repository"
	missionGitStateUnavailable   = "unavailable"

	missionReconcileMatched              = "matched"
	missionReconcileProjectUnavailable   = "project_unavailable"
	missionReconcileProjectMismatch      = "project_mismatch"
	missionReconcileGitChanged           = "git_changed"
	missionReconcileGitUnavailable       = "git_unavailable"
	missionReconcileInsufficientEvidence = "insufficient_evidence"

	missionTaskDispositionTerminal             = "terminal"
	missionTaskDispositionVerifyPostconditions = "verify_postconditions"
	missionTaskDispositionInterruptedUnknown   = "interrupted_unknown"
	missionTaskDispositionPending              = "pending"
	missionTaskDispositionBlocked              = "blocked_reconciliation"
)

var errMissionGitUnavailable = errors.New("git unavailable")

type MissionProjectBaseline struct {
	ProjectIdentitySHA256 string    `json:"project_identity_sha256,omitempty"`
	GitState              string    `json:"git_state"`
	GitRootSHA256         string    `json:"git_root_sha256,omitempty"`
	GitHead               string    `json:"git_head,omitempty"`
	GitStatusSHA256       string    `json:"git_status_sha256,omitempty"`
	CapturedAt            time.Time `json:"captured_at"`
}

type MissionRecoveryTaskReconciliation struct {
	TaskID       string         `json:"task_id"`
	DurableState AgentTaskState `json:"durable_state"`
	Disposition  string         `json:"disposition"`
	Reason       string         `json:"reason"`
}

type MissionRestartReconciliation struct {
	State      string                              `json:"state"`
	Reason     string                              `json:"reason"`
	ObservedAt time.Time                           `json:"observed_at"`
	Current    MissionProjectBaseline              `json:"current"`
	Tasks      []MissionRecoveryTaskReconciliation `json:"tasks"`
}

type missionGitReadOnlyRunner func(context.Context, ...string) ([]byte, error)

func missionSHA256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func missionSHA256Bytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func missionProjectIdentity(project string) string {
	project = filepath.Clean(project)
	if absolute, err := filepath.Abs(project); err == nil {
		project = absolute
	}
	if resolved, err := filepath.EvalSymlinks(project); err == nil {
		project = filepath.Clean(resolved)
	}
	if runtime.GOOS == "windows" {
		project = strings.ToLower(project)
	}
	return missionSHA256String(project)
}

func defaultMissionGitReadOnlyRunner(ctx context.Context, args ...string) ([]byte, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, errMissionGitUnavailable
	}
	cmd := exec.CommandContext(ctx, gitPath, args...)
	return cmd.Output()
}

func observeMissionProjectBaseline(project string, observedAt time.Time, run missionGitReadOnlyRunner) MissionProjectBaseline {
	baseline := MissionProjectBaseline{
		ProjectIdentitySHA256: missionProjectIdentity(project),
		GitState:              missionGitStateUnavailable,
		CapturedAt:            observedAt,
	}
	if run == nil {
		run = defaultMissionGitReadOnlyRunner
	}
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		return baseline
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	root, err := run(ctx, "-C", project, "rev-parse", "--show-toplevel")
	if errors.Is(err, errMissionGitUnavailable) {
		return baseline
	}
	if err != nil || strings.TrimSpace(string(root)) == "" {
		baseline.GitState = missionGitStateNotRepository
		return baseline
	}

	head, err := run(ctx, "-C", project, "rev-parse", "--verify", "HEAD")
	if err != nil || strings.TrimSpace(string(head)) == "" {
		return baseline
	}
	status, err := run(ctx, "-C", project, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return baseline
	}

	baseline.GitState = missionGitStateObserved
	baseline.GitRootSHA256 = missionSHA256String(filepath.Clean(strings.TrimSpace(string(root))))
	baseline.GitHead = strings.TrimSpace(string(head))
	baseline.GitStatusSHA256 = missionSHA256Bytes(status)
	return baseline
}

func captureMissionProjectBaseline(project string) MissionProjectBaseline {
	return observeMissionProjectBaseline(project, time.Now(), nil)
}

func missionTaskReconciliation(task MissionRecoveryTaskState, overallState string) MissionRecoveryTaskReconciliation {
	result := MissionRecoveryTaskReconciliation{
		TaskID:       task.ID,
		DurableState: task.State,
	}
	switch task.State {
	case AgentTaskFailed, AgentTaskCancelled:
		result.Disposition = missionTaskDispositionTerminal
		result.Reason = "durable_terminal_state"
		return result
	case AgentTaskRunning:
		result.Disposition = missionTaskDispositionInterruptedUnknown
		result.Reason = "running_at_interruption_is_not_success"
		return result
	case AgentTaskSucceeded, AgentTaskCompleted:
		if overallState == missionReconcileMatched {
			result.Disposition = missionTaskDispositionVerifyPostconditions
			result.Reason = "durable_success_requires_postcondition_evidence"
		} else {
			result.Disposition = missionTaskDispositionBlocked
			result.Reason = "project_reconciliation_not_matched"
		}
		return result
	default:
		if overallState == missionReconcileMatched {
			result.Disposition = missionTaskDispositionPending
			result.Reason = "not_started_or_not_terminal"
		} else {
			result.Disposition = missionTaskDispositionBlocked
			result.Reason = "project_reconciliation_not_matched"
		}
		return result
	}
}

func reconcileMissionRecoveryWithCurrent(mission *MissionRecoveryState, current MissionProjectBaseline, observedAt time.Time) *MissionRestartReconciliation {
	if mission == nil {
		return nil
	}
	reconciliation := &MissionRestartReconciliation{
		State:      missionReconcileInsufficientEvidence,
		Reason:     "missing_baseline",
		ObservedAt: observedAt,
		Current:    current,
	}

	baseline := mission.Baseline
	switch {
	case baseline == nil:
	case current.ProjectIdentitySHA256 == "":
		reconciliation.State = missionReconcileProjectUnavailable
		reconciliation.Reason = "project_unavailable"
	case baseline.ProjectIdentitySHA256 != "" && baseline.ProjectIdentitySHA256 != current.ProjectIdentitySHA256:
		reconciliation.State = missionReconcileProjectMismatch
		reconciliation.Reason = "project_identity_changed"
	case baseline.GitState != missionGitStateObserved:
		reconciliation.State = missionReconcileInsufficientEvidence
		reconciliation.Reason = "baseline_git_evidence_unavailable"
	case current.GitState == missionGitStateUnavailable:
		reconciliation.State = missionReconcileGitUnavailable
		reconciliation.Reason = "current_git_evidence_unavailable"
	case current.GitState != missionGitStateObserved:
		reconciliation.State = missionReconcileProjectMismatch
		reconciliation.Reason = "git_repository_identity_changed"
	case baseline.GitRootSHA256 != current.GitRootSHA256:
		reconciliation.State = missionReconcileProjectMismatch
		reconciliation.Reason = "git_root_changed"
	case baseline.GitHead != current.GitHead:
		reconciliation.State = missionReconcileGitChanged
		reconciliation.Reason = "git_head_changed"
	case baseline.GitStatusSHA256 != current.GitStatusSHA256:
		reconciliation.State = missionReconcileGitChanged
		reconciliation.Reason = "git_worktree_changed"
	default:
		reconciliation.State = missionReconcileMatched
		reconciliation.Reason = "project_and_git_match_baseline"
	}

	reconciliation.Tasks = make([]MissionRecoveryTaskReconciliation, 0, len(mission.Tasks))
	for _, task := range mission.Tasks {
		reconciliation.Tasks = append(reconciliation.Tasks, missionTaskReconciliation(task, reconciliation.State))
	}
	return reconciliation
}

func reconcileRecoverableMission(recovery *RunRecoveryState) *RunRecoveryState {
	if recovery == nil || recovery.Mission == nil {
		return recovery
	}
	copy := *recovery
	copy.Mission = cloneMissionRecoveryState(recovery.Mission)
	observedAt := time.Now()
	current := observeMissionProjectBaseline(copy.Mission.Project, observedAt, nil)
	if info, err := os.Stat(copy.Mission.Project); err != nil || !info.IsDir() {
		current.ProjectIdentitySHA256 = ""
		current.GitState = missionGitStateUnavailable
	}
	copy.Mission.Reconciliation = reconcileMissionRecoveryWithCurrent(copy.Mission, current, observedAt)
	return &copy
}
