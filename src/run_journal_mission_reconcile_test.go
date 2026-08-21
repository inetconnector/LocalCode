// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func missionObservedBaseline(project, root, head string, status []byte, at time.Time) MissionProjectBaseline {
	return MissionProjectBaseline{
		ProjectIdentitySHA256: missionProjectIdentity(project),
		GitState:              missionGitStateObserved,
		GitRootSHA256:         missionSHA256String(filepath.Clean(root)),
		GitHead:               head,
		GitStatusSHA256:       missionSHA256Bytes(status),
		CapturedAt:            at,
	}
}

func TestMissionRestartReconciliationMatchedNeverTreatsRunningAsSuccess(t *testing.T) {
	project := t.TempDir()
	now := time.Now()
	baseline := missionObservedBaseline(project, project, "abc123", nil, now.Add(-time.Minute))
	mission := &MissionRecoveryState{
		MissionID: "mission-reconcile",
		Project:   project,
		Baseline:  &baseline,
		Tasks: []MissionRecoveryTaskState{
			{ID: "done", State: AgentTaskSucceeded},
			{ID: "running", State: AgentTaskRunning, Running: true},
			{ID: "ready", State: AgentTaskReady},
			{ID: "failed", State: AgentTaskFailed},
		},
	}
	current := missionObservedBaseline(project, project, "abc123", nil, now)

	reconciliation := reconcileMissionRecoveryWithCurrent(mission, current, now)
	if reconciliation.State != missionReconcileMatched {
		t.Fatalf("state=%q reason=%q want=%q", reconciliation.State, reconciliation.Reason, missionReconcileMatched)
	}
	if len(reconciliation.Tasks) != 4 {
		t.Fatalf("task reconciliations=%d want=4", len(reconciliation.Tasks))
	}
	byID := make(map[string]MissionRecoveryTaskReconciliation, len(reconciliation.Tasks))
	for _, task := range reconciliation.Tasks {
		byID[task.TaskID] = task
	}
	if got := byID["done"].Disposition; got != missionTaskDispositionVerifyPostconditions {
		t.Fatalf("succeeded disposition=%q", got)
	}
	if got := byID["running"].Disposition; got != missionTaskDispositionInterruptedUnknown {
		t.Fatalf("running disposition=%q", got)
	}
	if got := byID["running"].Reason; got != "running_at_interruption_is_not_success" {
		t.Fatalf("running reason=%q", got)
	}
	if got := byID["ready"].Disposition; got != missionTaskDispositionPending {
		t.Fatalf("ready disposition=%q", got)
	}
	if got := byID["failed"].Disposition; got != missionTaskDispositionTerminal {
		t.Fatalf("failed disposition=%q", got)
	}
}

func TestMissionRestartReconciliationBlocksGitDrift(t *testing.T) {
	project := t.TempDir()
	now := time.Now()
	baseline := missionObservedBaseline(project, project, "abc123", nil, now.Add(-time.Minute))
	mission := &MissionRecoveryState{
		MissionID: "mission-drift",
		Project:   project,
		Baseline:  &baseline,
		Tasks: []MissionRecoveryTaskState{
			{ID: "done", State: AgentTaskSucceeded},
			{ID: "ready", State: AgentTaskReady},
			{ID: "running", State: AgentTaskRunning, Running: true},
		},
	}
	current := missionObservedBaseline(project, project, "abc123", []byte(" M changed.go\x00"), now)

	reconciliation := reconcileMissionRecoveryWithCurrent(mission, current, now)
	if reconciliation.State != missionReconcileGitChanged || reconciliation.Reason != "git_worktree_changed" {
		t.Fatalf("unexpected reconciliation: %#v", reconciliation)
	}
	byID := make(map[string]MissionRecoveryTaskReconciliation, len(reconciliation.Tasks))
	for _, task := range reconciliation.Tasks {
		byID[task.TaskID] = task
	}
	if byID["done"].Disposition != missionTaskDispositionBlocked || byID["ready"].Disposition != missionTaskDispositionBlocked {
		t.Fatalf("drift did not block reusable/pending work: %#v", reconciliation.Tasks)
	}
	if byID["running"].Disposition != missionTaskDispositionInterruptedUnknown {
		t.Fatalf("crash-running task lost unknown outcome: %#v", byID["running"])
	}
}

