// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"
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
		if !decision.Approved || decision.Persist || decision.Scope != "" {
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

func TestMobileSafeProductionRemoteStartsLiveLoopbackServer(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(probe.Addr().String())
	if err != nil {
		_ = probe.Close()
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		_ = probe.Close()
		t.Fatal(err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteEnabled = true
	state.Config.RemoteBindHost = "127.0.0.1"
	state.Config.RemotePort = port
	cfg := state.Config
	state.mu.Unlock()

	urls, err := startMobileSafeProductionRemoteServer(state, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) == 0 {
		t.Fatal("live mobile remote returned no URL")
	}
	state.mu.RLock()
	listenAddr := state.RemoteListenAddr
	state.mu.RUnlock()
	if listenAddr == "" {
		t.Fatal("live mobile remote did not record its listen address")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, requestErr := client.Get("http://" + listenAddr + "/remote/api/ping")
		if requestErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("mobile remote ping status = %d", resp.StatusCode)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mobile remote did not become reachable: %v", requestErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
