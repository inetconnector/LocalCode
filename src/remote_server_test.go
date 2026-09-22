// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRemoteTestState(t *testing.T) *AppState {
	t.Helper()
	base := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(base, "config"))
	root := filepath.Join(base, "projects")
	project := filepath.Join(root, "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.RemoteEnabled = true
	cfg.RemoteBindHost = "127.0.0.1"
	cfg.RemotePort = 0
	state := NewAppState(normalizeConfig(cfg), NewOllamaClient())
	t.Cleanup(state.Close)
	state.Project = project
	state.Model = "test-model"
	state.RemoteURLs = []string{"http://127.0.0.1:32146/remote"}
	return state
}

func serveHTTP(handler http.Handler, method, target, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+target, strings.NewReader(body))
	req.Host = "127.0.0.1"
	if token != "" {
		req.Header.Set("X-LocalCode-Remote-Token", token)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestRemoteRejectsCrossOriginMutations(t *testing.T) {
	state := newRemoteTestState(t)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/remote/api/pair", strings.NewReader(`{"code":"`+code+`","device_name":"test"}`))
	req.Host = "127.0.0.1:32146"
	req.Header.Set("Origin", "http://127.0.0.1:9999")
	rr := httptest.NewRecorder()
	NewRemoteServer(state).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("cross-origin pair status = %d", rr.Code)
	}
}

func TestRemotePairingTokenAndStatus(t *testing.T) {
	state := newRemoteTestState(t)
	remote := NewRemoteServer(state)

	unauth := serveHTTP(remote, http.MethodGet, "/remote/api/status", "", "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status code = %d", unauth.Code)
	}

	local := NewServer(state)
	pairing := serveHTTP(local, http.MethodPost, "/api/remote/pairing", "{}", "")
	if pairing.Code != http.StatusOK {
		t.Fatalf("pairing code status = %d body=%s", pairing.Code, pairing.Body.String())
	}
	var pairingBody struct {
		Code       string   `json:"code"`
		RemoteURLs []string `json:"remote_urls"`
	}
	if err := json.Unmarshal(pairing.Body.Bytes(), &pairingBody); err != nil {
		t.Fatal(err)
	}
	if len(pairingBody.Code) != 8 || len(pairingBody.RemoteURLs) == 0 {
		t.Fatalf("unexpected pairing response: %#v", pairingBody)
	}
	for _, r := range pairingBody.Code {
		if r < '0' || r > '9' {
			t.Fatalf("pairing code must be numeric: %q", pairingBody.Code)
		}
	}

	badPair := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"code":"00000000","device_name":"test"}`, "")
	if badPair.Code != http.StatusForbidden {
		t.Fatalf("bad pair status = %d", badPair.Code)
	}

	goodPair := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"code":"`+pairingBody.Code+`","device_name":"S25"}`, "")
	if goodPair.Code != http.StatusOK {
		t.Fatalf("good pair status = %d body=%s", goodPair.Code, goodPair.Body.String())
	}
	var paired struct {
		Token  string       `json:"token"`
		Device RemoteDevice `json:"device"`
	}
	if err := json.Unmarshal(goodPair.Body.Bytes(), &paired); err != nil {
		t.Fatal(err)
	}
	if paired.Token == "" || paired.Device.Name != "S25" {
		t.Fatalf("unexpected pair result: %#v", paired)
	}
	state.mu.RLock()
	devices := append([]RemoteDevice(nil), state.Config.RemoteDevices...)
	state.mu.RUnlock()
	if len(devices) != 1 || devices[0].TokenHash == paired.Token || devices[0].TokenHash == "" {
		t.Fatalf("token was not stored as hash only: %#v", devices)
	}

	status := serveHTTP(remote, http.MethodGet, "/remote/api/status", "", paired.Token)
	if status.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d body=%s", status.Code, status.Body.String())
	}
	if !strings.Contains(status.Body.String(), `"LocalCode Remote"`) || !strings.Contains(status.Body.String(), `"test-model"`) {
		t.Fatalf("status response missing remote facts: %s", status.Body.String())
	}

	queryToken := serveHTTP(remote, http.MethodGet, "/remote/api/status?token="+paired.Token, "", "")
	if queryToken.Code != http.StatusUnauthorized {
		t.Fatalf("ordinary API must reject query token, got %d", queryToken.Code)
	}
}

