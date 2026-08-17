// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFolderActionTestState(t *testing.T) (*AppState, string) {
	t.Helper()
	configHome := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", configHome)
	t.Setenv("LOCALCODE_CACHE_HOME", filepath.Join(configHome, "cache"))
	root := filepath.Join(t.TempDir(), "projects")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = ""
	cfg.ProjectAliases = map[string]string{}
	cfg.PinnedProjects = nil
	cfg.HiddenProjects = nil
	return &AppState{Config: cfg, Threads: map[string]*ChatThread{}}, root
}

func TestValidProjectFolderNameRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", `..\\escape`, "a/b", "bad:name", "trailing.", "CON", "LPT9"} {
		if err := validProjectFolderName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
	for _, name := range []string{"test", "Mein Projekt", "csharp-game", "LPT10"} {
		if err := validProjectFolderName(name); err != nil {
			t.Fatalf("expected %q to be accepted: %v", name, err)
		}
	}
}

func TestProjectActionCreateAndRenameFolderUpdatesReferences(t *testing.T) {
	state, root := newFolderActionTestState(t)
	created, err := state.ProjectAction(root, "create_folder", "OldName")
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(root, "OldName")
	if created.Path != oldPath {
		t.Fatalf("created path = %q, want %q", created.Path, oldPath)
	}
	if info, err := os.Stat(oldPath); err != nil || !info.IsDir() {
		t.Fatalf("created folder missing: %v", err)
	}

	state.Config.LastProject = oldPath
	state.Config.PinnedProjects = []string{oldPath}
	state.Config.ProjectAliases = map[string]string{oldPath: "Alias"}
	state.Project = oldPath
	state.Continuation = &AgentContinuation{Project: oldPath}
	state.Threads["thread-1"] = &ChatThread{ID: "thread-1", Project: oldPath, Title: "Test"}

	renamed, err := state.ProjectAction(oldPath, "rename_folder", "NewName")
	if err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(root, "NewName")
	if renamed.Path != newPath {
		t.Fatalf("renamed path = %q, want %q", renamed.Path, newPath)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old folder still exists or unexpected error: %v", err)
	}
	if info, err := os.Stat(newPath); err != nil || !info.IsDir() {
		t.Fatalf("new folder missing: %v", err)
	}
	if state.Project != newPath || state.Config.LastProject != newPath {
		t.Fatalf("active/config path not updated: state=%q config=%q", state.Project, state.Config.LastProject)
	}
	if state.Continuation == nil || state.Continuation.Project != newPath {
		t.Fatalf("continuation path not updated: %#v", state.Continuation)
	}
	if state.Threads["thread-1"].Project != newPath {
		t.Fatalf("thread path not updated: %q", state.Threads["thread-1"].Project)
	}
	if !projectListContains(state.Config.PinnedProjects, newPath) {
		t.Fatal("pinned project reference not migrated")
	}
	if state.Config.ProjectAliases[newPath] != "Alias" {
		t.Fatalf("project alias not migrated: %#v", state.Config.ProjectAliases)
	}
}

func TestProjectActionDeleteEmptyRequiresEmptyFolder(t *testing.T) {
	state, root := newFolderActionTestState(t)
	nonEmpty := filepath.Join(root, "NonEmpty")
	if err := os.Mkdir(nonEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmpty, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProjectAction(nonEmpty, "delete_empty", ""); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty rejection, got %v", err)
	}
	if _, err := os.Stat(nonEmpty); err != nil {
		t.Fatalf("non-empty folder was modified: %v", err)
	}

	empty := filepath.Join(root, "Empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProjectAction(empty, "delete_empty", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("empty folder still exists or unexpected error: %v", err)
	}
}

func TestProjectActionRecursiveDeleteRequiresExactFolderConfirmation(t *testing.T) {
	state, root := newFolderActionTestState(t)
	project := filepath.Join(root, "DangerProject")
	if err := os.MkdirAll(filepath.Join(project, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "nested", "data.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.Project = project
	state.Config.LastProject = project
	state.Threads["thread-1"] = &ChatThread{ID: "thread-1", Project: project, Title: "Keep history"}

	if _, err := state.ProjectAction(project, "delete_recursive", "wrong-name"); err == nil {
		t.Fatal("recursive delete accepted incorrect confirmation")
	}
	if _, err := os.Stat(project); err != nil {
		t.Fatalf("folder removed after incorrect confirmation: %v", err)
	}

	if _, err := state.ProjectAction(project, "delete_recursive", "DangerProject"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Fatalf("recursive folder still exists or unexpected error: %v", err)
	}
	if state.Project != "" || state.CurrentThread != "" {
		t.Fatalf("active project state not cleared: project=%q thread=%q", state.Project, state.CurrentThread)
	}
	if thread := state.Threads["thread-1"]; thread == nil || !thread.Archived {
		t.Fatalf("chat history should be preserved and archived: %#v", thread)
	}
}

func TestProjectFolderActionsRejectNestedOrOutsidePaths(t *testing.T) {
	state, root := newFolderActionTestState(t)
	project := filepath.Join(root, "Project")
	nested := filepath.Join(project, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProjectAction(nested, "delete_recursive", "nested"); err == nil {
		t.Fatal("nested folder operation should be rejected")
	}
	outside := filepath.Join(t.TempDir(), "Outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProjectAction(outside, "delete_recursive", "Outside"); err == nil {
		t.Fatal("outside folder operation should be rejected")
	}
}
