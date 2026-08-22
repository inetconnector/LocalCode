// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	errDesktopMissionRecoveryNotFound     = errors.New("desktop mission recovery is not available")
	errMissionRecoveryAdmissionPrecondition = errors.New("mission recovery admission precondition changed")
)

type DesktopMissionRecoveryTask struct {
	TaskID             string         `json:"task_id"`
	DurableState       AgentTaskState `json:"durable_state"`
	Action             string         `json:"action"`
	RequiresNewAttempt bool           `json:"requires_new_attempt"`
	CanContinue        bool           `json:"can_continue"`
}

type DesktopMissionRecoverySnapshot struct {
	Available           bool                         `json:"available"`
	RunID               string                       `json:"run_id"`
	MissionID           string                       `json:"mission_id"`
	ObservedAt          time.Time                    `json:"observed_at"`
	JournalSHA256       string                       `json:"journal_sha256"`
	SnapshotSHA256      string                       `json:"snapshot_sha256"`
	ReconciliationState string                       `json:"reconciliation_state"`
	Runnable            bool                         `json:"runnable"`
	Tasks               []DesktopMissionRecoveryTask `json:"tasks"`
}

type DesktopMissionRecoveryContinueRequest struct {
	RunID          string `json:"run_id"`
	MissionID      string `json:"mission_id"`
	TaskID         string `json:"task_id"`
	Action         string `json:"action"`
	JournalSHA256  string `json:"journal_sha256"`
	SnapshotSHA256 string `json:"snapshot_sha256"`
}

type MissionRecoveryContinuationPreconditions struct {
	MissionID      string
	Action         string
	JournalSHA256  string
	SnapshotSHA256 string
}

type MissionRecoveryContinuationAdmission struct {
	RunID       string    `json:"run_id"`
	ParentRunID string    `json:"parent_run_id"`
	MissionID   string    `json:"mission_id"`
	TaskID      string    `json:"task_id"`
	Action      string    `json:"action"`
	AcceptedAt  time.Time `json:"accepted_at"`
}

type missionRecoveryContinuationAdmissionState struct {
	public       MissionRecoveryContinuationAdmission
	materialized MissionRecoveryContinuationMaterialization
	graph        AgentTaskGraph
	cfg          Config
	missionCtx   context.Context
	cancel       context.CancelFunc
}

func desktopMissionRecoverySnapshotFromControl(snapshot MissionRecoveryControlSnapshot) DesktopMissionRecoverySnapshot {
	out := DesktopMissionRecoverySnapshot{
		Available:           true,
		RunID:               snapshot.RunID,
		MissionID:           snapshot.MissionID,
		ObservedAt:          snapshot.ObservedAt,
		JournalSHA256:       snapshot.JournalSHA256,
		SnapshotSHA256:      snapshot.SnapshotSHA256,
		ReconciliationState: snapshot.ReconciliationState,
		Runnable:            snapshot.Plan.Runnable,
		Tasks:               make([]DesktopMissionRecoveryTask, 0, len(snapshot.Plan.Tasks)),
	}
	for _, transition := range snapshot.Plan.Tasks {
		canContinue := transition.RequiresNewAttempt && (transition.Action == missionRecoveryTransitionResumeCandidate || transition.Action == missionRecoveryTransitionRetryCandidate)
		out.Tasks = append(out.Tasks, DesktopMissionRecoveryTask{
			TaskID:             transition.TaskID,
			DurableState:       transition.DurableState,
			Action:             transition.Action,
			RequiresNewAttempt: transition.RequiresNewAttempt,
			CanContinue:        canContinue,
		})
	}
	return out
}

func (s *AppState) DesktopMissionRecoverySnapshot(runID string) (DesktopMissionRecoverySnapshot, error) {
	if s == nil {
		return DesktopMissionRecoverySnapshot{}, errDesktopMissionRecoveryNotFound
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		s.mu.RLock()
		if s.Running {
			s.mu.RUnlock()
			return DesktopMissionRecoverySnapshot{}, errMissionRecoveryControlActiveRun
		}
		if s.Recovery != nil {
			runID = strings.TrimSpace(s.Recovery.RunID)
		}
		s.mu.RUnlock()
		if runID == "" {
			if recovered := loadRecoverableRun(); recovered != nil {
				runID = strings.TrimSpace(recovered.RunID)
			}
		}
	}
	if runID == "" {
		return DesktopMissionRecoverySnapshot{}, errDesktopMissionRecoveryNotFound
	}
	snapshot, err := s.MissionRecoveryControlSnapshot(runID)
	if err != nil {
		return DesktopMissionRecoverySnapshot{}, err
	}
	return desktopMissionRecoverySnapshotFromControl(snapshot), nil
}