func TestMissionRestartReconciliationWithoutBaselineIsInsufficientEvidence(t *testing.T) {
	project := t.TempDir()
	now := time.Now()
	mission := &MissionRecoveryState{
		MissionID: "legacy-mission",
		Project:   project,
		Tasks: []MissionRecoveryTaskState{{
			ID:    "done",
			State: AgentTaskSucceeded,
		}},
	}
	current := missionObservedBaseline(project, project, "abc123", nil, now)

	reconciliation := reconcileMissionRecoveryWithCurrent(mission, current, now)
	if reconciliation.State != missionReconcileInsufficientEvidence || reconciliation.Reason != "missing_baseline" {
		t.Fatalf("unexpected legacy reconciliation: %#v", reconciliation)
	}
	if len(reconciliation.Tasks) != 1 || reconciliation.Tasks[0].Disposition != missionTaskDispositionBlocked {
		t.Fatalf("legacy task was not blocked: %#v", reconciliation.Tasks)
	}
}

func TestObserveMissionProjectBaselineHashesPorcelainWithoutPersistingPaths(t *testing.T) {
	project := t.TempDir()
	secretPath := "sensitive/customer-name.txt"
	calls := 0
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "--show-toplevel"):
			return []byte(project + "\n"), nil
		case strings.Contains(joined, "--verify HEAD"):
			return []byte("abc123\n"), nil
		case strings.Contains(joined, "status --porcelain=v1"):
			return []byte("?? " + secretPath + "\x00"), nil
		default:
			return nil, errors.New("unexpected git args")
		}
	}

	baseline := observeMissionProjectBaseline(project, time.Now(), runner)
	if baseline.GitState != missionGitStateObserved || baseline.GitStatusSHA256 == "" || calls != 3 {
		t.Fatalf("unexpected baseline: %#v calls=%d", baseline, calls)
	}
	data, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secretPath) {
		t.Fatalf("raw git status path leaked into durable baseline: %s", data)
	}
}

func TestMissionGitObserverUsesFixedReadOnlyCommands(t *testing.T) {
	project := t.TempDir()
	var seen [][]string
	runner := func(_ context.Context, args ...string) ([]byte, error) {
		seen = append(seen, append([]string(nil), args...))
		switch len(seen) {
		case 1:
			return []byte(project + "\n"), nil
		case 2:
			return []byte("abc123\n"), nil
		case 3:
			return nil, nil
		default:
			return nil, errors.New("unexpected call")
		}
	}

	observeMissionProjectBaseline(project, time.Now(), runner)
	want := [][]string{
		{"-C", project, "rev-parse", "--show-toplevel"},
		{"-C", project, "rev-parse", "--verify", "HEAD"},
		{"-C", project, "status", "--porcelain=v1", "-z", "--untracked-files=all"},
	}
	if len(seen) != len(want) {
		t.Fatalf("git calls=%#v", seen)
	}
	for index := range want {
		if strings.Join(seen[index], "\x00") != strings.Join(want[index], "\x00") {
			t.Fatalf("git call %d=%#v want=%#v", index, seen[index], want[index])
		}
	}
}

func TestInterruptedMissionRemainsRecoverableWhenProjectIsUnavailable(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	project := filepath.Join(t.TempDir(), "missing-project")
	baseline := MissionProjectBaseline{
		ProjectIdentitySHA256: missionSHA256String(filepath.Clean(project)),
		GitState:              missionGitStateObserved,
		GitRootSHA256:         missionSHA256String(filepath.Clean(project)),
		GitHead:               "abc123",
		GitStatusSHA256:       missionSHA256Bytes(nil),
		CapturedAt:            time.Now().Add(-time.Minute),
	}
	state := RunRecoveryState{
		SchemaVersion: runJournalSchemaVersion,
		RunID:         "execution-missing-project",
		Project:       project,
		Phase:         "mission-read-only",
		StartedAt:     time.Now().Add(-time.Minute),
		Mission: &MissionRecoveryState{
			Kind:      missionRecoveryKindReadOnly,
			MissionID: "mission-missing-project",
			Project:   project,
			State:     missionRecoveryRunning,
			Baseline:  &baseline,
			Tasks: []MissionRecoveryTaskState{{
				ID:      "inspect",
				State:   AgentTaskRunning,
				Running: true,
			}},
		},
	}
	if err := writeRunJournal(state); err != nil {
		t.Fatal(err)
	}

	recovered := loadRecoverableRun()
	if recovered == nil || recovered.Mission == nil || recovered.Mission.Reconciliation == nil {
		t.Fatalf("missing-project Mission disappeared: %#v", recovered)
	}
	if recovered.Mission.Reconciliation.State != missionReconcileProjectUnavailable {
		t.Fatalf("state=%q want=%q", recovered.Mission.Reconciliation.State, missionReconcileProjectUnavailable)
	}
	if got := recovered.Mission.Reconciliation.Tasks[0].Disposition; got != missionTaskDispositionInterruptedUnknown {
		t.Fatalf("running disposition=%q", got)
	}
	if _, err := os.Stat(project); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("test project unexpectedly exists: %v", err)
	}
}
