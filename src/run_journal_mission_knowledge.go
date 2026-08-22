// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	missionKnowledgeSchemaVersion   = 1
	maxMissionKnowledgeEntries      = 48
	maxMissionKnowledgeSeedEntries  = 16
	maxMissionKnowledgeEncodedBytes = 24 * 1024
	maxMissionKnowledgeSummaryBytes = 640
	maxMissionKnowledgeStatusBytes  = 80
	maxMissionKnowledgeTaskIDBytes  = 160

	missionKnowledgeArchitectureDecision = "architecture_decision"
	missionKnowledgeSubsystemContract     = "subsystem_contract"
	missionKnowledgeKnownFailure          = "known_failure"
	missionKnowledgeVerificationEvidence  = "verification_evidence"
)

var errMissionKnowledgeInvalid = errors.New("invalid mission knowledge")

type MissionKnowledgeProposal struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Status  string `json:"status,omitempty"`
}

type MissionKnowledgeEntry struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Summary       string    `json:"summary"`
	Status        string    `json:"status,omitempty"`
	SourceTaskID  string    `json:"source_task_id,omitempty"`
	SourceRole    AgentRole `json:"source_role,omitempty"`
	ContentSHA256 string    `json:"content_sha256"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type MissionKnowledgeState struct {
	SchemaVersion int                     `json:"schema_version"`
	Entries       []MissionKnowledgeEntry `json:"entries,omitempty"`
}

type missionKnowledgeDigestInput struct {
	Kind         string    `json:"kind"`
	Summary      string    `json:"summary"`
	Status       string    `json:"status,omitempty"`
	SourceTaskID string    `json:"source_task_id,omitempty"`
	SourceRole   AgentRole `json:"source_role,omitempty"`
}

func validMissionKnowledgeKind(kind string) bool {
	switch strings.TrimSpace(kind) {
	case missionKnowledgeArchitectureDecision, missionKnowledgeSubsystemContract, missionKnowledgeKnownFailure, missionKnowledgeVerificationEvidence:
		return true
	default:
		return false
	}
}

func validMissionKnowledgeSourceRole(role AgentRole) bool {
	switch role {
	case "", AgentRoleExplorer, AgentRolePlanner, AgentRoleReviewer:
		return true
	default:
		return false
	}
}

func missionKnowledgeSensitive(value string) bool {
	if memoryLooksSensitive(value) {
		return true
	}
	return strings.Contains(sanitizeRunJournalText(value, 0), "[REDACTED]")
}

func missionKnowledgeContentDigest(kind, summary, status, sourceTaskID string, sourceRole AgentRole) (string, error) {
	data, err := json.Marshal(missionKnowledgeDigestInput{
		Kind:         kind,
		Summary:      summary,
		Status:       status,
		SourceTaskID: sourceTaskID,
		SourceRole:   sourceRole,
	})
	if err != nil {
		return "", err
	}
	return missionSHA256Bytes(data), nil
}

func buildMissionKnowledgeEntry(kind, summary, status, sourceTaskID string, sourceRole AgentRole, recordedAt time.Time, strict bool) (MissionKnowledgeEntry, error) {
	kind = strings.TrimSpace(kind)
	summary = strings.TrimSpace(summary)
	status = strings.TrimSpace(status)
	sourceTaskID = strings.TrimSpace(sourceTaskID)
	if !validMissionKnowledgeKind(kind) || summary == "" || !validMissionKnowledgeSourceRole(sourceRole) || recordedAt.IsZero() {
		return MissionKnowledgeEntry{}, errMissionKnowledgeInvalid
	}
	if missionKnowledgeSensitive(summary) || missionKnowledgeSensitive(status) {
		return MissionKnowledgeEntry{}, fmt.Errorf("%w: secret-like content", errMissionKnowledgeInvalid)
	}
	if strict {
		if len(summary) > maxMissionKnowledgeSummaryBytes || len(status) > maxMissionKnowledgeStatusBytes || len(sourceTaskID) > maxMissionKnowledgeTaskIDBytes {
			return MissionKnowledgeEntry{}, fmt.Errorf("%w: field limit exceeded", errMissionKnowledgeInvalid)
		}
	} else {
		summary = sanitizeRunJournalText(summary, maxMissionKnowledgeSummaryBytes)
		status = sanitizeRunJournalText(status, maxMissionKnowledgeStatusBytes)
		sourceTaskID = sanitizeRunJournalText(sourceTaskID, maxMissionKnowledgeTaskIDBytes)
	}
	if summary == "" || missionKnowledgeSensitive(summary) || missionKnowledgeSensitive(status) {
		return MissionKnowledgeEntry{}, errMissionKnowledgeInvalid
	}
	digest, err := missionKnowledgeContentDigest(kind, summary, status, sourceTaskID, sourceRole)
	if err != nil || len(digest) < 20 {
		return MissionKnowledgeEntry{}, errMissionKnowledgeInvalid
	}
	return MissionKnowledgeEntry{
		ID:            "knowledge-" + digest[:20],
		Kind:          kind,
		Summary:       summary,
		Status:        status,
		SourceTaskID:  sourceTaskID,
		SourceRole:    sourceRole,
		ContentSHA256: digest,
		RecordedAt:    recordedAt,
	}, nil
}

func validateMissionKnowledgeEntry(entry MissionKnowledgeEntry) error {
	if len(entry.Summary) > maxMissionKnowledgeSummaryBytes || len(entry.Status) > maxMissionKnowledgeStatusBytes || len(entry.SourceTaskID) > maxMissionKnowledgeTaskIDBytes {
		return errMissionKnowledgeInvalid
	}
	if missionKnowledgeSensitive(entry.Summary) || missionKnowledgeSensitive(entry.Status) {
		return errMissionKnowledgeInvalid
	}
	rebuilt, err := buildMissionKnowledgeEntry(entry.Kind, entry.Summary, entry.Status, entry.SourceTaskID, entry.SourceRole, entry.RecordedAt, true)
	if err != nil || rebuilt.ID != entry.ID || rebuilt.ContentSHA256 != entry.ContentSHA256 {
		return errMissionKnowledgeInvalid
	}
	return nil
}

func validateMissionKnowledgeState(state *MissionKnowledgeState) error {
	if state == nil {
		return nil
	}
	if state.SchemaVersion != missionKnowledgeSchemaVersion || len(state.Entries) > maxMissionKnowledgeEntries {
		return errMissionKnowledgeInvalid
	}
	seen := make(map[string]struct{}, len(state.Entries))
	for _, entry := range state.Entries {
		if err := validateMissionKnowledgeEntry(entry); err != nil {
			return err
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return errMissionKnowledgeInvalid
		}
		seen[entry.ID] = struct{}{}
	}
	data, err := json.Marshal(state)
	if err != nil || len(data) > maxMissionKnowledgeEncodedBytes {
		return errMissionKnowledgeInvalid
	}
	return nil
}

func usableMissionKnowledgeEntries(mission *MissionRecoveryState) ([]MissionKnowledgeEntry, error) {
	if mission == nil || mission.Knowledge == nil {
		return nil, nil
	}
	if err := validateMissionKnowledgeState(mission.Knowledge); err != nil {
		return nil, err
	}
	return append([]MissionKnowledgeEntry(nil), mission.Knowledge.Entries...), nil
}

func validateMissionKnowledgeProposals(proposals []MissionKnowledgeProposal) error {
	if len(proposals) > maxMissionKnowledgeSeedEntries {
		return fmt.Errorf("%w: too many initial entries", errMissionKnowledgeInvalid)
	}
	now := time.Unix(1, 0).UTC()
	entries := make([]MissionKnowledgeEntry, 0, len(proposals))
	for _, proposal := range proposals {
		entry, err := buildMissionKnowledgeEntry(proposal.Kind, proposal.Summary, proposal.Status, "", "", now, true)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
	}
	state := compactMissionKnowledgeEntries(entries)
	if len(state.Entries) != len(proposals) {
		return fmt.Errorf("%w: initial entries exceed retention limits or duplicate", errMissionKnowledgeInvalid)
	}
	return validateMissionKnowledgeState(&state)
}

func initialMissionKnowledgeState(proposals []MissionKnowledgeProposal, recordedAt time.Time) *MissionKnowledgeState {
	if len(proposals) == 0 || recordedAt.IsZero() {
		return nil
	}
	entries := make([]MissionKnowledgeEntry, 0, len(proposals))
	for _, proposal := range proposals {
		entry, err := buildMissionKnowledgeEntry(proposal.Kind, proposal.Summary, proposal.Status, "", "", recordedAt, true)
		if err != nil {
			return nil
		}
		entries = append(entries, entry)
	}
	state := compactMissionKnowledgeEntries(entries)
	if len(state.Entries) == 0 {
		return nil
	}
	return &state
}

func missionKnowledgeEntriesFromGraph(graph *AgentTaskGraph, recordedAt time.Time) []MissionKnowledgeEntry {
	if graph == nil || recordedAt.IsZero() {
		return nil
	}
	entries := make([]MissionKnowledgeEntry, 0)
	for _, task := range graph.Tasks {
		if task.State == AgentTaskCancelled || task.Result.Status == "" {
			continue
		}
		for _, finding := range task.Result.Findings {
			kind := strings.TrimSpace(finding.Category)
			if !validMissionKnowledgeKind(kind) || kind == missionKnowledgeVerificationEvidence {
				continue
			}
			entry, err := buildMissionKnowledgeEntry(kind, finding.Summary, "", task.ID, task.Role, recordedAt, false)
			if err == nil {
				entries = append(entries, entry)
			}
		}
		for _, testResult := range task.Result.Tests {
			summary := strings.TrimSpace(testResult.Name)
			if summary == "" {
				continue
			}
			entry, err := buildMissionKnowledgeEntry(missionKnowledgeVerificationEvidence, summary, testResult.Status, task.ID, task.Role, recordedAt, false)
			if err == nil {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func compactMissionKnowledgeEntries(entries []MissionKnowledgeEntry) MissionKnowledgeState {
	valid := make([]MissionKnowledgeEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if validateMissionKnowledgeEntry(entry) != nil {
			continue
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			continue
		}
		seen[entry.ID] = struct{}{}
		valid = append(valid, entry)
	}
	sort.SliceStable(valid, func(i, j int) bool {
		if valid[i].RecordedAt.Equal(valid[j].RecordedAt) {
			return valid[i].ID < valid[j].ID
		}
		return valid[i].RecordedAt.Before(valid[j].RecordedAt)
	})
	if len(valid) > maxMissionKnowledgeEntries {
		valid = append([]MissionKnowledgeEntry(nil), valid[len(valid)-maxMissionKnowledgeEntries:]...)
	}
	state := MissionKnowledgeState{SchemaVersion: missionKnowledgeSchemaVersion, Entries: valid}
	for len(state.Entries) > 0 {
		data, err := json.Marshal(state)
		if err == nil && len(data) <= maxMissionKnowledgeEncodedBytes {
			break
		}
		state.Entries = append([]MissionKnowledgeEntry(nil), state.Entries[1:]...)
	}
	return state
}

func mergeMissionKnowledge(existing *MissionKnowledgeState, additions []MissionKnowledgeEntry) *MissionKnowledgeState {
	entries := make([]MissionKnowledgeEntry, 0, len(additions)+maxMissionKnowledgeEntries)
	if existing != nil && validateMissionKnowledgeState(existing) == nil {
		entries = append(entries, existing.Entries...)
	}
	entries = append(entries, additions...)
	state := compactMissionKnowledgeEntries(entries)
	if len(state.Entries) == 0 {
		return nil
	}
	return &state
}

func applyMissionKnowledgeFromGraph(mission *MissionRecoveryState, graph *AgentTaskGraph, recordedAt time.Time) {
	if mission == nil || graph == nil {
		return
	}
	additions := missionKnowledgeEntriesFromGraph(graph, recordedAt)
	if len(additions) == 0 {
		return
	}
	mission.Knowledge = mergeMissionKnowledge(mission.Knowledge, additions)
}