func validateMissionRecoveryContinuationPreconditions(materialized MissionRecoveryContinuationMaterialization, expected MissionRecoveryContinuationPreconditions) error {
	expected.MissionID = strings.TrimSpace(expected.MissionID)
	expected.Action = strings.TrimSpace(expected.Action)
	expected.JournalSHA256 = strings.TrimSpace(expected.JournalSHA256)
	expected.SnapshotSHA256 = strings.TrimSpace(expected.SnapshotSHA256)
	if expected.MissionID == "" || (expected.Action != missionRecoveryTransitionResumeCandidate && expected.Action != missionRecoveryTransitionRetryCandidate) || !validMissionVerificationDigest(expected.JournalSHA256) || !validMissionVerificationDigest(expected.SnapshotSHA256) {
		return errMissionRecoveryAdmissionPrecondition
	}
	if materialized.MissionID != expected.MissionID || materialized.Action != expected.Action || materialized.JournalSHA256 != expected.JournalSHA256 || materialized.SnapshotSHA256 != expected.SnapshotSHA256 {
		return errMissionRecoveryAdmissionPrecondition
	}
	return nil
}

func (s *AppState) admitMissionRecoveryContinuation(ctx context.Context, runID, taskID string, expected MissionRecoveryContinuationPreconditions, observe missionRecoveryControlObserver) (missionRecoveryContinuationAdmissionState, error) {
	var admitted missionRecoveryContinuationAdmissionState
	if s == nil {
		return admitted, errMissionRecoveryContinuationUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return admitted, err
	}

	s.mu.Lock()
	defer func() {
		if admitted.public.RunID == "" {
			s.mu.Unlock()
		}
	}()
	if s.Running {
		return admitted, errMissionRecoveryControlActiveRun
	}
	cfg := s.Config
	materialized, err := buildStableMissionRecoveryContinuationWithObserver(strings.TrimSpace(runID), strings.TrimSpace(taskID), observe)
	if err != nil {
		return admitted, err
	}
	if err := validateMissionRecoveryContinuationPreconditions(materialized, expected); err != nil {
		return admitted, err
	}
	graph := materialized.Graph
	if err := prepareRecoveryGraphTaskBudgets(materialized, &graph, cfg); err != nil {
		return admitted, err
	}
	acceptedAt := time.Now()
	executionRunID := newID()
	if err := reserveMissionRecoveryContinuation(materialized, executionRunID, acceptedAt); err != nil {
		return admitted, err
	}
	// HTTP request cancellation is only an admission precondition. Once the
	// local operation is accepted, its lifetime is owned by AppState/StopAgent.
	missionCtx, cancel := context.WithCancel(context.Background())
	s.Running = true
	s.Cancel = cancel
	s.RunID = executionRunID
	s.RunPhase = "mission-read-only-continuation"
	s.RunStartedAt = acceptedAt
	s.LastProgressAt = acceptedAt
	s.Project = materialized.Project
	s.Model = materialized.Model
	s.Recovery = nil

	admitted = missionRecoveryContinuationAdmissionState{
		public: MissionRecoveryContinuationAdmission{
			RunID:       executionRunID,
			ParentRunID: materialized.RunID,
			MissionID:   materialized.MissionID,
			TaskID:      materialized.TaskID,
			Action:      materialized.Action,
			AcceptedAt:  acceptedAt,
		},
		materialized: materialized,
		graph:        graph,
		cfg:          cfg,
		missionCtx:   missionCtx,
		cancel:       cancel,
	}
	s.mu.Unlock()
	return admitted, nil
}

