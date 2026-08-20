// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectQuarantineConfirmationIsExactCaseSensitive(t *testing.T) {
	state, root := newFolderActionTestState(t)
	project := filepath.Join(root, "CaseProject")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProjectAction(project, "delete_recursive", "caseproject"); err == nil || !strings.Contains(strings.ToLower(err.Error()), "exactly") {
		t.Fatalf("case-mismatched confirmation should fail closed, got %v", err)
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatalf("project changed after case-mismatched confirmation: %v", err)
	}
}

func TestProjectQuarantineRestoreUnarchivesProjectThreads(t *testing.T) {
	state, root := newFolderActionTestState(t)
	project := filepath.Join(root, "RestoredProject")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.Threads["thread-restore"] = &ChatThread{ID: "thread-restore", Project: project, Title: "Restore me"}
	if _, err := state.ProjectAction(project, "delete_recursive", "RestoredProject"); err != nil {
		t.Fatal(err)
	}
	if !state.Threads["thread-restore"].Archived {
		t.Fatal("quarantine should archive project threads")
	}
	entries, err := state.ListProjectQuarantine()
	if err != nil || len(entries) != 1 {
		t.Fatalf("quarantine entries = %#v err=%v", entries, err)
	}
	restored, err := state.ProjectQuarantineAction("restore", entries[0].ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if restored.OriginalPath != project {
		t.Fatalf("restored path = %q, want %q", restored.OriginalPath, project)
	}
	if state.Threads["thread-restore"].Archived {
		t.Fatal("restoring a project should unarchive its preserved project threads")
	}
	if _, err := os.Stat(filepath.Join(project, "keep.txt")); err != nil {
		t.Fatalf("restored project content missing: %v", err)
	}
}

func desktopRequest(t *testing.T, server *Server, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, "http://127.0.0.1"+target, reader)
	req.Host = "127.0.0.1"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	return rr
}

func TestDesktopProjectTrashRoutesExposePreviewListAndRestore(t *testing.T) {
	state, root := newFolderActionTestState(t)
	server := NewServer(state)
	project := filepath.Join(root, "DesktopTrash")
	if err := os.MkdirAll(filepath.Join(project, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "nested", "data.txt"), []byte("desktop"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.Threads["desktop-thread"] = &ChatThread{ID: "desktop-thread", Project: project, Title: "Desktop"}

	previewRR := desktopRequest(t, server, http.MethodGet, "/api/project-delete-preview?path="+url.QueryEscape(project), nil)
	if previewRR.Code != http.StatusOK {
		t.Fatalf("desktop preview = %d body=%s", previewRR.Code, previewRR.Body.String())
	}
	var preview ProjectDeletePreview
	if err := json.Unmarshal(previewRR.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Empty || preview.Files != 1 || preview.Directories != 1 || preview.Bytes != int64(len("desktop")) || preview.Confirmation != "DesktopTrash" {
		t.Fatalf("unexpected desktop preview: %#v", preview)
	}

	if _, err := state.ProjectAction(project, "delete_recursive", preview.Confirmation); err != nil {
		t.Fatal(err)
	}
	listRR := desktopRequest(t, server, http.MethodGet, "/api/project-quarantine", nil)
	if listRR.Code != http.StatusOK {
		t.Fatalf("desktop trash list = %d body=%s", listRR.Code, listRR.Body.String())
	}
	var list struct {
		Quarantine []QuarantinedProject `json:"quarantine"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Quarantine) != 1 || list.Quarantine[0].Name != "DesktopTrash" {
		t.Fatalf("unexpected desktop trash list: %#v", list.Quarantine)
	}

	restoreRR := desktopRequest(t, server, http.MethodPost, "/api/project-quarantine-action", map[string]string{"action": "restore", "id": list.Quarantine[0].ID})
	if restoreRR.Code != http.StatusOK {
		t.Fatalf("desktop restore = %d body=%s", restoreRR.Code, restoreRR.Body.String())
	}
	if state.Threads["desktop-thread"].Archived {
		t.Fatal("desktop restore should restore archived project-thread visibility")
	}
}

func TestMobileProjectUXSupportsBareFolderAndTrashControls(t *testing.T) {
	state := newRemoteTestState(t)
	handler, token := pairedMobileHandler(t, state)
	state.mu.RLock()
	root := state.Config.RootProjectDir
	state.mu.RUnlock()

	create := serveHTTP(handler, http.MethodPost, "/remote/api/project-action", mobileProjectActionBody(t, root, "create_folder", "PhoneFolder"), token)
	if create.Code != http.StatusOK {
		t.Fatalf("mobile create_folder = %d body=%s", create.Code, create.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(root, "PhoneFolder"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("mobile create_folder must stay intentionally empty, got %d entries", len(entries))
	}

	page := serveHTTP(handler, http.MethodGet, "/remote", "", "")
	if page.Code != http.StatusOK {
		t.Fatalf("mobile remote page = %d body=%s", page.Code, page.Body.String())
	}
	content := page.Body.String()
	for _, marker := range []string{`data-view="trash"`, `action:'create_folder'`, `/remote/api/project-quarantine`, `/remote/api/project-quarantine-action`, `PURGE `} {
		if !strings.Contains(content, marker) {
			t.Fatalf("mobile project UX missing marker %q", marker)
		}
	}
}

func TestDesktopProjectPolishUsesReversibleTrashWording(t *testing.T) {
	data, err := staticFS.ReadFile("static/ui_polish.js")
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, marker := range []string{"create_project", "create_folder", "/api/project-delete-preview", "/api/project-quarantine-action", "PURGE ${entry.name}"} {
		if !strings.Contains(content, marker) {
			t.Fatalf("desktop project UX missing marker %q", marker)
		}
	}
	if strings.Contains(content, "Dateien und Unterordner werden dauerhaft gelöscht") {
		t.Fatal("desktop quarantine action still claims that files are permanently deleted")
	}
}
