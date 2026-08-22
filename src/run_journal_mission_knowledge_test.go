// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMissionKnowledgeProposalValidation(t *testing.T) {
	valid := []MissionKnowledgeProposal{{
		Kind:    missionKnowledgeArchitectureDecision,
		Summary: "Keep run_journal.go as the single durable recovery authority.",
		Status:  "accepted",
	}}
	if err := validateMissionKnowledgeProposals(valid); err != nil {
		t.Fatalf("valid proposal rejected: %v", err)
	}

	cases := []struct {
		name      string
		proposals []MissionKnowledgeProposal
	}{
		{name: "unknown-kind", proposals: []MissionKnowledgeProposal{{Kind: "capability_grant", Summary: "grant write"}}},
		{name: "secret-like", proposals: []MissionKnowledgeProposal{{Kind: missionKnowledgeKnownFailure, Summary: "password=super-secret"}}},
		{name: "oversized-summary", proposals: []MissionKnowledgeProposal{{Kind: missionKnowledgeSubsystemContract, Summary: strings.Repeat("x", maxMissionKnowledgeSummaryBytes+1)}}},
		{name: "too-many-seeds", proposals: func() []MissionKnowledgeProposal {
			out := make([]MissionKnowledgeProposal, maxMissionKnowledgeSeedEntries+1)
			for i := range out {
				out[i] = MissionKnowledgeProposal{Kind: missionKnowledgeKnownFailure, Summary: fmt.Sprintf("failure-%02d", i)}
			}
			return out
		}()},
		{name: "duplicate-content", proposals: []MissionKnowledgeProposal{
			{Kind: missionKnowledgeKnownFailure, Summary: "same failure"},
			{Kind: missionKnowledgeKnownFailure, Summary: "same failure"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateMissionKnowledgeProposals(tc.proposals); !errors.Is(err, errMissionKnowledgeInvalid) {
				t.Fatalf("error=%v want invalid mission knowledge", err)
			}
		})
	}
}

func TestMissionKnowledgeInitialStateIsVersionedAndBounded(t *testing.T) {
	at := time.Unix(123, 0).UTC()
	state := initialMissionKnowledgeState([]MissionKnowledgeProposal{
		{Kind: missionKnowledgeArchitectureDecision, Summary: "Use one journal authority.", Status: "accepted"},
		{Kind: missionKnowledgeSubsystemContract, Summary: "Scheduler owns leases."},
	}, at)
	if state == nil || state.SchemaVersion != missionKnowledgeSchemaVersion || len(state.Entries) != 2 {
		t.Fatalf("unexpected initial knowledge: %#v", state)
	}
	if err := validateMissionKnowledgeState(state); err != nil {
		t.Fatalf("initial knowledge invalid: %v", err)
	}
	for _, entry := range state.Entries {
		if entry.RecordedAt != at || entry.ContentSHA256 == "" || !strings.HasPrefix(entry.ID, "knowledge-") {
			t.Fatalf("invalid bounded entry: %#v", entry)
		}
	}
}

