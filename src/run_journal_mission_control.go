// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const missionRecoveryControlMaxSnapshotAttempts = 3

var (
	errMissionRecoveryControlInvalidRequest = errors.New("invalid mission recovery control request")
	errMissionRecoveryControlUnavailable    = errors.New("mission recovery control unavailable")
	errMissionRecoveryControlChanged        = errors.New("mission recovery state changed during observation")
)

type MissionRecoveryControlVerification struct {
	TaskID         string `json:"task_id"`
	Passed         bool   `json:"passed"`
	EvidenceSHA256 string `json:"evidence_sha256,omitempty"`
	CheckCount     int    `json:"check_count"`
}

type MissionRecoveryControlSnapshot struct {
	RunID                   string                               `json:"run_id"`
	MissionID               string                               `json:"mission_id"`
	ObservedAt              time.Time                            `json:"observed_at"`
	JournalSHA256           string                               `json:"journal_sha256"`
	SnapshotSHA256          string                               `json:"snapshot_sha256"`
	ReconciliationState     string                               `json:"reconciliation_state"`
	ReconciliationReason    string                               `json:"reconciliation_reason"`
	Verifications           []MissionRecoveryControlVerification `json:"verifications,omitempty"`
	Plan                    MissionRecoveryTransitionPlan        `json:"plan"`
	ReadOnly                bool                                 `json:"read_only"`
	ExecutionAuthorized     bool                                 `json:"execution_authorized"`
	SchedulerLeaseGranted   bool                                 `json:"scheduler_lease_granted"`
	PersistentStateModified bool                                 `json:"persistent_state_modified"`
}

type missionRecoveryControlJournalFingerprintInput struct {
	SchemaVersion int                   `json:"schema_version"`
	RunID         string                `json:"run_id"`
	Project       string                `json:"project"`
	Phase         string                `json:"phase"`
	UpdatedAt     time.Time             `json:"updated_at"`
	Terminal      bool                  `json:"terminal"`
	Mission       *MissionRecoveryState `json:"mission"`
}

type missionRecoveryControlSnapshotDigestInput struct {
	RunID                string                               `json:"run_id"`
	MissionID            string                               `json:"mission_id"`
	JournalSHA256        string                               `json:"journal_sha256"`
	ObservedAt           time.Time                            `json:"observed_at"`
	ReconciliationState  string                               `json:"reconciliation_state"`
	ReconciliationReason string                               `json:"reconciliation_reason"`
	Verifications        []MissionRecoveryControlVerification `json:"verifications,omitempty"`
	Plan                 MissionRecoveryTransitionPlan        `json:"plan"`
}

func missionRecoveryControlJournalFingerprint(state *RunRecoveryState) (string, error) {
	if state == nil {
		return "", errMissionRecoveryControlUnavailable
	}
	data, err := json.Marshal(missionRecoveryControlJournalFingerprintInput{
		SchemaVersion: state.SchemaVersion,
		RunID:         state.RunID,
		Project:       state.Project,
		Phase:         state.Phase,
		UpdatedAt:     state.UpdatedAt,
		Terminal:      state.Terminal,
		Mission:       state.Mission,
	})
	if err != nil {
		return "", fmt.Errorf("fingerprint mission recovery journal: %w", err)
	}
	return missionSHA256Bytes(data), nil
}

func loadMissionRecoveryControlState(runID string) (*RunRecoveryState, string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, "", errMissionRecoveryControlInvalidRequest
	}

	runJournalFileMu.Lock()
	defer runJournalFileMu.Unlock()
	state, err := loadRunJournal()
	if err != nil {
		return nil, "", err
	}
	if state == nil || state.Terminal || state.Mission == nil || state.RunID != runID || state.Mission.Kind != missionRecoveryKindReadOnly {
		return nil, "", errMissionRecoveryControlUnavailable
	}
	fingerprint, err := missionRecoveryControlJournalFingerprint(state)
	if err != nil {
		return nil, "", err
	}
	copyState := *state
	copyState.Mission = cloneMissionRecoveryState(state.Mission)
	copyState.Events = nil
	return &copyState, fingerprint, nil
}

func observeMissionRecoveryControlProject(project string, observedAt time.Time) MissionProjectBaseline {
	current := observeMissionProjectBaseline(project, observedAt, nil)
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		current.ProjectIdentitySHA256 = ""
		current.GitState = missionGitStateUnavailable
	}
	return current
}

func applyMissionRecoveryControlVerification(mission *MissionRecoveryState, result MissionTaskPostconditionVerification) {
	if mission == nil || !result.Passed || !validMissionVerificationDigest(result.EvidenceSHA256) || result.CheckCount <= 0 || result.CheckCount > maxMissionVerificationChecks {
		return
	}
	for index := range mission.Tasks {
		if mission.Tasks[index].ID != result.TaskID || mission.Tasks[index].CompletionEvidence == nil {
			continue
		}
		evidence := *mission.Tasks[index].CompletionEvidence
		switch evidence.VerificationState {
		case missionVerificationUnverified, missionVerificationFailed:
			if evidence.VerificationAttemptCount < 0 {
				return
			}
			evidence.VerificationAttemptCount++
		case missionVerificationVerified:
			if evidence.VerificationAttemptCount <= 0 {
				return
			}
			evidence.VerificationAttemptCount++
		default:
			return
		}
		evidence.VerificationState = missionVerificationVerified
		evidence.LastVerificationEvidenceSHA256 = result.EvidenceSHA256
		evidence.LastVerificationCheckCount = result.CheckCount
		evidence.VerificationUpdatedAt = result.ObservedAt
		mission.Tasks[index].CompletionEvidence = &evidence
		return
	}
}

