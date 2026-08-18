// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pairedMobileHandler(t *testing.T, state *AppState) (http.Handler, string) {
	t.Helper()
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "test-phone")
	if err != nil {
		t.Fatal(err)
	}
	remote := newMobileSafeRemoteServer(state)
	return mobileSafeRemoteHTTPHandler(remote), token
}

func TestMobileProjectActionsRequireAuthAndExactDeleteConfirmation(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)
	state.mu.RLock()
	root := state.Config.RootProjectDir
	state.mu.RUnlock()

	unauth := serveHTTP(handler, http.MethodPost, "/remote/api/project-action", `{"path":"`+root+`","action":"create_project","value":"PhoneProject"}`, "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated project action = %d", unauth.Code)
	}

	create := serveHTTP(handler, http.MethodPost, "/remote/api/project-action", `{"path":"`+root+`","action":"create_project","value":"PhoneProject"}`, token)
	if create.Code != http.StatusOK {
		t.Fatalf("create project = %d body=%s", create.Code, create.Body.String())
	}
	project := filepath.Join(root, "PhoneProject")
	for _, name := range []string{"README.md", "AGENTS.md", "STATE.md"} {
		if _, err := os.Stat(filepath.Join(project, name)); err != nil {
			t.Fatalf("mobile-created project missing %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview := serveHTTP(handler, http.MethodGet, "/remote/api/project-delete-preview?path="+urlQueryEscape(project), "", token)
	if preview.Code != http.StatusOK {
		t.Fatalf("delete preview = %d body=%s", preview.Code, preview.Body.String())
	}
	var p ProjectDeletePreview
	if err := json.Unmarshal(preview.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Empty || !p.ConfirmationRequired || p.Confirmation != "PhoneProject" || p.Files < 4 {
		t.Fatalf("unexpected delete preview: %#v", p)
	}

	wrong := serveHTTP(handler, http.MethodPost, "/remote/api/project-action", `{"path":"`+project+`","action":"delete_recursive","value":"wrong"}`, token)
	if wrong.Code != http.StatusConflict {
		t.Fatalf("wrong recursive delete = %d body=%s", wrong.Code, wrong.Body.String())
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatalf("project changed after wrong confirmation: %v", err)
	}

	remove := serveHTTP(handler, http.MethodPost, "/remote/api/project-action", `{"path":"`+project+`","action":"delete_recursive","value":"PhoneProject"}`, token)
	if remove.Code != http.StatusOK {
		t.Fatalf("confirmed recursive delete = %d body=%s", remove.Code, remove.Body.String())
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Fatalf("project still exists after confirmed deletion: %v", err)
	}
}

func TestMobileProjectActionAllowlistRejectsRename(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)
	state.mu.RLock()
	project := state.Project
	state.mu.RUnlock()
	rr := serveHTTP(handler, http.MethodPost, "/remote/api/project-action", `{"path":"`+project+`","action":"rename_folder","value":"renamed"}`, token)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("mobile rename should be outside narrow action allowlist, got %d", rr.Code)
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatalf("project changed after blocked action: %v", err)
	}
}

func TestMobileRemoteRejectsGlobalApprovalPersistence(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)
	pending := &PendingAction{
		ID:     "mobile-pending",
		Action: AgentAction{Action: "run_command", Message: "Run build?", Command: "go test ./..."},
		Result: make(chan ApprovalDecision, 1),
	}
	state.mu.Lock()
	state.Pending = pending
	state.mu.Unlock()

	rr := serveHTTP(handler, http.MethodPost, "/remote/api/approve", `{"id":"mobile-pending","decision":"global","approve":true}`, token)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("global mobile approval = %d body=%s", rr.Code, rr.Body.String())
	}
	select {
	case decision := <-pending.Result:
		t.Fatalf("blocked global approval reached pending action: %#v", decision)
	default:
	}
}

func urlQueryEscape(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, " ", "%20")
}
