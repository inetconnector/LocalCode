// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMultiProjectConcurrentRunTracking(t *testing.T) {
	tempRoot := t.TempDir()
	projA := filepath.Join(tempRoot, "ProjectAlpha")
	projB := filepath.Join(tempRoot, "ProjectBeta")

	cfg := Config{
		RootProjectDir: tempRoot,
		LastProject:    projA,
	}
	state := NewAppState(cfg, nil)
	defer state.Close()

	if len(state.GetRunningProjects()) != 0 {
		t.Fatalf("expected 0 running projects initially, got %d", len(state.GetRunningProjects()))
	}
	if state.IsProjectRunning(projA) {
		t.Fatalf("expected projA not running initially")
	}

	// Register run on Project A
	runA := &ActiveAgentRun{
		ID:             "run-A",
		Project:        projA,
		ThreadID:       "thread-A",
		Model:          "model-1",
		Phase:          "working",
		StartedAt:      time.Now(),
		LastProgressAt: time.Now(),
	}
	state.RegisterActiveRun(runA)

	if !state.IsProjectRunning(projA) {
		t.Errorf("expected projA to be running")
	}
	if state.IsProjectRunning(projB) {
		t.Errorf("expected projB NOT to be running")
	}
	if len(state.GetRunningProjects()) != 1 {
		t.Errorf("expected 1 running project, got %d", len(state.GetRunningProjects()))
	}

	// Register run on Project B concurrently
	runB := &ActiveAgentRun{
		ID:             "run-B",
		Project:        projB,
		ThreadID:       "thread-B",
		Model:          "model-2",
		Phase:          "working",
		StartedAt:      time.Now(),
		LastProgressAt: time.Now(),
	}
	state.RegisterActiveRun(runB)

	if !state.IsProjectRunning(projA) {
		t.Errorf("expected projA to still be running")
	}
	if !state.IsProjectRunning(projB) {
		t.Errorf("expected projB to also be running concurrently")
	}
	if len(state.GetRunningProjects()) != 2 {
		t.Errorf("expected 2 running projects, got %d", len(state.GetRunningProjects()))
	}

	// Unregister run on Project A
	state.UnregisterActiveRun(runA.ID)

	if state.IsProjectRunning(projA) {
		t.Errorf("expected projA to be finished")
	}
	if !state.IsProjectRunning(projB) {
		t.Errorf("expected projB to still be running")
	}
	if len(state.GetRunningProjects()) != 1 {
		t.Errorf("expected 1 running project left, got %d", len(state.GetRunningProjects()))
	}

	// Unregister run on Project B
	state.UnregisterActiveRun(runB.ID)
	if state.IsProjectRunning(projB) {
		t.Errorf("expected projB to be finished")
	}
	if len(state.GetRunningProjects()) != 0 {
		t.Errorf("expected 0 running projects, got %d", len(state.GetRunningProjects()))
	}
}
