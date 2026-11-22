// SPDX-License-Identifier: Apache-2.0
//
// Registry eviction regression: existing missions must never disappear just
// because another existing mission emits a new live status sample.
package main

import (
	"fmt"
	"testing"
	"time"
)

func TestMissionStatusFullRegistryUpdatePreservesOtherRuns(t *testing.T) {
	agentMissionDesktopStatuses.Lock()
	original := agentMissionDesktopStatuses.byRun
	agentMissionDesktopStatuses.byRun = make(map[string]AgentMissionDesktopStatus)
	agentMissionDesktopStatuses.Unlock()
	defer func() {
		agentMissionDesktopStatuses.Lock()
		agentMissionDesktopStatuses.byRun = original
		agentMissionDesktopStatuses.Unlock()
	}()

	base := time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)
	for i := 0; i < agentMissionDesktopStatusLimit; i++ {
		publishAgentMissionDesktopStatus(AgentMissionDesktopStatus{
			ExecutionRunID: fmt.Sprintf("eviction-run-%d", i),
			MissionID: fmt.Sprintf("mission-%d", i),
			State: "running",
			UpdatedAt: base.Add(time.Duration(i) * time.Second),
		})
	}

	// Previous behavior would evict eviction-run-0 on the very first update
	// to eviction-run-31 even though the registry did not grow.
	for round := 0; round < 3; round++ {
		publishAgentMissionDesktopStatus(AgentMissionDesktopStatus{
			ExecutionRunID: "eviction-run-31",
			MissionID: "mission-31",
			State: "running",
			UpdatedAt: base.Add(time.Duration(50+round) * time.Second),
		})
	}
	agentMissionDesktopStatuses.RLock()
	countAfterUpdates := len(agentMissionDesktopStatuses.byRun)
	missing := []string{}
	for i := 0; i < agentMissionDesktopStatusLimit; i++ {
		runID := fmt.Sprintf("eviction-run-%d", i)
		if _, ok := agentMissionDesktopStatuses.byRun[runID]; !ok {
			missing = append(missing, runID)
		}
	}
	agentMissionDesktopStatuses.RUnlock()
	if countAfterUpdates != agentMissionDesktopStatusLimit || len(missing) != 0 {
		t.Fatalf("existing-run status refresh evicted another run: count=%d missing=%v", countAfterUpdates, missing)
	}

	// A genuinely new run must still evict exactly the oldest status.
	publishAgentMissionDesktopStatus(AgentMissionDesktopStatus{
		ExecutionRunID: "eviction-new-run",
		MissionID: "mission-new",
		State: "running",
		UpdatedAt: base.Add(time.Minute),
	})
	agentMissionDesktopStatuses.RLock()
	defer agentMissionDesktopStatuses.RUnlock()
	if len(agentMissionDesktopStatuses.byRun) != agentMissionDesktopStatusLimit {
		t.Fatalf("new run exceeded capacity: got=%d", len(agentMissionDesktopStatuses.byRun))
	}
	if _, stillThere := agentMissionDesktopStatuses.byRun["eviction-run-0"]; stillThere {
		t.Fatal("oldest status not evicted on genuinely new insertion")
	}
	if _, exists := agentMissionDesktopStatuses.byRun["eviction-new-run"]; !exists {
		t.Fatal("new run missing")
	}
	for i := 1; i < agentMissionDesktopStatusLimit; i++ {
		if _, exists := agentMissionDesktopStatuses.byRun[fmt.Sprintf("eviction-run-%d", i)]; !exists {
			t.Fatalf("unrelated mission evicted: %d", i)
		}
	}
}
