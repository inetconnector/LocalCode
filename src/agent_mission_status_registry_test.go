// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"testing"
	"time"
)

func TestMissionDesktopStatusRegistryStaysBounded(t *testing.T) {
	agentMissionDesktopStatuses.Lock()
	original := agentMissionDesktopStatuses.byRun
	agentMissionDesktopStatuses.byRun = map[string]AgentMissionDesktopStatus{}
	agentMissionDesktopStatuses.Unlock()
	defer func() {
		agentMissionDesktopStatuses.Lock()
		agentMissionDesktopStatuses.byRun = original
		agentMissionDesktopStatuses.Unlock()
	}()

	base := time.Now().Add(-time.Hour)
	for i := 0; i < agentMissionDesktopStatusLimit+8; i++ {
		publishAgentMissionDesktopStatus(AgentMissionDesktopStatus{
			MissionID:      fmt.Sprintf("mission-%d", i),
			ExecutionRunID: fmt.Sprintf("run-%d", i),
			State:          "running",
			UpdatedAt:      base.Add(time.Duration(i) * time.Second),
		})
	}
	agentMissionDesktopStatuses.RLock()
	count := len(agentMissionDesktopStatuses.byRun)
	_, oldestExists := agentMissionDesktopStatuses.byRun["run-0"]
	agentMissionDesktopStatuses.RUnlock()
	if count > agentMissionDesktopStatusLimit {
		t.Fatalf("registry size=%d exceeds limit=%d", count, agentMissionDesktopStatusLimit)
	}
	if oldestExists {
		t.Fatal("oldest mission status was not evicted")
	}
}