func TestRemotePairingLocksAfterFailedAttempts(t *testing.T) {
	state := newRemoteTestState(t)
	remote := NewRemoteServer(state)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < remotePairingMaxAttempts; i++ {
		rr := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"code":"99999999","device_name":"attacker"}`, "")
		if rr.Code != http.StatusForbidden {
			t.Fatalf("failed pairing attempt %d = %d", i+1, rr.Code)
		}
	}
	state.mu.RLock()
	pairing := state.RemotePairing
	state.mu.RUnlock()
	if pairing != nil {
		t.Fatal("pairing window must be invalidated after failed-attempt budget")
	}
	validAfterLockout := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"code":"`+code+`","device_name":"phone"}`, "")
	if validAfterLockout.Code != http.StatusForbidden {
		t.Fatalf("consumed pairing session accepted valid code after lockout: %d", validAfterLockout.Code)
	}
}

func TestRemotePairingBodyLimit(t *testing.T) {
	state := newRemoteTestState(t)
	remote := NewRemoteServer(state)
	body := `{"code":"00000000","device_name":"` + strings.Repeat("x", remotePairingMaxBody) + `"}`
	rr := serveHTTP(remote, http.MethodPost, "/remote/api/pair", body, "")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("oversized pairing body = %d", rr.Code)
	}
}

func TestRemoteApprovalDecision(t *testing.T) {
	state := newRemoteTestState(t)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "phone")
	if err != nil {
		t.Fatal(err)
	}
	pending := &PendingAction{
		ID:     "p1",
		Action: AgentAction{Action: "run_command", Message: "Run tests?", Command: "go test ./..."},
		Result: make(chan ApprovalDecision, 1),
	}
	state.mu.Lock()
	state.Pending = pending
	state.mu.Unlock()

	remote := NewRemoteServer(state)
	rr := serveHTTP(remote, http.MethodPost, "/remote/api/approve", `{"id":"p1","decision":"project","approve":true}`, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("remote approve = %d body=%s", rr.Code, rr.Body.String())
	}
	decision := <-pending.Result
	if !decision.Approved || !decision.Persist || decision.Scope != "project" {
		t.Fatalf("unexpected approval decision: %#v", decision)
	}
}