func (s *AppState) executeAdmittedMissionRecoveryContinuation(admitted missionRecoveryContinuationAdmissionState, execute scheduledReadOnlyAgentExecutor) {
	if execute == nil {
		execute = s.runNativeReadOnlyAgentTask
	}
	executionRunID := admitted.public.RunID
	graph := admitted.graph
	materialized := admitted.materialized
	defer func() {
		admitted.cancel()
		s.mu.Lock()
		if s.RunID == executionRunID {
			s.Running = false
			s.Cancel = nil
			s.RunPhase = "idle"
			s.LastProgressAt = time.Now()
			s.Recovery = loadRecoverableRun()
		}
		s.mu.Unlock()
	}()

	tracker, err := newRecoveryMissionBudgetTracker(materialized.MissionBudget, materialized.MissionBudgetSnapshot.Usage, admitted.public.AcceptedAt)
	if err != nil {
		_ = finishMissionRecoveryContinuation(executionRunID, &graph, AgentScheduledRun{MissionID: graph.MissionID, UsageByTask: materialized.HistoricalUsageByTask}, time.Now(), err)
		return
	}
	budgetedExecute := func(childCtx context.Context, childProject string, childCfg Config, task AgentTask) (AgentResult, error) {
		remainingTask, remainingErr := capRecoveryExecutionTaskBudget(task, materialized.HistoricalUsageByTask[task.ID])
		if remainingErr != nil {
			return AgentResult{Status: AgentResultBudgetExhausted, Summary: remainingErr.Error()}, remainingErr
		}
		constrained, allowed := tracker.prepareTask(remainingTask)
		if !allowed {
			return AgentResult{Status: AgentResultBudgetExhausted, Summary: "Mission budget exhausted before task: " + tracker.blockedDimension()}, nil
		}
		result, childErr := execute(childCtx, childProject, childCfg, constrained)
		tracker.recordObservedUsage(result.Usage)
		return result, childErr
	}

	scheduler := NewAgentScheduler(admitted.missionCtx, AgentResourceLimits{})
	defer scheduler.missionCancel()
	checkpoint := func(snapshot AgentSchedulerSnapshot) {
		s.journalMissionSchedulerCheckpoint(executionRunID, snapshot, &graph)
	}
	run, runErr := s.runScheduledReadOnlyAgentGraphWithExecutorAndCheckpointSeeded(materialized.Project, admitted.cfg, &graph, scheduler, budgetedExecute, checkpoint, materialized.HistoricalUsageByTask)
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		cancelUnfinishedReadOnlyMissionTasks(&graph)
		run.Snapshot = scheduler.Snapshot(&graph, run.UsageByTask)
		s.journalMissionSchedulerCheckpoint(executionRunID, run.Snapshot, &graph)
	}
	_ = finishMissionRecoveryContinuation(executionRunID, &graph, run, time.Now(), runErr)
}

func (s *AppState) startMissionRecoveryContinuationWithExecutorAndObserver(ctx context.Context, runID, taskID string, expected MissionRecoveryContinuationPreconditions, execute scheduledReadOnlyAgentExecutor, observe missionRecoveryControlObserver) (MissionRecoveryContinuationAdmission, error) {
	if execute == nil {
		return MissionRecoveryContinuationAdmission{}, errors.New("read-only mission executor is nil")
	}
	admitted, err := s.admitMissionRecoveryContinuation(ctx, runID, taskID, expected, observe)
	if err != nil {
		return MissionRecoveryContinuationAdmission{}, err
	}
	go s.executeAdmittedMissionRecoveryContinuation(admitted, execute)
	return admitted.public, nil
}

// StartMissionRecoveryContinuation is the explicit Desktop continuation entry.
// UI-supplied digests are stale-state preconditions only; admission always
// recomputes trusted #67 evidence and performs the #68 journal CAS before this
// method returns success.
func (s *AppState) StartMissionRecoveryContinuation(ctx context.Context, runID, taskID string, expected MissionRecoveryContinuationPreconditions) (MissionRecoveryContinuationAdmission, error) {
	return s.startMissionRecoveryContinuationWithExecutorAndObserver(ctx, runID, taskID, expected, s.runNativeReadOnlyAgentTask, nil)
}

