// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMissionRecoveryTransportSnapshot(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{
		Config: Config{
			LastModel: "test-model",
		},
		Recovery: loadRecoverableRun(),
	}

	server := httptest.NewServer(NewServer(appState))
	defer server.Close()

	// 1. Explicit run_id in query
	resp, err := server.Client().Get(server.URL + "/api/mission/recovery?run_id=" + state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot GET status=%d body=%s", resp.StatusCode, body)
	}
	var snapshot MissionRecoveryControlSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot failed: %v\nbody: %s", err, body)
	}
	if snapshot.RunID != state.RunID || snapshot.MissionID != "admission-mission" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.ReconciliationState != "matched" {
		t.Fatalf("reconciliation state=%q want matched", snapshot.ReconciliationState)
	}

	// 2. Implicit run_id from AppState.Recovery
	respAuto, err := server.Client().Get(server.URL + "/api/mission/recovery")
	if err != nil {
		t.Fatal(err)
	}
	bodyAuto, _ := io.ReadAll(respAuto.Body)
	_ = respAuto.Body.Close()
	if respAuto.StatusCode != http.StatusOK {
		t.Fatalf("auto-recovery GET status=%d body=%s", respAuto.StatusCode, bodyAuto)
	}

	// 3. Rejection when agent is running (409 Conflict)
	appState.mu.Lock()
	appState.Running = true
	appState.mu.Unlock()

	respRunning, err := server.Client().Get(server.URL + "/api/mission/recovery?run_id=" + state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_ = respRunning.Body.Close()
	if respRunning.StatusCode != http.StatusConflict {
		t.Fatalf("running status=%d want 409", respRunning.StatusCode)
	}

	appState.mu.Lock()
	appState.Running = false
	appState.mu.Unlock()

	// 4. Missing run_id when Recovery is nil
	appState.mu.Lock()
	appState.Recovery = nil
	appState.mu.Unlock()

	respMissing, err := server.Client().Get(server.URL + "/api/mission/recovery")
	if err != nil {
		t.Fatal(err)
	}
	_ = respMissing.Body.Close()
	if respMissing.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing run_id status=%d want 400", respMissing.StatusCode)
	}
}

func TestMissionRecoveryTransportContinuationPreview(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{
		Config: Config{
			LastModel: "test-model",
		},
	}

	server := httptest.NewServer(NewServer(appState))
	defer server.Close()

	payload := `{"run_id":"` + state.RunID + `","task_id":"child","preview":true}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/mission/recovery/continuation", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", server.URL)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", resp.StatusCode, body)
	}

	var result struct {
		OK              bool                                       `json:"ok"`
		Preview         bool                                       `json:"preview"`
		Materialization MissionRecoveryContinuationMaterialization `json:"materialization"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal preview response failed: %v", err)
	}
	if !result.OK || !result.Preview || result.Materialization.TaskID != "child" {
		t.Fatalf("unexpected preview result: %#v", result)
	}

	// Verify journal on disk was NOT mutated by preview
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != state.RunID {
		t.Fatalf("journal RunID changed after preview: %q", loaded.RunID)
	}
	childTask := loaded.Mission.Tasks[1]
	if childTask.State != AgentTaskPending {
		t.Fatalf("child task state changed after preview: %q", childTask.State)
	}
}

func TestMissionRecoveryTransportContinuationExecution(t *testing.T) {
	withMissionRecoveryControlJournal(t)
	at := time.Now()
	state, _ := prepareMissionRecoveryAdmissionTestState(t, at)
	if err := writeRunJournal(*state); err != nil {
		t.Fatal(err)
	}

	appState := &AppState{
		Config: Config{
			LastModel: "test-model",
		},
	}

	server := httptest.NewServer(NewServer(appState))
	defer server.Close()

	payload := `{"run_id":"` + state.RunID + `","task_id":"child","preview":false}`
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/mission/recovery/continuation", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", server.URL)

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("execution status=%d body=%s", resp.StatusCode, body)
	}

	var result struct {
		OK        bool                                 `json:"ok"`
		Preview   bool                                 `json:"preview"`
		Execution MissionRecoveryContinuationExecution `json:"execution"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("unmarshal execution response failed: %v", err)
	}
	if !result.OK || result.Preview || result.Execution.TaskID != "child" {
		t.Fatalf("unexpected execution result: %#v", result)
	}

	// Verify journal was updated
	loaded, err := loadRunJournal()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID == state.RunID {
		t.Fatal("journal RunID was not rotated to fresh execution RunID")
	}
	childTask := loaded.Mission.Tasks[1]
	if childTask.State != AgentTaskSucceeded {
		t.Fatalf("child task state=%q want succeeded", childTask.State)
	}
}

func TestMissionRecoveryTransportSecurityAndIsolation(t *testing.T) {
	appState := &AppState{}
	desktop := httptest.NewServer(NewServer(appState))
	defer desktop.Close()

	// 1. Cross-Origin check: Forbidden Origin rejected with 403
	req, err := http.NewRequest(http.MethodPost, desktop.URL+"/api/mission/recovery/continuation", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil-site.com")
	resp, err := desktop.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin post status=%d want 403", resp.StatusCode)
	}

	// 2. Mobile Remote Isolation: /remote/api/mission/* does not exist (404)
	remote := httptest.NewServer(NewRemoteServer(appState))
	defer remote.Close()

	remoteResp, err := remote.Client().Get(remote.URL + "/remote/api/mission/recovery")
	if err != nil {
		t.Fatal(err)
	}
	_ = remoteResp.Body.Close()
	if remoteResp.StatusCode != http.StatusNotFound {
		t.Fatalf("remote mission recovery route status=%d want 404", remoteResp.StatusCode)
	}
}