func TestRemotePageServed(t *testing.T) {
	state := newRemoteTestState(t)
	rr := serveHTTP(NewRemoteServer(state), http.MethodGet, "/remote", "", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("remote page = %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "LocalCode Remote") || !strings.Contains(rr.Body.String(), "localcodeRemoteToken") {
		t.Fatalf("remote page is missing expected app markers")
	}
}

func TestRemoteAutoPairDirect(t *testing.T) {
	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteAutoPair = true
	state.mu.Unlock()

	remote := NewRemoteServer(state)
	rr := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"auto":true,"device_name":"Samsung Galaxy S24"}`, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("direct auto pair code = %d body=%s", rr.Code, rr.Body.String())
	}
	var res struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Token == "" {
		t.Fatal("expected non-empty token from direct auto pair")
	}

	// Verify token works on remote status
	statusRR := serveHTTP(remote, http.MethodGet, "/remote/api/status", "", res.Token)
	if statusRR.Code != http.StatusOK {
		t.Fatalf("status with direct auto pair token = %d", statusRR.Code)
	}
}

func TestRemoteAutoPairPendingApproval(t *testing.T) {
	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteAutoPair = false
	state.mu.Unlock()

	remote := NewRemoteServer(state)
	desktop := NewServer(state)

	rr := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"auto":true,"device_name":"Pixel 8"}`, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("pending auto pair code = %d body=%s", rr.Code, rr.Body.String())
	}
	var initRes struct {
		Pending   bool   `json:"pending"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &initRes); err != nil {
		t.Fatal(err)
	}
	if !initRes.Pending || initRes.RequestID == "" {
		t.Fatalf("expected pending request with request_id: %#v", initRes)
	}

	// Verify pending request is listable via desktop API
	pendingRR := serveHTTP(desktop, http.MethodGet, "/api/remote/pairing/pending", "", "")
	if pendingRR.Code != http.StatusOK {
		t.Fatalf("pending list code = %d body=%s", pendingRR.Code, pendingRR.Body.String())
	}
	var pendingList struct {
		Pending []struct {
			RequestID  string `json:"request_id"`
			DeviceName string `json:"device_name"`
		} `json:"pending"`
	}
	if err := json.Unmarshal(pendingRR.Body.Bytes(), &pendingList); err != nil {
		t.Fatal(err)
	}
	if len(pendingList.Pending) != 1 || pendingList.Pending[0].RequestID != initRes.RequestID {
		t.Fatalf("unexpected pending list: %#v", pendingList)
	}

	// Approve via desktop
	decisionRR := serveHTTP(desktop, http.MethodPost, "/api/remote/pairing/decision", `{"request_id":"`+initRes.RequestID+`","approve":true}`, "")
	if decisionRR.Code != http.StatusOK {
		t.Fatalf("decision code = %d body=%s", decisionRR.Code, decisionRR.Body.String())
	}

	// Check pair status returns token
	statusRR := serveHTTP(remote, http.MethodGet, "/remote/api/pair-status?request_id="+initRes.RequestID, "", "")
	if statusRR.Code != http.StatusOK {
		t.Fatalf("pair status code = %d body=%s", statusRR.Code, statusRR.Body.String())
	}
	var statusRes struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(statusRR.Body.Bytes(), &statusRes); err != nil {
		t.Fatal(err)
	}
	if statusRes.Token == "" {
		t.Fatal("expected token after desktop approval")
	}

	// Verify token works on remote status
	checkRR := serveHTTP(remote, http.MethodGet, "/remote/api/status", "", statusRes.Token)
	if checkRR.Code != http.StatusOK {
		t.Fatalf("status with approved token = %d", checkRR.Code)
	}
}

func TestRemoteAutoPairPendingRejection(t *testing.T) {
	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteAutoPair = false
	state.mu.Unlock()

	remote := NewRemoteServer(state)
	desktop := NewServer(state)

	rr := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"auto":true,"device_name":"Untrusted Device"}`, "")
	var initRes struct {
		Pending   bool   `json:"pending"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &initRes); err != nil {
		t.Fatal(err)
	}

	// Reject via desktop
	decisionRR := serveHTTP(desktop, http.MethodPost, "/api/remote/pairing/decision", `{"request_id":"`+initRes.RequestID+`","approve":false}`, "")
	if decisionRR.Code != http.StatusOK {
		t.Fatalf("decision code = %d body=%s", decisionRR.Code, decisionRR.Body.String())
	}

	// Check pair status returns rejected
	statusRR := serveHTTP(remote, http.MethodGet, "/remote/api/pair-status?request_id="+initRes.RequestID, "", "")
	var statusRes struct {
		Rejected bool   `json:"rejected"`
		Token    string `json:"token"`
	}
	if err := json.Unmarshal(statusRR.Body.Bytes(), &statusRes); err != nil {
		t.Fatal(err)
	}
	if !statusRes.Rejected || statusRes.Token != "" {
		t.Fatalf("expected rejected status without token: %#v", statusRes)
	}
}

func TestRemoteSelectProject(t *testing.T) {
	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteAutoPair = true
	state.mu.Unlock()

	remote := NewRemoteServer(state)

	// Unauthorized call
	unauth := serveHTTP(remote, http.MethodPost, "/remote/api/select-project", `{"path":"test"}`, "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth select-project code = %d", unauth.Code)
	}

	// Pair device to get token
	openRR := serveHTTP(remote, http.MethodPost, "/remote/api/pair", `{"auto":true,"device_name":"Phone"}`, "")
	var paired struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(openRR.Body.Bytes(), &paired); err != nil || paired.Token == "" {
		t.Fatalf("pair failed: %v", openRR.Body.String())
	}

	// Create sub-project inside root
	subProj := filepath.Join(state.Config.RootProjectDir, "SubProject")
	if err := os.MkdirAll(subProj, 0o755); err != nil {
		t.Fatal(err)
	}

	// Select project via remote API
	body, _ := json.Marshal(map[string]string{"path": subProj})
	selectRR := serveHTTP(remote, http.MethodPost, "/remote/api/select-project", string(body), paired.Token)
	if selectRR.Code != http.StatusOK {
		t.Fatalf("select-project code = %d body = %s", selectRR.Code, selectRR.Body.String())
	}

	var res struct {
		OK      bool   `json:"ok"`
		Project string `json:"project"`
	}
	if err := json.Unmarshal(selectRR.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Project != subProj {
		t.Fatalf("unexpected select-project response: %#v", res)
	}

	state.mu.RLock()
	lastProj := state.Config.LastProject
	activeProj := state.Project
	state.mu.RUnlock()

	if lastProj != subProj {
		t.Fatalf("expected config.LastProject=%q, got %q", subProj, lastProj)
	}
	if activeProj != subProj {
		t.Fatalf("expected state.Project=%q, got %q", subProj, activeProj)
	}
}
