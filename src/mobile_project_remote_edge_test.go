// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRemoteProjectDeletePreviewRejectsInvalidRequests(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)

	rr := serveHTTP(handler, http.MethodPost, "/remote/api/project-delete-preview", "", token)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("preview wrong method = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = serveHTTP(handler, http.MethodGet, "/remote/api/project-delete-preview", "", token)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("preview missing path = %d body=%s", rr.Code, rr.Body.String())
	}

	state.mu.RLock()
	project := state.Project
	state.mu.RUnlock()
	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		state.Running = false
		state.mu.Unlock()
	}()

	rr = serveHTTP(handler, http.MethodGet, "/remote/api/project-delete-preview?path="+url.QueryEscape(project), "", token)
	if rr.Code != http.StatusConflict {
		t.Fatalf("preview while running = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRemoteProjectDeletePreviewRejectsMissingProject(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)
	state.mu.RLock()
	root := state.Config.RootProjectDir
	state.mu.RUnlock()

	rr := serveHTTP(handler, http.MethodGet, "/remote/api/project-delete-preview?path="+url.QueryEscape(root+"/does-not-exist"), "", token)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("preview missing project = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRemoteProjectActionRejectsMalformedOrForbiddenRequests(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)

	rr := serveHTTP(handler, http.MethodGet, "/remote/api/project-action", "", token)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("project action wrong method = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = serveHTTP(handler, http.MethodPost, "/remote/api/project-action", "{", token)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("malformed project action = %d body=%s", rr.Code, rr.Body.String())
	}

	state.mu.RLock()
	project := state.Project
	state.mu.RUnlock()
	rr = serveHTTP(handler, http.MethodPost, "/remote/api/project-action", mobileProjectActionBody(t, project, "rename_folder", "Renamed"), token)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("forbidden mobile project action = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMobileRemoteAllowsOneTimeApprovalThroughNormalHandler(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)
	pending := &PendingAction{
		ID:     "mobile-once",
		Action: AgentAction{Action: "run_command", Message: "Run tests?", Command: "go test ./..."},
		Result: make(chan ApprovalDecision, 1),
	}
	state.mu.Lock()
	state.Pending = pending
	state.mu.Unlock()

	rr := serveHTTP(handler, http.MethodPost, "/remote/api/approve", `{"id":"mobile-once","decision":"once","approve":true}`, token)
	if rr.Code != http.StatusOK {
		t.Fatalf("one-time mobile approval = %d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case decision := <-pending.Result:
		if !decision.Approved || decision.Persist || decision.Scope != "once" {
			t.Fatalf("unexpected one-time mobile approval: %#v", decision)
		}
	default:
		t.Fatal("one-time mobile approval did not reach pending action")
	}
}

func TestMobileSafeRemoteStartDisabledIsNoOp(t *testing.T) {
	cfg := defaultConfig()
	cfg.RemoteEnabled = false
	state := &AppState{}

	urls, err := startMobileSafeRemoteHTTPServer(state, cfg)
	if err != nil || urls != nil {
		t.Fatalf("disabled HTTP remote = urls=%#v err=%v", urls, err)
	}
	urls, err = startMobileSafeProductionRemoteServer(state, cfg)
	if err != nil || urls != nil {
		t.Fatalf("disabled production remote = urls=%#v err=%v", urls, err)
	}
}