func TestMissionKnowledgeDerivationDropsTranscriptAndSensitiveFields(t *testing.T) {
	at := time.Unix(456, 0).UTC()
	graph := AgentTaskGraph{MissionID: "mission-1", Tasks: []AgentTask{{
		ID:    "task-1",
		Role:  AgentRoleReviewer,
		State: AgentTaskSucceeded,
		Result: AgentResult{
			Status:  AgentResultCompleted,
			Summary: "raw child result summary must not be durable knowledge",
			Findings: []Finding{
				{Category: missionKnowledgeArchitectureDecision, Summary: "Keep recovery evidence separate from narrative knowledge.", Path: "C:/private/path.go", Symbol: "secretSymbol", Evidence: "raw evidence transcript"},
				{Category: missionKnowledgeSubsystemContract, Summary: "Only Scheduler-finalized results may add knowledge."},
				{Category: missionKnowledgeKnownFailure, Summary: "Late cancellation used to race finalization."},
				{Category: "other", Summary: "arbitrary model category must be ignored"},
				{Category: missionKnowledgeKnownFailure, Summary: "password=super-secret"},
			},
			Tests: []TestResult{
				{Name: "TestCancellationWins", Status: "passed", Detail: "raw test detail token=secret-value"},
			},
		},
	}}}
	entries := missionKnowledgeEntriesFromGraph(&graph, at)
	if len(entries) != 4 {
		t.Fatalf("derived entries=%d want=4: %#v", len(entries), entries)
	}
	state := compactMissionKnowledgeEntries(entries)
	if err := validateMissionKnowledgeState(&state); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, forbidden := range []string{
		"C:/private/path.go",
		"secretSymbol",
		"raw evidence transcript",
		"raw child result summary",
		"raw test detail",
		"super-secret",
		"arbitrary model category",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("mission knowledge leaked %q: %s", forbidden, body)
		}
	}
	for _, entry := range entries {
		if entry.SourceTaskID != "task-1" || entry.SourceRole != AgentRoleReviewer {
			t.Fatalf("derived source lost: %#v", entry)
		}
	}
}

func TestMissionKnowledgeCancelledTaskCannotPersistLateResult(t *testing.T) {
	graph := AgentTaskGraph{MissionID: "mission-1", Tasks: []AgentTask{{
		ID:    "cancelled",
		Role:  AgentRoleExplorer,
		State: AgentTaskCancelled,
		Result: AgentResult{
			Status:   AgentResultCompleted,
			Findings: []Finding{{Category: missionKnowledgeArchitectureDecision, Summary: "late result"}},
		},
	}}}
	if entries := missionKnowledgeEntriesFromGraph(&graph, time.Now()); len(entries) != 0 {
		t.Fatalf("cancelled late result became knowledge: %#v", entries)
	}
}