func buildMissionRecoveryControlSnapshot(state *RunRecoveryState, journalFingerprint string, current MissionProjectBaseline, observedAt time.Time) (MissionRecoveryControlSnapshot, error) {
	var snapshot MissionRecoveryControlSnapshot
	if state == nil || state.Terminal || state.Mission == nil || state.Mission.Kind != missionRecoveryKindReadOnly || strings.TrimSpace(state.RunID) == "" {
		return snapshot, errMissionRecoveryControlUnavailable
	}
	if !validMissionVerificationDigest(journalFingerprint) {
		return snapshot, errMissionRecoveryControlInvalidRequest
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	mission := cloneMissionRecoveryState(state.Mission)
	mission.Reconciliation = reconcileMissionRecoveryWithCurrent(mission, current, observedAt)
	initialPlan := planMissionRecoveryTransitions(mission, observedAt)
	verifications := make([]MissionRecoveryControlVerification, 0)
	if initialPlan.Valid && mission.Reconciliation != nil {
		for _, transition := range initialPlan.Tasks {
			if transition.Action != missionRecoveryTransitionVerifyPostconditions {
				continue
			}
			verification, err := evaluateMissionTaskPostconditions(mission, transition.TaskID, mission.Reconciliation, observedAt)
			if err != nil {
				return snapshot, err
			}
			verifications = append(verifications, MissionRecoveryControlVerification{
				TaskID:         verification.TaskID,
				Passed:         verification.Passed,
				EvidenceSHA256: verification.EvidenceSHA256,
				CheckCount:     verification.CheckCount,
			})
			applyMissionRecoveryControlVerification(mission, verification)
		}
	}

	plan := planMissionRecoveryTransitions(mission, observedAt)
	snapshot = MissionRecoveryControlSnapshot{
		RunID:                   state.RunID,
		MissionID:               mission.MissionID,
		ObservedAt:              observedAt,
		JournalSHA256:           journalFingerprint,
		Verifications:           verifications,
		Plan:                    plan,
		ReadOnly:                true,
		ExecutionAuthorized:     false,
		SchedulerLeaseGranted:   false,
		PersistentStateModified: false,
	}
	if mission.Reconciliation != nil {
		snapshot.ReconciliationState = mission.Reconciliation.State
		snapshot.ReconciliationReason = mission.Reconciliation.Reason
	}
	digestData, err := json.Marshal(missionRecoveryControlSnapshotDigestInput{
		RunID:                snapshot.RunID,
		MissionID:            snapshot.MissionID,
		JournalSHA256:        journalFingerprint,
		ObservedAt:           snapshot.ObservedAt,
		ReconciliationState:  snapshot.ReconciliationState,
		ReconciliationReason: snapshot.ReconciliationReason,
		Verifications:        snapshot.Verifications,
		Plan:                 snapshot.Plan,
	})
	if err != nil {
		return MissionRecoveryControlSnapshot{}, fmt.Errorf("fingerprint mission recovery control snapshot: %w", err)
	}
	snapshot.SnapshotSHA256 = missionSHA256Bytes(digestData)
	if !validMissionVerificationDigest(snapshot.SnapshotSHA256) {
		return MissionRecoveryControlSnapshot{}, fmt.Errorf("invalid mission recovery control snapshot digest")
	}
	return snapshot, nil
}

func buildStableMissionRecoveryControlSnapshotWithObserver(runID string, observe func(string, time.Time) MissionProjectBaseline) (MissionRecoveryControlSnapshot, error) {
	if observe == nil {
		observe = observeMissionRecoveryControlProject
	}
	for attempt := 0; attempt < missionRecoveryControlMaxSnapshotAttempts; attempt++ {
		state, journalFingerprint, err := loadMissionRecoveryControlState(runID)
		if err != nil {
			return MissionRecoveryControlSnapshot{}, err
		}
		observedAt := time.Now()
		current := observe(state.Mission.Project, observedAt)
		snapshot, err := buildMissionRecoveryControlSnapshot(state, journalFingerprint, current, observedAt)
		if err != nil {
			return MissionRecoveryControlSnapshot{}, err
		}
		_, currentFingerprint, err := loadMissionRecoveryControlState(runID)
		if err != nil {
			return MissionRecoveryControlSnapshot{}, err
		}
		if currentFingerprint == journalFingerprint {
			return snapshot, nil
		}
	}
	return MissionRecoveryControlSnapshot{}, errMissionRecoveryControlChanged
}

func buildStableMissionRecoveryControlSnapshot(runID string) (MissionRecoveryControlSnapshot, error) {
	return buildStableMissionRecoveryControlSnapshotWithObserver(runID, nil)
}
