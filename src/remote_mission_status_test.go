// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRemoteStatusExposesMissionPhaseWithoutMissionPayload(t *testing.T) {
	state := newRemoteTestState(t)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "phone")
	if err != nil {
		t.Fatal(err)
	}

	state.mu.Lock()
	state.Running = true
	state.RunID = "execution-test"
	state.RunPhase = "mission-read-only"
	state.mu.Unlock()

	rr := serveHTTP(NewRemoteServer(state), http.MethodGet, "/remote/api/status", "", token)
	if rr.Code != http.StatusOK {
		t.Fatalf("remote status = %d body=%s", rr.Code, rr.Body.String())
	}
	var status map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["run_phase"] != "mission-read-only" || status["running"] != true {
		t.Fatalf("remote status missing active mission phase: %#v", status)
	}
	if _, exists := status["mission"]; exists {
		t.Fatal("Remote status must not expose the richer Desktop mission payload")
	}
}

func TestRemoteMissionUIUsesOnlyExistingReadOnlyRunStatus(t *testing.T) {
	data, err := staticFS.ReadFile("static/remote.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, marker := range []string{
		"state.status?.run_phase==='mission-read-only'",
		"mission_running:'Read-only-Mission läuft auf dem Windows-Rechner.'",
		"mission_running:'Read-only Mission is running on the Windows host.'",
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("Remote mission status marker missing: %q", marker)
		}
	}
	for _, forbidden := range []string{
		"/remote/api/mission",
		"mission_id",
		"budget_exhausted_by",
		"usage_by_task",
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("Remote mission UI must stay narrow; found %q", forbidden)
		}
	}
}
