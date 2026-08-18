// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReplaceTextAtVersionRejectsChangedApprovedBytes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nconst value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expected, err := readFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	// Keep the requested text present but change unrelated bytes. A plain
	// search/replace would otherwise silently apply to a file the user did not
	// approve.
	if err := os.WriteFile(path, []byte("package main\n\n// external edit\nconst value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = replaceTextAtVersion(root, "main.go", "const value = 1", "const value = 2", expected)
	if !isEditConflict(err) {
		t.Fatalf("expected EDIT_CONFLICT, got %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if strings.Contains(string(data), "value = 2") || !strings.Contains(string(data), "external edit") {
		t.Fatalf("stale approved replacement modified external content: %q", data)
	}
}

func TestWriteProjectFileAtVersionRejectsCreateAfterPreview(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "new.txt")
	expected, err := readFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if expected.Exists {
		t.Fatal("new file unexpectedly exists")
	}
	if err := os.WriteFile(path, []byte("created externally\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = writeProjectFileAtVersion(root, "new.txt", "localcode\n", expected)
	if !isEditConflict(err) {
		t.Fatalf("expected EDIT_CONFLICT, got %v", err)
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(data) != "created externally\n" {
		t.Fatalf("external create was overwritten: %q", data)
	}
}

func TestPerformApprovedBindsPreviewToExactFileVersion(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = root
	cfg.ApprovalMode = "strict"
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)

	type outcome struct {
		result string
		done   bool
	}
	resultCh := make(chan outcome, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		result, done := state.performApproved(ctx, root, AgentAction{
			Action:  "write_file",
			Message: "Update target",
			Path:    "target.txt",
			Content: "after\n",
		})
		resultCh <- outcome{result: result, done: done}
	}()

	var pending *PendingAction
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		pending = state.Pending
		state.mu.RUnlock()
		if pending != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pending == nil {
		t.Fatal("approval preview was not published")
	}
	if !strings.Contains(pending.Preview, "-before") || !strings.Contains(pending.Preview, "+after") {
		t.Fatalf("unexpected preview:\n%s", pending.Preview)
	}

	if err := os.WriteFile(path, []byte("external change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pending.Result <- ApprovalDecision{Approved: true}

	select {
	case got := <-resultCh:
		if !strings.Contains(got.result, "EDIT_CONFLICT") {
			t.Fatalf("expected approval-bound conflict, got result=%q done=%v", got.result, got.done)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("approved action did not complete")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "external change\n" {
		t.Fatalf("approved stale preview overwrote external edit: %q", data)
	}
}
