// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateFolderStaysBareWhileCreateProjectAlwaysScaffolds(t *testing.T) {
	state, root := newFolderActionTestState(t)
	state.Config.CreateProjectDocs = false
	state.Config.AutoStateUpdate = true
	state.Config.StateFile = "STATE.md"

	bare, err := state.ProjectAction(root, "create_folder", "BareFolder")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(bare.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("create_folder must remain intentionally bare, got %d entries", len(entries))
	}

	project, err := state.ProjectAction(root, "create_project", "ScaffoldedProject")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "AGENTS.md", "STATE.md"} {
		info, err := os.Stat(filepath.Join(project.Path, name))
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("create_project must create %s even when CreateProjectDocs=false: %v", name, err)
		}
	}
}

func TestProjectActionRecursiveDeleteQuarantinesAndCanRestore(t *testing.T) {
	state, root := newFolderActionTestState(t)
	project := filepath.Join(root, "RecoverableProject")
	if err := os.MkdirAll(filepath.Join(project, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("important data")
	if err := os.WriteFile(filepath.Join(project, "nested", "data.txt"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	state.Project = project
	state.Config.LastProject = project
	state.Config.PinnedProjects = []string{project}
	state.Config.ProjectAliases[project] = "Important"
	state.Threads["thread-1"] = &ChatThread{ID: "thread-1", Project: project, Title: "History"}

	if _, err := state.ProjectAction(project, "delete_recursive", "RecoverableProject"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Fatalf("project should have moved out of its original path: %v", err)
	}
	entries, err := listQuarantinedProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "RecoverableProject" {
		t.Fatalf("expected one matching quarantine entry, got %#v", entries)
	}
	if state.Project != "" || state.Config.LastProject != "" {
		t.Fatalf("active project references must be cleared after quarantine: project=%q last=%q", state.Project, state.Config.LastProject)
	}
	if projectListContains(state.Config.PinnedProjects, project) {
		t.Fatal("quarantined project must be removed from pinned projects")
	}
	if _, ok := state.Config.ProjectAliases[project]; ok {
		t.Fatal("quarantined project alias must be removed")
	}
	if thread := state.Threads["thread-1"]; thread == nil || !thread.Archived {
		t.Fatalf("project chat history must be preserved and archived: %#v", thread)
	}

	restored, err := restoreQuarantinedProject(root, entries[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.OriginalPath != project {
		t.Fatalf("restored path = %q, want %q", restored.OriginalPath, project)
	}
	data, err := os.ReadFile(filepath.Join(project, "nested", "data.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("restored file content changed: %q", data)
	}
}

func TestProjectActionsProtectQuarantineDirectory(t *testing.T) {
	state, root := newFolderActionTestState(t)
	quarantine, err := ensureProjectQuarantineRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ProjectAction(quarantine, "delete_empty", ""); err == nil || !strings.Contains(strings.ToLower(err.Error()), "quarantine") {
		t.Fatalf("quarantine root must not be exposed as a project folder action target, got %v", err)
	}
	if info, err := os.Stat(quarantine); err != nil || !info.IsDir() {
		t.Fatalf("quarantine root was modified: %v", err)
	}
}
