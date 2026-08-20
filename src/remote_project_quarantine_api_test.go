// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newRemoteQuarantineTestState(t *testing.T) (*AppState, string) {
	t.Helper()
	root := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	return state, root
}

func makeRemoteQuarantineEntry(t *testing.T, root, name string) QuarantinedProject {
	t.Helper()
	project := filepath.Join(root, name)
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "data.txt"), []byte("important"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := quarantineProject(root, project)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func quarantineActionRequest(t *testing.T, remote *RemoteServer, body map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/remote/api/project-quarantine-action", bytes.NewReader(data))
	rr := httptest.NewRecorder()
	remote.handleRemoteProjectQuarantineAction(rr, req)
	return rr
}

func TestRemoteProjectQuarantineRoutesRequireAuth(t *testing.T) {
	state, root := newRemoteQuarantineTestState(t)
	entry := makeRemoteQuarantineEntry(t, root, "AuthProtected")
	handler, token := pairedMobileHandler(t, state)

	unauthList := serveHTTP(handler, http.MethodGet, "/remote/api/project-quarantine", "", "")
	if unauthList.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated quarantine list = %d body=%s", unauthList.Code, unauthList.Body.String())
	}
	actionData, err := json.Marshal(map[string]string{"action": "restore", "id": entry.ID})
	if err != nil {
		t.Fatal(err)
	}
	unauthAction := serveHTTP(handler, http.MethodPost, "/remote/api/project-quarantine-action", string(actionData), "")
	if unauthAction.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated quarantine action = %d body=%s", unauthAction.Code, unauthAction.Body.String())
	}

	authList := serveHTTP(handler, http.MethodGet, "/remote/api/project-quarantine", "", token)
	if authList.Code != http.StatusOK || !strings.Contains(authList.Body.String(), entry.ID) {
		t.Fatalf("authenticated quarantine list = %d body=%s", authList.Code, authList.Body.String())
	}
	authRestore := serveHTTP(handler, http.MethodPost, "/remote/api/project-quarantine-action", string(actionData), token)
	if authRestore.Code != http.StatusOK {
		t.Fatalf("authenticated quarantine restore = %d body=%s", authRestore.Code, authRestore.Body.String())
	}
}

func TestRemoteProjectQuarantineListAndRestore(t *testing.T) {
	state, root := newRemoteQuarantineTestState(t)
	entry := makeRemoteQuarantineEntry(t, root, "RestoreMe")
	remote := NewRemoteServer(state)

	req := httptest.NewRequest(http.MethodGet, "/remote/api/project-quarantine", nil)
	rr := httptest.NewRecorder()
	remote.handleRemoteProjectQuarantineList(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), entry.ID) || !strings.Contains(rr.Body.String(), "RestoreMe") {
		t.Fatalf("unexpected quarantine list response %d: %s", rr.Code, rr.Body.String())
	}

	rr = quarantineActionRequest(t, remote, map[string]string{"action": "restore", "id": entry.ID})
	if rr.Code != http.StatusOK {
		t.Fatalf("restore failed %d: %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "RestoreMe", "data.txt")); err != nil {
		t.Fatalf("restored project missing: %v", err)
	}
	entries, err := listQuarantinedProjects(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("restored entry remained in quarantine: entries=%#v err=%v", entries, err)
	}
}

func TestRemoteProjectQuarantinePurgeRequiresExactConfirmation(t *testing.T) {
	state, root := newRemoteQuarantineTestState(t)
	entry := makeRemoteQuarantineEntry(t, root, "DestroyMe")
	remote := NewRemoteServer(state)

	rr := quarantineActionRequest(t, remote, map[string]string{"action": "purge", "id": entry.ID, "confirmation": "DestroyMe"})
	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "PURGE DestroyMe") {
		t.Fatalf("weak purge confirmation was not rejected: %d %s", rr.Code, rr.Body.String())
	}
	if _, _, err := loadQuarantinedProject(root, entry.ID); err != nil {
		t.Fatalf("failed purge attempt must preserve quarantine entry: %v", err)
	}

	rr = quarantineActionRequest(t, remote, map[string]string{"action": "purge", "id": entry.ID, "confirmation": "PURGE DestroyMe"})
	if rr.Code != http.StatusOK {
		t.Fatalf("exact purge confirmation failed: %d %s", rr.Code, rr.Body.String())
	}
	if _, _, err := loadQuarantinedProject(root, entry.ID); err == nil {
		t.Fatal("purged quarantine entry still exists")
	}
}

func TestRemoteProjectQuarantineActionsFailClosed(t *testing.T) {
	state, root := newRemoteQuarantineTestState(t)
	entry := makeRemoteQuarantineEntry(t, root, "Blocked")
	remote := NewRemoteServer(state)

	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()
	rr := quarantineActionRequest(t, remote, map[string]string{"action": "restore", "id": entry.ID})
	if rr.Code != http.StatusConflict {
		t.Fatalf("running agent must block restore: %d %s", rr.Code, rr.Body.String())
	}
	state.mu.Lock()
	state.Running = false
	state.mu.Unlock()

	rr = quarantineActionRequest(t, remote, map[string]string{"action": "restore", "id": "../escape"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid quarantine id must be rejected: %d %s", rr.Code, rr.Body.String())
	}
	rr = quarantineActionRequest(t, remote, map[string]string{"action": "shell", "id": entry.ID})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("unsupported mobile quarantine action must be forbidden: %d %s", rr.Code, rr.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/remote/api/project-quarantine", nil)
	listRR := httptest.NewRecorder()
	remote.handleRemoteProjectQuarantineList(listRR, req)
	if listRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("quarantine list must be GET-only: %d", listRR.Code)
	}
}
