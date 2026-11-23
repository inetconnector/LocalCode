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
	"time"
)

func TestTurnCheckpointsMultiStageRollbackIncludingDeletions(t *testing.T) {
	tmpAppData := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmpAppData)
	t.Setenv("XDG_CONFIG_HOME", tmpAppData)
	t.Setenv("HOME", tmpAppData)

	root := t.TempDir()
	proj := filepath.Join(root, "demo_proj")
	if err := os.MkdirAll(filepath.Join(proj, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "old_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "a.txt"), []byte("v0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "sub", "will_delete.txt"), []byte("precious data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "old_dir", "secret.txt"), []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)

	thread := newThread(proj, cfg.OllamaDefaultModel)
	state.mu.Lock()
	state.Threads[thread.ID] = thread
	state.CurrentThread = thread.ID
	state.Project = proj
	state.mu.Unlock()

	// Turn 1: cp1 before Turn 1 mutations
	cp1, err := state.CreateTurnCheckpoint(thread.ID, proj, "Turn 1: update a.txt and add t1.txt")
	if err != nil {
		t.Fatalf("CreateTurnCheckpoint cp1: %v", err)
	}
	state.AddEvent(UIEvent{Type: "user", Message: "Turn 1", CheckpointID: cp1.ID})
	state.AddEvent(UIEvent{Type: "final", Message: "Done 1"})
	if err := os.WriteFile(filepath.Join(proj, "a.txt"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "t1.txt"), []byte("from turn 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	state.Threads[thread.ID].Title = "Turn 1"
	state.mu.Unlock()

	// Turn 2: cp2 before Turn 2 mutations (deletes files + whole directory, modifies a.txt, creates new_dir/t2.txt)
	cp2, err := state.CreateTurnCheckpoint(thread.ID, proj, "Turn 2: delete old_dir and sub/will_delete.txt")
	if err != nil {
		t.Fatalf("CreateTurnCheckpoint cp2: %v", err)
	}
	state.AddEvent(UIEvent{Type: "user", Message: "Turn 2", CheckpointID: cp2.ID})
	state.AddEvent(UIEvent{Type: "final", Message: "Done 2"})
	_ = os.Remove(filepath.Join(proj, "sub", "will_delete.txt"))
	_ = os.RemoveAll(filepath.Join(proj, "old_dir"))
	if err := os.WriteFile(filepath.Join(proj, "a.txt"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "new_dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "new_dir", "t2.txt"), []byte("from turn 2"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Turn 3: cp3 before Turn 3 mutations (deletes t1.txt, edits a.txt to v3, creates t3.txt)
	cp3, err := state.CreateTurnCheckpoint(thread.ID, proj, "Turn 3: modify again")
	if err != nil {
		t.Fatalf("CreateTurnCheckpoint cp3: %v", err)
	}
	state.AddEvent(UIEvent{Type: "user", Message: "Turn 3", CheckpointID: cp3.ID})
	state.AddEvent(UIEvent{Type: "final", Message: "Done 3"})
	_ = os.Remove(filepath.Join(proj, "t1.txt"))
	if err := os.WriteFile(filepath.Join(proj, "a.txt"), []byte("v3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "t3.txt"), []byte("from turn 3"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Preview rollback to before Turn 2 (multi-stage: undoes Turn 2 AND Turn 3)
	preview, err := state.PreviewRollbackToCheckpoint(thread.ID, cp2.ID)
	if err != nil {
		t.Fatalf("PreviewRollbackToCheckpoint: %v", err)
	}
	if !preview.Restorable {
		t.Fatalf("expected restorable=true, got reason=%q", preview.Reason)
	}
	if preview.TurnsRemoved != 2 || preview.EventsRemoved != 4 {
		t.Fatalf("unexpected preview counts: turns=%d events=%d", preview.TurnsRemoved, preview.EventsRemoved)
	}
	if len(preview.FilesToRestore) != 3 || len(preview.FilesToRevert) != 1 || len(preview.FilesToDelete) != 2 {
		t.Fatalf("unexpected diff plan: restore=%v revert=%v delete=%v", preview.FilesToRestore, preview.FilesToRevert, preview.FilesToDelete)
	}

	ch := state.Subscribe()
	defer state.Unsubscribe(ch)

	// Execute rollback to before Turn 2
	res, err := state.RollbackToCheckpoint(thread.ID, cp2.ID)
	if err != nil {
		t.Fatalf("RollbackToCheckpoint cp2: %v", err)
	}
	if !res.OK || res.RestoredFiles != 3 || res.RevertedFiles != 1 || res.DeletedFiles != 2 || res.TurnsRemoved != 2 {
		t.Fatalf("unexpected rollback result: %+v", res)
	}

	// Verify SSE broadcast
	select {
	case ev := <-ch:
		if ev.Type != "thread_rolled_back" || ev.ThreadID != thread.ID {
			t.Fatalf("unexpected SSE event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("expected thread_rolled_back SSE broadcast")
	}

	// Verify exact disk state matches post-Turn-1 / pre-Turn-2
	gotA, _ := os.ReadFile(filepath.Join(proj, "a.txt"))
	gotDel, _ := os.ReadFile(filepath.Join(proj, "sub", "will_delete.txt"))
	gotSecret, _ := os.ReadFile(filepath.Join(proj, "old_dir", "secret.txt"))
	gotT1, _ := os.ReadFile(filepath.Join(proj, "t1.txt"))
	if string(gotA) != "v1" || string(gotDel) != "precious data" || string(gotSecret) != "top secret" || string(gotT1) != "from turn 1" {
		t.Fatalf("restored disk mismatch: a=%q del=%q secret=%q t1=%q", gotA, gotDel, gotSecret, gotT1)
	}
	if _, err := os.Stat(filepath.Join(proj, "new_dir")); !os.IsNotExist(err) {
		t.Fatalf("expected new_dir to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "t3.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected t3.txt to be removed, stat err=%v", err)
	}

	// Verify thread events completely purged Turn 2 and Turn 3
	state.mu.RLock()
	remaining := append([]UIEvent(nil), state.Threads[thread.ID].Events...)
	state.mu.RUnlock()
	if len(remaining) != 2 || remaining[0].Message != "Turn 1" {
		t.Fatalf("expected only Turn 1 events remaining, got %+v", remaining)
	}

	// Now roll all the way back to before Turn 1
	res1, err := state.RollbackToCheckpoint(thread.ID, cp1.ID)
	if err != nil {
		t.Fatalf("RollbackToCheckpoint cp1: %v", err)
	}
	if !res1.OK || res1.TurnsRemoved != 1 {
		t.Fatalf("unexpected cp1 rollback result: %+v", res1)
	}
	gotA0, _ := os.ReadFile(filepath.Join(proj, "a.txt"))
	if string(gotA0) != "v0" {
		t.Fatalf("expected a.txt back to v0, got %q", gotA0)
	}
	if _, err := os.Stat(filepath.Join(proj, "t1.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected t1.txt removed on cp1 rollback")
	}
	state.mu.RLock()
	finalEvents := state.Threads[thread.ID].Events
	finalTitle := state.Threads[thread.ID].Title
	state.mu.RUnlock()
	if len(finalEvents) != 0 || finalTitle != "Neuer Chat" {
		t.Fatalf("expected empty thread with reset title, got len=%d title=%q", len(finalEvents), finalTitle)
	}
}

func TestTurnCheckpointsHTTPAndProjectSubdirsEndpoints(t *testing.T) {
	tmpAppData := t.TempDir()
	t.Setenv("LOCALAPPDATA", tmpAppData)
	t.Setenv("XDG_CONFIG_HOME", tmpAppData)
	t.Setenv("HOME", tmpAppData)

	root := t.TempDir()
	kiDir := filepath.Join(root, "KI")
	if err := os.MkdirAll(filepath.Join(kiDir, "LocalCode", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kiDir, "NomaMica"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kiDir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(kiDir, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.RootProjectDir = root
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	srv := NewServer(state)

	// 1. Subdirs endpoint lists visible non-ignored subfolders with has_subdirs metadata
	req := httptest.NewRequest(http.MethodGet, "/api/project-subdirs?path="+filepath.ToSlash(kiDir), nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/project-subdirs status=%d body=%s", rec.Code, rec.Body.String())
	}
	var subResp struct {
		Subdirs []ProjectSubdirItem `json:"subdirs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &subResp); err != nil {
		t.Fatal(err)
	}
	if len(subResp.Subdirs) != 2 || subResp.Subdirs[0].Name != "LocalCode" || !subResp.Subdirs[0].HasSubdirs || subResp.Subdirs[1].Name != "NomaMica" {
		t.Fatalf("unexpected subdirs: %+v", subResp.Subdirs)
	}

	// 2. Rejects traversal outside RootProjectDir
	badReq := httptest.NewRequest(http.MethodGet, "/api/project-subdirs?path=..", nil)
	badRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for outside root, got %d", badRec.Code)
	}

	// 3. Running guard blocks rollback with 409
	thread := newThread(kiDir, cfg.OllamaDefaultModel)
	state.mu.Lock()
	state.Threads[thread.ID] = thread
	state.CurrentThread = thread.ID
	state.Project = kiDir
	state.mu.Unlock()
	cp, err := state.CreateTurnCheckpoint(thread.ID, kiDir, "Prompt")
	if err != nil {
		t.Fatal(err)
	}
	state.AddEvent(UIEvent{Type: "user", Message: "Prompt", CheckpointID: cp.ID})

	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()

	rbBody, _ := json.Marshal(map[string]string{"thread_id": thread.ID, "checkpoint_id": cp.ID})
	rbReq := httptest.NewRequest(http.MethodPost, "/api/checkpoints/rollback", bytes.NewReader(rbBody))
	rbRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rbRec, rbReq)
	if rbRec.Code != http.StatusConflict {
		t.Fatalf("expected 409 while agent running, got %d (%s)", rbRec.Code, rbRec.Body.String())
	}

	state.mu.Lock()
	state.Running = false
	state.mu.Unlock()

	// 4. Corrupt blob fails pre-flight preview safely
	if err := os.WriteFile(filepath.Join(kiDir, "sample.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp2, err := state.CreateTurnCheckpoint(thread.ID, kiDir, "Prompt 2")
	if err != nil {
		t.Fatal(err)
	}
	state.AddEvent(UIEvent{Type: "user", Message: "Prompt 2", CheckpointID: cp2.ID})
	if err := os.Remove(filepath.Join(kiDir, "sample.txt")); err != nil {
		t.Fatal(err)
	}
	sampleHash := cp2.Files["sample.txt"].SHA256
	if err := os.WriteFile(checkpointBlobPath(sampleHash), []byte("corrupted"), 0o644); err != nil {
		t.Fatal(err)
	}
	prevReq := httptest.NewRequest(http.MethodGet, "/api/checkpoints/preview?thread_id="+thread.ID+"&checkpoint_id="+cp2.ID, nil)
	prevRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(prevRec, prevReq)
	if prevRec.Code != http.StatusOK {
		t.Fatalf("preview status=%d", prevRec.Code)
	}
	var prev RollbackPreview
	if err := json.Unmarshal(prevRec.Body.Bytes(), &prev); err != nil {
		t.Fatal(err)
	}
	if prev.Restorable || !strings.Contains(prev.Reason, "corrupt") {
		t.Fatalf("expected pre-flight corruption detection, got %+v", prev)
	}
}

func TestSidebarIconsTreeExpandAndUndoUIContract(t *testing.T) {
	polishBytes, err := os.ReadFile(filepath.Join("static", "ui_polish.js"))
	if err != nil {
		t.Fatal(err)
	}
	polish := string(polishBytes)
	for _, req := range []string{
		`newProjectBtn: '<svg`,
		`newFolderBtn: '<svg`,
		`projectTrashBtn: '<svg`,
		`button.title = tr(key);`,
		`button.setAttribute('aria-label', tr(key));`,
	} {
		if !strings.Contains(polish, req) {
			t.Fatalf("ui_polish.js icon/tooltip contract missing %q", req)
		}
	}

	indexBytes, err := os.ReadFile(filepath.Join("static", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := string(indexBytes)
	for _, req := range []string{
		`function toggleFolderExpanded(path,forceOpen=null)`,
		`function renderSubdirNodes(parentPath,activeProject,q,depth)`,
		`/api/project-subdirs?path=`,
		`class="chevron-hit"`,
		`class="msg-undo-btn"`,
		`async function rollbackToCheckpoint(checkpointId)`,
		`danger:true,input:false`,
		`/api/checkpoints/preview?thread_id=`,
		`/api/checkpoints/rollback`,
		`ev.type==='thread_rolled_back'`,
	} {
		if !strings.Contains(indexHTML, req) {
			t.Fatalf("index.html tree/undo contract missing %q", req)
		}
	}
}