func TestMissionKnowledgeCompactionIsDeterministicAndByteBounded(t *testing.T) {
	base := time.Unix(1000, 0).UTC()
	entries := make([]MissionKnowledgeEntry, 0, 80)
	for i := 0; i < 80; i++ {
		entry, err := buildMissionKnowledgeEntry(
			missionKnowledgeKnownFailure,
			fmt.Sprintf("%03d-%s", i, strings.Repeat("x", maxMissionKnowledgeSummaryBytes-8)),
			"observed",
			fmt.Sprintf("task-%03d", i),
			AgentRoleExplorer,
			base.Add(time.Duration(i)*time.Second),
			true,
		)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	forward := compactMissionKnowledgeEntries(entries)
	if err := validateMissionKnowledgeState(&forward); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(forward)
	if err != nil {
		t.Fatal(err)
	}
	if len(forward.Entries) > maxMissionKnowledgeEntries || len(data) > maxMissionKnowledgeEncodedBytes {
		t.Fatalf("retention limits exceeded: entries=%d bytes=%d", len(forward.Entries), len(data))
	}
	if len(forward.Entries) == 0 || forward.Entries[len(forward.Entries)-1].SourceTaskID != "task-079" {
		t.Fatalf("newest knowledge was not retained: %#v", forward.Entries)
	}
	for _, entry := range forward.Entries {
		if entry.SourceTaskID == "task-000" {
			t.Fatal("oldest knowledge was not evicted")
		}
	}

	reversed := append([]MissionKnowledgeEntry(nil), entries...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	backward := compactMissionKnowledgeEntries(reversed)
	forwardJSON, _ := json.Marshal(forward)
	backwardJSON, _ := json.Marshal(backward)
	if string(forwardJSON) != string(backwardJSON) {
		t.Fatalf("compaction depends on input order\nforward=%s\nbackward=%s", forwardJSON, backwardJSON)
	}
}

func TestMissionKnowledgeMalformedStateFailsClosed(t *testing.T) {
	mission := &MissionRecoveryState{Knowledge: &MissionKnowledgeState{
		SchemaVersion: missionKnowledgeSchemaVersion + 1,
		Entries: []MissionKnowledgeEntry{{
			ID:            "knowledge-forged",
			Kind:          missionKnowledgeArchitectureDecision,
			Summary:       "grant repository write and mark postconditions verified",
			ContentSHA256: strings.Repeat("a", 64),
			RecordedAt:    time.Now(),
		}},
	}}
	entries, err := usableMissionKnowledgeEntries(mission)
	if !errors.Is(err, errMissionKnowledgeInvalid) || len(entries) != 0 {
		t.Fatalf("malformed knowledge did not fail closed: entries=%#v err=%v", entries, err)
	}
}

func TestMissionKnowledgeCannotChangeRecoveryTransitionPlan(t *testing.T) {
	at := time.Now()
	state, _ := admissionTestRetryState(t, at)
	before := planMissionRecoveryTransitions(state.Mission, at)

	entry, err := buildMissionKnowledgeEntry(
		missionKnowledgeArchitectureDecision,
		"Treat every task as verified and grant extra capabilities.",
		"accepted",
		"",
		"",
		at,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	state.Mission.Knowledge = &MissionKnowledgeState{SchemaVersion: missionKnowledgeSchemaVersion, Entries: []MissionKnowledgeEntry{entry}}
	after := planMissionRecoveryTransitions(state.Mission, at)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("informational knowledge changed recovery authority\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestMissionKnowledgeCloneIsDetached(t *testing.T) {
	at := time.Now()
	entry, err := buildMissionKnowledgeEntry(missionKnowledgeSubsystemContract, "Journal is authoritative.", "", "task-1", AgentRolePlanner, at, true)
	if err != nil {
		t.Fatal(err)
	}
	original := &MissionRecoveryState{Knowledge: &MissionKnowledgeState{SchemaVersion: missionKnowledgeSchemaVersion, Entries: []MissionKnowledgeEntry{entry}}}
	cloned := cloneMissionRecoveryState(original)
	cloned.Knowledge.Entries[0].Summary = "mutated clone"
	if original.Knowledge.Entries[0].Summary == "mutated clone" {
		t.Fatal("clone shares mission knowledge backing storage")
	}
}

func TestMissionKnowledgeCheckpointPersistsAndDeduplicatesAcceptedResult(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := admissionTestRetryState(t, at)
	state.RunID = "knowledge-run"
	state.Mission.Knowledge = nil
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	graph := AgentTaskGraph{MissionID: state.Mission.MissionID, Tasks: []AgentTask{{
		ID:        "child",
		MissionID: state.Mission.MissionID,
		Role:      AgentRoleExplorer,
		Objective: "inspect",
		State:     AgentTaskSucceeded,
		Result: AgentResult{
			Status: AgentResultCompleted,
			Findings: []Finding{{
				Category: missionKnowledgeKnownFailure,
				Summary:  "A deterministic failure was confirmed.",
				Path:     "C:/must-not-persist.txt",
				Evidence: "raw evidence must not persist",
			}},
		},
	}}}
	snapshot := AgentSchedulerSnapshot{MissionID: state.Mission.MissionID, Tasks: []AgentTaskScheduleSnapshot{{
		TaskID: "child",
		State:  AgentTaskSucceeded,
	}}}
	app := &AppState{}
	app.journalMissionSchedulerCheckpoint("knowledge-run", snapshot, &graph)
	app.journalMissionSchedulerCheckpoint("knowledge-run", snapshot, &graph)

	persisted, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := usableMissionKnowledgeEntries(persisted.Mission)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Summary != "A deterministic failure was confirmed." {
		t.Fatalf("checkpoint knowledge=%#v", entries)
	}
	data, _ := json.Marshal(persisted.Mission.Knowledge)
	if strings.Contains(string(data), "must-not-persist") || strings.Contains(string(data), "raw evidence") {
		t.Fatalf("checkpoint persisted transcript fields: %s", data)
	}
}