func validDesktopMissionRecoveryContinueRequest(req DesktopMissionRecoveryContinueRequest) bool {
	if strings.TrimSpace(req.RunID) == "" || len(strings.TrimSpace(req.RunID)) > 160 || strings.TrimSpace(req.MissionID) == "" || len(strings.TrimSpace(req.MissionID)) > 160 || strings.TrimSpace(req.TaskID) == "" || len(strings.TrimSpace(req.TaskID)) > 160 {
		return false
	}
	if req.Action != missionRecoveryTransitionResumeCandidate && req.Action != missionRecoveryTransitionRetryCandidate {
		return false
	}
	return validMissionVerificationDigest(strings.TrimSpace(req.JournalSHA256)) && validMissionVerificationDigest(strings.TrimSpace(req.SnapshotSHA256))
}

func decodeDesktopMissionRecoveryContinueRequest(w http.ResponseWriter, r *http.Request) (DesktopMissionRecoveryContinueRequest, error) {
	var req DesktopMissionRecoveryContinueRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return req, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return req, errors.New("multiple JSON values are not allowed")
		}
		return req, err
	}
	if !validDesktopMissionRecoveryContinueRequest(req) {
		return req, errors.New("invalid mission recovery request")
	}
	return req, nil
}

type desktopMissionRecoverySnapshotFunc func(string) (DesktopMissionRecoverySnapshot, error)
type desktopMissionRecoveryStartFunc func(context.Context, string, string, MissionRecoveryContinuationPreconditions) (MissionRecoveryContinuationAdmission, error)

func handleDesktopMissionRecoverySnapshot(w http.ResponseWriter, r *http.Request, inspect desktopMissionRecoverySnapshotFunc) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	snapshot, err := inspect(strings.TrimSpace(r.URL.Query().Get("run_id")))
	if errors.Is(err, errDesktopMissionRecoveryNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		http.Error(w, "mission recovery state changed; refresh required", http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, snapshot)
}

func handleDesktopMissionRecoveryContinue(w http.ResponseWriter, r *http.Request, start desktopMissionRecoveryStartFunc) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeDesktopMissionRecoveryContinueRequest(w, r)
	if err != nil {
		http.Error(w, "invalid mission recovery request", http.StatusBadRequest)
		return
	}
	admitted, err := start(r.Context(), strings.TrimSpace(req.RunID), strings.TrimSpace(req.TaskID), MissionRecoveryContinuationPreconditions{
		MissionID:      strings.TrimSpace(req.MissionID),
		Action:         strings.TrimSpace(req.Action),
		JournalSHA256:  strings.TrimSpace(req.JournalSHA256),
		SnapshotSHA256: strings.TrimSpace(req.SnapshotSHA256),
	})
	if err != nil {
		status := http.StatusConflict
		if !errors.Is(err, errMissionRecoveryControlActiveRun) && !errors.Is(err, errMissionRecoveryAdmissionStale) && !errors.Is(err, errMissionRecoveryAdmissionPrecondition) && !errors.Is(err, errMissionRecoveryContinuationCandidate) && !errors.Is(err, errMissionRecoveryContinuationBudget) && !errors.Is(err, errMissionRecoveryContinuationUnavailable) {
			status = http.StatusInternalServerError
		}
		http.Error(w, "mission recovery admission rejected; refresh required", status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = writeJSON(w, admitted)
}

func (s *Server) handleMissionRecovery(w http.ResponseWriter, r *http.Request) {
	handleDesktopMissionRecoverySnapshot(w, r, s.state.DesktopMissionRecoverySnapshot)
}

func (s *Server) handleMissionRecoveryContinue(w http.ResponseWriter, r *http.Request) {
	handleDesktopMissionRecoveryContinue(w, r, s.state.StartMissionRecoveryContinuation)
}

func desktopMissionRecoveryDebugString(snapshot DesktopMissionRecoverySnapshot) string {
	// Bounded diagnostic helper for tests/logging; deliberately excludes paths,
	// objectives, raw Child output, capabilities and accounting detail.
	return fmt.Sprintf("run=%s mission=%s reconciliation=%s tasks=%d", snapshot.RunID, snapshot.MissionID, snapshot.ReconciliationState, len(snapshot.Tasks))
}
